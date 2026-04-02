package monitor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DioSaputra28/monitor-vps/internal/app"
	"github.com/DioSaputra28/monitor-vps/internal/config"
	gocpu "github.com/shirou/gopsutil/v4/cpu"
	godisk "github.com/shirou/gopsutil/v4/disk"
	gohost "github.com/shirou/gopsutil/v4/host"
	goload "github.com/shirou/gopsutil/v4/load"
	gomem "github.com/shirou/gopsutil/v4/mem"
	gonet "github.com/shirou/gopsutil/v4/net"
)

var commonServices = []string{"nginx", "apache2", "httpd", "mysql", "mariadb", "postgresql", "redis", "docker", "caddy", "fail2ban"}

type Collector struct {
	cfg      config.Config
	mu       sync.Mutex
	prevNet  map[string]netCounters
	prevTime time.Time
	procPrev map[int]procCPU
}

type netCounters struct {
	rx uint64
	tx uint64
}

type procCPU struct {
	total float64
	at    time.Time
}

func NewCollector(cfg config.Config) *Collector {
	return &Collector{cfg: cfg, prevNet: map[string]netCounters{}, procPrev: map[int]procCPU{}}
}

func (c *Collector) DiscoverServices(ctx context.Context) ([]string, error) {
	discovered := make([]string, 0)
	for _, service := range commonServices {
		if c.serviceExists(ctx, service) {
			discovered = append(discovered, service)
		}
	}
	return discovered, nil
}

func (c *Collector) serviceExists(ctx context.Context, service string) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "status", service)
	err := cmd.Run()
	return err == nil || exitCode(err) == 3
}

func (c *Collector) Collect(ctx context.Context) app.HostSnapshot {
	snap := app.HostSnapshot{CollectedAt: time.Now()}
	hostname, _ := os.Hostname()
	snap.Hostname = hostname

	uptime, err := gohost.UptimeWithContext(ctx)
	if err == nil {
		snap.Uptime = time.Duration(uptime) * time.Second
	} else {
		snap.Errors = append(snap.Errors, "uptime unavailable: "+err.Error())
	}

	percents, err := gocpu.PercentWithContext(ctx, 200*time.Millisecond, false)
	if err == nil && len(percents) > 0 {
		snap.CPUPercent = percents[0]
	} else if err != nil {
		snap.Errors = append(snap.Errors, "cpu unavailable: "+err.Error())
		snap.CPUStatus = app.StatusUnknown
	}
	if avg, err := goload.AvgWithContext(ctx); err == nil {
		snap.CPULoad1 = avg.Load1
	}

	vm, err := gomem.VirtualMemoryWithContext(ctx)
	if err == nil {
		snap.MemoryUsed = vm.Used
		snap.MemoryTotal = vm.Total
		snap.MemoryUsedPct = vm.UsedPercent
	} else {
		snap.Errors = append(snap.Errors, "memory unavailable: "+err.Error())
		snap.MemoryStatus = app.StatusUnknown
	}

	cpuThreshold := app.Threshold{Warning: c.cfg.Thresholds.CPU.Warning, Critical: c.cfg.Thresholds.CPU.Critical}
	memoryThreshold := app.Threshold{Warning: c.cfg.Thresholds.Memory.Warning, Critical: c.cfg.Thresholds.Memory.Critical}
	diskThreshold := app.Threshold{Warning: c.cfg.Thresholds.Disk.Warning, Critical: c.cfg.Thresholds.Disk.Critical}

	if snap.CPUStatus == "" {
		snap.CPUStatus = cpuThreshold.Evaluate(snap.CPUPercent)
	}
	if snap.MemoryStatus == "" {
		snap.MemoryStatus = memoryThreshold.Evaluate(snap.MemoryUsedPct)
	}

	snap.Disks = c.collectDisks(ctx, diskThreshold)
	snap.DiskStatus = aggregateDiskStatus(snap.Disks)
	snap.Network = c.collectNetwork(ctx)
	if snap.Network.Error != "" {
		snap.Errors = append(snap.Errors, snap.Network.Error)
	}
	snap.Services = c.collectServices(ctx)
	snap.ServiceStatus = aggregateServiceStatus(snap.Services)
	snap.Processes = c.collectProcesses(ctx)
	return snap
}

func (c *Collector) collectDisks(ctx context.Context, threshold app.Threshold) []app.DiskMetric {
	partitions, err := godisk.PartitionsWithContext(ctx, true)
	if err != nil {
		return []app.DiskMetric{{Mountpoint: "/", Status: app.StatusUnknown, Error: err.Error()}}
	}
	out := make([]app.DiskMetric, 0, len(partitions))
	seen := map[string]bool{}
	for _, part := range partitions {
		if seen[part.Mountpoint] {
			continue
		}
		seen[part.Mountpoint] = true
		usage, err := godisk.UsageWithContext(ctx, part.Mountpoint)
		metric := app.DiskMetric{Mountpoint: part.Mountpoint}
		if err != nil {
			metric.Status = app.StatusUnknown
			metric.Error = err.Error()
		} else {
			metric.TotalBytes = usage.Total
			metric.UsedBytes = usage.Used
			metric.UsedPct = usage.UsedPercent
			metric.Status = threshold.Evaluate(usage.UsedPercent)
		}
		out = append(out, metric)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mountpoint < out[j].Mountpoint })
	return out
}

func aggregateDiskStatus(disks []app.DiskMetric) app.Status {
	status := app.StatusHealthy
	for _, disk := range disks {
		if rank(disk.Status) > rank(status) {
			status = disk.Status
		}
	}
	if len(disks) == 0 {
		return app.StatusUnknown
	}
	return status
}

func (c *Collector) collectNetwork(ctx context.Context) app.NetworkMetric {
	iface := c.cfg.Monitor.PrimaryInterface
	if iface == "" {
		detected, err := detectPrimaryInterface()
		if err == nil {
			iface = detected
		}
	}
	metric := app.NetworkMetric{Interface: iface}
	counters, err := gonet.IOCountersWithContext(ctx, true)
	if err != nil {
		metric.Error = "network unavailable: " + err.Error()
		return metric
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, counter := range counters {
		if counter.Name != iface {
			continue
		}
		prev, ok := c.prevNet[counter.Name]
		if ok && !c.prevTime.IsZero() {
			seconds := now.Sub(c.prevTime).Seconds()
			if seconds > 0 {
				metric.RXBytesPerS = float64(counter.BytesRecv-prev.rx) / seconds
				metric.TXBytesPerS = float64(counter.BytesSent-prev.tx) / seconds
			}
		}
		c.prevNet[counter.Name] = netCounters{rx: counter.BytesRecv, tx: counter.BytesSent}
		c.prevTime = now
		return metric
	}
	metric.Error = "network interface unavailable"
	return metric
}

func detectPrimaryInterface() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		return iface.Name, nil
	}
	return "", errors.New("no active interface found")
}

func (c *Collector) collectServices(ctx context.Context) []app.ServiceMetric {
	out := make([]app.ServiceMetric, 0, len(c.cfg.Monitor.TrackedServices))
	for _, service := range c.cfg.Monitor.TrackedServices {
		metric := app.ServiceMetric{Name: service, UpdatedAt: time.Now()}
		cmd := exec.CommandContext(ctx, "systemctl", "show", service, "--property=ActiveState,SubState", "--value")
		output, err := cmd.Output()
		if err != nil {
			metric.Status = app.StatusUnknown
			metric.Error = err.Error()
			out = append(out, metric)
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 0 {
			metric.State = lines[0]
		}
		if len(lines) > 1 {
			metric.SubState = lines[1]
		}
		metric.Status = serviceStatus(metric.State, metric.SubState)
		out = append(out, metric)
	}
	return out
}

func serviceStatus(state, subState string) app.Status {
	switch state {
	case "active":
		return app.StatusHealthy
	case "activating", "reloading":
		return app.StatusWarning
	case "failed", "inactive", "deactivating":
		return app.StatusCritical
	default:
		if state == "" && subState == "" {
			return app.StatusUnknown
		}
		return app.StatusUnknown
	}
}

func aggregateServiceStatus(services []app.ServiceMetric) app.Status {
	if len(services) == 0 {
		return app.StatusHealthy
	}
	status := app.StatusHealthy
	for _, service := range services {
		if rank(service.Status) > rank(status) {
			status = service.Status
		}
	}
	return status
}

func (c *Collector) collectProcesses(ctx context.Context) []app.ProcessMetric {
	cmd := exec.CommandContext(ctx, "ps", "-eo", "pid,user,comm,%cpu,rss", "--sort=-%cpu")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	first := true
	out := make([]app.ProcessMetric, 0, 5)
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		cpuPct, err2 := strconv.ParseFloat(fields[3], 64)
		rssKB, err3 := strconv.ParseUint(fields[4], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		out = append(out, app.ProcessMetric{
			PID:       pid,
			User:      fields[1],
			Command:   fields[2],
			CPU:       cpuPct,
			MemoryRSS: rssKB * 1024,
		})
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func rank(status app.Status) int {
	switch status {
	case app.StatusCritical:
		return 4
	case app.StatusWarning:
		return 3
	case app.StatusUnknown:
		return 2
	default:
		return 1
	}
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

type Sampler struct {
	Collector *Collector
}

func (s Sampler) EmitEvents(previous, current app.HostSnapshot) []app.Event {
	now := current.CollectedAt
	var events []app.Event
	compare := []struct {
		source string
		prev   app.Status
		curr   app.Status
		title  string
	}{
		{"cpu", previous.CPUStatus, current.CPUStatus, "CPU health changed"},
		{"memory", previous.MemoryStatus, current.MemoryStatus, "Memory health changed"},
		{"disk", previous.DiskStatus, current.DiskStatus, "Disk health changed"},
		{"services", previous.ServiceStatus, current.ServiceStatus, "Service health changed"},
	}
	for _, item := range compare {
		if item.prev != "" && item.prev != item.curr {
			events = append(events, app.Event{
				Kind:      "threshold_transition",
				Source:    item.source,
				Status:    item.curr,
				Title:     item.title,
				Details:   fmt.Sprintf("%s -> %s", item.prev.Label(), item.curr.Label()),
				CreatedAt: now,
			})
		}
	}

	prevServices := map[string]app.ServiceMetric{}
	for _, service := range previous.Services {
		prevServices[service.Name] = service
	}
	for _, service := range current.Services {
		prev, ok := prevServices[service.Name]
		if ok && prev.State != service.State {
			events = append(events, app.Event{
				Kind:      "service_state_change",
				Source:    service.Name,
				Status:    service.Status,
				Title:     fmt.Sprintf("Service %s changed state", service.Name),
				Details:   fmt.Sprintf("%s -> %s", prev.State, service.State),
				CreatedAt: now,
			})
		}
	}

	prevErrs := app.JoinErrors(previous.Errors)
	currErrs := app.JoinErrors(current.Errors)
	switch {
	case prevErrs == "" && currErrs != "":
		events = append(events, app.Event{Kind: "collector_failure", Source: "collector", Status: app.StatusUnknown, Title: "Collector issue detected", Details: currErrs, CreatedAt: now})
	case prevErrs != "" && currErrs == "":
		events = append(events, app.Event{Kind: "collector_recovery", Source: "collector", Status: app.StatusHealthy, Title: "Collector recovered", Details: "Collectors returned to healthy state", CreatedAt: now})
	}
	return events
}
