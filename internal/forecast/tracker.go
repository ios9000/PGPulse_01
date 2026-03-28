package forecast

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// operationMetrics maps Core 4 operation types to their progress metric prefix
// and the work-done field name (C8).
var operationMetrics = map[string]struct {
	completionMetric string
	workDoneField    string
}{
	"vacuum":      {completionMetric: "progress.vacuum.completion_pct", workDoneField: "progress.vacuum.heap_blks_vacuumed"},
	"analyze":     {completionMetric: "progress.analyze.completion_pct", workDoneField: "progress.analyze.sample_blks_scanned"},
	"create_index": {completionMetric: "progress.create_index.completion_pct", workDoneField: "progress.create_index.blocks_done"},
	"basebackup":  {completionMetric: "progress.basebackup.completion_pct", workDoneField: "progress.basebackup.backup_streamed"},
}

// OperationTracker is a background goroutine that monitors progress metrics
// to detect when maintenance operations start and complete.
type OperationTracker struct {
	metricStore   collector.MetricStore
	forecastStore ForecastStore
	connProv      InstanceConnProvider
	logger        *slog.Logger
	pollInterval  time.Duration
	windowSize    int

	mu        sync.RWMutex
	activeOps map[string]*TrackedOperation // key: "{instance}:{pid}:{operation}"
}

// NewOperationTracker creates a new tracker.
func NewOperationTracker(
	metricStore collector.MetricStore,
	forecastStore ForecastStore,
	connProv InstanceConnProvider,
	pollInterval time.Duration,
	windowSize int,
	logger *slog.Logger,
) *OperationTracker {
	if pollInterval == 0 {
		pollInterval = 15 * time.Second
	}
	if windowSize == 0 {
		windowSize = 10
	}
	return &OperationTracker{
		metricStore:   metricStore,
		forecastStore: forecastStore,
		connProv:      connProv,
		logger:        logger,
		pollInterval:  pollInterval,
		windowSize:    windowSize,
		activeOps:     make(map[string]*TrackedOperation),
	}
}

// SetConnProvider sets the connection provider after construction.
// Called from main.go after the orchestrator is created.
func (t *OperationTracker) SetConnProvider(cp InstanceConnProvider) {
	t.connProv = cp
}

// Run is the main loop. Polls MetricStore every pollInterval.
func (t *OperationTracker) Run(ctx context.Context) {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.flushAll(ctx, "unknown")
			return
		case <-ticker.C:
			t.tick(ctx)
		}
	}
}

// GetActiveOps returns a snapshot of active operations for a given instance.
// Called by ETACalculator (read-only, under RLock).
func (t *OperationTracker) GetActiveOps(instanceID string) []TrackedOperation {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var ops []TrackedOperation
	for _, op := range t.activeOps {
		if op.InstanceID == instanceID {
			// Deep copy the samples slice.
			cp := *op
			cp.Samples = make([]ProgressSample, len(op.Samples))
			copy(cp.Samples, op.Samples)
			ops = append(ops, cp)
		}
	}
	return ops
}

// tick executes one observation cycle.
func (t *OperationTracker) tick(ctx context.Context) {
	now := time.Now()

	// Query recent progress metrics for all operations.
	currentOps := make(map[string]*currentOp)

	for opType, meta := range operationMetrics {
		points, err := t.metricStore.Query(ctx, collector.MetricQuery{
			Metric: meta.completionMetric,
			Start:  now.Add(-2 * t.pollInterval),
			End:    now,
		})
		if err != nil {
			t.logger.Warn("forecast: tracker query failed", "metric", meta.completionMetric, "err", err)
			continue
		}

		// Also query work-done metric.
		workPoints, _ := t.metricStore.Query(ctx, collector.MetricQuery{
			Metric: meta.workDoneField,
			Start:  now.Add(-2 * t.pollInterval),
			End:    now,
		})
		workByKey := make(map[string]float64)
		for _, wp := range workPoints {
			wk := wp.InstanceID + ":" + wp.Labels["pid"]
			workByKey[wk] = wp.Value
		}

		for _, p := range points {
			pid := p.Labels["pid"]
			if pid == "" {
				continue
			}
			instanceID := p.InstanceID
			key := fmt.Sprintf("%s:%s:%s", instanceID, pid, opType)

			workKey := instanceID + ":" + pid
			workDone := workByKey[workKey]

			currentOps[key] = &currentOp{
				instanceID: instanceID,
				pid:        atoi(pid),
				opType:     opType,
				database:   p.Labels["datname"],
				table:      p.Labels["relname"],
				pctDone:    p.Value,
				workDone:   workDone,
				phase:      p.Labels["phase"],
				timestamp:  p.Timestamp,
			}
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Step 3: Update or start operations.
	for key, cop := range currentOps {
		if existing, ok := t.activeOps[key]; ok {
			// Update existing.
			existing.MissedPolls = 0
			existing.LastSeenAt = cop.timestamp
			appendSample(existing, ProgressSample{
				Timestamp: cop.timestamp,
				WorkDone:  cop.workDone,
				WorkTotal: 0, // not always available
				PctDone:   cop.pctDone,
			}, t.windowSize)
		} else {
			// New operation — check REINDEX gate for create_index.
			actualOp := cop.opType
			if cop.opType == "create_index" {
				isReindex := t.identifyReindex(ctx, cop.instanceID, cop.pid)
				if !isReindex {
					continue // Not in Core 4 scope — skip.
				}
				actualOp = "reindex_concurrent"
			}

			op := &TrackedOperation{
				InstanceID: cop.instanceID,
				PID:        cop.pid,
				Operation:  actualOp,
				Database:   cop.database,
				Table:      cop.table,
				StartedAt:  cop.timestamp,
				LastSeenAt: cop.timestamp,
			}
			appendSample(op, ProgressSample{
				Timestamp: cop.timestamp,
				WorkDone:  cop.workDone,
				PctDone:   cop.pctDone,
			}, t.windowSize)
			t.activeOps[key] = op
			t.logger.Info("forecast: operation started",
				"key", key, "operation", actualOp,
				"database", cop.database, "table", cop.table)
		}
	}

	// Step 4: Check for completed operations (debounce).
	for key, op := range t.activeOps {
		if _, stillActive := currentOps[key]; stillActive {
			continue
		}
		op.MissedPolls++
		if op.MissedPolls < 2 {
			continue // Debounce: tolerate single scrape gap.
		}
		// Finalize.
		outcome := t.classifyOutcome(op)
		t.finalizeOp(ctx, key, op, outcome)
		delete(t.activeOps, key)
	}
}

// identifyReindex queries pg_stat_activity for the PID to determine
// if a create_index operation is actually a REINDEX CONCURRENTLY.
func (t *OperationTracker) identifyReindex(ctx context.Context, instanceID string, pid int) bool {
	if t.connProv == nil {
		return false
	}
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	conn, err := t.connProv.ConnFor(queryCtx, instanceID)
	if err != nil {
		t.logger.Debug("forecast: reindex check connection failed", "instance", instanceID, "err", err)
		return false
	}
	defer func() { _ = conn.Close(queryCtx) }()

	var queryText string
	err = conn.QueryRow(queryCtx, `SELECT query FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&queryText)
	if err != nil {
		t.logger.Debug("forecast: reindex check query failed", "pid", pid, "err", err)
		return false
	}
	return strings.Contains(strings.ToUpper(queryText), "REINDEX")
}

// classifyOutcome determines the outcome of a completed operation.
func (t *OperationTracker) classifyOutcome(op *TrackedOperation) string {
	if len(op.Samples) == 0 {
		return "unknown"
	}
	finalPct := op.Samples[len(op.Samples)-1].PctDone
	elapsed := op.LastSeenAt.Sub(op.StartedAt).Seconds()

	if finalPct >= 99.0 {
		return "completed"
	}
	if elapsed > 2.0 {
		return "disappeared"
	}
	return "unknown"
}

// finalizeOp computes and persists a MaintenanceOperation record.
func (t *OperationTracker) finalizeOp(ctx context.Context, key string, op *TrackedOperation, outcome string) {
	now := time.Now()
	duration := op.LastSeenAt.Sub(op.StartedAt).Seconds()

	var finalPct, avgRate float64
	if len(op.Samples) > 0 {
		finalPct = op.Samples[len(op.Samples)-1].PctDone
		lastWork := op.Samples[len(op.Samples)-1].WorkDone
		firstWork := op.Samples[0].WorkDone
		if duration > 0 {
			avgRate = (lastWork - firstWork) / duration
		}
	}

	record := &MaintenanceOperation{
		InstanceID:     op.InstanceID,
		Operation:      op.Operation,
		Outcome:        outcome,
		Database:       op.Database,
		Table:          op.Table,
		TableSizeBytes: op.TableSizeBytes,
		StartedAt:      op.StartedAt,
		CompletedAt:    now,
		DurationSec:    duration,
		FinalPct:       finalPct,
		AvgRatePerSec:  avgRate,
		Metadata:       make(map[string]any),
	}

	if err := t.forecastStore.WriteOperation(ctx, record); err != nil {
		t.logger.Error("forecast: failed to persist operation", "key", key, "err", err)
	} else {
		t.logger.Info("forecast: operation finalized",
			"key", key, "outcome", outcome,
			"duration", fmt.Sprintf("%.0fs", duration),
			"final_pct", fmt.Sprintf("%.1f%%", finalPct))
	}
}

// flushAll writes all active operations to store on shutdown.
func (t *OperationTracker) flushAll(ctx context.Context, outcome string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for key, op := range t.activeOps {
		t.finalizeOp(ctx, key, op, outcome)
	}
	t.activeOps = make(map[string]*TrackedOperation)
}

// appendSample adds a sample to op.Samples, evicting the oldest if at capacity.
func appendSample(op *TrackedOperation, sample ProgressSample, maxSize int) {
	if len(op.Samples) >= maxSize {
		op.Samples = op.Samples[1:]
	}
	op.Samples = append(op.Samples, sample)
}

// currentOp is a transient struct for data extracted from progress metrics.
type currentOp struct {
	instanceID string
	pid        int
	opType     string
	database   string
	table      string
	pctDone    float64
	workDone   float64
	phase      string
	timestamp  time.Time
}

// atoi converts string to int, returning 0 on failure.
func atoi(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
