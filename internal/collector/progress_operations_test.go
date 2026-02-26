package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestCreateIndexProgress_NameAndInterval(t *testing.T) {
	c := NewCreateIndexProgressCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "progress_create_index" {
		t.Errorf("Name() = %q, want %q", c.Name(), "progress_create_index")
	}
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

func TestBasebackupProgress_NameAndInterval(t *testing.T) {
	c := NewBasebackupProgressCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "progress_basebackup" {
		t.Errorf("Name() = %q, want %q", c.Name(), "progress_basebackup")
	}
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

func TestCopyProgress_NameAndInterval(t *testing.T) {
	c := NewCopyProgressCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "progress_copy" {
		t.Errorf("Name() = %q, want %q", c.Name(), "progress_copy")
	}
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

// TestCreateIndexProgress_Integration is a stub for future Docker-based integration testing.
// It verifies that create index progress metrics are emitted during CREATE INDEX.
func TestCreateIndexProgress_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with an active CREATE INDEX operation")
}

// TestBasebackupProgress_Integration is a stub for future Docker-based integration testing.
// It verifies that basebackup progress metrics are emitted during pg_basebackup.
func TestBasebackupProgress_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with an active pg_basebackup")
}

// TestCopyProgress_Integration is a stub for future Docker-based integration testing.
// It verifies that copy progress metrics are emitted during COPY operations.
func TestCopyProgress_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with an active COPY operation")
}
