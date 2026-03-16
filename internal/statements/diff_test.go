package statements

import (
	"testing"
	"time"
)

func pf64(v float64) *float64 { return &v }

func makeEntry(queryID int64, calls int64, execTime float64, rows int64, blksHit, blksRead int64) SnapshotEntry {
	return SnapshotEntry{
		QueryID:       queryID,
		UserID:        10,
		DbID:          20,
		Query:         "SELECT 1",
		Calls:         calls,
		TotalExecTime: execTime,
		Rows:          rows,
		SharedBlksHit: blksHit,
		SharedBlksRead: blksRead,
	}
}

func TestComputeDiff(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-10 * time.Minute)

	tests := []struct {
		name               string
		from               Snapshot
		to                 Snapshot
		fromEntries        []SnapshotEntry
		toEntries          []SnapshotEntry
		opts               DiffOptions
		wantEntries        int
		wantNew            int
		wantEvicted        int
		wantResetWarning   bool
		wantTotalCalls     int64
		wantTotalExecTime  float64
		wantTotalEntries   int
	}{
		{
			name:        "normal diff with continuing queries",
			from:        Snapshot{ID: 1, CapturedAt: earlier},
			to:          Snapshot{ID: 2, CapturedAt: now},
			fromEntries: []SnapshotEntry{makeEntry(100, 10, 100.0, 50, 1000, 100)},
			toEntries:   []SnapshotEntry{makeEntry(100, 20, 250.0, 120, 2000, 150)},
			opts:        DiffOptions{},
			wantEntries: 1, wantNew: 0, wantEvicted: 0,
			wantTotalCalls: 10, wantTotalExecTime: 150.0, wantTotalEntries: 1,
		},
		{
			name: "stats reset detected",
			from: Snapshot{ID: 1, CapturedAt: earlier, StatsReset: timePtr(earlier.Add(-1 * time.Hour))},
			to:   Snapshot{ID: 2, CapturedAt: now, StatsReset: timePtr(now.Add(-5 * time.Minute))},
			fromEntries: []SnapshotEntry{makeEntry(100, 10, 100.0, 50, 1000, 100)},
			toEntries:   []SnapshotEntry{makeEntry(100, 5, 30.0, 20, 500, 50)},
			opts:        DiffOptions{},
			wantEntries: 1, wantResetWarning: true,
			wantTotalCalls: -5, wantTotalExecTime: -70.0, wantTotalEntries: 1,
		},
		{
			name:        "new queries only",
			from:        Snapshot{ID: 1, CapturedAt: earlier},
			to:          Snapshot{ID: 2, CapturedAt: now},
			fromEntries: nil,
			toEntries:   []SnapshotEntry{makeEntry(200, 5, 50.0, 10, 100, 10)},
			opts:        DiffOptions{},
			wantEntries: 0, wantNew: 1, wantEvicted: 0,
			wantTotalCalls: 5, wantTotalExecTime: 50.0, wantTotalEntries: 0,
		},
		{
			name:        "evicted queries only",
			from:        Snapshot{ID: 1, CapturedAt: earlier},
			to:          Snapshot{ID: 2, CapturedAt: now},
			fromEntries: []SnapshotEntry{makeEntry(300, 15, 200.0, 80, 500, 50)},
			toEntries:   nil,
			opts:        DiffOptions{},
			wantEntries: 0, wantNew: 0, wantEvicted: 1,
			wantTotalCalls: 0, wantTotalExecTime: 0, wantTotalEntries: 0,
		},
		{
			name:        "PG12 null pointer columns",
			from:        Snapshot{ID: 1, CapturedAt: earlier},
			to:          Snapshot{ID: 2, CapturedAt: now},
			fromEntries: []SnapshotEntry{{QueryID: 100, UserID: 10, DbID: 20, Query: "SELECT 1", Calls: 5, TotalExecTime: 50.0, Rows: 10}},
			toEntries:   []SnapshotEntry{{QueryID: 100, UserID: 10, DbID: 20, Query: "SELECT 1", Calls: 15, TotalExecTime: 150.0, Rows: 30}},
			opts:        DiffOptions{},
			wantEntries: 1, wantTotalCalls: 10, wantTotalExecTime: 100.0, wantTotalEntries: 1,
		},
		{
			name:        "div-by-zero guards — zero calls",
			from:        Snapshot{ID: 1, CapturedAt: earlier},
			to:          Snapshot{ID: 2, CapturedAt: now},
			fromEntries: []SnapshotEntry{makeEntry(100, 10, 100.0, 50, 0, 0)},
			toEntries:   []SnapshotEntry{makeEntry(100, 10, 100.0, 50, 0, 0)},
			opts:        DiffOptions{},
			wantEntries: 1, wantTotalCalls: 0, wantTotalExecTime: 0, wantTotalEntries: 1,
		},
		{
			name:        "empty snapshots",
			from:        Snapshot{ID: 1, CapturedAt: earlier},
			to:          Snapshot{ID: 2, CapturedAt: now},
			fromEntries: nil,
			toEntries:   nil,
			opts:        DiffOptions{},
			wantEntries: 0, wantNew: 0, wantEvicted: 0, wantTotalEntries: 0,
		},
		{
			name: "sort by calls",
			from: Snapshot{ID: 1, CapturedAt: earlier},
			to:   Snapshot{ID: 2, CapturedAt: now},
			fromEntries: []SnapshotEntry{
				makeEntry(100, 10, 500.0, 50, 100, 10),
				makeEntry(200, 5, 100.0, 20, 50, 5),
			},
			toEntries: []SnapshotEntry{
				makeEntry(100, 15, 600.0, 60, 150, 15),
				makeEntry(200, 50, 200.0, 40, 80, 8),
			},
			opts:        DiffOptions{SortBy: "calls"},
			wantEntries: 2, wantTotalEntries: 2,
			wantTotalCalls: 50, wantTotalExecTime: 200.0,
		},
		{
			name: "pagination limit and offset",
			from: Snapshot{ID: 1, CapturedAt: earlier},
			to:   Snapshot{ID: 2, CapturedAt: now},
			fromEntries: []SnapshotEntry{
				makeEntry(100, 10, 500.0, 50, 100, 10),
				makeEntry(200, 5, 100.0, 20, 50, 5),
				makeEntry(300, 3, 50.0, 10, 30, 3),
			},
			toEntries: []SnapshotEntry{
				makeEntry(100, 20, 1000.0, 100, 200, 20),
				makeEntry(200, 15, 300.0, 40, 100, 10),
				makeEntry(300, 8, 100.0, 25, 60, 6),
			},
			opts:           DiffOptions{Limit: 1, Offset: 1},
			wantEntries:    1,
			wantTotalEntries: 3,
			wantTotalCalls: 25, wantTotalExecTime: 750.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeDiff(tt.from, tt.to, tt.fromEntries, tt.toEntries, tt.opts)

			if len(result.Entries) != tt.wantEntries {
				t.Errorf("entries: got %d, want %d", len(result.Entries), tt.wantEntries)
			}
			if len(result.NewQueries) != tt.wantNew {
				t.Errorf("new queries: got %d, want %d", len(result.NewQueries), tt.wantNew)
			}
			if len(result.EvictedQueries) != tt.wantEvicted {
				t.Errorf("evicted queries: got %d, want %d", len(result.EvictedQueries), tt.wantEvicted)
			}
			if result.StatsResetWarning != tt.wantResetWarning {
				t.Errorf("stats_reset_warning: got %v, want %v", result.StatsResetWarning, tt.wantResetWarning)
			}
			if result.TotalCallsDelta != tt.wantTotalCalls {
				t.Errorf("total_calls_delta: got %d, want %d", result.TotalCallsDelta, tt.wantTotalCalls)
			}
			if result.TotalExecTimeDelta != tt.wantTotalExecTime {
				t.Errorf("total_exec_time_delta: got %f, want %f", result.TotalExecTimeDelta, tt.wantTotalExecTime)
			}
			if result.TotalEntries != tt.wantTotalEntries {
				t.Errorf("total_entries: got %d, want %d", result.TotalEntries, tt.wantTotalEntries)
			}
		})
	}
}

func TestComputeDiffDerivedFields(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-10 * time.Minute)

	from := Snapshot{ID: 1, CapturedAt: earlier}
	to := Snapshot{ID: 2, CapturedAt: now}

	fromEntries := []SnapshotEntry{{
		QueryID: 100, UserID: 10, DbID: 20, Query: "SELECT 1",
		Calls: 10, TotalExecTime: 100.0, Rows: 50,
		SharedBlksHit: 800, SharedBlksRead: 200,
		BlkReadTime: 20.0, BlkWriteTime: 5.0,
		TotalPlanTime: pf64(10.0), WALBytes: pf64(1000.0),
	}}
	toEntries := []SnapshotEntry{{
		QueryID: 100, UserID: 10, DbID: 20, Query: "SELECT 1",
		Calls: 20, TotalExecTime: 300.0, Rows: 120,
		SharedBlksHit: 1600, SharedBlksRead: 400,
		BlkReadTime: 40.0, BlkWriteTime: 10.0,
		TotalPlanTime: pf64(30.0), WALBytes: pf64(3000.0),
	}}

	result := ComputeDiff(from, to, fromEntries, toEntries, DiffOptions{})

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}

	e := result.Entries[0]

	// AvgExecTimePerCall = 200.0 / 10 = 20.0
	if e.AvgExecTimePerCall != 20.0 {
		t.Errorf("AvgExecTimePerCall: got %f, want 20.0", e.AvgExecTimePerCall)
	}

	// IOTimePct = (20+5) / 200 * 100 = 12.5
	if e.IOTimePct != 12.5 {
		t.Errorf("IOTimePct: got %f, want 12.5", e.IOTimePct)
	}

	// CPUTimeDelta = 200 - 20 - 5 = 175
	if e.CPUTimeDelta != 175.0 {
		t.Errorf("CPUTimeDelta: got %f, want 175.0", e.CPUTimeDelta)
	}

	// SharedHitRatio = 800 / (800 + 200) * 100 = 80.0
	if e.SharedHitRatio != 80.0 {
		t.Errorf("SharedHitRatio: got %f, want 80.0", e.SharedHitRatio)
	}

	// PlanTimeDelta = 30.0 - 10.0 = 20.0
	if e.PlanTimeDelta == nil || *e.PlanTimeDelta != 20.0 {
		t.Errorf("PlanTimeDelta: got %v, want 20.0", e.PlanTimeDelta)
	}

	// WALBytesDelta = 3000.0 - 1000.0 = 2000.0
	if e.WALBytesDelta == nil || *e.WALBytesDelta != 2000.0 {
		t.Errorf("WALBytesDelta: got %v, want 2000.0", e.WALBytesDelta)
	}
}

func TestComputeDiffSortByDifferentFields(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-10 * time.Minute)

	from := Snapshot{ID: 1, CapturedAt: earlier}
	to := Snapshot{ID: 2, CapturedAt: now}

	fromEntries := []SnapshotEntry{
		makeEntry(100, 10, 500.0, 50, 100, 10),
		makeEntry(200, 5, 100.0, 200, 50, 100),
		makeEntry(300, 3, 50.0, 10, 30, 3),
	}
	toEntries := []SnapshotEntry{
		makeEntry(100, 20, 600.0, 60, 150, 15),
		makeEntry(200, 50, 200.0, 250, 80, 200),
		makeEntry(300, 8, 800.0, 25, 60, 6),
	}

	sortFields := []string{"total_exec_time", "calls", "rows", "shared_blks_read", "avg_exec_time"}
	for _, field := range sortFields {
		t.Run(field, func(t *testing.T) {
			result := ComputeDiff(from, to, fromEntries, toEntries, DiffOptions{SortBy: field})
			if len(result.Entries) != 3 {
				t.Fatalf("expected 3 entries, got %d", len(result.Entries))
			}
			// Verify descending order.
			for i := 0; i < len(result.Entries)-1; i++ {
				a, b := result.Entries[i], result.Entries[i+1]
				switch field {
				case "calls":
					if a.CallsDelta < b.CallsDelta {
						t.Errorf("not sorted descending by %s at index %d", field, i)
					}
				case "rows":
					if a.RowsDelta < b.RowsDelta {
						t.Errorf("not sorted descending by %s at index %d", field, i)
					}
				case "shared_blks_read":
					if a.SharedBlksReadDelta < b.SharedBlksReadDelta {
						t.Errorf("not sorted descending by %s at index %d", field, i)
					}
				case "avg_exec_time":
					if a.AvgExecTimePerCall < b.AvgExecTimePerCall {
						t.Errorf("not sorted descending by %s at index %d", field, i)
					}
				default: // total_exec_time
					if a.ExecTimeDelta < b.ExecTimeDelta {
						t.Errorf("not sorted descending by %s at index %d", field, i)
					}
				}
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
