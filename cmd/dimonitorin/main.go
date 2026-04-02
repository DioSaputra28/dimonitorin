package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DioSaputra28/monitor-vps/internal/auth"
	"github.com/DioSaputra28/monitor-vps/internal/config"
	"github.com/DioSaputra28/monitor-vps/internal/db"
	"github.com/DioSaputra28/monitor-vps/internal/monitor"
	"github.com/DioSaputra28/monitor-vps/internal/web"
)

const defaultSystemdUnitPath = "/etc/systemd/system/dimonitorin.service"

var stdinReader = bufio.NewReader(os.Stdin)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

type initOptions struct {
	Host                 string
	Port                 int
	NonInteractive       bool
	SkipServiceDiscovery bool
	ServicesCSV          string
	AdminUser            string
	AdminPasswordStdin   bool
}

type adminOptions struct {
	Username          string
	PasswordStdin     bool
	NonInteractive    bool
	PromptLabelPrefix string
}

type serviceInstallOptions struct {
	OutputPath string
	BinPath    string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	appDir := resolveAppDir(args)
	filtered := filteredArgs(args)
	if len(filtered) == 0 {
		return usage()
	}
	command := filtered[0]
	subArgs := filtered[1:]
	switch command {
	case "init":
		return cmdInit(appDir, subArgs)
	case "run":
		return cmdRun(appDir)
	case "create-admin":
		return cmdCreateAdmin(appDir, false, subArgs)
	case "reset-password":
		return cmdCreateAdmin(appDir, true, subArgs)
	case "config":
		return cmdConfig(appDir, subArgs)
	case "doctor":
		return cmdDoctor(appDir)
	case "backup":
		return cmdBackup(appDir, subArgs)
	case "service":
		return cmdService(appDir, subArgs)
	case "version":
		return cmdVersion()
	default:
		return usage()
	}
}

func usage() error {
	return errors.New("usage: dimonitorin [--app-dir PATH] <init|run|create-admin|reset-password|config|doctor|backup|service|version>")
}

func resolveAppDir(args []string) string {
	if env := os.Getenv("DIMONITORIN_APP_DIR"); env != "" {
		return env
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--app-dir" {
			return args[i+1]
		}
	}
	return config.DefaultAppDir
}

func filteredArgs(args []string) []string {
	out := make([]string, 0, len(args))
	skip := false
	for i, arg := range args {
		if skip {
			skip = false
			continue
		}
		if arg == "--app-dir" && i < len(args)-1 {
			skip = true
			continue
		}
		out = append(out, arg)
	}
	return out
}

func cmdInit(appDir string, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts initOptions
	fs.StringVar(&opts.Host, "host", "", "bind host override")
	fs.IntVar(&opts.Port, "port", 0, "bind port override")
	fs.BoolVar(&opts.NonInteractive, "non-interactive", false, "disable prompts and require stdin password")
	fs.BoolVar(&opts.SkipServiceDiscovery, "skip-service-discovery", false, "skip systemd service discovery")
	fs.StringVar(&opts.ServicesCSV, "services", "", "comma-separated tracked services")
	fs.StringVar(&opts.AdminUser, "admin-user", "", "admin username")
	fs.BoolVar(&opts.AdminPasswordStdin, "admin-password-stdin", false, "read admin password from stdin")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: dimonitorin [--app-dir PATH] init [--host HOST] [--port PORT] [--non-interactive] [--skip-service-discovery] [--services csv] [--admin-user USER] [--admin-password-stdin]: %w", err)
	}
	if fs.NArg() != 0 {
		return errors.New("usage: dimonitorin [--app-dir PATH] init [--host HOST] [--port PORT] [--non-interactive] [--skip-service-discovery] [--services csv] [--admin-user USER] [--admin-password-stdin]")
	}
	if opts.NonInteractive && !opts.AdminPasswordStdin {
		return errors.New("--admin-password-stdin is required with --non-interactive")
	}

	cfgPath := config.ConfigPath(appDir)
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("config already exists at %s", cfgPath)
	}
	if err := config.EnsureAppDirs(appDir); err != nil {
		return err
	}
	cfg := config.Default(appDir)
	if opts.Host != "" {
		cfg.Server.Host = opts.Host
	}
	if opts.Port != 0 {
		cfg.Server.Port = opts.Port
	}

	collector := monitor.NewCollector(cfg)
	discovered, err := discoverTrackedServices(context.Background(), collector, opts)
	if err != nil {
		return err
	}
	cfg.Monitor.TrackedServices = discovered

	store, err := db.Open(cfg.Paths.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	if err := createOrResetAdmin(store, false, adminOptions{
		Username:          opts.AdminUser,
		PasswordStdin:     opts.AdminPasswordStdin,
		NonInteractive:    opts.NonInteractive,
		PromptLabelPrefix: "Create",
	}); err != nil {
		return err
	}
	if err := web.WriteReverseProxyExamples(appDir); err != nil {
		return err
	}

	fmt.Println("DiMonitorin init")
	fmt.Printf("App dir: %s\n", appDir)
	fmt.Printf("Created config: %s\n", cfgPath)
	fmt.Printf("Created database: %s\n", cfg.Paths.Database)
	if len(cfg.Monitor.TrackedServices) > 0 {
		fmt.Printf("Tracked services: %s\n", strings.Join(cfg.Monitor.TrackedServices, ", "))
	} else {
		fmt.Println("Tracked services: none")
	}
	fmt.Println("Next steps:")
	fmt.Printf("  dimonitorin --app-dir %s run\n", appDir)
	fmt.Printf("  dimonitorin --app-dir %s service install\n", appDir)
	fmt.Printf("  Review reverse proxy examples in %s\n", appDir)
	return nil
}

func discoverTrackedServices(ctx context.Context, collector *monitor.Collector, opts initOptions) ([]string, error) {
	if strings.TrimSpace(opts.ServicesCSV) != "" {
		return splitCSV(opts.ServicesCSV), nil
	}
	if opts.SkipServiceDiscovery {
		return nil, nil
	}
	discovered, err := collector.DiscoverServices(ctx)
	if err != nil {
		return nil, err
	}
	if len(discovered) == 0 {
		return nil, nil
	}
	if opts.NonInteractive {
		return discovered, nil
	}
	fmt.Printf("Discovered services: %s\n", strings.Join(discovered, ", "))
	confirmed, err := promptCSV("Choose tracked services (comma-separated, blank to accept all discovered): ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(confirmed) == "" {
		return discovered, nil
	}
	return splitCSV(confirmed), nil
}

func cmdRun(appDir string) error {
	cfg, err := config.Load(config.ConfigPath(appDir))
	if err != nil {
		return err
	}
	store, err := db.Open(cfg.Paths.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	collector := monitor.NewCollector(cfg)
	server := web.New(cfg, store, collector)
	if err := server.BootstrapSample(context.Background()); err != nil {
		return err
	}
	handler, err := server.Routes()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go server.StartBackground(ctx)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	log.Printf("DiMonitorin listening on %s", addr)
	err = httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func cmdCreateAdmin(appDir string, reset bool, args []string) error {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts adminOptions
	fs.StringVar(&opts.Username, "username", "", "admin username")
	fs.BoolVar(&opts.PasswordStdin, "password-stdin", false, "read password from stdin")
	fs.BoolVar(&opts.NonInteractive, "non-interactive", false, "disable prompts and require --password-stdin")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: dimonitorin [--app-dir PATH] %s [--username USER] [--password-stdin] [--non-interactive]: %w", adminCommandName(reset), err)
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: dimonitorin [--app-dir PATH] %s [--username USER] [--password-stdin] [--non-interactive]", adminCommandName(reset))
	}
	if opts.NonInteractive && !opts.PasswordStdin {
		return errors.New("--password-stdin is required with --non-interactive")
	}

	cfg, err := config.Load(config.ConfigPath(appDir))
	if err != nil {
		return err
	}
	store, err := db.Open(cfg.Paths.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	if reset {
		opts.PromptLabelPrefix = "Reset"
	} else {
		opts.PromptLabelPrefix = "Create"
	}
	return createOrResetAdmin(store, reset, opts)
}

func adminCommandName(reset bool) string {
	if reset {
		return "reset-password"
	}
	return "create-admin"
}

func createOrResetAdmin(store *db.Store, reset bool, opts adminOptions) error {
	action := opts.PromptLabelPrefix
	if action == "" {
		action = "Create"
		if reset {
			action = "Reset"
		}
	}
	username := strings.TrimSpace(opts.Username)
	if username == "" {
		if opts.NonInteractive {
			username = "admin"
		} else {
			prompted, err := promptString(action + " admin username [admin]: ")
			if err != nil {
				return err
			}
			username = prompted
		}
	}
	if username == "" {
		username = "admin"
	}

	var password string
	var err error
	if opts.PasswordStdin {
		password, err = readLineFromStdin()
	} else if opts.NonInteractive {
		return fmt.Errorf("admin password is not set yet; provide one with --password-stdin/--admin-password-stdin (minimum %d characters)", auth.MinPasswordLength)
	} else {
		password, err = promptPassword(fmt.Sprintf("%s admin password (minimum %d characters): ", action, auth.MinPasswordLength))
	}
	if err != nil {
		return err
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("password cannot be empty (minimum %d characters)", auth.MinPasswordLength)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := store.EnsureAdmin(context.Background(), username, hash); err != nil {
		return err
	}
	fmt.Printf("Admin %s updated\n", username)
	return nil
}

func cmdConfig(appDir string, args []string) error {
	if len(args) < 3 || args[0] != "set" {
		return errors.New("usage: dimonitorin config set <key> <value>")
	}
	path := config.ConfigPath(appDir)
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if err := config.Set(&cfg, args[1], args[2]); err != nil {
		return err
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Printf("Updated %s\n", args[1])
	return nil
}

func cmdDoctor(appDir string) error {
	cfg, err := config.Load(config.ConfigPath(appDir))
	if err != nil {
		return err
	}
	store, err := db.Open(cfg.Paths.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	collector := monitor.NewCollector(cfg)
	snapshot := collector.Collect(context.Background())
	admins, err := store.AdminCount(context.Background())
	if err != nil {
		return err
	}
	if err := store.Health(context.Background()); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Paths.Database), 0o755); err != nil {
		return err
	}
	fmt.Println("Doctor checks")
	fmt.Printf("  Config: OK (%s)\n", config.ConfigPath(appDir))
	fmt.Printf("  Database: OK (%s)\n", cfg.Paths.Database)
	fmt.Printf("  Admin bootstrap: %d admin account(s)\n", admins)
	fmt.Printf("  Collector hostname: %s\n", snapshot.Hostname)
	fmt.Printf("  Collector errors: %s\n", emptyIfNone(strings.Join(snapshot.Errors, "; ")))
	return nil
}

func cmdBackup(appDir string, args []string) error {
	if len(args) == 0 || args[0] != "export" {
		return errors.New("usage: dimonitorin backup export")
	}
	cfg, err := config.Load(config.ConfigPath(appDir))
	if err != nil {
		return err
	}
	store, err := db.Open(cfg.Paths.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	dest := filepath.Join(appDir, "backups", fmt.Sprintf("dimonitorin-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405")))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := store.ExportBackup(context.Background(), config.ConfigPath(appDir), cfg.Paths.Database, dest); err != nil {
		return err
	}
	fmt.Printf("Backup written to %s\n", dest)
	return nil
}

func cmdService(appDir string, args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return errors.New("usage: dimonitorin service install [--output PATH] [--bin-path PATH]")
	}
	fs := flag.NewFlagSet("service install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts serviceInstallOptions
	fs.StringVar(&opts.OutputPath, "output", "", "service unit path override")
	fs.StringVar(&opts.BinPath, "bin-path", "", "binary path override")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("usage: dimonitorin service install [--output PATH] [--bin-path PATH]: %w", err)
	}
	if fs.NArg() != 0 {
		return errors.New("usage: dimonitorin service install [--output PATH] [--bin-path PATH]")
	}
	return installServiceUnit(appDir, opts)
}

func installServiceUnit(appDir string, opts serviceInstallOptions) error {
	binPath := opts.BinPath
	if binPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		binPath = exe
	}
	target, err := serviceTargetPath(opts.OutputPath)
	if err != nil {
		return err
	}
	unit := renderMainServiceUnit(appDir, binPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(unit), 0o644); err != nil {
		return err
	}
	fmt.Printf("Service file written to %s\n", target)
	fmt.Println("Suggested next steps:")
	fmt.Println("  sudo systemctl daemon-reload")
	fmt.Println("  sudo systemctl enable --now dimonitorin")
	return nil
}

func serviceTargetPath(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return override, nil
	}
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("service install requires root to write %s; rerun with sudo or pass --output", defaultSystemdUnitPath)
	}
	return defaultSystemdUnitPath, nil
}

func renderMainServiceUnit(appDir, binPath string) string {
	return fmt.Sprintf(`[Unit]
Description=DiMonitorin server monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s --app-dir %s run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, appDir, binPath, appDir)
}

func cmdVersion() error {
	fmt.Printf("DiMonitorin %s\n", version)
	fmt.Printf("commit: %s\n", commit)
	fmt.Printf("build date: %s\n", buildDate)
	return nil
}

func promptString(label string) (string, error) {
	fmt.Print(label)
	return readLineFromStdin()
}

func readLineFromStdin() (string, error) {
	text, err := stdinReader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func promptCSV(label string) (string, error) { return promptString(label) }

func promptPassword(label string) (string, error) {
	first, err := promptString(label)
	if err != nil {
		return "", err
	}
	second, err := promptString("Confirm password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("passwords do not match")
	}
	return first, nil
}

func splitCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func emptyIfNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}
