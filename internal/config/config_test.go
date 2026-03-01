package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTempYAML writes content to a temp file and returns its path.
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTempYAML(t, `
server:
  listen: ":9090"
  log_level: "debug"
storage:
  dsn: "postgres://user:pass@localhost/pgpulse"
  use_timescaledb: true
  retention_days: 60
instances:
  - id: "test-db"
    dsn: "postgres://monitor:pass@localhost/postgres"
    enabled: true
    intervals:
      high: 5s
      medium: 30s
      low: 120s
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Listen != ":9090" {
		t.Errorf("Listen = %q, want %q", cfg.Server.Listen, ":9090")
	}
	if cfg.Server.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.Server.LogLevel, "debug")
	}
	if cfg.Storage.RetentionDays != 60 {
		t.Errorf("RetentionDays = %d, want 60", cfg.Storage.RetentionDays)
	}
	if cfg.Storage.DSN != "postgres://user:pass@localhost/pgpulse" {
		t.Errorf("Storage.DSN = %q", cfg.Storage.DSN)
	}
	if len(cfg.Instances) != 1 {
		t.Fatalf("Instances count = %d, want 1", len(cfg.Instances))
	}
	inst := cfg.Instances[0]
	if inst.ID != "test-db" {
		t.Errorf("ID = %q, want %q", inst.ID, "test-db")
	}
	if inst.Intervals.High != 5*time.Second {
		t.Errorf("High = %v, want 5s", inst.Intervals.High)
	}
	if inst.Intervals.Medium != 30*time.Second {
		t.Errorf("Medium = %v, want 30s", inst.Intervals.Medium)
	}
	if inst.Intervals.Low != 120*time.Second {
		t.Errorf("Low = %v, want 120s", inst.Intervals.Low)
	}
}

func TestLoad_Defaults(t *testing.T) {
	path := writeTempYAML(t, `
instances:
  - id: "minimal"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Listen != ":8080" {
		t.Errorf("Listen = %q, want \":8080\"", cfg.Server.Listen)
	}
	if cfg.Server.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want \"info\"", cfg.Server.LogLevel)
	}
	if cfg.Storage.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", cfg.Storage.RetentionDays)
	}
	inst := cfg.Instances[0]
	if inst.Enabled == nil || !*inst.Enabled {
		t.Errorf("Enabled: want non-nil true, got %v", inst.Enabled)
	}
	if inst.Intervals.High != 10*time.Second {
		t.Errorf("High = %v, want 10s", inst.Intervals.High)
	}
	if inst.Intervals.Medium != 60*time.Second {
		t.Errorf("Medium = %v, want 60s", inst.Intervals.Medium)
	}
	if inst.Intervals.Low != 300*time.Second {
		t.Errorf("Low = %v, want 300s", inst.Intervals.Low)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yml")
	if err == nil {
		t.Error("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempYAML(t, "{{invalid")
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error for invalid YAML, got nil")
	}
}

func TestLoad_NoInstances(t *testing.T) {
	path := writeTempYAML(t, `
server:
  listen: ":8080"
instances: []
`)
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected validation error for empty instances, got nil")
	}
}

func TestLoad_EmptyDSN(t *testing.T) {
	path := writeTempYAML(t, `
instances:
  - id: "no-dsn"
    dsn: ""
`)
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected validation error for empty DSN, got nil")
	}
}

func TestLoad_AuthEnabled_ValidConfig(t *testing.T) {
	path := writeTempYAML(t, `
storage:
  dsn: "postgres://user:pass@localhost/pgpulse"
auth:
  enabled: true
  jwt_secret: "this-is-a-secret-that-is-32-chars!!"
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Auth.Enabled {
		t.Error("Auth.Enabled should be true")
	}
	if cfg.Auth.AccessTokenTTL != 24*time.Hour {
		t.Errorf("AccessTokenTTL = %v, want 24h (default)", cfg.Auth.AccessTokenTTL)
	}
	if cfg.Auth.BcryptCost != 12 {
		t.Errorf("BcryptCost = %d, want 12 (default)", cfg.Auth.BcryptCost)
	}
}

func TestLoad_AuthEnabled_ShortSecret(t *testing.T) {
	path := writeTempYAML(t, `
storage:
  dsn: "postgres://user:pass@localhost/pgpulse"
auth:
  enabled: true
  jwt_secret: "tooshort"
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error for short jwt_secret, got nil")
	}
}

func TestLoad_AuthEnabled_NoDSN(t *testing.T) {
	path := writeTempYAML(t, `
auth:
  enabled: true
  jwt_secret: "this-is-a-secret-that-is-32-chars!!"
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error when auth enabled without storage.dsn, got nil")
	}
}

func TestLoad_AlertingDefaults(t *testing.T) {
	path := writeTempYAML(t, `
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Alerting.Enabled {
		t.Error("Alerting.Enabled should default to false")
	}
	if cfg.Alerting.DefaultConsecutiveCount != 3 {
		t.Errorf("DefaultConsecutiveCount = %d, want 3", cfg.Alerting.DefaultConsecutiveCount)
	}
	if cfg.Alerting.DefaultCooldownMinutes != 15 {
		t.Errorf("DefaultCooldownMinutes = %d, want 15", cfg.Alerting.DefaultCooldownMinutes)
	}
	if cfg.Alerting.EvaluationTimeoutSec != 5 {
		t.Errorf("EvaluationTimeoutSec = %d, want 5", cfg.Alerting.EvaluationTimeoutSec)
	}
	if cfg.Alerting.HistoryRetentionDays != 30 {
		t.Errorf("HistoryRetentionDays = %d, want 30", cfg.Alerting.HistoryRetentionDays)
	}
}

func TestLoad_AlertingEnabled_NoDSN(t *testing.T) {
	path := writeTempYAML(t, `
alerting:
  enabled: true
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error when alerting enabled without storage.dsn, got nil")
	}
}

func TestLoad_EnabledExplicitFalse(t *testing.T) {
	path := writeTempYAML(t, `
instances:
  - id: "disabled-db"
    dsn: "postgres://user:pass@localhost/postgres"
    enabled: false
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	inst := cfg.Instances[0]
	if inst.Enabled == nil {
		t.Fatal("Enabled is nil, want non-nil")
	}
	if *inst.Enabled {
		t.Error("Enabled = true, want false")
	}
}

func TestLoad_EmailChannel_MissingEmailConfig(t *testing.T) {
	path := writeTempYAML(t, `
storage:
  dsn: "postgres://user:pass@localhost/pgpulse"
alerting:
  enabled: true
  default_channels: ["email"]
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error when email channel configured without email section, got nil")
	}
}

func TestLoad_EmailChannel_MissingHost(t *testing.T) {
	path := writeTempYAML(t, `
storage:
  dsn: "postgres://user:pass@localhost/pgpulse"
alerting:
  enabled: true
  default_channels: ["email"]
  email:
    from: "alerts@pgpulse.local"
    recipients: ["admin@example.com"]
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error for missing email host, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "host") {
		t.Errorf("error = %q, want error containing 'host'", err)
	}
}

func TestLoad_EmailChannel_Valid(t *testing.T) {
	path := writeTempYAML(t, `
storage:
  dsn: "postgres://user:pass@localhost/pgpulse"
alerting:
  enabled: true
  default_channels: ["email"]
  email:
    host: "smtp.example.com"
    from: "alerts@pgpulse.local"
    recipients: ["admin@example.com"]
instances:
  - id: "db"
    dsn: "postgres://user:pass@localhost/postgres"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Alerting.Email == nil {
		t.Fatal("Alerting.Email is nil")
	}
	if cfg.Alerting.Email.Port != 587 {
		t.Errorf("Email.Port = %d, want 587 (default)", cfg.Alerting.Email.Port)
	}
	if cfg.Alerting.Email.SendTimeoutSeconds != 10 {
		t.Errorf("Email.SendTimeoutSeconds = %d, want 10 (default)", cfg.Alerting.Email.SendTimeoutSeconds)
	}
}
