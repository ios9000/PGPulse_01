package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

func TestReplicationSlots_NameAndInterval(t *testing.T) {
	c := NewReplicationSlotsCollector("test", version.PGVersion{Major: 16})
	if c.Name() != "replication_slots" {
		t.Errorf("Name() = %q, want %q", c.Name(), "replication_slots")
	}
	if c.Interval() != 60*time.Second {
		t.Errorf("Interval() = %v, want 60s", c.Interval())
	}
}

func TestReplicationSlots_GateSelectPG14(t *testing.T) {
	v := version.PGVersion{Major: 14, Minor: 0, Num: 140000}
	sql, ok := replicationSlotsGate.Select(v)
	if !ok {
		t.Fatal("expected gate to select a variant for PG 14")
	}
	// PG 14 variant uses empty string literals — no actual two_phase or conflicting columns.
	if strings.Contains(sql, "two_phase::text") {
		t.Error("PG 14 variant must not reference two_phase::text")
	}
	if strings.Contains(sql, "conflicting::text") {
		t.Error("PG 14 variant must not reference conflicting::text")
	}
	if !strings.Contains(sql, "'' AS two_phase") {
		t.Error("PG 14 variant must have empty string placeholder for two_phase")
	}
	if !strings.Contains(sql, "'' AS conflicting") {
		t.Error("PG 14 variant must have empty string placeholder for conflicting")
	}
}

func TestReplicationSlots_GateSelectPG15(t *testing.T) {
	v := version.PGVersion{Major: 15, Minor: 0, Num: 150000}
	sql, ok := replicationSlotsGate.Select(v)
	if !ok {
		t.Fatal("expected gate to select a variant for PG 15")
	}
	// PG 15 variant has two_phase column but no conflicting column.
	if !strings.Contains(sql, "two_phase::text") {
		t.Error("PG 15 variant must include two_phase::text")
	}
	if strings.Contains(sql, "conflicting::text") {
		t.Error("PG 15 variant must not include conflicting::text")
	}
	if !strings.Contains(sql, "'' AS conflicting") {
		t.Error("PG 15 variant must have empty string placeholder for conflicting")
	}
}

func TestReplicationSlots_GateSelectPG16(t *testing.T) {
	v := version.PGVersion{Major: 16, Minor: 0, Num: 160000}
	sql, ok := replicationSlotsGate.Select(v)
	if !ok {
		t.Fatal("expected gate to select a variant for PG 16")
	}
	// PG 16+ variant has both two_phase and conflicting columns.
	if !strings.Contains(sql, "two_phase::text") {
		t.Error("PG 16 variant must include two_phase::text")
	}
	if !strings.Contains(sql, "conflicting::text") {
		t.Error("PG 16 variant must include conflicting::text")
	}
}

func TestReplicationSlots_GateSelectPG17(t *testing.T) {
	// PG 17 must use the same PG 16+ variant (MaxMajor=99 catches all future versions).
	v := version.PGVersion{Major: 17, Minor: 0, Num: 170000}
	sql, ok := replicationSlotsGate.Select(v)
	if !ok {
		t.Fatal("expected gate to select a variant for PG 17")
	}
	if !strings.Contains(sql, "two_phase::text") {
		t.Error("PG 17 variant must include two_phase::text")
	}
	if !strings.Contains(sql, "conflicting::text") {
		t.Error("PG 17 variant must include conflicting::text")
	}
}

// TestReplicationSlots_Integration is a stub for future Docker-based integration testing.
// It verifies that retained_bytes and active metrics are emitted for each slot.
func TestReplicationSlots_Integration(t *testing.T) {
	t.Skip("integration test: requires Docker with at least one replication slot")
}
