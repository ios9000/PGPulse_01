# PGPulse — Iteration Handoff: M8_01 → M8_02
**Date:** 2026-03-08
**From:** M8_01 — P1 Features (Session Kill, Query Plans, Settings Diff)
**To:** M8_02 — Auto-Capture Plans + Temporal Settings Diff (or ML Baseline)

---

## DO NOT RE-DISCUSS

- Session kill uses `PermInstanceManagement` — maps to dba + super_admin. Locked.
- EXPLAIN query body is intentionally not parameterized — documented with comment. Do not change.
- Settings diff is stateless in M8_01 — no snapshots table yet. M8_02 adds temporal diff.
- Migration sequence is now at 007 — next migration must be 008.
- DBCollector interface is separate from Collector — parallel dispatch path, never merge them.
- DBRunner owns the pool map — never pass *pgxpool.Pool directly to a DBCollector.
- Collector interface signature is frozen: `Collect(ctx, *pgx.Conn, InstanceContext)` — no new parameters.
- TimescaleDB is conditional — migrations fall back gracefully when extension absent.
- No COPY TO PROGRAM ever — OS metrics via Go agent only.
- golangci-lint v2 config format — do not downgrade or change linter config.
- RBAC: 4 roles (super_admin, roles_admin, dba, app_admin) — locked, no changes.

---

## What Exists Now

### New interfaces — none added in M8_01. All interfaces from M7_01 still current:

```go
// internal/collector/collector.go (unchanged)
type Queryer interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type DBCollector interface {
    Name() string
    Interval() time.Duration
    CollectDB(ctx context.Context, q Queryer, dbName string, ic InstanceContext) ([]MetricPoint, error)
}
```

### New API endpoints added in M8_01

```
POST /api/v1/instances/:id/sessions/:pid/cancel     — dba + super_admin only
POST /api/v1/instances/:id/sessions/:pid/terminate  — dba + super_admin only
POST /api/v1/instances/:id/explain                  — dba + super_admin only
GET  /api/v1/settings/compare                       — all authenticated users
```

### New helper in internal/api/plans.go

```go
// SubstituteDatabase returns a copy of dsn with the database name replaced.
// Handles both key=value and postgres:// URL DSN formats.
func SubstituteDatabase(dsn, dbName string) (string, error)
```

### New table in migrations/007_session_audit_log.sql

```sql
CREATE TABLE IF NOT EXISTS session_audit_log (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     TEXT        NOT NULL,
    operator_user   TEXT        NOT NULL,
    target_pid      INT         NOT NULL,
    operation       TEXT        NOT NULL CHECK (operation IN ('cancel','terminate')),
    result          TEXT        NOT NULL CHECK (result IN ('ok','error')),
    error_message   TEXT,
    executed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### New files added in M8_01

| File | Description |
|------|-------------|
| `internal/storage/migrations/007_session_audit_log.sql` | Audit log table |
| `internal/api/sessions.go` | Cancel + terminate handlers + audit log writer |
| `internal/api/plans.go` | On-demand EXPLAIN handler + DSN substitution helper |
| `internal/api/settings_diff.go` | Stateless cross-instance pg_settings comparison |
| `web/src/components/SessionKillButtons.tsx` | Kill buttons with confirmation modals |
| `web/src/pages/QueryPlanViewer.tsx` | EXPLAIN UI: recursive plan tree + raw JSON toggle |
| `web/src/pages/SettingsDiff.tsx` | Settings diff: accordion by category + CSV export |

### Modified files in M8_01

| File | Change |
|------|--------|
| `internal/api/server.go` | 4 new routes (both auth-enabled and auth-disabled branches) |
| `web/src/types/models.ts` | 6 new interfaces appended |
| `web/src/pages/ServerDetail.tsx` | "Explain Query" link added |
| `web/src/App.tsx` | 2 new routes (/instances/:id/explain, /settings/diff) |
| `web/src/components/layout/Sidebar.tsx` | "Settings Diff" nav item added |

---

## What Was Just Completed

M8_01 shipped three P1 features making PGPulse interactive rather than read-only:

**Session Kill:** DBA and super_admin can cancel or terminate PostgreSQL backend
sessions from the activity table in ServerDetail. Both actions write to
session_audit_log (migration 007). Confirmation modals differentiate cancel
(safe, keeps connection) from terminate (destructive, closes connection).

**On-Demand Query Plans:** EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) via API.
Opens a one-shot pgx.Conn per request (not pool) with statement_timeout=30s and
application_name=pgpulse_explain. SubstituteDatabase helper handles DSN dbname
substitution for both key=value and postgres:// formats. Frontend renders a
recursive plan tree with cost/row discrepancy highlighting and raw JSON toggle.

**Cross-Instance Settings Diff:** Fetches pg_settings from two instances
concurrently (errgroup, 10s timeout each). Returns changed / only_in_a / only_in_b
/ matching_count. Noise-filtered by default (server_version, data_directory, lc_*
etc). Frontend accordion grouped by pg_settings.category with CSV export.

Build: go build ✅ · go vet ✅ · go test 14 packages ✅ · golangci-lint 0 issues ✅
npm run build ✅ · npx tsc --noEmit 0 errors ✅
Note: 1 pre-existing lint error in Administration.tsx (not introduced by M8_01).

---

## PGAM Query Porting Status (unchanged from M7_01)

| Source | Queries | Status |
|--------|---------|--------|
| analiz2.php Q1–Q19 | Instance metrics | ✅ M1 |
| analiz2.php Q20–Q41 | Replication | ✅ M2 (Q41 deferred) |
| analiz2.php Q42–Q47 | Progress | ✅ M2 |
| analiz2.php Q48–Q52 | Statements | ✅ M3 |
| analiz2.php Q53–Q58 | Locks/wait events | ✅ M4 |
| OS/cluster Q4–Q8, Q22–Q35 | COPY TO PROGRAM → Go | ✅ M6 |
| analiz_db.php Q2–Q18 | Per-DB analysis | ✅ M7 |
| Q41 | Logical replication | 🔲 Deferred |
| Q36, Q39 | PG <10 xlog functions | ⏭ Skipped (below PG 14 minimum) |
| Q52 | Normalized totals | ⏭ Covered by Q50+Q51 |

**~69/76 ported.**

---

## Known Issues

- Pre-existing lint error in `Administration.tsx` — not introduced by M8_01.
  Should be fixed in a cleanup commit before or during M8_02.
- Logical replication (Q41) deferred — DBCollector interface is the correct hook.
- Large object orphan detection (lo.php) not implemented — low priority, no plan.

---

## Build & Test Commands (unchanged)

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

**NEVER use `go test ./...`** — scans web/node_modules/ and fails.

---

## Next Task: M8_02

Two options — discuss and decide in Claude.ai before designing:

**Option A: M8_02 = Auto-Capture Plans + Temporal Settings Diff**
- Auto-capture: new `PlansCollector` implementing `DBCollector`. Background worker
  polls pg_stat_statements for queries exceeding a configurable mean_time threshold.
  Runs `EXPLAIN (FORMAT JSON)` (no ANALYZE — background, no execution cost).
  New `query_plans` table (migration 008): instance_id, database, query_fingerprint,
  plan_json, captured_at, mean_time_ms, calls.
- Temporal settings diff: snapshot pg_settings on a schedule (configurable, default daily).
  New `settings_snapshots` table (migration 009). GET /settings/history/:instance_id
  returns list of snapshots. GET /settings/diff?instance=X&snapshot_a=T1&snapshot_b=T2
  returns diff between two snapshots.
- Frontend: plan history list per instance, snapshot diff UI.

**Option B: M8_02 = ML Baseline / Anomaly Detection (original M8 plan)**
- Baseline computation: rolling mean + stddev per metric per instance (7-day window).
- Z-score anomaly detection: flag values > N stddev from baseline.
- STL decomposition: seasonal/trend/residual for metrics with daily cycles.
- Anomaly alert integration: emit alert when anomaly detected (existing AlertEvaluator).
- Disk forecast: linear regression on disk usage → days until full.
- New package: `internal/ml/` — gonum-based.

**Open question to resolve:** Which option is higher priority for your use case?
Auto-capture plans is operationally very useful but heavy backend work.
ML baseline closes the original M8 vision. Recommend discussing in next Claude.ai session.
