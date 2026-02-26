# M1_05 Team Prompt — Locks & Wait Events Collectors

> **Paste this into Claude Code Agent Teams.**
> Agents create files only — developer runs bash manually (Windows bash bug).

---

Build the locks & wait events collectors for PGPulse.
Read `.claude/CLAUDE.md` for project context and interfaces.
Read `docs/iterations/M1_05_.../design.md` for detailed SQL and struct specifications.

**⚠️ CRITICAL: Agents CANNOT run shell commands on this platform.**
Do NOT attempt `go build`, `go test`, `git commit`, or any bash commands.
Create and edit files only. Developer will run all bash commands manually.

Create a team of 2 specialists:

---

## COLLECTOR AGENT

You own `internal/collector/`. Create three new collector files following the **exact patterns** from existing collectors (e.g., `connections.go`, `replication_lag.go`, `statements_top.go`).

**Before writing any code**, read these files to understand the patterns:
- `internal/collector/collector.go` — interfaces (MetricPoint, Collector, InstanceContext)
- `internal/collector/base.go` — Base struct, point(), queryContext(), newBase()
- `internal/collector/connections.go` — example of a simple GROUP BY collector
- `internal/collector/statements_top.go` — example of a collector with Go-side post-processing

### File 1: `internal/collector/wait_events.go`

**Purpose:** Port PGAM Q53/Q54 — wait event summary.

**SQL:**
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

**Struct:** `WaitEventsCollector` with `Base` embed.
- Constructor: `NewWaitEventsCollector(instanceID string, v version.PGVersion) *WaitEventsCollector`
- Name: `"wait_events"`
- Interval: `10 * time.Second`

**Metrics:**
- `pgpulse.wait_events.count` with labels `{wait_event_type, wait_event}` — one point per row
- `pgpulse.wait_events.total_backends` with no labels — sum of all counts

**Collect() logic:**
1. queryContext(ctx) for 5s timeout
2. conn.Query() with the SQL above (no params)
3. Iterate rows: emit `wait_events.count` per row, accumulate total
4. Emit `wait_events.total_backends` with accumulated total
5. Return slice

### File 2: `internal/collector/lock_tree.go`

**Purpose:** Port PGAM Q55 — lock blocking tree (summary metrics only).

**SQL:**
```sql
SELECT
    a.pid AS blocked_pid,
    unnest(pg_blocking_pids(a.pid)) AS blocker_pid
FROM pg_stat_activity a
WHERE cardinality(pg_blocking_pids(a.pid)) > 0
  AND a.pid != pg_backend_pid()
```

**Struct:** `LockTreeCollector` with `Base` embed.
- Constructor: `NewLockTreeCollector(instanceID string, v version.PGVersion) *LockTreeCollector`
- Name: `"lock_tree"`
- Interval: `10 * time.Second`

**Exported types (used by tests):**

```go
// lockEdge represents a single blocking relationship.
type lockEdge struct {
    BlockedPID int
    BlockerPID int
}

// lockStats holds summary metrics computed from blocking edges.
type lockStats struct {
    BlockerCount  int
    BlockedCount  int
    MaxChainDepth int
}
```

**Pure function — `computeLockStats(edges []lockEdge) lockStats`:**
- Build adjacency maps from edges
- Find root blockers (in blockerSet but NOT in blockedSet)
- Calculate max chain depth via BFS with visited set (cycle protection)
- Return lockStats

**Helper — `bfsMaxDepth(startPID int, blocks map[int]map[int]bool) int`:**
- BFS from startPID through the "blocks" adjacency map
- Track visited to prevent cycles
- Return max depth reached

**Metrics:**
- `pgpulse.locks.blocker_count` — root blockers (not themselves blocked)
- `pgpulse.locks.blocked_count` — distinct blocked PIDs
- `pgpulse.locks.max_chain_depth` — longest chain (0 if no blocking)

**Collect() logic:**
1. queryContext(ctx) for 5s timeout
2. conn.Query() — collect all rows into `[]lockEdge`
3. Call `computeLockStats(edges)`
4. Emit three MetricPoints from the stats
5. If 0 rows → computeLockStats returns zeros → emit three zero-value points

### File 3: `internal/collector/long_transactions.go`

**Purpose:** Port PGAM Q56/Q57 — long active + waiting transactions (merged).

**SQL (parameterized threshold):**
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

**Constant:**
```go
const longTxnThreshold = "5 seconds"
```

**Struct:** `LongTransactionsCollector` with `Base` embed.
- Constructor: `NewLongTransactionsCollector(instanceID string, v version.PGVersion) *LongTransactionsCollector`
- Name: `"long_transactions"`
- Interval: `10 * time.Second`

**Metrics:**
- `pgpulse.long_transactions.count` with label `{type: "active"|"waiting"}`
- `pgpulse.long_transactions.oldest_seconds` with label `{type: "active"|"waiting"}`

**Collect() logic:**
1. queryContext(ctx) for 5s timeout
2. conn.Query() with `longTxnThreshold` as $1
3. Iterate rows: for each (txn_type, cnt, oldest_seconds), emit two points
4. Track which types were seen (active, waiting)
5. For any missing type, emit count=0, oldest_seconds=0
6. Always emit exactly 4 points (2 types × 2 metrics)

### Final checklist for Collector Agent:
- [ ] All three files compile (no syntax errors)
- [ ] All use `queryContext(ctx)` for timeout
- [ ] All exclude `pg_backend_pid()`
- [ ] All follow Base struct pattern
- [ ] No query text in labels
- [ ] No version gates (not needed for PG 14–17)
- [ ] lockEdge, lockStats, computeLockStats, bfsMaxDepth are unexported but accessible from test file (same package)

---

## QA AGENT

You own all `*_test.go` files in `internal/collector/`. Create three test files.

**Before writing any code**, read these files to understand test patterns:
- `internal/collector/testutil_test.go` — mock helpers
- `internal/collector/connections_test.go` — example unit test
- `internal/collector/statements_top_test.go` — example test with row mocking

### File 1: `internal/collector/wait_events_test.go`

| Test | What to verify |
|------|---------------|
| TestWaitEventsCollector_Name | Returns "wait_events" |
| TestWaitEventsCollector_Interval | Returns 10s |
| TestWaitEventsCollector_Collect | Mock 3 rows (IO, Lock, CPU) → verify 3 count points + 1 total point, correct labels, correct total sum |
| TestWaitEventsCollector_Collect_Empty | Mock 0 rows → verify 1 point (total_backends = 0) |

### File 2: `internal/collector/lock_tree_test.go`

**Focus heavily on `computeLockStats` — it's a pure function, no mocks needed.**

| Test | Input edges | Expected |
|------|-------------|----------|
| TestComputeLockStats_NoEdges | nil | {0, 0, 0} |
| TestComputeLockStats_SingleBlocker | A→B | {blockers:1, blocked:1, depth:1} |
| TestComputeLockStats_Chain | A→B, B→C | {blockers:1, blocked:2, depth:2} |
| TestComputeLockStats_Wide | A→B, A→C | {blockers:1, blocked:2, depth:1} |
| TestComputeLockStats_MultiRoot | A→B, C→D | {blockers:2, blocked:2, depth:1} |
| TestComputeLockStats_Diamond | A→B, A→C, B→D, C→D | {blockers:1, blocked:3, depth:2} |
| TestComputeLockStats_Cycle | A→B, B→A | {blockers:0, blocked:2, depth:0} |

Also:

| Test | What to verify |
|------|---------------|
| TestLockTreeCollector_Name | Returns "lock_tree" |
| TestLockTreeCollector_Interval | Returns 10s |
| TestLockTreeCollector_Collect | Mock 2 edge rows → verify 3 metric points |
| TestLockTreeCollector_Collect_NoBlocking | Mock 0 rows → verify 3 points all zero |

### File 3: `internal/collector/long_transactions_test.go`

| Test | What to verify |
|------|---------------|
| TestLongTransactionsCollector_Name | Returns "long_transactions" |
| TestLongTransactionsCollector_Interval | Returns 10s |
| TestLongTransactionsCollector_Collect_Both | Mock 2 rows (active + waiting) → verify 4 points with correct labels |
| TestLongTransactionsCollector_Collect_ActiveOnly | Mock 1 row (active) → verify 4 points (active values + waiting zeros) |
| TestLongTransactionsCollector_Collect_None | Mock 0 rows → verify 4 points all zero |
| TestLongTransactionsCollector_Collect_WaitingOnly | Mock 1 row (waiting) → verify 4 points (waiting values + active zeros) |

### Final checklist for QA Agent:
- [ ] All test files compile
- [ ] Tests use established mock patterns from testutil_test.go
- [ ] computeLockStats tests are pure (no mocks, no DB)
- [ ] Each collector test covers: name, interval, normal collect, edge cases
- [ ] No `//go:build integration` tags — these are unit tests

---

## Coordination

- Collector Agent and QA Agent can start in parallel
- QA Agent: read the Collector Agent's files as soon as they're created to verify struct names and method signatures
- If a struct name or method signature doesn't match between collector and test, coordinate via task list
- **Do NOT attempt to run `go build` or `go test`** — developer will do this

## Output

When both agents are done, list all files created so the developer can run:
```
go build ./...
go vet ./...
golangci-lint run
go test -v ./internal/collector/ -run "TestWaitEvents|TestLockTree|TestLongTransactions|TestComputeLockStats"
```
