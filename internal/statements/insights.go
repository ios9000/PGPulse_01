package statements

import "time"

// BuildQueryInsight builds a time-series insight for a single queryid.
// Entries must be for a single queryid, sorted by snapshot captured_at ASC.
// Snapshots array provides timestamps and stats_reset detection.
// The first entry is used as the baseline; subsequent entries compute deltas.
func BuildQueryInsight(entries []SnapshotEntry, snapshots []Snapshot) *QueryInsight {
	if len(entries) == 0 || len(snapshots) == 0 {
		return nil
	}

	insight := &QueryInsight{
		QueryID:      entries[0].QueryID,
		Query:        entries[0].Query,
		DatabaseName: entries[0].DatabaseName,
		UserName:     entries[0].UserName,
		FirstSeen:    snapshots[0].CapturedAt,
	}

	// Need at least 2 entries to compute deltas.
	if len(entries) < 2 {
		return insight
	}

	for i := 1; i < len(entries); i++ {
		prev := &entries[i-1]
		curr := &entries[i]

		var capturedAt time.Time
		if i < len(snapshots) {
			capturedAt = snapshots[i].CapturedAt
		}

		callsDelta := curr.Calls - prev.Calls
		execDelta := curr.TotalExecTime - prev.TotalExecTime
		rowsDelta := curr.Rows - prev.Rows
		hitDelta := curr.SharedBlksHit - prev.SharedBlksHit
		readDelta := curr.SharedBlksRead - prev.SharedBlksRead

		// If delta is negative (stats_reset), use current entry values as delta.
		if callsDelta < 0 || execDelta < 0 {
			callsDelta = curr.Calls
			execDelta = curr.TotalExecTime
			rowsDelta = curr.Rows
			hitDelta = curr.SharedBlksHit
			readDelta = curr.SharedBlksRead
		}

		avgExec := safeDivFloat(execDelta, float64(callsDelta))
		hitRatio := safeDivFloat(float64(hitDelta)*100, float64(hitDelta+readDelta))

		insight.Points = append(insight.Points, QueryInsightPoint{
			CapturedAt:     capturedAt,
			CallsDelta:     callsDelta,
			ExecTimeDelta:  execDelta,
			RowsDelta:      rowsDelta,
			AvgExecTime:    avgExec,
			SharedHitRatio: hitRatio,
		})
	}

	return insight
}
