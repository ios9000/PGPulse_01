# PGPulse — M1_01 Requirements: Instance Metrics Collector

**Milestone:** M1 — Core Collector
**Iteration:** M1_01
**Date:** 2026-02-25
**Input:** HANDOFF_M0_to_M1.md, PGAM_FEATURE_AUDIT.md queries 1–19

---

## Objective

Port PGAM instance-level queries (Q2–Q3, Q9–Q19) into Go collectors that implement
the `Collector` interface from M0. This creates the first real metric collection
pipeline — proving the architecture works end-to-end from SQL → MetricPoint → consumer.

Queries Q1 (version detection) was completed in M0. Queries Q4–Q8 (OS metrics via
COPY TO PROGRAM) are deferred to M6 (OS Agent).

---

## Scope

### In Scope

1. **7 collector modules** implementing the `Collector` interface:
   - ServerInfoCollector — PG start time, uptime, recovery state, backup state
   - ConnectionsCollector — active/idle/total counts, max, reserved, utilization %
   - CacheCollector — global buffer cache hit ratio
   - TransactionsCollector — commit/rollback ratio per database
   - DatabaseSizesCollector — size of each non-template database
   - SettingsCollector — key runtime settings (track_io_timing, etc.)
   - ExtensionsCollector — pg_stat_statements presence and fill %

2. **Collector registry** — central registration and batch execution

3. **Unit tests** for each collector using testcontainers-go (PG 14, 15, 16, 17)

4. **Version gate** for pg_is_in_backup() (PG 14 only, removed in PG 15+)

5. **PGAM bug fixes** applied during port:
   - Q11: exclude own connection from count
   - Q14: NULLIF guard against division by zero

### Out of Scope

- OS metrics (Q4–Q8) → M6
- Checkpoint stats / bgwriter stats (PG 17 split: pg_stat_checkpointer) → M1_03
- pg_stat_io (PG 16+ only) → M1_03
- WAL generation rate → M1_03
- Storage layer (MetricStore implementation) → M2
- REST API endpoints → M2
- Alert evaluation → M4
- Connection pooling / scheduling → M2
- Replication queries (Q20–41) → M1_02
- Progress queries (Q42–47) → M1_03
- Statement queries (Q48–52) → M1_04
- Lock queries (Q53–58) → M1_05

---

## Functional Requirements

### FR-1: ServerInfoCollector

| ID | Requirement |
|----|-------------|
| FR-1.1 | Collect PG postmaster start time as a Unix timestamp metric |
| FR-1.2 | Compute uptime in seconds in Go (time.Since), not via SQL |
| FR-1.3 | Report recovery state as boolean metric (1.0 = replica, 0.0 = primary) |
| FR-1.4 | On PG 14: report backup state via pg_is_in_backup() as boolean metric |
| FR-1.5 | On PG 15+: skip backup state query entirely (function removed) |
| FR-1.6 | Version gate must use the Gate pattern from internal/version/gate.go |

### FR-2: ConnectionsCollector

| ID | Requirement |
|----|-------------|
| FR-2.1 | Count total connections excluding PGPulse's own backend PID |
| FR-2.2 | Break down connections by state: active, idle, idle_in_transaction, fastpath |
| FR-2.3 | Collect max_connections from pg_settings |
| FR-2.4 | Collect superuser_reserved_connections from pg_settings |
| FR-2.5 | Compute utilization percentage: total / (max - superuser_reserved) × 100 |
| FR-2.6 | Each state count emitted as a separate MetricPoint with state label |

### FR-3: CacheCollector

| ID | Requirement |
|----|-------------|
| FR-3.1 | Compute global cache hit ratio from pg_stat_database |
| FR-3.2 | Use NULLIF to guard against division by zero (PGAM bug fix) |
| FR-3.3 | Return ratio as percentage (0.0–100.0) |

### FR-4: TransactionsCollector

| ID | Requirement |
|----|-------------|
| FR-4.1 | Compute commit ratio per database: commits / (commits + rollbacks) × 100 |
| FR-4.2 | Use NULLIF guard for databases with zero transactions |
| FR-4.3 | Include database name as label on each MetricPoint |
| FR-4.4 | Report total deadlocks per database |

### FR-5: DatabaseSizesCollector

| ID | Requirement |
|----|-------------|
| FR-5.1 | Report size in bytes for each non-template database |
| FR-5.2 | Include database name as label |
| FR-5.3 | Skip template0 and template1 |

### FR-6: SettingsCollector

| ID | Requirement |
|----|-------------|
| FR-6.1 | Check track_io_timing (on/off as 1.0/0.0) |
| FR-6.2 | Check shared_buffers (report raw value in 8KB pages) |
| FR-6.3 | Check max_locks_per_transaction |
| FR-6.4 | Check max_prepared_transactions |
| FR-6.5 | Extensible: adding new settings should require only adding to a list |

### FR-7: ExtensionsCollector

| ID | Requirement |
|----|-------------|
| FR-7.1 | Report whether pg_stat_statements is installed (1.0/0.0) |
| FR-7.2 | If installed: report fill percentage (current count / max × 100) |
| FR-7.3 | If not installed: skip fill query, emit only presence metric |
| FR-7.4 | On PG ≥ 14 with pgss installed: report stats_reset timestamp |

### FR-8: Registry

| ID | Requirement |
|----|-------------|
| FR-8.1 | Provide RegisterCollector() to add a collector to the registry |
| FR-8.2 | Provide CollectAll() that runs all registered collectors and returns combined []MetricPoint |
| FR-8.3 | CollectAll() must respect context cancellation |
| FR-8.4 | CollectAll() must log (slog) each collector's duration and any errors |
| FR-8.5 | Individual collector failure must NOT abort the entire collection cycle |

---

## Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NFR-1 | All SQL must use pgx parameterized queries — zero string concatenation |
| NFR-2 | Every query must set statement_timeout = 5s |
| NFR-3 | Every connection must set application_name = 'pgpulse_collector' |
| NFR-4 | Each collector must complete within 5 seconds (enforced by context timeout) |
| NFR-5 | Unit tests must run against real PG via testcontainers-go |
| NFR-6 | Test matrix: PG 14, 16, 17 minimum (15 optional for CI speed) |
| NFR-7 | Zero golangci-lint warnings with project .golangci.yml |
| NFR-8 | All exported types and functions must have GoDoc comments |
| NFR-9 | Errors must be wrapped with context: fmt.Errorf("collectX: %w", err) |

---

## Acceptance Criteria

1. `go build ./...` passes with zero errors
2. `go vet ./...` passes with zero warnings
3. `golangci-lint run` passes with zero issues
4. All unit tests pass: `go test ./internal/collector/... ./internal/version/...`
5. Integration tests verify SQL executes without error on PG 14 and PG 17
6. Version gate test confirms pg_is_in_backup() runs on PG 14, is skipped on PG 17
7. ConnectionsCollector test verifies own PID is excluded from count
8. CacheCollector test verifies no panic on empty pg_stat_database
9. Registry test verifies one failing collector doesn't abort others
10. All MetricPoint values have non-empty InstanceID, Metric name, and valid Timestamp

---

## Dependencies

| Dependency | Source | Status |
|------------|--------|--------|
| Collector interface | internal/collector/collector.go | ✅ Done (M0) |
| PGVersion + Detect() | internal/version/version.go | ✅ Done (M0) |
| Gate + Select() | internal/version/gate.go | ✅ Done (M0) |
| pgx v5 | go.mod | ✅ Done (M0) |
| testcontainers-go | go.mod | ✅ Done (M0) |

---

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| testcontainers-go requires Docker Desktop on Windows | Tests can't run locally | Run tests in CI (GitHub Actions has Docker); local tests use mock |
| PG 14 image may not be available in testcontainers | Can't test version gate | Pin specific image tags; fallback to PG 15 minimum |
| pg_stat_statements not installed in test PG | Q18–Q19 tests fail | Create extension in test setup SQL |
