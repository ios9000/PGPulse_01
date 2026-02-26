package collector

import (
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestVacuumProgress_NameAndInterval(t *testing.T) {
	c := NewVacuumProgressCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "progress_vacuum" {
		t.Errorf("Name() = %q, want %q", c.Name(), "progress_vacuum")
	}
	if c.Interval() != 10*time.Second {
		t.Errorf("Interval() = %v, want 10s", c.Interval())
	}
}

func TestCompletionPct(t *testing.T) {
	tests := []struct {
		name  string
		done  float64
		total float64
		want  float64
	}{
		{"zero/zero", 0, 0, 0},
		{"half", 50, 100, 50},
		{"full", 100, 100, 100},
		{"zero/nonzero", 0, 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := completionPct(tt.done, tt.total)
			if got != tt.want {
				t.Errorf("completionPct(%v, %v) = %v, want %v", tt.done, tt.total, got, tt.want)
			}
		})
	}
}

// TestVacuumProgress_Integration is a stub for future Docker-based integration testing.
// It verifies that vacuum progress metrics are emitted when a VACUUM is in progress.
func TestVacuumProgress_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with an active VACUUM operation")
}
