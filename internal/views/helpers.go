package views

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DioSaputra28/monitor-vps/internal/app"
)

func themeRoot(theme string) string {
	if theme == "light" {
		return "light"
	}
	return "dark"
}

func shellBackground(theme string) string {
	if theme == "light" {
		return "min-h-screen bg-mesh-light text-slate-900"
	}
	return "min-h-screen bg-mesh-dark text-on-surface"
}

func contentTone(theme string) string {
	if theme == "light" {
		return "text-slate-600"
	}
	return "text-on-surface-variant"
}

func cardTone(theme string) string {
	if theme == "light" {
		return "text-slate-900"
	}
	return "text-on-surface"
}

func navLink(path, current string) string {
	base := "rounded-full px-4 py-2 text-sm font-semibold transition-colors "
	if path == current {
		return base + "bg-primary/15 text-primary"
	}
	return base + "text-on-surface-variant hover:text-on-surface hover:bg-white/5"
}

func fmtPct(v float64) string            { return fmt.Sprintf("%.1f%%", v) }
func fmtFloat(v float64) string          { return fmt.Sprintf("%.2f", v) }
func fmtBytes(v uint64) string           { return app.HumanBytes(v) }
func fmtRate(v float64) string           { return app.HumanRate(v) }
func fmtDuration(v time.Duration) string { return app.HumanDuration(v) }
func fmtTime(v time.Time) string         { return v.Local().Format("15:04:05") }
func fmtDateTime(v time.Time) string     { return v.Local().Format("2006-01-02 15:04:05") }
func widthStyle(v float64) string        { return fmt.Sprintf("width:%.0f%%", v) }

func statusText(s app.Status) string { return s.Label() }
func statusClass(s app.Status) string {
	return s.CSSClass() + " inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-bold uppercase tracking-[0.2em]"
}

func overallStatus(snapshot app.HostSnapshot) app.Status { return snapshot.OverallStatus() }

func toggleTheme(theme string) string {
	if theme == "light" {
		return "dark"
	}
	return "light"
}

func themeButtonLabel(theme string) string {
	if theme == "light" {
		return "Dark"
	}
	return "Light"
}

func processMemory(snapshot app.HostSnapshot, p app.ProcessMetric) string {
	if snapshot.MemoryTotal == 0 {
		return fmtBytes(p.MemoryRSS)
	}
	pct := (float64(p.MemoryRSS) / float64(snapshot.MemoryTotal)) * 100
	return fmt.Sprintf("%s • %.1f%%", fmtBytes(p.MemoryRSS), pct)
}

func sortedConfig(in map[string]string) [][2]string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, [2]string{key, in[key]})
	}
	return out
}

func joinServices(services []string) string {
	if len(services) == 0 {
		return "None configured"
	}
	return strings.Join(services, ", ")
}
