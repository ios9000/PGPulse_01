package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Load reads configuration from the YAML file at path, applies
// PGPULSE_* environment variable overrides, and validates the result.
func Load(path string) (Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return Config{}, fmt.Errorf("load config file %q: %w", path, err)
	}

	if err := k.Load(env.Provider("PGPULSE_", ".", transformEnvKey), nil); err != nil {
		return Config{}, fmt.Errorf("load env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// transformEnvKey maps PGPULSE_SERVER_LISTEN → server.listen.
// koanf passes the full env key name; we strip the prefix, lowercase, and
// replace underscores with dots to match the YAML key hierarchy.
func transformEnvKey(s string) string {
	s = strings.TrimPrefix(s, "PGPULSE_")
	return strings.ReplaceAll(strings.ToLower(s), "_", ".")
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// validate applies defaults and enforces required fields.
func validate(cfg *Config) error {
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8080"
	}
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = "info"
	}
	if !validLogLevels[cfg.Server.LogLevel] {
		return fmt.Errorf("server.log_level %q must be one of: debug, info, warn, error", cfg.Server.LogLevel)
	}

	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 60 * time.Second
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 10 * time.Second
	}
	if cfg.Server.CORSOrigin == "" {
		cfg.Server.CORSOrigin = "http://localhost:5173"
	}

	if cfg.Storage.RetentionDays == 0 {
		cfg.Storage.RetentionDays = 30
	}

	if err := validateAuth(cfg); err != nil {
		return err
	}

	if err := validateAlerting(cfg); err != nil {
		return err
	}

	// ML forecast defaults.
	if cfg.ML.Forecast.AlertMinConsecutive == 0 {
		cfg.ML.Forecast.AlertMinConsecutive = 3
	}

	if len(cfg.Instances) == 0 {
		return fmt.Errorf("at least one instance must be configured")
	}

	for i := range cfg.Instances {
		inst := &cfg.Instances[i]

		if inst.ID == "" {
			return fmt.Errorf("instance[%d]: id is required", i)
		}
		if inst.DSN == "" {
			return fmt.Errorf("instance[%d]: dsn is required", i)
		}
		if inst.Enabled == nil {
			v := true
			inst.Enabled = &v
		}
		if inst.MaxConns == 0 {
			inst.MaxConns = 5
		}
		if inst.Intervals.High == 0 {
			inst.Intervals.High = 10 * time.Second
		}
		if inst.Intervals.Medium == 0 {
			inst.Intervals.Medium = 60 * time.Second
		}
		if inst.Intervals.Low == 0 {
			inst.Intervals.Low = 300 * time.Second
		}
	}

	return nil
}

// validateAlerting applies alerting defaults and validates alerting config when enabled.
func validateAlerting(cfg *Config) error {
	alertingDefaults(&cfg.Alerting)
	if cfg.Alerting.Enabled && cfg.Storage.DSN == "" {
		return fmt.Errorf("alerting.enabled=true requires storage.dsn to be configured")
	}
	for _, ch := range cfg.Alerting.DefaultChannels {
		if ch == "email" {
			if cfg.Alerting.Email == nil {
				return fmt.Errorf("alerting.email config required when 'email' is in default_channels")
			}
			if cfg.Alerting.Email.Host == "" {
				return fmt.Errorf("alerting.email.host is required")
			}
			if cfg.Alerting.Email.From == "" {
				return fmt.Errorf("alerting.email.from is required")
			}
			if len(cfg.Alerting.Email.Recipients) == 0 {
				return fmt.Errorf("alerting.email.recipients must not be empty")
			}
		}
	}
	return nil
}

func alertingDefaults(c *AlertingConfig) {
	if c.DefaultConsecutiveCount == 0 {
		c.DefaultConsecutiveCount = 3
	}
	if c.DefaultCooldownMinutes == 0 {
		c.DefaultCooldownMinutes = 15
	}
	if c.EvaluationTimeoutSec == 0 {
		c.EvaluationTimeoutSec = 5
	}
	if c.HistoryRetentionDays == 0 {
		c.HistoryRetentionDays = 30
	}
	if c.Email != nil {
		if c.Email.Port == 0 {
			c.Email.Port = 587
		}
		if c.Email.SendTimeoutSeconds == 0 {
			c.Email.SendTimeoutSeconds = 10
		}
	}
}

// validateAuth applies auth defaults and validates auth config when enabled.
func validateAuth(cfg *Config) error {
	if !cfg.Auth.Enabled {
		return nil
	}
	if cfg.Storage.DSN == "" {
		return fmt.Errorf("auth.enabled=true requires storage.dsn to be configured")
	}
	if len(cfg.Auth.JWTSecret) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 characters")
	}
	if cfg.Auth.AccessTokenTTL == 0 {
		cfg.Auth.AccessTokenTTL = 24 * time.Hour
	}
	if cfg.Auth.RefreshTokenTTL == 0 {
		cfg.Auth.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	if cfg.Auth.BcryptCost == 0 {
		cfg.Auth.BcryptCost = 12
	}
	return nil
}
