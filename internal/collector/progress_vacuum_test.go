package collector

import (
	"fmt"
	"strings"
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

// TestVacuumProgress_GateSelect verifies the pg_stat_progress_vacuum columns
// chosen for PG 16 (tuple counts) and PG 17/18 (num_dead_item_ids + byte sizes).
func TestVacuumProgress_GateSelect(t *testing.T) {
	tests := []struct {
		major   int
		want    []string
		notWant []string
	}{
		{
			major:   16,
			want:    []string{"p.max_dead_tuples", "p.num_dead_tuples", "-1 AS dead_tuple_bytes", "-1 AS max_dead_tuple_bytes"},
			notWant: []string{"num_dead_item_ids", "p.dead_tuple_bytes", "p.max_dead_tuple_bytes"},
		},
		{
			major:   17,
			want:    []string{"-1 AS max_dead_tuples", "p.num_dead_item_ids AS num_dead_tuples", "p.dead_tuple_bytes", "p.max_dead_tuple_bytes"},
			notWant: []string{"p.max_dead_tuples", "p.num_dead_tuples"},
		},
		{
			major:   18,
			want:    []string{"-1 AS max_dead_tuples", "p.num_dead_item_ids AS num_dead_tuples", "p.dead_tuple_bytes", "p.max_dead_tuple_bytes"},
			notWant: []string{"p.max_dead_tuples", "p.num_dead_tuples"},
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("PG%d", tt.major), func(t *testing.T) {
			v := version.PGVersion{Major: tt.major, Num: tt.major * 10000}
			sql, ok := progressVacuumGate.Select(v)
			if !ok {
				t.Fatalf("expected gate to select a variant for PG %d", tt.major)
			}
			for _, s := range tt.want {
				if !strings.Contains(sql, s) {
					t.Errorf("PG %d variant missing %q", tt.major, s)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(sql, s) {
					t.Errorf("PG %d variant must not contain %q", tt.major, s)
				}
			}
		})
	}
}

func metricKeys(points []MetricPoint) map[string]float64 {
	m := make(map[string]float64, len(points))
	for _, p := range points {
		m[strings.TrimPrefix(p.Metric, metricPrefix+".")] = p.Value
	}
	return m
}

func TestVacuumProgress_RowPointsPG16(t *testing.T) {
	c := NewVacuumProgressCollector("test", version.PGVersion{Major: 16})
	got := metricKeys(c.rowPoints(vacuumProgressRow{
		pid: "1", phase: "scanning heap",
		blksTotal: 100, blksScanned: 50,
		maxDeadTuples: 1000, numDeadTuples: 42,
		deadTupleBytes: -1, maxDeadTupleBytes: -1,
	}))
	if got["progress.vacuum.max_dead_tuples"] != 1000 {
		t.Errorf("max_dead_tuples = %v, want 1000", got["progress.vacuum.max_dead_tuples"])
	}
	if got["progress.vacuum.num_dead_tuples"] != 42 {
		t.Errorf("num_dead_tuples = %v, want 42", got["progress.vacuum.num_dead_tuples"])
	}
	if got["progress.vacuum.completion_pct"] != 50 {
		t.Errorf("completion_pct = %v, want 50", got["progress.vacuum.completion_pct"])
	}
	for _, k := range []string{"progress.vacuum.dead_tuple_bytes", "progress.vacuum.max_dead_tuple_bytes"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s must not be emitted on PG 16", k)
		}
	}
}

func TestVacuumProgress_RowPointsPG17(t *testing.T) {
	c := NewVacuumProgressCollector("test", version.PGVersion{Major: 17})
	got := metricKeys(c.rowPoints(vacuumProgressRow{
		pid: "1", phase: "scanning heap",
		maxDeadTuples: -1, numDeadTuples: 7,
		deadTupleBytes: 8192, maxDeadTupleBytes: 67108864,
	}))
	if _, ok := got["progress.vacuum.max_dead_tuples"]; ok {
		t.Error("max_dead_tuples must not be emitted on PG 17+")
	}
	want := map[string]float64{
		"progress.vacuum.num_dead_tuples":      7,
		"progress.vacuum.dead_tuple_bytes":     8192,
		"progress.vacuum.max_dead_tuple_bytes": 67108864,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}
