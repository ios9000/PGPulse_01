package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestClusterProgress_NameAndInterval(t *testing.T) {
	c := NewClusterProgressCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "progress_cluster" {
		t.Errorf("Name() = %q, want %q", c.Name(), "progress_cluster")
	}
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

func TestAnalyzeProgress_NameAndInterval(t *testing.T) {
	c := NewAnalyzeProgressCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "progress_analyze" {
		t.Errorf("Name() = %q, want %q", c.Name(), "progress_analyze")
	}
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

// TestClusterProgress_Integration is a stub for future Docker-based integration testing.
// It verifies that cluster progress metrics are emitted during a CLUSTER or VACUUM FULL.
func TestClusterProgress_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with an active CLUSTER operation")
}

// TestAnalyzeProgress_Integration is a stub for future Docker-based integration testing.
// It verifies that analyze progress metrics are emitted during an ANALYZE operation.
func TestAnalyzeProgress_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with an active ANALYZE operation")
}
