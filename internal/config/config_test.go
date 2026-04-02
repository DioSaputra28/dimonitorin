package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultAndSet(t *testing.T) {
	cfg := Default(t.TempDir())
	if cfg.Server.Port != 8080 {
		t.Fatalf("unexpected port %d", cfg.Server.Port)
	}
	if err := Set(&cfg, "server.host", "0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("unexpected host %q", cfg.Server.Host)
	}
	if err := Set(&cfg, "sampling.interval", "30s"); err != nil {
		t.Fatal(err)
	}
	if cfg.Sampling.Interval != 30*time.Second {
		t.Fatalf("unexpected sampling interval %s", cfg.Sampling.Interval)
	}
	if err := Set(&cfg, "monitor.tracked_services", "nginx,postgresql"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Monitor.TrackedServices) != 2 {
		t.Fatalf("unexpected tracked services: %#v", cfg.Monitor.TrackedServices)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := Default(dir)
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Paths.Database != cfg.Paths.Database {
		t.Fatalf("db path mismatch: %s vs %s", loaded.Paths.Database, cfg.Paths.Database)
	}
}
