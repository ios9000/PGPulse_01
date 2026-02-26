package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// checkpointGate selects the appropriate SQL for checkpoint/bgwriter stats based on PG version.
//
// PG 14–16: all stats are in pg_stat_bgwriter (single view).
// PG 17+: checkpoint stats moved to pg_stat_checkpointer; bgwriter stats remain in pg_stat_bgwriter.
//
// Both variants return exactly 13 columns in the same order. Columns unavailable
// in a version are returned as -1 sentinel values:
//   - PG 14–16: restartpoints_timed/done/req = -1 (not tracked separately)
//   - PG 17+: buffers_backend/buffers_backend_fsync = -1 (removed from bgwriter)
var checkpointGate = version.Gate{
	Name: "checkpoint",
	Variants: []version.SQLVariant{
		{
			// PG 14–16: everything in pg_stat_bgwriter
			Range: version.VersionRange{MinMajor: 14, MinMinor: 0, MaxMajor: 16, MaxMinor: 99},
			SQL: `SELECT
    checkpoints_timed, checkpoints_req, checkpoint_write_time,
    checkpoint_sync_time, buffers_checkpoint, buffers_clean, maxwritten_clean,
    buffers_alloc, buffers_backend, buffers_backend_fsync,
    -1 AS restartpoints_timed, -1 AS restartpoints_done, -1 AS restartpoints_req
FROM pg_stat_bgwriter`,
		},
		{
			// PG 17+: checkpoint stats in pg_stat_checkpointer, bgwriter stats in pg_stat_bgwriter
			Range: version.VersionRange{MinMajor: 17, MinMinor: 0, MaxMajor: 99, MaxMinor: 99},
			SQL: `SELECT
    c.num_timed AS checkpoints_timed, c.num_requested AS checkpoints_req,
    c.write_time AS checkpoint_write_time, c.sync_time AS checkpoint_sync_time,
    c.buffers_written AS buffers_checkpoint,
    b.buffers_clean, b.maxwritten_clean, b.buffers_alloc,
    -1 AS buffers_backend, -1 AS buffers_backend_fsync,
    c.restartpoints_timed, c.restartpoints_done, c.restartpoints_req
FROM pg_stat_checkpointer c CROSS JOIN pg_stat_bgwriter b`,
		},
	},
}

// checkpointSnapshot holds a single snapshot of checkpoint and bgwriter counters.
// Fields set to -1 indicate the value is unavailable for the current PG version.
type checkpointSnapshot struct {
	checkpointsTimed float64
	checkpointsReq   float64
	writeTimeMs      float64
	syncTimeMs       float64
	buffersWritten   float64
	buffersClean     float64
	maxwrittenClean  float64
	buffersAlloc     float64
	buffersBackend      float64 // -1 on PG 17+
	buffersBackendFsync float64 // -1 on PG 17+
	restartpointsTimed     float64 // -1 on PG <= 16
	restartpointsDone      float64 // -1 on PG <= 16
	restartpointsRequested float64 // -1 on PG <= 16
}

// CheckpointCollector collects checkpoint and background writer statistics.
// This is a stateful collector that computes rate metrics by comparing
// consecutive snapshots of cumulative counters.
type CheckpointCollector struct {
	Base
	sqlGate  version.Gate
	mu       sync.Mutex
	prev     *checkpointSnapshot
	prevTime time.Time
}

// NewCheckpointCollector creates a new CheckpointCollector for the given instance.
func NewCheckpointCollector(instanceID string, v version.PGVersion) *CheckpointCollector {
	return &CheckpointCollector{
		Base:    newBase(instanceID, v, 60*time.Second),
		sqlGate: checkpointGate,
	}
}

// Name returns the collector's identifier.
func (c *CheckpointCollector) Name() string { return "checkpoint" }

// Collect queries checkpoint and bgwriter stats and returns metric points.
// On the first call, only absolute (gauge) metrics are emitted.
// On subsequent calls, rate (per-second) metrics are also computed from
// the delta between the current and previous snapshots.
// If a stats reset is detected (any counter decreased), rate metrics are skipped.
func (c *CheckpointCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	sql, ok := c.sqlGate.Select(c.pgVersion)
	if !ok {
		return nil, fmt.Errorf("checkpoint: no SQL variant for PG %s", c.pgVersion.Full)
	}

	qCtx, cancel := queryContext(ctx)
	defer cancel()

	var curr checkpointSnapshot
	err := conn.QueryRow(qCtx, sql).Scan(
		&curr.checkpointsTimed, &curr.checkpointsReq,
		&curr.writeTimeMs, &curr.syncTimeMs,
		&curr.buffersWritten, &curr.buffersClean, &curr.maxwrittenClean,
		&curr.buffersAlloc, &curr.buffersBackend, &curr.buffersBackendFsync,
		&curr.restartpointsTimed, &curr.restartpointsDone, &curr.restartpointsRequested,
	)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: %w", err)
	}

	now := time.Now()

	c.mu.Lock()
	points := c.computeMetrics(curr, c.prev, c.prevTime, now)
	c.prev = &curr
	c.prevTime = now
	c.mu.Unlock()

	return points, nil
}

// computeMetrics returns absolute metrics and, when a previous snapshot exists
// and no stats reset is detected, also returns rate-based metrics.
func (c *CheckpointCollector) computeMetrics(curr checkpointSnapshot, prev *checkpointSnapshot, prevTime time.Time, now time.Time) []MetricPoint {
	points := c.absolutePoints(curr)

	if prev != nil {
		elapsed := now.Sub(prevTime).Seconds()
		if elapsed > 0 && !c.isStatsReset(curr) {
			points = append(points, c.ratePoints(curr, elapsed)...)
		}
	}

	return points
}

// absolutePoints emits gauge metrics from the current snapshot.
// Metrics for columns unavailable in the current PG version (sentinel -1) are skipped.
func (c *CheckpointCollector) absolutePoints(snap checkpointSnapshot) []MetricPoint {
	points := []MetricPoint{
		c.point("checkpoint.timed", snap.checkpointsTimed, nil),
		c.point("checkpoint.requested", snap.checkpointsReq, nil),
		c.point("checkpoint.write_time_ms", snap.writeTimeMs, nil),
		c.point("checkpoint.sync_time_ms", snap.syncTimeMs, nil),
		c.point("checkpoint.buffers_written", snap.buffersWritten, nil),
		c.point("bgwriter.buffers_clean", snap.buffersClean, nil),
		c.point("bgwriter.maxwritten_clean", snap.maxwrittenClean, nil),
		c.point("bgwriter.buffers_alloc", snap.buffersAlloc, nil),
	}

	// PG 14–16 only
	if snap.buffersBackend >= 0 {
		points = append(points, c.point("bgwriter.buffers_backend", snap.buffersBackend, nil))
	}
	if snap.buffersBackendFsync >= 0 {
		points = append(points, c.point("bgwriter.buffers_backend_fsync", snap.buffersBackendFsync, nil))
	}

	// PG 17+ only
	if snap.restartpointsTimed >= 0 {
		points = append(points, c.point("checkpoint.restartpoints_timed", snap.restartpointsTimed, nil))
	}
	if snap.restartpointsDone >= 0 {
		points = append(points, c.point("checkpoint.restartpoints_done", snap.restartpointsDone, nil))
	}
	if snap.restartpointsRequested >= 0 {
		points = append(points, c.point("checkpoint.restartpoints_req", snap.restartpointsRequested, nil))
	}

	return points
}

// ratePoints computes per-second rate metrics from the delta between current and
// previous snapshots. Must only be called when c.prev is non-nil and elapsed > 0.
func (c *CheckpointCollector) ratePoints(curr checkpointSnapshot, elapsedSec float64) []MetricPoint {
	points := []MetricPoint{
		c.point("checkpoint.timed_per_second", (curr.checkpointsTimed-c.prev.checkpointsTimed)/elapsedSec, nil),
		c.point("checkpoint.requested_per_second", (curr.checkpointsReq-c.prev.checkpointsReq)/elapsedSec, nil),
		c.point("checkpoint.buffers_written_per_second", (curr.buffersWritten-c.prev.buffersWritten)/elapsedSec, nil),
		c.point("bgwriter.buffers_clean_per_second", (curr.buffersClean-c.prev.buffersClean)/elapsedSec, nil),
		c.point("bgwriter.buffers_alloc_per_second", (curr.buffersAlloc-c.prev.buffersAlloc)/elapsedSec, nil),
	}

	// PG 14–16 only
	if c.prev.buffersBackend >= 0 {
		points = append(points,
			c.point("bgwriter.buffers_backend_per_second", (curr.buffersBackend-c.prev.buffersBackend)/elapsedSec, nil),
		)
	}

	return points
}

// isStatsReset detects if pg_stat_bgwriter / pg_stat_checkpointer was reset
// by checking if any cumulative counter decreased. Only compares counters
// that are available (>= 0) in both the current and previous snapshots.
func (c *CheckpointCollector) isStatsReset(curr checkpointSnapshot) bool {
	if c.prev == nil {
		return false
	}

	// Always-available counters
	if curr.checkpointsTimed < c.prev.checkpointsTimed ||
		curr.checkpointsReq < c.prev.checkpointsReq ||
		curr.buffersWritten < c.prev.buffersWritten ||
		curr.buffersClean < c.prev.buffersClean ||
		curr.buffersAlloc < c.prev.buffersAlloc {
		return true
	}

	// PG 14–16 only
	if c.prev.buffersBackend >= 0 && curr.buffersBackend >= 0 &&
		curr.buffersBackend < c.prev.buffersBackend {
		return true
	}

	// PG 17+ only
	if c.prev.restartpointsTimed >= 0 && curr.restartpointsTimed >= 0 &&
		curr.restartpointsTimed < c.prev.restartpointsTimed {
		return true
	}

	return false
}
