package statements

import "sort"

// GenerateReport creates a WorkloadReport from a DiffResult.
// Each section contains up to topN entries sorted by the primary metric.
func GenerateReport(diff *DiffResult, topN int) *WorkloadReport {
	if diff == nil {
		return &WorkloadReport{}
	}
	if topN <= 0 {
		topN = 20
	}

	report := &WorkloadReport{
		FromTime:          diff.FromSnapshot.CapturedAt,
		ToTime:            diff.ToSnapshot.CapturedAt,
		Duration:          diff.Duration.String(),
		StatsResetWarning: diff.StatsResetWarning,
	}

	// Compute summary from all entries (continuing + new).
	allEntries := make([]DiffEntry, 0, len(diff.Entries)+len(diff.NewQueries))
	allEntries = append(allEntries, diff.Entries...)
	allEntries = append(allEntries, diff.NewQueries...)

	var totalRows int64
	for i := range allEntries {
		totalRows += allEntries[i].RowsDelta
	}

	report.Summary = ReportSummary{
		TotalQueries:       diff.TotalEntries + len(diff.NewQueries),
		TotalCallsDelta:    diff.TotalCallsDelta,
		TotalExecTimeDelta: diff.TotalExecTimeDelta,
		TotalRowsDelta:     totalRows,
		UniqueQueries:      diff.TotalEntries,
		NewQueries:         len(diff.NewQueries),
		EvictedQueries:     len(diff.EvictedQueries),
	}

	// Top by exec time.
	report.TopByExecTime = topNBySorter(allEntries, topN, func(i, j int) bool {
		return allEntries[i].ExecTimeDelta > allEntries[j].ExecTimeDelta
	})

	// Top by calls.
	report.TopByCalls = topNBySorter(allEntries, topN, func(i, j int) bool {
		return allEntries[i].CallsDelta > allEntries[j].CallsDelta
	})

	// Top by rows.
	report.TopByRows = topNBySorter(allEntries, topN, func(i, j int) bool {
		return allEntries[i].RowsDelta > allEntries[j].RowsDelta
	})

	// Top by IO reads.
	report.TopByIOReads = topNBySorter(allEntries, topN, func(i, j int) bool {
		return allEntries[i].SharedBlksReadDelta > allEntries[j].SharedBlksReadDelta
	})

	// Top by avg time.
	report.TopByAvgTime = topNBySorter(allEntries, topN, func(i, j int) bool {
		return allEntries[i].AvgExecTimePerCall > allEntries[j].AvgExecTimePerCall
	})

	// Copy new and evicted queries.
	report.NewQueries = copyEntries(diff.NewQueries)
	report.EvictedQueries = copyEntries(diff.EvictedQueries)

	return report
}

// topNBySorter copies entries, sorts by the given comparator, and returns up to topN.
func topNBySorter(entries []DiffEntry, topN int, less func(i, j int) bool) []DiffEntry {
	sorted := make([]DiffEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, less)
	if len(sorted) > topN {
		sorted = sorted[:topN]
	}
	return sorted
}

func copyEntries(src []DiffEntry) []DiffEntry {
	if src == nil {
		return nil
	}
	dst := make([]DiffEntry, len(src))
	copy(dst, src)
	return dst
}
