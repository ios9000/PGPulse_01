# M1_05 Design — Locks & Wait Events Collectors

**Iteration:** M1_05
**Date:** 2026-02-26

---

## 1. Collector: WaitEventsCollector

### File: `internal/collector/wait_events.go`

### SQL Query

```sql
SELECT
    COALESCE(wait_event_type, 'CPU') AS wait_event_type,
    COALESCE(wait_event, 'Running') AS wait_event,
    count(*) AS cnt
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
  AND backend_type = 'client backend'
GROUP BY 1, 2
ORDER BY 3 DESC
```

**Design notes:**
- NULL wait_event_type means the backend is actively running on CPU (not waiting). We label this `CPU` / `Running` rather than omitting it — it's valuable to see CPU-active backends alongside wait events.
- Filter to `client backend` to exclude background workers, autovacuum, WAL sender, etc. These are infrastructure processes that would add noise. (PGAM included everything; this is a deliberate improvement.)
- No version gate needed — columns stable PG 14–17.

### Struct

```go
type WaitEventsCollector struct {
    Base
}

func NewWaitEventsCollector(instanceID string, v version.PGVersion) *WaitEventsCollector {
    return &WaitEventsCollector{
        Base: newBase(instanceID, v, 10*time.Second),
    }
}

func (c *WaitEventsCollector) Name() string { return "wait_events" }
```

### Metrics Emitted

| Metric | Labels | Description |
|--------|--------|-------------|
| `pgpulse.wait_events.count` | `{wait_event_type, wait_event}` | Backend count per event |
| `pgpulse.wait_events.total_backends` | none | Sum of all counted backends |

### Collect() Logic

1. Run query via queryContext()
2. Iterate rows: for each (wait_event_type, wait_event, count), emit a `wait_events.count` point with labels
3. Accumulate total across rows
4. Emit `wait_events.total_backends` with the total
5. Return slice

---

## 2. Collector: LockTreeCollector

### File: `internal/collector/lock_tree.go`

### SQL Query

```sql
SELECT
    a.pid AS blocked_pid,
    unnest(pg_blocking_pids(a.pid)) AS blocker_pid
FROM pg_stat_activity a
WHERE cardinality(pg_blocking_pids(a.pid)) > 0
  AND a.pid != pg_backend_pid()
```

**Design notes:**
- Uses `pg_blocking_pids()` (available since PG 9.6) instead of PGAM's complex recursive CTE on pg_locks. This is simpler, more reliable, and the function is specifically designed for this purpose.
- Returns one row per (blocked_pid, blocker_pid) pair. If PID 100 is blocked by PIDs 200 and 300, two rows are returned.
- The recursive CTE for tree structure is replaced by Go graph traversal — more testable, no SQL recursion complexity.

### Graph Computation (Pure Go — `computeLockStats()`)

```go
// lockEdge represents a single blocking relationship.
type lockEdge struct {
    BlockedPID int
    BlockerPID int
}

// lockStats holds the summary metrics computed from blocking edges.
type lockStats struct {
    BlockerCount  int // distinct root blockers
    BlockedCount  int // distinct blocked processes
    MaxChainDepth int // longest blocker chain
}

// computeLockStats builds an adjacency graph from edges and computes
// summary statistics. This is a pure function — no DB access — making
// it easily unit-testable.
func computeLockStats(edges []lockEdge) lockStats {
    if len(edges) == 0 {
        return lockStats{}
    }

    // Build sets.
    blockedSet := make(map[int]bool)   // PIDs that are blocked
    blockerSet := make(map[int]bool)   // PIDs that are blocking someone
    // blockedBy maps: blocked_pid → set of blocker_pids
    blockedBy := make(map[int]map[int]bool)

    for _, e := range edges {
        blockedSet[e.BlockedPID] = true
        blockerSet[e.BlockerPID] = true
        if blockedBy[e.BlockedPID] == nil {
            blockedBy[e.BlockedPID] = make(map[int]bool)
        }
        blockedBy[e.BlockedPID][e.BlockerPID] = true
    }

    // Root blockers: blocking someone but not themselves blocked.
    roots := 0
    for pid := range blockerSet {
        if !blockedSet[pid] {
            roots++
        }
    }

    // Max chain depth via BFS from root blockers downward.
    // "blocks" map: blocker_pid → set of blocked_pids (reverse of blockedBy).
    blocks := make(map[int]map[int]bool)
    for blocked, blockers := range blockedBy {
        for blocker := range blockers {
            if blocks[blocker] == nil {
                blocks[blocker] = make(map[int]bool)
            }
            blocks[blocker][blocked] = true
        }
    }

    maxDepth := 0
    // BFS from each root.
    for pid := range blockerSet {
        if blockedSet[pid] {
            continue // not a root
        }
        depth := bfsMaxDepth(pid, blocks)
        if depth > maxDepth {
            maxDepth = depth
        }
    }

    return lockStats{
        BlockerCount:  roots,
        BlockedCount:  len(blockedSet),
        MaxChainDepth: maxDepth,
    }
}

// bfsMaxDepth returns the maximum depth reachable from startPID
// through the blocks adjacency map. Depth 1 means startPID directly
// blocks someone. Includes cycle protection via visited set.
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
```

### Struct

```go
type LockTreeCollector struct {
    Base
}

func NewLockTreeCollector(instanceID string, v version.PGVersion) *LockTreeCollector {
    return &LockTreeCollector{
        Base: newBase(instanceID, v, 10*time.Second),
    }
}

func (c *LockTreeCollector) Name() string { return "lock_tree" }
```

### Metrics Emitted

| Metric | Labels | Description |
|--------|--------|-------------|
| `pgpulse.locks.blocker_count` | none | Root blockers (blocking others, not blocked themselves) |
| `pgpulse.locks.blocked_count` | none | Distinct blocked processes |
| `pgpulse.locks.max_chain_depth` | none | Deepest blocking chain (0 = no blocking) |

### Collect() Logic

1. Run query via queryContext()
2. Collect all (blocked_pid, blocker_pid) pairs into `[]lockEdge`
3. Call `computeLockStats(edges)` — pure function, no DB
4. Emit three MetricPoints (blocker_count, blocked_count, max_chain_depth)
5. If query returns 0 rows → emit all three as 0 (healthy state)

### Edge Cases

- **Circular blocking (deadlocks):** pg_blocking_pids() can return cycles. The BFS visited set prevents infinite loops. In a true deadlock, all participants are both blockers and blocked, so `BlockerCount` (roots) may be 0 while `BlockedCount` > 0. This is correct — it signals a deadlock situation.
- **PG detects and breaks deadlocks** within `deadlock_timeout` (default 1s), so cycles are transient. Still, the visited set is essential for correctness.
- **Self-blocking:** pg_blocking_pids() excludes self. No special handling needed.
- **High lock contention:** The query itself takes locks on pg_stat_activity (lightweight). Under extreme contention (1000+ blocked PIDs), the unnest expansion could produce many rows. The 5s statement timeout protects against this.

---

## 3. Collector: LongTransactionsCollector

### File: `internal/collector/long_transactions.go`

### SQL Query

```sql
SELECT
    CASE WHEN wait_event IS NULL THEN 'active' ELSE 'waiting' END AS txn_type,
    count(*) AS cnt,
    COALESCE(extract(epoch FROM max(now() - xact_start)), 0) AS oldest_seconds
FROM pg_stat_activity
WHERE xact_start < now() - $1::interval
  AND state = 'active'
  AND pid != pg_backend_pid()
  AND backend_type = 'client backend'
GROUP BY 1
```

**Design notes:**
- Merges Q56 and Q57 into a single query with a CASE expression. PGAM ran two separate queries differing only in `wait_event IS NULL` vs `IS NOT NULL`.
- Uses parameterized threshold (`$1`) for the interval. Initially passed as `'5 seconds'` string. This makes future configurability trivial.
- Filter to `client backend` (same rationale as wait events).
- `max(now() - xact_start)` gives the oldest transaction age in each category.
- If no transactions match a category, that category simply has no row — the Go code emits 0 for missing categories.

### Constants

```go
const (
    // longTxnThreshold is the minimum transaction age to be considered "long".
    // Matches PGAM's hardcoded 5-second threshold. Will become configurable in M2.
    longTxnThreshold = "5 seconds"
)
```

### Struct

```go
type LongTransactionsCollector struct {
    Base
}

func NewLongTransactionsCollector(instanceID string, v version.PGVersion) *LongTransactionsCollector {
    return &LongTransactionsCollector{
        Base: newBase(instanceID, v, 10*time.Second),
    }
}

func (c *LongTransactionsCollector) Name() string { return "long_transactions" }
```

### Metrics Emitted

| Metric | Labels | Description |
|--------|--------|-------------|
| `pgpulse.long_transactions.count` | `{type: "active"\|"waiting"}` | Count of long transactions |
| `pgpulse.long_transactions.oldest_seconds` | `{type: "active"\|"waiting"}` | Age of oldest in seconds |

### Collect() Logic

1. Run query with `longTxnThreshold` as `$1`
2. Iterate rows: for each (txn_type, cnt, oldest_seconds), emit two points (count + oldest)
3. Track which types were seen
4. For any missing type (no rows returned for "active" or "waiting"), emit count=0, oldest_seconds=0
5. Return slice

---

## 4. Test Strategy

### File: `internal/collector/wait_events_test.go`

| Test | Description |
|------|-------------|
| TestWaitEventsCollector_Name | Returns "wait_events" |
| TestWaitEventsCollector_Interval | Returns 10s |
| TestWaitEventsCollector_Collect | Mock returns 3 event types → verify point count and labels |
| TestWaitEventsCollector_Collect_Empty | Mock returns 0 rows → verify total_backends = 0 |
| TestWaitEventsCollector_Collect_NullEventHandling | Verify CPU/Running label for active backends |

### File: `internal/collector/lock_tree_test.go`

| Test | Description |
|------|-------------|
| TestLockTreeCollector_Name | Returns "lock_tree" |
| TestLockTreeCollector_Interval | Returns 10s |
| TestComputeLockStats_NoEdges | Empty input → all zeros |
| TestComputeLockStats_SingleBlocker | A blocks B → blockers=1, blocked=1, depth=1 |
| TestComputeLockStats_Chain | A blocks B blocks C → blockers=1, blocked=2, depth=2 |
| TestComputeLockStats_Wide | A blocks B, A blocks C → blockers=1, blocked=2, depth=1 |
| TestComputeLockStats_MultiRoot | A blocks B, C blocks D → blockers=2, blocked=2, depth=1 |
| TestComputeLockStats_Diamond | A blocks B, A blocks C, B blocks D, C blocks D → blockers=1, blocked=3, depth=2 |
| TestComputeLockStats_Cycle | A blocks B, B blocks A → blockers=0, blocked=2, depth=0 (deadlock) |
| TestLockTreeCollector_Collect | Mock returns edges → verify 3 metrics emitted |
| TestLockTreeCollector_Collect_NoBlocking | Mock returns 0 rows → all metrics = 0 |

The `computeLockStats()` function is pure and gets the most thorough testing — no mocks needed for the graph logic tests.

### File: `internal/collector/long_transactions_test.go`

| Test | Description |
|------|-------------|
| TestLongTransactionsCollector_Name | Returns "long_transactions" |
| TestLongTransactionsCollector_Interval | Returns 10s |
| TestLongTransactionsCollector_Collect_Both | Mock returns active + waiting rows → 4 points |
| TestLongTransactionsCollector_Collect_ActiveOnly | Mock returns only active → active points + waiting zeros |
| TestLongTransactionsCollector_Collect_None | Mock returns 0 rows → all zeros for both types |
| TestLongTransactionsCollector_Collect_WaitingOnly | Mock returns only waiting → waiting points + active zeros |

---

## 5. Version Gate Summary

**None required.** All views and functions used are stable across PG 14–17:
- `pg_stat_activity` — stable since PG 9.6
- `pg_blocking_pids()` — stable since PG 9.6
- `backend_type` column — stable since PG 10

---

## 6. File Summary

| File | Lines (est.) | Agent |
|------|-------------|-------|
| `internal/collector/wait_events.go` | ~80 | Collector |
| `internal/collector/lock_tree.go` | ~160 | Collector |
| `internal/collector/long_transactions.go` | ~100 | Collector |
| `internal/collector/wait_events_test.go` | ~120 | QA |
| `internal/collector/lock_tree_test.go` | ~200 | QA |
| `internal/collector/long_transactions_test.go` | ~140 | QA |
| **Total** | **~800** | |

---

## 7. Patterns to Follow (from existing collectors)

All three collectors follow the identical pattern established in M1_01–M1_04:

```go
// Constructor
func NewXxxCollector(instanceID string, v version.PGVersion) *XxxCollector

// Collector interface
func (c *XxxCollector) Name() string
func (c *XxxCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
func (c *XxxCollector) Interval() time.Duration  // via Base

// Query execution
qCtx, cancel := queryContext(ctx)  // 5s timeout
defer cancel()
rows, err := conn.Query(qCtx, sql, args...)

// Point creation
c.point("metric_name", value, map[string]string{"label": "value"})
```

Tests follow the mock pattern from testutil_test.go.
