package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Auth struct {
		CookieSecret string `yaml:"cookie_secret"`
	} `yaml:"auth"`
	Sampling struct {
		Interval time.Duration `yaml:"interval"`
	} `yaml:"sampling"`
	Retention struct {
		Days int `yaml:"days"`
	} `yaml:"retention"`
	UI struct {
		DefaultTheme string `yaml:"default_theme"`
	} `yaml:"ui"`
	Monitor struct {
		PrimaryInterface string   `yaml:"primary_interface"`
		TrackedServices  []string `yaml:"tracked_services"`
	} `yaml:"monitor"`
	Thresholds struct {
		CPU    ThresholdConfig `yaml:"cpu"`
		Memory ThresholdConfig `yaml:"memory"`
		Disk   ThresholdConfig `yaml:"disk"`
	} `yaml:"thresholds"`
	Security struct {
		TrustedProxies []string `yaml:"trusted_proxies"`
	} `yaml:"security"`
	Paths struct {
		Database string `yaml:"database"`
		LogDir   string `yaml:"log_dir"`
	} `yaml:"paths"`
}

type ThresholdConfig struct {
	Warning  float64 `yaml:"warning"`
	Critical float64 `yaml:"critical"`
}

const (
	DefaultAppDir = "/opt/dimonitorin"
	ConfigName    = "config.yaml"
	DBName        = "dimonitorin.db"
)

func Default(appDir string) Config {
	if appDir == "" {
		appDir = DefaultAppDir
	}
	var cfg Config
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 8080
	cfg.Auth.CookieSecret = mustSecret()
	cfg.Sampling.Interval = 10 * time.Second
	cfg.Retention.Days = 14
	cfg.UI.DefaultTheme = "dark"
	cfg.Thresholds.CPU = ThresholdConfig{Warning: 80, Critical: 90}
	cfg.Thresholds.Memory = ThresholdConfig{Warning: 80, Critical: 90}
	cfg.Thresholds.Disk = ThresholdConfig{Warning: 80, Critical: 90}
	cfg.Paths.Database = filepath.Join(appDir, DBName)
	cfg.Paths.LogDir = filepath.Join(appDir, "logs")
	return cfg
}

func mustSecret() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default(filepath.Dir(path))
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, cfg.Validate()
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c Config) Validate() error {
	switch {
	case strings.TrimSpace(c.Server.Host) == "":
		return errors.New("server.host is required")
	case c.Server.Port <= 0 || c.Server.Port > 65535:
		return errors.New("server.port must be between 1 and 65535")
	case c.Auth.CookieSecret == "":
		return errors.New("auth.cookie_secret is required")
	case c.Sampling.Interval < time.Second:
		return errors.New("sampling.interval must be at least 1s")
	case c.Retention.Days < 1:
		return errors.New("retention.days must be >= 1")
	case c.Paths.Database == "":
		return errors.New("paths.database is required")
	}
	if c.Thresholds.CPU.Warning <= 0 || c.Thresholds.CPU.Critical <= c.Thresholds.CPU.Warning {
		return errors.New("thresholds.cpu are invalid")
	}
	if c.Thresholds.Memory.Warning <= 0 || c.Thresholds.Memory.Critical <= c.Thresholds.Memory.Warning {
		return errors.New("thresholds.memory are invalid")
	}
	if c.Thresholds.Disk.Warning <= 0 || c.Thresholds.Disk.Critical <= c.Thresholds.Disk.Warning {
		return errors.New("thresholds.disk are invalid")
	}
	return nil
}

func ConfigPath(appDir string) string {
	if appDir == "" {
		appDir = DefaultAppDir
	}
	return filepath.Join(appDir, ConfigName)
}

func EnsureAppDirs(appDir string) error {
	paths := []string{appDir, filepath.Join(appDir, "logs"), filepath.Join(appDir, "backups")}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func Set(cfg *Config, key string, value string) error {
	switch key {
	case "server.host":
		cfg.Server.Host = strings.TrimSpace(value)
	case "server.port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.Server.Port = port
	case "auth.cookie_secret":
		cfg.Auth.CookieSecret = strings.TrimSpace(value)
	case "sampling.interval":
		dur, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		cfg.Sampling.Interval = dur
	case "retention.days":
		days, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		cfg.Retention.Days = days
	case "ui.default_theme":
		cfg.UI.DefaultTheme = strings.ToLower(value)
	case "monitor.primary_interface":
		cfg.Monitor.PrimaryInterface = value
	case "monitor.tracked_services":
		if strings.TrimSpace(value) == "" {
			cfg.Monitor.TrackedServices = nil
		} else {
			parts := strings.Split(value, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					out = append(out, part)
				}
			}
			cfg.Monitor.TrackedServices = out
		}
	case "thresholds.cpu.warning":
		return setFloat(&cfg.Thresholds.CPU.Warning, value)
	case "thresholds.cpu.critical":
		return setFloat(&cfg.Thresholds.CPU.Critical, value)
	case "thresholds.memory.warning":
		return setFloat(&cfg.Thresholds.Memory.Warning, value)
	case "thresholds.memory.critical":
		return setFloat(&cfg.Thresholds.Memory.Critical, value)
	case "thresholds.disk.warning":
		return setFloat(&cfg.Thresholds.Disk.Warning, value)
	case "thresholds.disk.critical":
		return setFloat(&cfg.Thresholds.Disk.Critical, value)
	case "security.trusted_proxies":
		if strings.TrimSpace(value) == "" {
			cfg.Security.TrustedProxies = nil
		} else {
			cfg.Security.TrustedProxies = strings.Split(value, ",")
		}
	default:
		return fmt.Errorf("unsupported config key %q", key)
	}
	return cfg.Validate()
}

func setFloat(target *float64, value string) error {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	*target = parsed
	return nil
}
