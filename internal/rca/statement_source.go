package rca

import (
	"context"
	"time"

	"github.com/ios9000/PGPulse_01/internal/statements"
)

// StatementDiffSource produces synthetic anomaly events from pg_stat_statements
// snapshot diffs. It detects query regressions and new queries as anomalies.
type StatementDiffSource struct {
	store statements.SnapshotStore
	// RegressionThreshold is the minimum ratio of new avg_exec_time to old avg_exec_time
	// for a query to be considered a regression. Default: 2.0 (2x slower).
	RegressionThreshold float64
	// NewQueryMinCalls is the minimum number of calls for a new query to be considered significant.
	NewQueryMinCalls int64
}

// NewStatementDiffSource creates a new StatementDiffSource.
func NewStatementDiffSource(store statements.SnapshotStore) *StatementDiffSource {
	return &StatementDiffSource{
		store:               store,
		RegressionThreshold: 2.0,
		NewQueryMinCalls:    10,
	}
}

// GetAnomalies produces synthetic anomaly events for query regressions and
// new queries by comparing the two most recent snapshots within the window.
func (s *StatementDiffSource) GetAnomalies(
	ctx context.Context,
	instanceID string,
	from, to time.Time,
) (map[string][]AnomalyEvent, error) {
	result := make(map[string][]AnomalyEvent)

	// Get the latest 2 snapshots for this instance.
	snaps, err := s.store.GetLatestSnapshots(ctx, instanceID, 2)
	if err != nil || len(snaps) < 2 {
		return result, nil // not enough data
	}

	// snaps[0] is newest, snaps[1] is older.
	newer := snaps[0]
	older := snaps[1]

	// Check if both snapshots are within a reasonable time frame.
	if newer.CapturedAt.Before(from.Add(-1*time.Hour)) || older.CapturedAt.After(to.Add(1*time.Hour)) {
		return result, nil
	}

	// Get entries for both snapshots.
	olderEntries, _, err := s.store.GetSnapshotEntries(ctx, older.ID, 1000, 0)
	if err != nil {
		return result, nil
	}
	newerEntries, _, err := s.store.GetSnapshotEntries(ctx, newer.ID, 1000, 0)
	if err != nil {
		return result, nil
	}

	// Compute diff.
	diff := statements.ComputeDiff(older, newer, olderEntries, newerEntries, statements.DiffOptions{
		SortBy: "total_exec_time",
		Limit:  100,
	})
	if diff == nil {
		return result, nil
	}

	// Build old entry map for regression detection.
	oldAvgMap := make(map[int64]float64)
	for _, e := range olderEntries {
		if e.Calls > 0 {
			oldAvgMap[e.QueryID] = e.TotalExecTime / float64(e.Calls)
		}
	}

	// Detect regressions: entries where avg exec time increased significantly.
	regressionKey := "pg.statements.regression"
	for _, entry := range diff.Entries {
		if entry.CallsDelta <= 0 {
			continue
		}
		oldAvg, ok := oldAvgMap[entry.QueryID]
		if !ok || oldAvg <= 0 {
			continue
		}
		ratio := entry.AvgExecTimePerCall / oldAvg
		if ratio >= s.RegressionThreshold {
			strength := 0.5
			if ratio >= 5.0 {
				strength = 0.9
			} else if ratio >= 3.0 {
				strength = 0.7
			}
			result[regressionKey] = append(result[regressionKey], AnomalyEvent{
				InstanceID:  instanceID,
				MetricKey:   regressionKey,
				Timestamp:   newer.CapturedAt,
				Value:       entry.AvgExecTimePerCall,
				BaselineVal: oldAvg,
				ZScore:      ratio,
				Strength:    strength,
				Source:      "statement_diff",
			})
		}
	}

	// Detect significant new queries.
	newQueryKey := "pg.statements.new_query"
	for _, entry := range diff.NewQueries {
		if entry.CallsDelta < s.NewQueryMinCalls {
			continue
		}
		strength := 0.4
		if entry.ExecTimeDelta > 10000 { // > 10 seconds total exec time
			strength = 0.8
		} else if entry.ExecTimeDelta > 1000 { // > 1 second total exec time
			strength = 0.6
		}
		result[newQueryKey] = append(result[newQueryKey], AnomalyEvent{
			InstanceID:  instanceID,
			MetricKey:   newQueryKey,
			Timestamp:   newer.CapturedAt,
			Value:       entry.AvgExecTimePerCall,
			BaselineVal: 0,
			Strength:    strength,
			Source:      "statement_diff",
		})
	}

	return result, nil
}
