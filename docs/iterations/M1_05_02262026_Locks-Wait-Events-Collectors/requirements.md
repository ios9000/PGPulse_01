# M1_05 Requirements — Locks & Wait Events Collectors

**Iteration:** M1_05
**Milestone:** M1 (Core Collectors — final iteration for analiz2.php instance-level queries)
**PGAM Queries:** Q53, Q54, Q55, Q56, Q57
**Date:** 2026-02-26

---

## Goal

Port PGAM queries Q53–Q57, covering wait event monitoring, lock blocking detection, and long transaction tracking. This completes all analiz2.php instance-level collectors.

## Scope

### In Scope

| Collector | PGAM Queries | Output |
|-----------|-------------|--------|
| WaitEventsCollector | Q53 + Q54 (merged) | Per wait_event_type/wait_event counts |
| LockTreeCollector | Q55 (summary metrics) | Blocker count, blocked count, max chain depth |
| LongTransactionsCollector | Q56 + Q57 (merged) | Count + oldest age, labeled active vs waiting |

All three are high-frequency collectors (10s interval).

### Out of Scope

- Full lock tree structure (per-PID details, query text, lock modes) → deferred to M2/API layer
- VTB wait-event description join → not replicated; descriptions are an API/UI concern
- Per-PID transaction details (query text, client_addr) → API layer
- Configurable long-transaction threshold → M2 (config layer); use 5s constant for now

## Functional Requirements

### FR-1: WaitEventsCollector

- Query pg_stat_activity grouped by wait_event_type and wait_event
- Exclude own backend (pid != pg_backend_pid())
- Emit `pgpulse.wait_events.count` per (wait_event_type, wait_event) pair
- Emit `pgpulse.wait_events.total_backends` as the sum total
- NULL wait_event_type means the backend is active (not waiting); include as label "Active" or skip — design.md decides

### FR-2: LockTreeCollector

- Detect blocking relationships using `pg_blocking_pids()` function (PG 9.6+, within our PG 14+ minimum)
- Query returns (blocked_pid, blocker_pid) pairs
- Build adjacency graph in Go, compute:
  - Number of distinct root blockers (blockers that are not themselves blocked)
  - Number of distinct blocked processes
  - Maximum chain depth (via BFS/DFS in Go)
- Emit summary metrics only:
  - `pgpulse.locks.blocker_count`
  - `pgpulse.locks.blocked_count`
  - `pgpulse.locks.max_chain_depth`
- When no blocking exists, emit all three as 0 (not nil)

### FR-3: LongTransactionsCollector

- Query pg_stat_activity for transactions older than 5 seconds
- Exclude own backend
- Split into two categories via label:
  - `type=active` — state='active' AND wait_event IS NULL (Q56)
  - `type=waiting` — state='active' AND wait_event IS NOT NULL (Q57)
- Emit per type:
  - `pgpulse.long_transactions.count` — number of long transactions
  - `pgpulse.long_transactions.oldest_seconds` — age of the oldest (xact_start based)
- When none found for a type, emit count=0, oldest_seconds=0

## Non-Functional Requirements

- **NFR-1:** All queries use 5s statement timeout via queryContext()
- **NFR-2:** All queries exclude own backend (pg_backend_pid())
- **NFR-3:** No query text in metric labels (avoid high cardinality)
- **NFR-4:** No version gates required — pg_stat_activity and pg_blocking_pids() are stable PG 14–17
- **NFR-5:** Follow established Base struct pattern, point() helper, Collector interface
- **NFR-6:** Unit tests with pgx mock for all three collectors
- **NFR-7:** Integration tests marked with build tag (CI-only, Docker required)

## PGAM Bugs / Improvements

| PGAM Issue | PGPulse Fix |
|------------|-------------|
| Q53/Q54 are separate queries for identical data | Merged into single collector |
| Q55 uses complex recursive CTE in SQL | Use pg_blocking_pids() + Go graph traversal (simpler, more testable) |
| Q56/Q57 are nearly identical queries | Merged into single collector with type label |
| Q56/Q57 hardcode `interval '5 seconds'` in SQL | Use parameterized threshold ($1) for future configurability |
| Q55 includes query text in output | Defer to API layer — collectors emit numeric summaries only |

## Acceptance Criteria

1. `go build ./...` passes
2. `go vet ./...` passes
3. `golangci-lint run` reports 0 issues
4. All unit tests pass
5. Each collector returns correct MetricPoint slice shape
6. Lock tree correctly identifies root blockers, blocked processes, and chain depth
7. Long transactions correctly splits active vs waiting
8. Wait events excludes own backend
