# M8_03 Design
## Instance Lister Fix + Session Kill API + ML Model Persistence

**Iteration:** M8_03
**Date:** 2026-03-09
**Depends on:** M8_02 (STLBaseline, Detector, MetricAlertAdapter)

---

## 1. DBInstanceLister

Simple query wrapper. No caching — called once at Bootstrap and the overhead
of a single `SELECT id FROM instances` is negligible.

```go
// internal/ml/lister.go

package ml

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
)

type DBInstanceLister struct {
    pool *pgxpool.Pool
}

func NewDBInstanceLister(pool *pgxpool.Pool) *DBInstanceLister {
    return &DBInstanceLister{pool: pool}
}

func (l *DBInstanceLister) ListInstances(ctx context.Context) ([]string, error) {
    rows, err := l.pool.Query(ctx,
        `SELECT id FROM instances WHERE enabled = true ORDER BY id`)
    if err != nil { return nil, err }
    defer rows.Close()

    var ids []string
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil { return nil, err }
        ids = append(ids, id)
    }
    return ids, rows.Err()
}
```

**main.go change:** one line swap:
```go
// Before:
lister := newConfigInstanceLister(cfg.Instances)

// After:
lister := ml.NewDBInstanceLister(storagePool)
```

Delete `configInstanceLister` and its `newConfigInstanceLister` constructor
from `main.go` entirely.

---

## 2. Session Kill / Terminate API

### Handler File Structure

```go
// internal/api/session_actions.go

package api

import (
    "context"
    "log/slog"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
)

// POST /api/v1/instances/{id}/sessions/{pid}/cancel
func (s *APIServer) handleSessionCancel(w http.ResponseWriter, r *http.Request) {
    s.handleSessionAction(w, r, "cancel")
}

// POST /api/v1/instances/{id}/sessions/{pid}/terminate
func (s *APIServer) handleSessionTerminate(w http.ResponseWriter, r *http.Request) {
    s.handleSessionAction(w, r, "terminate")
}

func (s *APIServer) handleSessionAction(w http.ResponseWriter, r *http.Request, action string) {
    instanceID := chi.URLParam(r, "id")
    pidStr     := chi.URLParam(r, "pid")

    // 1. Validate pid
    pid, err := strconv.Atoi(pidStr)
    if err != nil || pid <= 0 {
        writeError(w, http.StatusBadRequest, "invalid pid")
        return
    }

    // 2. Get connection pool for this instance
    pool, err := s.orchestrator.PoolForInstance(instanceID)
    if err != nil {
        writeError(w, http.StatusNotFound, "instance not found")
        return
    }

    ctx := r.Context()

    // 3. Guard: never kill own backend
    var ownPID int
    _ = pool.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&ownPID)
    if pid == ownPID {
        writeError(w, http.StatusBadRequest, "cannot target PGPulse's own backend")
        return
    }

    // 4. Guard: never kill superuser session
    var isSuper bool
    err = pool.QueryRow(ctx, `
        SELECT COALESCE(u.usesuper, false)
        FROM pg_stat_activity a
        LEFT JOIN pg_user u ON u.usename = a.usename
        WHERE a.pid = $1
    `, pid).Scan(&isSuper)
    if err != nil {
        // pid not found — treat as already gone
        writeJSON(w, http.StatusOK, sessionActionResponse{PID: pid, Action: action, Success: false})
        return
    }
    if isSuper {
        writeError(w, http.StatusForbidden, "cannot target superuser session")
        return
    }

    // 5. Execute
    var success bool
    var fn string
    switch action {
    case "cancel":
        fn = "pg_cancel_backend($1)"
    default:
        fn = "pg_terminate_backend($1)"
    }
    _ = pool.QueryRow(ctx, "SELECT "+fn, pid).Scan(&success)

    // 6. Audit log
    subject := jwtSubjectFromContext(r.Context()) // helper reading JWT claim
    slog.Info("session action",
        "instance", instanceID,
        "pid", pid,
        "action", action,
        "success", success,
        "user", subject)

    writeJSON(w, http.StatusOK, sessionActionResponse{
        PID: pid, Action: action, Success: success,
    })
}

type sessionActionResponse struct {
    PID     int    `json:"pid"`
    Action  string `json:"action"`
    Success bool   `json:"success"`
}
```

### Route Registration (server.go)

```go
// In the admin-only route group (existing pattern):
r.Post("/instances/{id}/sessions/{pid}/cancel",    s.handleSessionCancel)
r.Post("/instances/{id}/sessions/{pid}/terminate", s.handleSessionTerminate)
```

Both routes must sit behind the existing admin-only middleware. No new middleware
needed.

### jwtSubjectFromContext helper

If this doesn't already exist in the auth package, add it:

```go
// internal/auth/context.go (or existing file)

func SubjectFromContext(ctx context.Context) string {
    claims, ok := ctx.Value(claimsKey{}).(*Claims)
    if !ok || claims == nil { return "unknown" }
    return claims.Subject
}
```

Call it in `session_actions.go` as `auth.SubjectFromContext(r.Context())`.

---

## 3. ML Model Persistence

### BaselineSnapshot and STLBaseline Methods

```go
// internal/ml/baseline.go — add to existing file

type BaselineSnapshot struct {
    InstanceID string    `json:"instance_id"`
    MetricKey  string    `json:"metric_key"`
    Period     int       `json:"period"`
    WindowSize int       `json:"window_size"`
    EWMA       float64   `json:"ewma"`
    EWMAAlpha  float64   `json:"ewma_alpha"`
    Seasonal   []float64 `json:"seasonal"`
    SeasonN    []int     `json:"season_n"`
    Residuals  []float64 `json:"residuals"`  // only ResCount entries, not full ring
    ResCount   int       `json:"res_count"`
    TotalSeen  int       `json:"total_seen"`
    SumAll     float64   `json:"sum_all"`
    UpdatedAt  time.Time `json:"updated_at"`
}

func (b *STLBaseline) Snapshot(instanceID string) BaselineSnapshot {
    // Export only the live residuals (not the full ring buffer with stale slots)
    liveResiduals := make([]float64, b.resCount)
    for i := 0; i < b.resCount; i++ {
        idx := (b.resHead - b.resCount + i + b.windowSize) % b.windowSize
        liveResiduals[i] = b.residuals[idx]
    }
    return BaselineSnapshot{
        InstanceID: instanceID,
        MetricKey:  b.MetricKey,
        Period:     b.Period,
        WindowSize: b.windowSize,
        EWMA:       b.ewma,
        EWMAAlpha:  b.ewmaAlpha,
        Seasonal:   append([]float64{}, b.seasonal...),
        SeasonN:    append([]int{}, b.seasonN...),
        Residuals:  liveResiduals,
        ResCount:   b.resCount,
        TotalSeen:  b.totalSeen,
        SumAll:     b.sumAll,
        UpdatedAt:  time.Now(),
    }
}

func LoadFromSnapshot(s BaselineSnapshot) *STLBaseline {
    b := &STLBaseline{
        MetricKey:  s.MetricKey,
        Period:     s.Period,
        windowSize: s.WindowSize,
        ring:       make([]float64, s.WindowSize),
        residuals:  make([]float64, s.WindowSize),
        ewma:       s.EWMA,
        ewmaAlpha:  s.EWMAAlpha,
        seasonal:   append([]float64{}, s.Seasonal...),
        seasonN:    append([]int{}, s.SeasonN...),
        resCount:   s.ResCount,
        totalSeen:  s.TotalSeen,
        sumAll:     s.SumAll,
    }
    // Restore residuals into ring buffer starting at position 0
    copy(b.residuals, s.Residuals)
    b.resHead = s.ResCount % s.WindowSize
    return b
}
```

### PersistenceStore Interface and Implementation

```go
// internal/ml/persistence.go

package ml

import (
    "context"
    "encoding/json"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type PersistenceStore interface {
    Save(ctx context.Context, snap BaselineSnapshot) error
    Load(ctx context.Context, instanceID, metricKey string) (*BaselineSnapshot, error)
    LoadAll(ctx context.Context) ([]BaselineSnapshot, error)
}

type DBPersistenceStore struct {
    pool *pgxpool.Pool
}

func NewDBPersistenceStore(pool *pgxpool.Pool) *DBPersistenceStore {
    return &DBPersistenceStore{pool: pool}
}

func (s *DBPersistenceStore) Save(ctx context.Context, snap BaselineSnapshot) error {
    data, err := json.Marshal(snap)
    if err != nil { return err }
    _, err = s.pool.Exec(ctx, `
        INSERT INTO ml_baseline_snapshots (instance_id, metric_key, period, state, updated_at)
        VALUES ($1, $2, $3, $4, now())
        ON CONFLICT (instance_id, metric_key)
        DO UPDATE SET state = EXCLUDED.state, updated_at = now()
    `, snap.InstanceID, snap.MetricKey, snap.Period, data)
    return err
}

func (s *DBPersistenceStore) Load(ctx context.Context, instanceID, metricKey string) (*BaselineSnapshot, error) {
    var data []byte
    err := s.pool.QueryRow(ctx, `
        SELECT state FROM ml_baseline_snapshots
        WHERE instance_id = $1 AND metric_key = $2
    `, instanceID, metricKey).Scan(&data)
    if err != nil { return nil, err }
    var snap BaselineSnapshot
    if err := json.Unmarshal(data, &snap); err != nil { return nil, err }
    return &snap, nil
}

func (s *DBPersistenceStore) LoadAll(ctx context.Context) ([]BaselineSnapshot, error) {
    rows, err := s.pool.Query(ctx,
        `SELECT state FROM ml_baseline_snapshots ORDER BY instance_id, metric_key`)
    if err != nil { return nil, err }
    defer rows.Close()

    var snaps []BaselineSnapshot
    for rows.Next() {
        var data []byte
        if err := rows.Scan(&data); err != nil { return nil, err }
        var snap BaselineSnapshot
        if err := json.Unmarshal(data, &snap); err != nil { continue }
        snaps = append(snaps, snap)
    }
    return snaps, rows.Err()
}
```

### Modified Detector.Bootstrap

```go
func (d *Detector) Bootstrap(ctx context.Context) error {
    // Phase 1: Load persisted snapshots
    persisted := map[string]bool{} // "instanceID:metricKey" → loaded successfully

    if d.persist != nil {
        snaps, err := d.persist.LoadAll(ctx)
        if err != nil {
            slog.Warn("ML persistence load failed, will replay from TimescaleDB", "err", err)
        } else {
            staleness := time.Duration(2*d.config.Metrics[0].Period) * d.config.CollectionInterval
            // Use per-metric staleness
            for _, snap := range snaps {
                age := time.Since(snap.UpdatedAt)
                mc := d.metricConfig(snap.MetricKey) // find config for this metric
                if mc == nil { continue }
                metricStaleness := time.Duration(2*mc.Period) * d.config.CollectionInterval
                if age > metricStaleness {
                    slog.Warn("ML baseline snapshot stale, will replay",
                        "instance", snap.InstanceID,
                        "metric", snap.MetricKey,
                        "age", age.Round(time.Minute))
                    continue
                }
                key := snap.InstanceID + ":" + snap.MetricKey
                b := LoadFromSnapshot(snap)
                d.mu.Lock()
                d.baselines[key] = b
                d.mu.Unlock()
                persisted[key] = true
                slog.Info("ML baseline loaded from snapshot",
                    "instance", snap.InstanceID,
                    "metric", snap.MetricKey,
                    "age", age.Round(time.Minute))
            }
        }
    }

    // Phase 2: Replay from TimescaleDB for metrics not loaded from snapshot
    instances, err := d.lister.ListInstances(ctx)
    if err != nil { return fmt.Errorf("listing instances: %w", err) }

    for _, instanceID := range instances {
        for _, mc := range d.config.Metrics {
            if !mc.Enabled { continue }
            key := instanceID + ":" + mc.Key
            if persisted[key] { continue } // already loaded

            // existing replay logic (unchanged from M8_02)
            ...
        }
    }
    return nil
}
```

### Modified Detector.Evaluate — persist after each cycle

```go
func (d *Detector) Evaluate(ctx context.Context, points []collector.MetricPoint) ([]AnomalyResult, error) {
    // ... existing scoring + alert dispatch logic unchanged ...

    // After processing all points: persist updated baselines
    if d.persist != nil {
        d.mu.RLock()
        for key, b := range d.baselines {
            // key format: "instanceID:metricKey"
            parts := strings.SplitN(key, ":", 2)
            if len(parts) != 2 { continue }
            snap := b.Snapshot(parts[0])
            if err := d.persist.Save(ctx, snap); err != nil {
                slog.Warn("ML baseline persist failed", "key", key, "err", err)
            }
        }
        d.mu.RUnlock()
    }

    return results, nil
}
```

### Detector struct — add persist field

```go
type Detector struct {
    config    DetectorConfig
    baselines map[string]*STLBaseline
    mu        sync.RWMutex
    store     collector.MetricStore
    lister    InstanceLister           // was added in M8_02
    evaluator collector.AlertEvaluator
    persist   PersistenceStore         // NEW — nil = no persistence
}

func NewDetector(cfg DetectorConfig, store collector.MetricStore,
    lister InstanceLister, evaluator collector.AlertEvaluator,
    persist PersistenceStore) *Detector   // persist may be nil
```

`NewDetector` signature gains `persist PersistenceStore` as final param.
Passing `nil` disables persistence (existing tests stay valid, no changes to
test constructors needed — just pass `nil`).

### Configuration Addition

```yaml
# configs/pgpulse.example.yml — under ml:
ml:
  persistence:
    enabled: true
    staleness_multiplier: 2  # snapshot older than 2*period*interval → replay
```

### Migration

```sql
-- migrations/010_ml_baseline_snapshots.sql

CREATE TABLE IF NOT EXISTS ml_baseline_snapshots (
    id           BIGSERIAL    PRIMARY KEY,
    instance_id  TEXT         NOT NULL,
    metric_key   TEXT         NOT NULL,
    period       INT          NOT NULL,
    state        JSONB        NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (instance_id, metric_key)
);

CREATE INDEX IF NOT EXISTS idx_ml_baseline_snapshots_instance
    ON ml_baseline_snapshots(instance_id);
```

---

## 4. main.go Wiring Summary

```go
// 1. Replace configInstanceLister
lister := ml.NewDBInstanceLister(storagePool)

// 2. Initialize persistence store
var persistStore ml.PersistenceStore
if cfg.ML.Persistence.Enabled {
    persistStore = ml.NewDBPersistenceStore(storagePool)
}

// 3. NewDetector gains persist param
mlDetector = ml.NewDetector(cfg.ML, metricStore, lister, noOpEvaluator, persistStore)

// 4. Bootstrap (unchanged — now loads from DB first)
bootstrapCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
if err := mlDetector.Bootstrap(bootstrapCtx); err != nil {
    slog.Warn("ML bootstrap incomplete", "err", err)
}
cancel()

// 5. Session kill routes — registered in server.go, no main.go change needed
```

---

## 5. Open Questions (Resolved)

| Question | Decision |
|----------|----------|
| Session kill permission | Admin only — no new sub-role |
| Persist every Evaluate cycle or on timer? | Every Evaluate (60s) — state size is small, continuity matters more than write overhead |
| Staleness threshold | 2 × period × collectionInterval — configurable via multiplier |
| Persist field nil-safe? | Yes — passing nil disables persistence, all code guards `if d.persist != nil` |
| Forecast this iteration? | No — M8_04 |
