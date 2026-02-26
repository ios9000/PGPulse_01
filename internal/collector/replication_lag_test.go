package collector

import (
	"context"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestReplicationLag_NameAndInterval(t *testing.T) {
	c := NewReplicationLagCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "replication_lag" {
		t.Errorf("Name() = %q, want %q", c.Name(), "replication_lag")
	}
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

func TestReplicationLag_SkipsOnReplica(t *testing.T) {
	// When IsRecovery=true, Collect must return nil, nil immediately without
	// touching the connection — pg_stat_replication is empty on standbys.
	c := NewReplicationLagCollector("test", version.PGVersion{Major: 16})
	ctx := context.Background()

	points, err := c.Collect(ctx, nil, InstanceContext{IsRecovery: true})
	if err != nil {
		t.Fatalf("expected nil error on replica, got: %v", err)
	}
	if points != nil {
		t.Fatalf("expected nil points on replica, got %d points", len(points))
	}
}

// TestReplicationLag_Integration_PG16 is a stub for future Docker-based integration testing.
// It verifies that the collector returns 8 metrics per connected replica when run against
// a real primary with at least one standby.
func TestReplicationLag_Integration_PG16(t *testing.T) {
	t.Skip("integration test: requires Docker with primary+replica setup")
}
