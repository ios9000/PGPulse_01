package config

import "time"

// Config is the top-level PGPulse configuration.
type Config struct {
	Server    ServerConfig     `koanf:"server"`
	Storage   StorageConfig    `koanf:"storage"`
	Instances []InstanceConfig `koanf:"instances"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Listen          string        `koanf:"listen"`
	LogLevel        string        `koanf:"log_level"`
	CORSEnabled     bool          `koanf:"cors_enabled"`
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
