package main

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DioSaputra28/monitor-vps/internal/config"
	"github.com/DioSaputra28/monitor-vps/internal/db"
)

func TestCmdInitNonInteractive(t *testing.T) {
	appDir := t.TempDir()
	prevReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("supersecret123\n"))
	defer func() { stdinReader = prevReader }()

	err := cmdInit(appDir, []string{
		"--host", "0.0.0.0",
		"--port", "18080",
		"--non-interactive",
		"--skip-service-discovery",
		"--services", "nginx,redis",
		"--admin-user", "ops",
		"--admin-password-stdin",
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.ConfigPath(appDir))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("unexpected host %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 18080 {
		t.Fatalf("unexpected port %d", cfg.Server.Port)
	}
	if got := strings.Join(cfg.Monitor.TrackedServices, ","); got != "nginx,redis" {
		t.Fatalf("unexpected tracked services %q", got)
	}

	store, err := db.Open(cfg.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, err := store.GetAdminHash(context.Background(), "ops")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("expected stored admin hash")
	}
	for _, name := range []string{"reverse-proxy.caddy", "reverse-proxy.nginx"} {
		if _, err := os.Stat(filepath.Join(appDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func TestInstallServiceUnitWritesExpectedFile(t *testing.T) {
	appDir := t.TempDir()
	unitPath := filepath.Join(t.TempDir(), "dimonitorin.service")
	err := installServiceUnit(appDir, serviceInstallOptions{
		OutputPath: unitPath,
		BinPath:    "/usr/local/bin/dimonitorin",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"WorkingDirectory=" + appDir,
		"ExecStart=/usr/local/bin/dimonitorin --app-dir " + appDir + " run",
		"Restart=always",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("service unit missing %q in %s", want, body)
		}
	}
}
