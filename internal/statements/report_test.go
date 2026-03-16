package statements

import (
	"testing"
	"time"
)

func TestGenerateReport(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-10 * time.Minute)

	tests := []struct {
		name           string
		diff           *DiffResult
		topN           int
		wantSummary    ReportSummary
		wantTopExec    int
		wantTopCalls   int
		wantNewQueries int
	}{
		{
			name: "normal report generation",
			diff: &DiffResult{
				FromSnapshot:       Snapshot{ID: 1, CapturedAt: earlier},
				ToSnapshot:         Snapshot{ID: 2, CapturedAt: now},
				Duration:           10 * time.Minute,
				TotalCallsDelta:    100,
				TotalExecTimeDelta: 5000.0,
				TotalEntries:       3,
				Entries: []DiffEntry{
					{QueryID: 100, CallsDelta: 50, ExecTimeDelta: 3000.0, RowsDelta: 200, SharedBlksReadDelta: 500},
					{QueryID: 200, CallsDelta: 30, ExecTimeDelta: 1500.0, RowsDelta: 100, SharedBlksReadDelta: 200},
					{QueryID: 300, CallsDelta: 10, ExecTimeDelta: 300.0, RowsDelta: 50, SharedBlksReadDelta: 50},
				},
				NewQueries: []DiffEntry{
					{QueryID: 400, CallsDelta: 10, ExecTimeDelta: 200.0, RowsDelta: 30},
				},
				EvictedQueries: []DiffEntry{
					{QueryID: 500, CallsDelta: 5, ExecTimeDelta: 100.0, RowsDelta: 10},
				},
			},
			topN: 2,
			wantSummary: ReportSummary{
				TotalQueries:       4,
				TotalCallsDelta:    100,
				TotalExecTimeDelta: 5000.0,
				TotalRowsDelta:     380,
				UniqueQueries:      3,
				NewQueries:         1,
				EvictedQueries:     1,
			},
			wantTopExec:    2,
			wantTopCalls:   2,
			wantNewQueries: 1,
		},
		{
			name: "topN greater than total entries",
			diff: &DiffResult{
				FromSnapshot: Snapshot{ID: 1, CapturedAt: earlier},
				ToSnapshot:   Snapshot{ID: 2, CapturedAt: now},
				Duration:     10 * time.Minute,
				TotalEntries: 1,
				Entries: []DiffEntry{
					{QueryID: 100, CallsDelta: 10, ExecTimeDelta: 100.0, RowsDelta: 50},
				},
			},
			topN:       20,
			wantTopExec: 1,
			wantTopCalls: 1,
		},
		{
			name:         "nil diff",
			diff:         nil,
			topN:         10,
			wantTopExec:  0,
			wantTopCalls: 0,
		},
		{
			name: "empty diff",
			diff: &DiffResult{
				FromSnapshot: Snapshot{ID: 1, CapturedAt: earlier},
				ToSnapshot:   Snapshot{ID: 2, CapturedAt: now},
				Duration:     10 * time.Minute,
			},
			topN:         10,
			wantTopExec:  0,
			wantTopCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := GenerateReport(tt.diff, tt.topN)
			if report == nil {
				t.Fatal("expected non-nil report")
			}

			if len(report.TopByExecTime) != tt.wantTopExec {
				t.Errorf("TopByExecTime: got %d, want %d", len(report.TopByExecTime), tt.wantTopExec)
			}
			if len(report.TopByCalls) != tt.wantTopCalls {
				t.Errorf("TopByCalls: got %d, want %d", len(report.TopByCalls), tt.wantTopCalls)
			}
			if len(report.NewQueries) != tt.wantNewQueries {
				t.Errorf("NewQueries: got %d, want %d", len(report.NewQueries), tt.wantNewQueries)
			}

			if tt.name == "normal report generation" {
				if report.Summary != tt.wantSummary {
					t.Errorf("summary: got %+v, want %+v", report.Summary, tt.wantSummary)
				}
			}
		})
	}
}

func TestGenerateReportSorting(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-10 * time.Minute)

	diff := &DiffResult{
		FromSnapshot: Snapshot{ID: 1, CapturedAt: earlier},
		ToSnapshot:   Snapshot{ID: 2, CapturedAt: now},
		Duration:     10 * time.Minute,
		TotalEntries: 3,
		Entries: []DiffEntry{
			{QueryID: 100, CallsDelta: 50, ExecTimeDelta: 100.0, RowsDelta: 10, SharedBlksReadDelta: 500, AvgExecTimePerCall: 2.0},
			{QueryID: 200, CallsDelta: 10, ExecTimeDelta: 3000.0, RowsDelta: 200, SharedBlksReadDelta: 50, AvgExecTimePerCall: 300.0},
			{QueryID: 300, CallsDelta: 30, ExecTimeDelta: 500.0, RowsDelta: 500, SharedBlksReadDelta: 200, AvgExecTimePerCall: 16.67},
		},
	}

	report := GenerateReport(diff, 10)

	// TopByExecTime: 200 (3000) > 300 (500) > 100 (100)
	if report.TopByExecTime[0].QueryID != 200 {
		t.Errorf("TopByExecTime[0]: got queryid %d, want 200", report.TopByExecTime[0].QueryID)
	}

	// TopByCalls: 100 (50) > 300 (30) > 200 (10)
	if report.TopByCalls[0].QueryID != 100 {
		t.Errorf("TopByCalls[0]: got queryid %d, want 100", report.TopByCalls[0].QueryID)
	}

	// TopByRows: 300 (500) > 200 (200) > 100 (10)
	if report.TopByRows[0].QueryID != 300 {
		t.Errorf("TopByRows[0]: got queryid %d, want 300", report.TopByRows[0].QueryID)
	}

	// TopByIOReads: 100 (500) > 300 (200) > 200 (50)
	if report.TopByIOReads[0].QueryID != 100 {
		t.Errorf("TopByIOReads[0]: got queryid %d, want 100", report.TopByIOReads[0].QueryID)
	}

	// TopByAvgTime: 200 (300) > 300 (16.67) > 100 (2.0)
	if report.TopByAvgTime[0].QueryID != 200 {
		t.Errorf("TopByAvgTime[0]: got queryid %d, want 200", report.TopByAvgTime[0].QueryID)
	}
}
