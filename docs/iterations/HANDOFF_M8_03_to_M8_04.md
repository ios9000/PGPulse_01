# PGPulse — Iteration Handoff: M8_03 → M8_04

---

## DO NOT RE-DISCUSS

- `DBInstanceLister` queries the `instances` table — `configInstanceLister` is deleted, do not reintroduce it
- `NewDetector` takes 5 parameters — the 5th (`persist PersistenceStore`) may be nil to disable persistence
- Session kill is admin-only — no sub-roles, no viewer access
- Forecast horizon is M8_04 scope — was explicitly deferred from M8_02 and M8_03
- `.claude/worktrees/` is in `.gitignore` — do not remove this line
- ML persistence frequency is per-Evaluate cycle (~60s) — not on a separate timer

---

## What Was Just Completed (M8_03)

### 1. DBInstanceLister
Replaces static `configInstanceLister`. Queries `instances WHERE enabled = true`
on each Bootstrap call. ML anomaly detection now covers instances added via API
after startup without requiring a restart.

### 2. ML Model Persistence
STLBaseline fitted state is serialized to `ml_baseline_snapshots` (JSONB upsert)
after every `Evaluate()` cycle. On startup, `Bootstrap()` loads persisted state
first; falls back to full TimescaleDB replay only for stale or missing snapshots.
Staleness threshold: `2 * period * collectionInterval` per metric.

### 3. Session Cancel / Terminate API
Two new endpoints, admin only:
- `POST /api/v1/instances/{id}/sessions/{pid}/cancel` → `pg_cancel_backend(pid)`
- `POST /api/v1/instances/{id}/sessions/{pid}/terminate` → `pg_terminate_backend(pid)`

Safety guards: blocks own-PID (400), blocks superuser targets (403).
Returns `{pid, action, success}` — `success: false` when PID already gone (200, not 404).
Every action logged via slog at INFO level including JWT subject.

---

## Key Interfaces (actual signatures from committed code)

```go
// internal/ml/detector.go

func NewDetector(
    cfg       DetectorConfig,
    store     collector.MetricStore,
    lister    InstanceLister,
    evaluator collector.AlertEvaluator,
    persist   PersistenceStore,          // may be nil
) *Detector

func (d *Detector) Bootstrap(ctx context.Context) error
func (d *Detector) Evaluate(ctx context.Context, points []collector.MetricPoint) ([]AnomalyResult, error)
func (d *Detector) SetAlertEvaluator(e collector.AlertEvaluator)
```

```go
// internal/ml/lister.go

type DBInstanceLister struct { /* unexported */ }

func NewDBInstanceLister(pool *pgxpool.Pool) *DBInstanceLister
func (l *DBInstanceLister) ListInstances(ctx context.Context) ([]string, error)
```

```go
// internal/ml/persistence.go

type PersistenceStore interface {
    Save(ctx context.Context, snap BaselineSnapshot) error
    Load(ctx context.Context, instanceID, metricKey string) (*BaselineSnapshot, error)
    LoadAll(ctx context.Context) ([]BaselineSnapshot, error)
}

type DBPersistenceStore struct { /* unexported */ }

func NewDBPersistenceStore(pool *pgxpool.Pool) *DBPersistenceStore
```

```go
// internal/ml/baseline.go

type BaselineSnapshot struct {
    InstanceID string
    MetricKey  string
    Period     int
    WindowSize int
    EWMA       float64
    EWMAAlpha  float64
    Seasonal   []float64
    SeasonN    []int
    Residuals  []float64  // live entries only, chronological order
    ResCount   int
    TotalSeen  int
    SumAll     float64
    UpdatedAt  time.Time
}

func (b *STLBaseline) Snapshot(instanceID string) BaselineSnapshot
func LoadFromSnapshot(s BaselineSnapshot) *STLBaseline
```

```go
// internal/api/session_actions.go

// Routes (admin group):
// POST /api/v1/instances/{id}/sessions/{pid}/cancel
// POST /api/v1/instances/{id}/sessions/{pid}/terminate

type sessionActionResponse struct {
    PID     int    `json:"pid"`
    Action  string `json:"action"`
    Success bool   `json:"success"`
}
```

---

## Files Added/Modified in M8_03

```
internal/ml/
  lister.go           ← NEW: DBInstanceLister
  persistence.go      ← NEW: PersistenceStore interface + DBPersistenceStore
  baseline.go         ← MODIFIED: BaselineSnapshot, Snapshot(), LoadFromSnapshot()
  detector.go         ← MODIFIED: 5th persist param, two-phase Bootstrap, Evaluate persists

internal/api/
  session_actions.go  ← NEW: cancel + terminate handlers
  server.go           ← MODIFIED: session routes registered

internal/storage/migrations/
  010_ml_baseline_snapshots.sql  ← NEW

internal/config/
  config.go           ← MODIFIED: MLPersistenceConfig added

cmd/pgpulse-server/
  main.go             ← MODIFIED: DBInstanceLister + DBPersistenceStore wired

.gitignore            ← MODIFIED: .claude/worktrees/ added
```

---

## Known Issues

| Issue | Status |
|-------|--------|
| Pre-existing lint warning in `web/src/pages/Administration.tsx` | Pre-existing from before M8_03, not introduced here |
| Session kill has no UI | API complete, frontend deferred |
| Settings diff + plan capture have no UI | API complete from M8_02, frontend deferred |
| ML forecast not yet implemented | Explicitly deferred — M8_04 scope |
| `pg_signal_backend` role required on monitoring user | Must be granted manually on monitored instances; not yet documented in deployment guide |

---

## Next Task: M8_04 — Forecast Horizon

### Scope (to confirm at start of M8_04 session)
- STL-based next-N point forecast using fitted trend + seasonal components
- Alert integration: fire alert when forecast crosses a configurable threshold
- API endpoint: expose predictions for UI charting
- Primary use case confirmed: **both** — alert when forecast crosses threshold AND API for UI charting

### Design questions to resolve at M8_04 session start
1. **Forecast horizon N**: how many points ahead? Fixed config value (e.g. 60 points = 1h at 1min collection), or per-metric?
2. **Confidence interval**: return point forecast only, or include upper/lower bounds (e.g. ±2σ of residuals)?
3. **Which metrics get forecast by default?** All ML-enabled metrics, or a separate forecast-specific list?
4. **Alert threshold format**: absolute value (e.g. `connections > 180`) or rate-of-change (e.g. `trend slope > X`)?

### Suggested STL forecast approach
```
forecast[t+k] = trend[t] + (trend[t] - trend[t-1]) * k   // linear trend extrapolation
              + seasonal[(t + k) mod period]               // seasonal component repeats
```
Confidence bounds: `forecast ± z * residual_stddev` where z=1.96 for 95%.

---

## Workflow Reminder

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6

# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run

# Commit
git add . && git commit -m "..." && git push
```

**Remember:** `git add .` will not pick up `.claude/worktrees/` anymore — it is gitignored. Safe to use `git add .` going forward.
