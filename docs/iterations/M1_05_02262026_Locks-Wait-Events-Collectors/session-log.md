# Session: 2026-02-26 — M1_05 Locks & Wait Events Collectors

## Goal

Port PGAM queries Q53–Q57 (wait events, lock blocking tree, long transactions). Final M1 iteration for analiz2.php instance-level queries.

## Agent Team Configuration

- **Team Lead:** Opus 4.6 (Claude Code Agent Teams)
- **Specialists:** Collector Agent + QA Agent (2 agents)
- **Duration:** ~5m 46s
- **Mode:** Agent Teams (first use on this project)

## PGAM Queries Ported

| Query # | Description | Target Function | Agent |
|---------|-------------|-----------------|-------|
| Q53 | Wait event summary (verbose) | wait_events.go: WaitEventsCollector | Collector |
| Q54 | Wait event summary (minimal) | Merged with Q53 (identical without VTB join) | Collector |
| Q55 | Lock blocking tree | lock_tree.go: LockTreeCollector | Collector |
| Q56 | Long active transactions | long_transactions.go: LongTransactionsCollector | Collector |
| Q57 | Long waiting transactions | Merged with Q56 (single query, type label) | Collector |

## Agent Activity Summary

### Collector Agent
- Created: `internal/collector/wait_events.go` (90 lines)
- Created: `internal/collector/lock_tree.go` (160 lines)
- Created: `internal/collector/long_transactions.go` (100 lines)

### QA Agent
- Created: `internal/collector/wait_events_test.go` (110 lines, 5 tests)
- Created: `internal/collector/lock_tree_test.go` (130 lines, 11 tests — 7 pure computeLockStats + 4 collector)
- Created: `internal/collector/long_transactions_test.go` (110 lines, 6 tests)

### Build Verification (developer — manual)
- `go build ./...` ✅
- `go vet ./...` ✅
- `golangci-lint run` ✅ 0 issues
- M1_05 tests (23 run) ✅ All pass
- Full collector suite ✅ All pass

## Architecture Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Q53+Q54 merged into single WaitEventsCollector | Identical queries — VTB description join not replicated |
| D2 | Lock tree uses pg_blocking_pids() + Go BFS instead of recursive SQL CTE | Simpler, more testable, pg_blocking_pids() purpose-built for this |
| D3 | Lock tree emits summary metrics only (3 numbers), full tree deferred to M2/API | MetricPoint is numeric — hierarchical tree data doesn't fit; API serves rich views |
| D4 | Q56+Q57 merged with CASE expression and type label | Eliminates duplicate query, single scan of pg_stat_activity |
| D5 | Parameterized threshold ($1) for long transaction age | Future configurability via M2 config layer; hardcoded "5 seconds" constant for now |
| D6 | Filter backend_type='client backend' in wait_events and long_transactions | Excludes background workers/autovacuum noise — improvement over PGAM |
| D7 | NULL wait_event_type mapped to 'CPU'/'Running' labels | Active backends are valuable to monitor alongside wait events |
| D8 | Always emit 4 points from long_transactions (zeros for missing types) | Consistent cardinality simplifies downstream consumers |

## PGAM Bugs / Improvements Fixed

| Issue | Fix |
|-------|-----|
| Q53/Q54 ran as two separate identical queries | Merged into one collector |
| Q55 used complex recursive CTE (fragile, hard to test) | pg_blocking_pids() + Go graph with BFS (7 topology tests) |
| Q56/Q57 ran as two nearly identical queries | Merged with CASE expression |
| PGAM included all backend types in wait events | Filtered to client backends only |
| PGAM Q55 included query text in lock tree output | Deferred to API — no high-cardinality text in metrics |

## Agent Teams Notes (First Use)

- 2-agent configuration (Collector + QA) worked well for this scope
- ~5m46s total execution time — comparable to single-session for 6 files
- Main benefit: QA agent started writing test stubs immediately while collector agent worked
- Coordination overhead was minimal since both agents work in same package
- Hybrid workflow (agents create files, developer runs bash) functioned as expected

## Files Created

| File | Lines |
|------|-------|
| `internal/collector/wait_events.go` | 90 |
| `internal/collector/lock_tree.go` | 160 |
| `internal/collector/long_transactions.go` | 100 |
| `internal/collector/wait_events_test.go` | 110 |
| `internal/collector/lock_tree_test.go` | 130 |
| `internal/collector/long_transactions_test.go` | 110 |
| **Total** | **700** |

## M1 Milestone Completion

With M1_05 done, all analiz2.php instance-level queries are ported:

| Iteration | Scope | Queries | Status |
|-----------|-------|---------|--------|
| M1_01 | Instance metrics | Q1–Q19 (12 new) | ✅ Done |
| M1_02b | Replication | Q20–Q21, Q37–Q38, Q40 | ✅ Done |
| M1_03 | Progress + Checkpoint/BGWriter | Q42–Q47 + new | ✅ Done |
| M1_03b | pg_stat_io | New (PG 16+) | ✅ Done |
| M1_04 | pg_stat_statements | Q48–Q51 | ✅ Done |
| M1_05 | Locks & wait events | Q53–Q57 | ✅ Done |

### Deferred items (by design):
- Q4–Q8: OS metrics → M6 (Go agent)
- Q22–Q35: OS/cluster/overview → M6
- Q36, Q39: PG < 10 replication → below minimum version
- Q41: Logical replication → PerDatabaseCollector pattern
- Q52: Normalized text report → M2/API layer

### Final M1 query count:
- **33 PGAM queries ported** (Q1–Q3, Q9–Q21, Q37–Q38, Q40, Q42–Q51, Q53–Q57)
- **2 new collectors** (checkpoint/bgwriter, pg_stat_io)
- **~20 collector files** + test files in internal/collector/
