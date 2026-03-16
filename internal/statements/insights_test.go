package statements

import (
	"testing"
	"time"
)

func TestBuildQueryInsight(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		entries    []SnapshotEntry
		snapshots  []Snapshot
		wantNil    bool
		wantPoints int
	}{
		{
			name:    "empty entries",
			entries: nil, snapshots: nil,
			wantNil: true,
		},
		{
			name: "single snapshot — no points",
			entries: []SnapshotEntry{
				{QueryID: 100, Calls: 10, TotalExecTime: 100.0, Rows: 50, SharedBlksHit: 800, SharedBlksRead: 200},
			},
			snapshots: []Snapshot{
				{ID: 1, CapturedAt: now},
			},
			wantPoints: 0,
		},
		{
			name: "normal 3-snapshot insight",
			entries: []SnapshotEntry{
				{QueryID: 100, Query: "SELECT 1", DatabaseName: "mydb", UserName: "user1",
					Calls: 10, TotalExecTime: 100.0, Rows: 50, SharedBlksHit: 800, SharedBlksRead: 200},
				{QueryID: 100, Query: "SELECT 1", DatabaseName: "mydb", UserName: "user1",
					Calls: 20, TotalExecTime: 250.0, Rows: 120, SharedBlksHit: 1600, SharedBlksRead: 400},
				{QueryID: 100, Query: "SELECT 1", DatabaseName: "mydb", UserName: "user1",
					Calls: 35, TotalExecTime: 500.0, Rows: 200, SharedBlksHit: 2800, SharedBlksRead: 600},
			},
			snapshots: []Snapshot{
				{ID: 1, CapturedAt: now.Add(-20 * time.Minute)},
				{ID: 2, CapturedAt: now.Add(-10 * time.Minute)},
				{ID: 3, CapturedAt: now},
			},
			wantPoints: 2,
		},
		{
			name: "stats reset in middle — negative delta uses current values",
			entries: []SnapshotEntry{
				{QueryID: 100, Query: "SELECT 1",
					Calls: 100, TotalExecTime: 1000.0, Rows: 500, SharedBlksHit: 5000, SharedBlksRead: 1000},
				{QueryID: 100, Query: "SELECT 1",
					Calls: 5, TotalExecTime: 30.0, Rows: 20, SharedBlksHit: 100, SharedBlksRead: 10},
			},
			snapshots: []Snapshot{
				{ID: 1, CapturedAt: now.Add(-10 * time.Minute)},
				{ID: 2, CapturedAt: now},
			},
			wantPoints: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insight := BuildQueryInsight(tt.entries, tt.snapshots)

			if tt.wantNil {
				if insight != nil {
					t.Errorf("expected nil insight, got %+v", insight)
				}
				return
			}
			if insight == nil {
				t.Fatal("expected non-nil insight")
			}
			if len(insight.Points) != tt.wantPoints {
				t.Errorf("points: got %d, want %d", len(insight.Points), tt.wantPoints)
			}
		})
	}
}

func TestBuildQueryInsightNormalValues(t *testing.T) {
	now := time.Now()

	entries := []SnapshotEntry{
		{QueryID: 100, Query: "SELECT 1", DatabaseName: "mydb", UserName: "user1",
			Calls: 10, TotalExecTime: 100.0, Rows: 50, SharedBlksHit: 800, SharedBlksRead: 200},
		{QueryID: 100, Query: "SELECT 1", DatabaseName: "mydb", UserName: "user1",
			Calls: 20, TotalExecTime: 300.0, Rows: 120, SharedBlksHit: 1800, SharedBlksRead: 200},
	}
	snapshots := []Snapshot{
		{ID: 1, CapturedAt: now.Add(-10 * time.Minute)},
		{ID: 2, CapturedAt: now},
	}

	insight := BuildQueryInsight(entries, snapshots)
	if insight == nil {
		t.Fatal("expected non-nil insight")
	}
	if insight.QueryID != 100 {
		t.Errorf("QueryID: got %d, want 100", insight.QueryID)
	}
	if insight.DatabaseName != "mydb" {
		t.Errorf("DatabaseName: got %s, want mydb", insight.DatabaseName)
	}

	if len(insight.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(insight.Points))
	}

	p := insight.Points[0]
	if p.CallsDelta != 10 {
		t.Errorf("CallsDelta: got %d, want 10", p.CallsDelta)
	}
	if p.ExecTimeDelta != 200.0 {
		t.Errorf("ExecTimeDelta: got %f, want 200.0", p.ExecTimeDelta)
	}
	if p.RowsDelta != 70 {
		t.Errorf("RowsDelta: got %d, want 70", p.RowsDelta)
	}
	// AvgExecTime = 200.0 / 10 = 20.0
	if p.AvgExecTime != 20.0 {
		t.Errorf("AvgExecTime: got %f, want 20.0", p.AvgExecTime)
	}
	// SharedHitRatio = 1000 / (1000 + 0) * 100 = 100.0
	if p.SharedHitRatio != 100.0 {
		t.Errorf("SharedHitRatio: got %f, want 100.0", p.SharedHitRatio)
	}
}

func TestBuildQueryInsightStatsReset(t *testing.T) {
	now := time.Now()

	entries := []SnapshotEntry{
		{QueryID: 100, Query: "SELECT 1",
			Calls: 100, TotalExecTime: 1000.0, Rows: 500, SharedBlksHit: 5000, SharedBlksRead: 1000},
		{QueryID: 100, Query: "SELECT 1",
			Calls: 5, TotalExecTime: 30.0, Rows: 20, SharedBlksHit: 100, SharedBlksRead: 10},
	}
	snapshots := []Snapshot{
		{ID: 1, CapturedAt: now.Add(-10 * time.Minute)},
		{ID: 2, CapturedAt: now},
	}

	insight := BuildQueryInsight(entries, snapshots)
	if insight == nil {
		t.Fatal("expected non-nil insight")
	}
	if len(insight.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(insight.Points))
	}

	p := insight.Points[0]
	// After stats reset, delta uses current values directly.
	if p.CallsDelta != 5 {
		t.Errorf("CallsDelta after reset: got %d, want 5", p.CallsDelta)
	}
	if p.ExecTimeDelta != 30.0 {
		t.Errorf("ExecTimeDelta after reset: got %f, want 30.0", p.ExecTimeDelta)
	}
}
