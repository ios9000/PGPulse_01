package orchestrator

import (
	"testing"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// discardLogger, mockStore are defined in group_test.go and shared across this package.

func TestNew(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{Listen: ":8080", LogLevel: "info"},
		Instances: []config.InstanceConfig{
			{ID: "test", DSN: "postgres://localhost/test"},
		},
	}
	store := &mockStore{}
	logger := discardLogger()

	orch := New(cfg, store, logger, nil, nil)

	if orch == nil {
		t.Fatal("New() returned nil")
	}
	if orch.cfg.Server.Listen != ":8080" {
		t.Errorf("cfg.Server.Listen = %q, want \":8080\"", orch.cfg.Server.Listen)
	}
	if orch.store != collector.MetricStore(store) {
		t.Error("store not set correctly on orchestrator")
	}
	if orch.logger != logger {
		t.Error("logger not set correctly on orchestrator")
	}
	if len(orch.runners) != 0 {
		t.Errorf("runners len = %d before Start(), want 0", len(orch.runners))
	}
}
