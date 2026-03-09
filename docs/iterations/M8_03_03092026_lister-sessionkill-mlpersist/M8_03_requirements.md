# M8_03 Requirements
## Instance Lister Fix + Session Kill API + ML Model Persistence

**Iteration:** M8_03
**Date:** 2026-03-09
**Milestone:** M8 — ML Phase 1
**Status:** Ready for implementation

---

## Context

M8_02 shipped plan capture, settings snapshots, and STL-based ML anomaly detection.
Three correctness gaps remain before M8 is considered complete:

1. ML Bootstrap only sees instances from the config file — instances added via API
   after startup are invisible to anomaly detection until restart
2. Session kill/terminate was implemented in M8_01 then deleted for lint (no routes
   registered) — it needs a clean reintroduction with proper wiring
3. ML model state recomputes from raw TimescaleDB history on every restart — on
   large deployments this is slow and loses continuity with pre-restart predictions

This iteration closes all three.

---

## Feature 1: Fix configInstanceLister

### Problem
`configInstanceLister` in `main.go` wraps the static config instance list.
Instances added via `POST /api/v1/instances` after startup are stored in the
`instances` DB table but never reach the ML detector's Bootstrap or Evaluate loop.

### Fix
Replace `configInstanceLister` with a `DBInstanceLister` that queries the
`instances` table directly:

```go
// internal/ml/lister.go

type DBInstanceLister struct {
    pool *pgxpool.Pool
}

func NewDBInstanceLister(pool *pgxpool.Pool) *DBInstanceLister

func (l *DBInstanceLister) ListInstances(ctx context.Context) ([]string, error) {
    // SELECT id FROM instances WHERE enabled = true
}
```

Wire `DBInstanceLister` in `main.go` replacing the current `configInstanceLister`.

### Scope
- New file: `internal/ml/lister.go`
- Modified: `cmd/pgpulse-server/main.go` (swap lister)
- New test: `internal/ml/lister_test.go` (testcontainers)

---

## Feature 2: Session Kill / Terminate API

### Goal
Allow admin users to cancel or terminate PostgreSQL backend sessions visible
in `pg_stat_activity` from the PGPulse API. This is a direct port of the
functionality that existed in PGAM as a manual DBA action.

### Permission Model
Admin only. No new sub-roles. Viewer role receives 403.

### PostgreSQL Functions Used

| Action | Function | Effect |
|--------|----------|--------|
| Cancel | `pg_cancel_backend(pid)` | Sends SIGINT to backend — cancels current query, session stays alive |
| Terminate | `pg_terminate_backend(pid)` | Sends SIGTERM to backend — kills session entirely |

Both functions return `bool` — `true` if signal sent successfully, `false` if
PID not found or already gone. Both require `pg_signal_backend` role or
superuser. PGPulse monitoring user must have `pg_signal_backend` granted.

### API Endpoints

| Method | Path | Body | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/instances/{id}/sessions/{pid}/cancel` | — | Cancel query on pid (pg_cancel_backend) |
| `POST` | `/api/v1/instances/{id}/sessions/{pid}/terminate` | — | Terminate session (pg_terminate_backend) |

Response shape:

```json
{"pid": 12345, "action": "cancel", "success": true}
```

If `success: false` — pid was not found or already gone. Return 200 (not 404) —
the session disappearing between list and kill is normal, not an error.

### Safety Constraints
- Never allow killing PGPulse's own backend: check `pid != pg_backend_pid()` before executing
- Never allow killing superuser backends: check `SELECT usesuper FROM pg_user JOIN pg_stat_activity USING (usename) WHERE pid=$1` — if true, return 403 with `{"error": "cannot terminate superuser session"}`
- pid must be a valid integer — validate and return 400 on non-integer input

### Audit
Every cancel/terminate action must be logged via `slog` at INFO level:
```
session action: instance=<id> pid=<pid> action=cancel|terminate success=true|false user=<jwt_subject>
```

### Scope
- New file: `internal/api/session_actions.go`
- New test: `internal/api/session_actions_test.go`
- Modified: `internal/api/server.go` (register routes)
- Migration: none (no schema changes)

---

## Feature 3: ML Model Persistence

### Problem
On startup, `Detector.Bootstrap()` queries TimescaleDB for up to
`max(3*period, 1000)` raw metric points per metric per instance and
replays them through `STLBaseline.Update()` sequentially. For the default
config (5 metrics × period=1440 → up to 4320 points each), this means
up to 21,600 `Update()` calls plus the TimescaleDB queries. On a monitored
fleet with many instances this grows linearly.

More importantly: the fitted baseline has accumulated context from weeks of
observations. A restart discards all of it and starts fresh, meaning anomaly
detection is blind for `Period*2` observations (roughly 2 days at 1-minute
collection interval) after every restart.

### Solution
Serialize the fitted `STLBaseline` state to the PGPulse storage DB after each
`Evaluate()` cycle. On startup, load serialized state first; fall back to
full replay only if no persisted state exists or state is stale.

### What Gets Persisted

The minimal state needed to resume scoring without replay:

```go
type BaselineSnapshot struct {
    InstanceID  string
    MetricKey   string
    Period      int
    WindowSize  int
    EWMA        float64
    EWMAAlpha   float64
    Seasonal    []float64  // length = Period
    SeasonN     []int      // length = Period
    Residuals   []float64  // ring buffer, up to windowSize entries
    ResHead     int
    ResCount    int
    TotalSeen   int
    SumAll      float64
    UpdatedAt   time.Time
}
```

### Storage Schema

```sql
CREATE TABLE ml_baseline_snapshots (
    id           BIGSERIAL    PRIMARY KEY,
    instance_id  TEXT         NOT NULL,
    metric_key   TEXT         NOT NULL,
    period       INT          NOT NULL,
    state        JSONB        NOT NULL,  -- serialized BaselineSnapshot
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (instance_id, metric_key)
);

CREATE INDEX idx_ml_baseline_instance ON ml_baseline_snapshots(instance_id);
```

Upsert on `(instance_id, metric_key)` — one row per metric per instance, always
the latest state.

### Persistence Frequency
After every `Evaluate()` call that processes at least one point for a given
baseline, persist that baseline's state. `Evaluate()` runs on each collection
cycle (default 60s), so persistence frequency matches collection frequency.
This is acceptable — the state JSONB is small (~50KB worst case for period=1440).

### Bootstrap Sequence (modified)

```
1. Query ml_baseline_snapshots for all (instance_id, metric_key) rows
2. For each row:
   a. Deserialize BaselineSnapshot → STLBaseline via LoadFromSnapshot()
   b. Check staleness: if updated_at < now() - 2*Period*collectionInterval:
      log WARNING, fall back to full replay for this metric
   c. Otherwise: baseline is ready immediately, no replay needed
3. For metrics with no persisted state: full replay from TimescaleDB (existing behavior)
4. Log: how many baselines loaded from snapshot vs replayed
```

Staleness threshold is 2× the period in collection intervals. For period=1440
at 60s intervals that's 48 hours — if PGPulse was down for more than 2 days,
replay is safer than continuing from stale state.

### New Methods on STLBaseline

```go
func (b *STLBaseline) Snapshot() BaselineSnapshot
func LoadFromSnapshot(s BaselineSnapshot) *STLBaseline
```

### Scope
- New file: `internal/ml/persistence.go` — `BaselineSnapshot`, `Snapshot()`, `LoadFromSnapshot()`, `PersistenceStore` interface, `DBPersistenceStore`
- New file: `migrations/010_ml_baseline_snapshots.sql`
- Modified: `internal/ml/detector.go` — Bootstrap loads from DB first; Evaluate persists after each cycle
- Modified: `internal/ml/baseline.go` — add `Snapshot()` and `LoadFromSnapshot()`
- New test: `internal/ml/persistence_test.go`

---

## Files to Create / Modify

### New Files

| File | Owner | Description |
|------|-------|-------------|
| `internal/ml/lister.go` | Collector Agent | `DBInstanceLister` querying `instances` table |
| `internal/ml/lister_test.go` | QA Agent | testcontainers test for `ListInstances` |
| `internal/ml/persistence.go` | Collector Agent | `BaselineSnapshot`, serialize/deserialize, `DBPersistenceStore` |
| `internal/ml/persistence_test.go` | QA Agent | round-trip serialization tests + store tests |
| `internal/api/session_actions.go` | API Agent | cancel/terminate handlers |
| `internal/api/session_actions_test.go` | QA Agent | auth, safety guards, success/notfound cases |
| `migrations/010_ml_baseline_snapshots.sql` | API Agent | `ml_baseline_snapshots` table |

### Modified Files

| File | Change |
|------|--------|
| `internal/ml/baseline.go` | Add `Snapshot()` and `LoadFromSnapshot()` methods |
| `internal/ml/detector.go` | Bootstrap: load from DB first, fall back to replay; Evaluate: persist after cycle |
| `internal/api/server.go` | Register session cancel/terminate routes |
| `cmd/pgpulse-server/main.go` | Swap `configInstanceLister` → `DBInstanceLister`; wire `DBPersistenceStore` into Detector |
| `configs/pgpulse.example.yml` | Add `ml.persistence` section |

---

## Testing Requirements

- `TestDBInstanceLister_Empty`: no instances → returns empty slice, no error
- `TestDBInstanceLister_ReturnsEnabled`: two instances (one enabled, one disabled) → returns only enabled ID
- `TestBaselineSnapshot_RoundTrip`: `Snapshot()` → `LoadFromSnapshot()` → verify all fields match
- `TestBaselineSnapshot_ScoringContinuity`: fit baseline, snapshot, load, score same value → same Z-score
- `TestDBPersistenceStore_Upsert`: persist twice → one row in DB, updated_at refreshed
- `TestDetector_Bootstrap_LoadsFromSnapshot`: snapshot in DB → Bootstrap does not query TimescaleDB for that metric
- `TestDetector_Bootstrap_FallbackOnStale`: stale snapshot (updated_at > staleness threshold) → falls back to replay
- `TestSessionCancel_RequiresAdmin`: viewer JWT → 403
- `TestSessionCancel_Success`: mock `pg_cancel_backend` returns true → 200 `{success: true}`
- `TestSessionCancel_PidGone`: `pg_cancel_backend` returns false → 200 `{success: false}`
- `TestSessionTerminate_SuperuserBlocked`: target pid is superuser → 403
- `TestSessionTerminate_OwnPidBlocked`: target pid = pg_backend_pid() → 400
- golangci-lint: 0 issues
- `go test -race ./internal/ml/... ./internal/api/...` — all pass

---

## Non-Goals for M8_03

- Forecast horizon (predict next N points) — M8_04
- Bulk session kill (kill all idle > N seconds) — future iteration
- Session kill UI — frontend work deferred
- ML persistence compression (JSONB is fine for M8_03 scale)
