package config

import "time"

// Config is the top-level PGPulse configuration.
type Config struct {
	Server    ServerConfig     `koanf:"server"`
	Storage   StorageConfig    `koanf:"storage"`
	Auth      AuthConfig       `koanf:"auth"`
	Alerting  AlertingConfig   `koanf:"alerting"`
	Instances []InstanceConfig `koanf:"instances"`
}

// AlertingConfig holds alerting engine settings.
type AlertingConfig struct {
	Enabled                 bool         `koanf:"enabled"`
	DefaultConsecutiveCount int          `koanf:"default_consecutive_count"`
	DefaultCooldownMinutes  int          `koanf:"default_cooldown_minutes"`
	DefaultChannels         []string     `koanf:"default_channels"`
	EvaluationTimeoutSec    int          `koanf:"evaluation_timeout_seconds"`
	HistoryRetentionDays    int          `koanf:"history_retention_days"`
	DashboardURL            string       `koanf:"dashboard_url"`
	Email                   *EmailConfig `koanf:"email"`
}

// EmailConfig holds SMTP email notification settings.
type EmailConfig struct {
	Host               string   `koanf:"host"`
	Port               int      `koanf:"port"`
	Username           string   `koanf:"username"`
	Password           string   `koanf:"password"`
	From               string   `koanf:"from"`
	Recipients         []string `koanf:"recipients"`
	TLSSkipVerify      bool     `koanf:"tls_skip_verify"`
	SendTimeoutSeconds int      `koanf:"send_timeout_seconds"`
}

// AuthConfig holds JWT authentication settings.
type AuthConfig struct {
	Enabled         bool                `koanf:"enabled"`           // default false
	JWTSecret       string              `koanf:"jwt_secret"`        // required when enabled
	RefreshSecret   string              `koanf:"refresh_secret"`    // separate secret for refresh tokens; defaults to jwt_secret
	AccessTokenTTL  time.Duration       `koanf:"access_token_ttl"`  // default 24h
	RefreshTokenTTL time.Duration       `koanf:"refresh_token_ttl"` // default 168h (7d)
	BcryptCost      int                 `koanf:"bcrypt_cost"`       // default 12
	InitialAdmin    *InitialAdminConfig `koanf:"initial_admin"`
}

// InitialAdminConfig holds credentials for the first admin user seeded on startup.
type InitialAdminConfig struct {
	Username string `koanf:"username"`
	Password string `koanf:"password"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Listen          string        `koanf:"listen"`
	LogLevel        string        `koanf:"log_level"`
	CORSEnabled     bool          `koanf:"cors_enabled"`
	CORSOrigin      string        `koanf:"cors_origin"`
	ReadTimeout     time.Duration `koanf:"read_timeout"`
	WriteTimeout    time.Duration `koanf:"write_timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
}

// StorageConfig holds the PGPulse metadata storage settings.
type StorageConfig struct {
	DSN            string `koanf:"dsn"`
	UseTimescaleDB bool   `koanf:"use_timescaledb"`
	RetentionDays  int    `koanf:"retention_days"`
}

// InstanceConfig holds configuration for a single monitored PostgreSQL instance.
type InstanceConfig struct {
	ID          string         `koanf:"id"`
	DSN         string         `koanf:"dsn"`
	Description string         `koanf:"description"` // optional human-readable label
	Enabled     *bool          `koanf:"enabled"`     // pointer: nil means "not set" → default true
	Intervals   IntervalConfig `koanf:"intervals"`
}

// IntervalConfig defines collection frequency tiers for an instance.
type IntervalConfig struct {
	High   time.Duration `koanf:"high"`   // default 10s  — connections, locks, wait events
	Medium time.Duration `koanf:"medium"` // default 60s  — replication, statements, checkpoint
	Low    time.Duration `koanf:"low"`    // default 300s — server info, sizes, settings
}
