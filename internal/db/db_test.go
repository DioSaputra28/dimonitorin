package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DioSaputra28/monitor-vps/internal/app"
)

func TestStoreSampleEventBackupAndPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dimonitorin.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	snapshot := app.HostSnapshot{
		Hostname:      "test-host",
		CollectedAt:   time.Now(),
		CPUPercent:    42,
		CPULoad1:      0.7,
		MemoryUsed:    1024,
		MemoryTotal:   2048,
		MemoryUsedPct: 50,
		CPUStatus:     app.StatusHealthy,
		MemoryStatus:  app.StatusHealthy,
		DiskStatus:    app.StatusHealthy,
		ServiceStatus: app.StatusHealthy,
		Network:       app.NetworkMetric{Interface: "eth0", RXBytesPerS: 10, TXBytesPerS: 20},
	}
	if err := store.InsertSample(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.AddEvent(context.Background(), app.Event{Kind: "test", Source: "unit", Status: app.StatusHealthy, Title: "ok", Details: "fine", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	latest, err := store.LatestSample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest.Hostname != snapshot.Hostname {
		t.Fatalf("unexpected latest snapshot host %s", latest.Hostname)
	}
	backup := filepath.Join(dir, "backup.tar.gz")
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.ExportBackup(context.Background(), configPath, path, backup); err != nil {
		t.Fatal(err)
	}
	if err := store.Prune(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}
