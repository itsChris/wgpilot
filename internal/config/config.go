package config

import (
	"fmt"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

// Config holds all configuration for wgpilot.
type Config struct {
	Server   ServerConfig   `koanf:"server"`
	Database DatabaseConfig `koanf:"database"`
	Auth     AuthConfig     `koanf:"auth"`
	TLS      TLSConfig      `koanf:"tls"`
	Logging  LoggingConfig  `koanf:"logging"`
	Monitor  MonitorConfig  `koanf:"monitor"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Listen  string `koanf:"listen"`
	DevMode bool   `koanf:"dev_mode"`
}

// DatabaseConfig holds SQLite settings.
type DatabaseConfig struct {
	Path string `koanf:"path"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	SessionTTL    string `koanf:"session_ttl"`
	BcryptCost    int    `koanf:"bcrypt_cost"`
	RateLimitRPM  int    `koanf:"rate_limit_rpm"`
}

// TLSConfig holds TLS certificate settings.
type TLSConfig struct {
	Mode     string `koanf:"mode"`
	CertFile string `koanf:"cert_file"`
	KeyFile  string `koanf:"key_file"`
	ACMEEmail string `koanf:"acme_email"`
	ACMEDomain string `koanf:"acme_domain"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// MonitorConfig holds monitoring/polling settings.
type MonitorConfig struct {
	PollInterval       string `koanf:"poll_interval"`
	SnapshotRetention  string `koanf:"snapshot_retention"`
	CompactionInterval string `koanf:"compaction_interval"`
}

// Load reads configuration with priority: flags > env > yaml file > defaults.
func Load(configPath string, flags *pflag.FlagSet) (*Config, error) {
	k := koanf.New(".")

	// 1. Load defaults.
	if err := loadDefaults(k); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	// 2. Load YAML config file (if exists).
	if configPath != "" {
		if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load config file %s: %w", configPath, err)
		}
	}

	// 3. Load environment variables (WGPILOT_ prefix).
	if err := k.Load(env.Provider("WGPILOT_", ".", func(s string) string {
		return strings.Replace(
			strings.ToLower(strings.TrimPrefix(s, "WGPILOT_")),
			"_", ".", -1,
		)
	}), nil); err != nil {
		return nil, fmt.Errorf("load env vars: %w", err)
	}

	// 4. Load CLI flags (highest priority).
	if flags != nil {
		if err := k.Load(posflag.Provider(flags, ".", k), nil); err != nil {
			return nil, fmt.Errorf("load flags: %w", err)
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func loadDefaults(k *koanf.Koanf) error {
	defaults := map[string]any{
		"server.listen":               "0.0.0.0:443",
		"server.dev_mode":             false,
		"database.path":               "/var/lib/wgpilot/wgpilot.db",
		"auth.session_ttl":            "24h",
		"auth.bcrypt_cost":            12,
		"auth.rate_limit_rpm":         5,
		"tls.mode":                    "self-signed",
		"logging.level":               "info",
		"logging.format":              "json",
		"monitor.poll_interval":       "30s",
		"monitor.snapshot_retention":  "30d",
		"monitor.compaction_interval": "24h",
	}

	for key, val := range defaults {
		if err := k.Set(key, val); err != nil {
			return fmt.Errorf("set default %s: %w", key, err)
		}
	}

	return nil
}
