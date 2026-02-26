package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestReplicationStatus_NameAndInterval(t *testing.T) {
	c := NewReplicationStatusCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "replication_status" {
		t.Errorf("Name() = %q, want %q", c.Name(), "replication_status")
	}
	if c.Interval() != 60*time.Second {
		t.Errorf("Interval() = %v, want 60s", c.Interval())
	}
}

// TestReplicationStatus_Integration_Primary is a stub for future Docker-based testing.
// It verifies that active_replicas > 0 and replica.connected metrics are emitted
// when a standby is connected to the primary.
func TestReplicationStatus_Integration_Primary(t *testing.T) {
	t.Skip("integration test: requires Docker with primary+replica setup")
}

// TestReplicationStatus_Integration_Replica is a stub for future Docker-based testing.
// It verifies that wal_receiver.connected=1 and wal_receiver.lag_bytes are emitted
// when the standby is streaming from a primary.
func TestReplicationStatus_Integration_Replica(t *testing.T) {
	t.Skip("integration test: requires Docker with primary+replica setup")
}
