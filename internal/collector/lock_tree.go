package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// sqlLockTree fetches all (blocked_pid, blocker_pid) pairs using
// pg_blocking_pids(), which is simpler and more reliable than the
// recursive pg_locks CTE used in PGAM. One row per blocking relationship
// (a PID blocked by N blockers produces N rows).
// PGAM source: analiz2.php Q55.
const sqlLockTree = `
SELECT
    a.pid                              AS blocked_pid,
    unnest(pg_blocking_pids(a.pid))    AS blocker_pid
FROM pg_stat_activity a
WHERE cardinality(pg_blocking_pids(a.pid)) > 0
  AND a.pid != pg_backend_pid()`

// lockEdge represents a single blocking relationship: BlockerPID is holding
// a lock that is preventing BlockedPID from proceeding.
type lockEdge struct {
	BlockedPID int
	BlockerPID int
}

// lockStats holds summary metrics computed from a set of blocking edges.
type lockStats struct {
	BlockerCount  int // root blockers: blocking someone but not themselves blocked
	BlockedCount  int // distinct blocked PIDs
	MaxChainDepth int // longest chain (0 = no blocking)
}

// LockTreeCollector reports lock blocking summary metrics by querying
// pg_blocking_pids() and computing graph statistics in Go.
// PGAM source: analiz2.php Q55.
type LockTreeCollector struct {
	Base
}

// NewLockTreeCollector creates a new LockTreeCollector for the given instance.
func NewLockTreeCollector(instanceID string, v version.PGVersion) *LockTreeCollector {
	return &LockTreeCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *LockTreeCollector) Name() string { return "lock_tree" }

// Collect queries pg_stat_activity for blocking relationships and emits
// three summary metrics. Emits all-zero metrics when no blocking exists.
func (c *LockTreeCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sqlLockTree)
	if err != nil {
		return nil, fmt.Errorf("lock_tree: %w", err)
	}
	defer rows.Close()

	var edges []lockEdge
	for rows.Next() {
		var e lockEdge
		if err := rows.Scan(&e.BlockedPID, &e.BlockerPID); err != nil {
			return nil, fmt.Errorf("lock_tree scan: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lock_tree rows: %w", err)
	}

	return c.statsToPoints(computeLockStats(edges)), nil
}

// statsToPoints converts a lockStats into three MetricPoints.
// Extracted as a method so it can be tested without a database connection.
func (c *LockTreeCollector) statsToPoints(stats lockStats) []MetricPoint {
	return []MetricPoint{
		c.point("locks.blocker_count", float64(stats.BlockerCount), nil),
		c.point("locks.blocked_count", float64(stats.BlockedCount), nil),
		c.point("locks.max_chain_depth", float64(stats.MaxChainDepth), nil),
	}
}

// computeLockStats builds an adjacency graph from blocking edges and computes
// summary statistics. This is a pure function — no DB access — making it
// directly unit-testable.
//
// Cycle handling: in a deadlock (A blocks B, B blocks A), both PIDs appear in
// both blockedSet and blockerSet, so BlockerCount (roots) is 0. BlockedCount
// is still correct. The BFS visited set prevents infinite loops.
func computeLockStats(edges []lockEdge) lockStats {
	if len(edges) == 0 {
		return lockStats{}
	}

	blockedSet := make(map[int]bool) // PIDs that are blocked by someone
	blockerSet := make(map[int]bool) // PIDs that are blocking someone

	// "blocks" maps blocker_pid → set of blocked_pids (direction: blocker → victim)
	blocks := make(map[int]map[int]bool)

	for _, e := range edges {
		blockedSet[e.BlockedPID] = true
		blockerSet[e.BlockerPID] = true
		if blocks[e.BlockerPID] == nil {
			blocks[e.BlockerPID] = make(map[int]bool)
		}
		blocks[e.BlockerPID][e.BlockedPID] = true
	}

	// Root blockers: in blockerSet but NOT in blockedSet.
	roots := 0
	for pid := range blockerSet {
		if !blockedSet[pid] {
			roots++
		}
	}

	// Max chain depth: BFS from each root blocker downward.
	maxDepth := 0
	for pid := range blockerSet {
		if blockedSet[pid] {
			continue // not a root — skip
		}
		if d := bfsMaxDepth(pid, blocks); d > maxDepth {
			maxDepth = d
		}
	}

	return lockStats{
		BlockerCount:  roots,
		BlockedCount:  len(blockedSet),
		MaxChainDepth: maxDepth,
	}
}

// bfsMaxDepth returns the maximum depth reachable from startPID through
// the blocks adjacency map. Depth 1 means startPID directly blocks someone.
// A visited set prevents infinite loops in cyclic graphs (deadlocks).
func bfsMaxDepth(startPID int, blocks map[int]map[int]bool) int {
	type item struct {
		pid   int
		depth int
	}
	queue := []item{{startPID, 0}}
	visited := map[int]bool{startPID: true}
	maxDepth := 0

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr.depth > maxDepth {
			maxDepth = curr.depth
		}
		for child := range blocks[curr.pid] {
			if !visited[child] {
				visited[child] = true
				queue = append(queue, item{child, curr.depth + 1})
			}
		}
	}
	return maxDepth
}
