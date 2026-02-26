package config

import (
	"os"
	"path/filepath"
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
