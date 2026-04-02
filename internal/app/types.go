package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusWarning  Status = "warning"
	StatusCritical Status = "critical"
	StatusUnknown  Status = "unknown"
)

func (s Status) Label() string {
	switch s {
	case StatusHealthy:
		return "Healthy"
	case StatusWarning:
		return "Warning"
	case StatusCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

func (s Status) CSSClass() string {
	switch s {
	case StatusHealthy:
		return "status-healthy"
	case StatusWarning:
		return "status-warning"
	case StatusCritical:
		return "status-critical"
	default:
		return "status-unknown"
	}
}

type Threshold struct {
	Warning  float64 `yaml:"warning"`
	Critical float64 `yaml:"critical"`
}

func (t Threshold) Evaluate(value float64) Status {
	if value >= t.Critical {
		return StatusCritical
	}
	if value >= t.Warning {
		return StatusWarning
	}
	return StatusHealthy
}

type DiskMetric struct {
	Mountpoint string
	UsedBytes  uint64
	TotalBytes uint64
	UsedPct    float64
	Status     Status
	Error      string
}

type ServiceMetric struct {
	Name      string
	State     string
	SubState  string
	Status    Status
	Error     string
	UpdatedAt time.Time
}

type ProcessMetric struct {
	PID       int
	User      string
	Command   string
	CPU       float64
	MemoryPct float64
	MemoryRSS uint64
}

type NetworkMetric struct {
	Interface   string
	RXBytesPerS float64
	TXBytesPerS float64
	Error       string
}

type HostSnapshot struct {
	Hostname      string
	CollectedAt   time.Time
	Uptime        time.Duration
	CPUPercent    float64
	CPULoad1      float64
	MemoryUsed    uint64
	MemoryTotal   uint64
	MemoryUsedPct float64
	CPUStatus     Status
	MemoryStatus  Status
	DiskStatus    Status
	ServiceStatus Status
	Network       NetworkMetric
	Disks         []DiskMetric
	Services      []ServiceMetric
	Processes     []ProcessMetric
	Errors        []string
}

func (s HostSnapshot) OverallStatus() Status {
	statuses := []Status{s.CPUStatus, s.MemoryStatus, s.DiskStatus, s.ServiceStatus}
	sort.Slice(statuses, func(i, j int) bool {
		return statusRank(statuses[i]) > statusRank(statuses[j])
	})
	return statuses[0]
}

func statusRank(s Status) int {
	switch s {
	case StatusCritical:
		return 4
	case StatusWarning:
		return 3
	case StatusUnknown:
		return 2
	case StatusHealthy:
		return 1
	default:
		return 0
	}
}

type Event struct {
	ID        int64
	Kind      string
	Source    string
	Status    Status
	Title     string
	Details   string
	CreatedAt time.Time
}

type MetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type ChartSeries struct {
	Name   string        `json:"name"`
	Points []MetricPoint `json:"points"`
}

type DashboardData struct {
	Snapshot        HostSnapshot
	Events          []Event
	Theme           string
	CSRFToken       string
	ChartRanges     []string
	SelectedRange   string
	RefreshSummary  string
	RefreshServices string
	RefreshEvents   string
	RefreshChart    string
}

type LoginPageData struct {
	Theme     string
	CSRFToken string
	Error     string
}

type ServicesPageData struct {
	Snapshot  HostSnapshot
	Events    []Event
	Theme     string
	CSRFToken string
}

type HistoryPageData struct {
	Snapshot      HostSnapshot
	Theme         string
	CSRFToken     string
	SelectedRange string
	Available     []string
}

type SettingsPageData struct {
	Hostname  string
	Theme     string
	CSRFToken string
	Config    map[string]string
}

func HumanBytes(v uint64) string {
	if v == 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(v)
	idx := 0
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", v, units[idx])
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}

func HumanRate(v float64) string {
	if v < 0 {
		return "0 B/s"
	}
	return fmt.Sprintf("%s/s", HumanBytes(uint64(v)))
}

func HumanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	if days > 0 {
		return fmt.Sprintf("%dd %02dh %02dm", days, hours, minutes)
	}
	return fmt.Sprintf("%02dh %02dm", hours, minutes)
}

func JoinErrors(errs []string) string {
	filtered := make([]string, 0, len(errs))
	for _, err := range errs {
		if strings.TrimSpace(err) != "" {
			filtered = append(filtered, err)
		}
	}
	return strings.Join(filtered, "; ")
}
