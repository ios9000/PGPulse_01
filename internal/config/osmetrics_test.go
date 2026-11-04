package config

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func TestEffectiveOSMetricsMethod(t *testing.T) {
	tests := []struct {
		name     string
		global   string
		instance string
		want     string
	}{
		{"default is sql", "", "", "sql"},
		{"global agent", "agent", "", "agent"},
		{"global disabled", "disabled", "", "disabled"},
		{"instance overrides global", "agent", "sql", "sql"},
		{"instance disabled overrides default", "", "disabled", "disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{OSMetrics: OSMetricsConfig{Method: tt.global}}
			got := cfg.EffectiveOSMetricsMethod(InstanceConfig{ID: "a", OSMetricsMethod: tt.instance})
			if got != tt.want {
				t.Errorf("EffectiveOSMetricsMethod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSQLOSMetricsInstances(t *testing.T) {
	off := false
	on := true
	cfg := Config{
		OSMetrics: OSMetricsConfig{Method: "agent"},
		Instances: []InstanceConfig{
			{ID: "sql-override", OSMetricsMethod: "sql"},                     // enabled (nil), sql
			{ID: "global-agent"},                                             // agent via global
			{ID: "disabled-instance", OSMetricsMethod: "sql", Enabled: &off}, // skipped
			{ID: "explicit-on", OSMetricsMethod: "sql", Enabled: &on},
		},
	}
	got := cfg.SQLOSMetricsInstances()
	want := []string{"sql-override", "explicit-on"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SQLOSMetricsInstances() = %v, want %v", got, want)
	}
}

func TestWarnOSMetricsSQL(t *testing.T) {
	newLogger := func(buf *bytes.Buffer) *slog.Logger {
		return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	t.Run("default method warns once", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := Config{Instances: []InstanceConfig{{ID: "a"}, {ID: "b"}}}
		if !WarnOSMetricsSQL(newLogger(&buf), cfg) {
			t.Fatal("expected warning to be logged")
		}
		out := buf.String()
		if n := strings.Count(out, "\n"); n != 1 {
			t.Errorf("expected exactly one log line, got %d: %q", n, out)
		}
		for _, s := range []string{"level=WARN", "pg_read_file", "docs/security/VETTING.md", "instances=\"[a b]\""} {
			if !strings.Contains(out, s) {
				t.Errorf("log line missing %q: %q", s, out)
			}
		}
	})

	t.Run("agent or disabled does not warn", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := Config{
			OSMetrics: OSMetricsConfig{Method: "agent"},
			Instances: []InstanceConfig{{ID: "a"}, {ID: "b", OSMetricsMethod: "disabled"}},
		}
		if WarnOSMetricsSQL(newLogger(&buf), cfg) {
			t.Fatal("expected no warning")
		}
		if buf.Len() != 0 {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})
}
