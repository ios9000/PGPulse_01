package statements

import (
	"sort"
	"time"
)

// entryKey uniquely identifies a query across snapshots.
type entryKey struct {
	queryID int64
	dbID    uint32
	userID  uint32
}

// ComputeDiff computes the delta between two snapshots' entries.
// Entries present in "to" but not "from" are new queries.
// Entries present in "from" but not "to" are evicted queries.
func ComputeDiff(from, to Snapshot, fromEntries, toEntries []SnapshotEntry, opts DiffOptions) *DiffResult {
	result := &DiffResult{
		FromSnapshot: from,
		ToSnapshot:   to,
		Duration:     to.CapturedAt.Sub(from.CapturedAt),
	}

	// Detect stats reset.
	result.StatsResetWarning = statsResetDetected(from.StatsReset, to.StatsReset)

	// Index "from" entries by key.
	fromMap := make(map[entryKey]*SnapshotEntry, len(fromEntries))
	for i := range fromEntries {
		k := entryKey{queryID: fromEntries[i].QueryID, dbID: fromEntries[i].DbID, userID: fromEntries[i].UserID}
		fromMap[k] = &fromEntries[i]
	}

	// Iterate "to" entries: compute deltas for existing, mark new.
	for i := range toEntries {
		te := &toEntries[i]
		k := entryKey{queryID: te.QueryID, dbID: te.DbID, userID: te.UserID}

		if fe, ok := fromMap[k]; ok {
			de := computeEntryDelta(fe, te)
			result.Entries = append(result.Entries, de)
			delete(fromMap, k)
		} else {
			// New query — use absolute values as delta.
			de := entryAsDelta(te)
			result.NewQueries = append(result.NewQueries, de)
		}
	}

	// Remaining fromMap keys are evicted queries.
	for _, fe := range fromMap {
		de := entryAsDelta(fe)
		result.EvictedQueries = append(result.EvictedQueries, de)
	}

	// Compute totals from all continuing entries.
	for i := range result.Entries {
		result.TotalCallsDelta += result.Entries[i].CallsDelta
		result.TotalExecTimeDelta += result.Entries[i].ExecTimeDelta
	}
	for i := range result.NewQueries {
		result.TotalCallsDelta += result.NewQueries[i].CallsDelta
		result.TotalExecTimeDelta += result.NewQueries[i].ExecTimeDelta
	}

	// Sort entries.
	sortEntries(result.Entries, opts.SortBy)

	// Set total before pagination.
	result.TotalEntries = len(result.Entries)

	// Apply pagination.
	if opts.Offset > 0 && opts.Offset < len(result.Entries) {
		result.Entries = result.Entries[opts.Offset:]
	} else if opts.Offset >= len(result.Entries) {
		result.Entries = nil
	}
	if opts.Limit > 0 && opts.Limit < len(result.Entries) {
		result.Entries = result.Entries[:opts.Limit]
	}

	return result
}

func computeEntryDelta(from, to *SnapshotEntry) DiffEntry {
	callsDelta := to.Calls - from.Calls
	execDelta := to.TotalExecTime - from.TotalExecTime
	rowsDelta := to.Rows - from.Rows
	blkReadDelta := to.SharedBlksRead - from.SharedBlksRead
	blkHitDelta := to.SharedBlksHit - from.SharedBlksHit
	tempReadDelta := to.TempBlksRead - from.TempBlksRead
	tempWrittenDelta := to.TempBlksWritten - from.TempBlksWritten
	blkReadTimeDelta := to.BlkReadTime - from.BlkReadTime
	blkWriteTimeDelta := to.BlkWriteTime - from.BlkWriteTime

	de := DiffEntry{
		QueryID:              to.QueryID,
		UserID:               to.UserID,
		DbID:                 to.DbID,
		Query:                to.Query,
		DatabaseName:         to.DatabaseName,
		UserName:             to.UserName,
		CallsDelta:           callsDelta,
		ExecTimeDelta:        execDelta,
		RowsDelta:            rowsDelta,
		SharedBlksReadDelta:  blkReadDelta,
		SharedBlksHitDelta:   blkHitDelta,
		TempBlksReadDelta:    tempReadDelta,
		TempBlksWrittenDelta: tempWrittenDelta,
		BlkReadTimeDelta:     blkReadTimeDelta,
		BlkWriteTimeDelta:    blkWriteTimeDelta,
	}

	// Plan time delta (nullable).
	if from.TotalPlanTime != nil && to.TotalPlanTime != nil {
		v := *to.TotalPlanTime - *from.TotalPlanTime
		de.PlanTimeDelta = &v
	}

	// WAL bytes delta (nullable).
	if from.WALBytes != nil && to.WALBytes != nil {
		v := *to.WALBytes - *from.WALBytes
		de.WALBytesDelta = &v
	}

	// Derived fields.
	de.AvgExecTimePerCall = safeDivFloat(execDelta, float64(callsDelta))
	ioTime := blkReadTimeDelta + blkWriteTimeDelta
	de.IOTimePct = safeDivFloat(ioTime*100, execDelta)
	cpuDelta := execDelta - ioTime
	if cpuDelta < 0 {
		cpuDelta = 0
	}
	de.CPUTimeDelta = cpuDelta
	de.SharedHitRatio = safeDivFloat(float64(blkHitDelta)*100, float64(blkHitDelta+blkReadDelta))

	return de
}

func entryAsDelta(e *SnapshotEntry) DiffEntry {
	de := DiffEntry{
		QueryID:              e.QueryID,
		UserID:               e.UserID,
		DbID:                 e.DbID,
		Query:                e.Query,
		DatabaseName:         e.DatabaseName,
		UserName:             e.UserName,
		CallsDelta:           e.Calls,
		ExecTimeDelta:        e.TotalExecTime,
		RowsDelta:            e.Rows,
		SharedBlksReadDelta:  e.SharedBlksRead,
		SharedBlksHitDelta:   e.SharedBlksHit,
		TempBlksReadDelta:    e.TempBlksRead,
		TempBlksWrittenDelta: e.TempBlksWritten,
		BlkReadTimeDelta:     e.BlkReadTime,
		BlkWriteTimeDelta:    e.BlkWriteTime,
	}

	if e.TotalPlanTime != nil {
		v := *e.TotalPlanTime
		de.PlanTimeDelta = &v
	}
	if e.WALBytes != nil {
		v := *e.WALBytes
		de.WALBytesDelta = &v
	}

	de.AvgExecTimePerCall = safeDivFloat(e.TotalExecTime, float64(e.Calls))
	ioTime := e.BlkReadTime + e.BlkWriteTime
	de.IOTimePct = safeDivFloat(ioTime*100, e.TotalExecTime)
	cpuDelta := e.TotalExecTime - ioTime
	if cpuDelta < 0 {
		cpuDelta = 0
	}
	de.CPUTimeDelta = cpuDelta
	de.SharedHitRatio = safeDivFloat(float64(e.SharedBlksHit)*100, float64(e.SharedBlksHit+e.SharedBlksRead))

	return de
}

func statsResetDetected(from, to *time.Time) bool {
	if from == nil && to == nil {
		return false
	}
	if from == nil || to == nil {
		return true
	}
	return !from.Equal(*to)
}

func safeDivFloat(num, denom float64) float64 {
	if denom == 0 {
		return 0
	}
	return num / denom
}

func sortEntries(entries []DiffEntry, sortBy string) {
	if sortBy == "" {
		sortBy = "total_exec_time"
	}
	sort.Slice(entries, func(i, j int) bool {
		switch sortBy {
		case "calls":
			return entries[i].CallsDelta > entries[j].CallsDelta
		case "rows":
			return entries[i].RowsDelta > entries[j].RowsDelta
		case "shared_blks_read":
			return entries[i].SharedBlksReadDelta > entries[j].SharedBlksReadDelta
		case "avg_exec_time":
			return entries[i].AvgExecTimePerCall > entries[j].AvgExecTimePerCall
		default: // total_exec_time
			return entries[i].ExecTimeDelta > entries[j].ExecTimeDelta
		}
	})
}
