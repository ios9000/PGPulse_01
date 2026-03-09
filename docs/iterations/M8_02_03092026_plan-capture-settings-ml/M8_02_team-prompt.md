# M8_02 Team Prompt
## Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection

**Read before spawning agents:**
- `.claude/CLAUDE.md` — current iteration, module ownership, shared interfaces
- `docs/iterations/M8_02_03092026_plan-capture-settings-ml/M8_02_design.md` — full design
- `docs/iterations/M8_02_03092026_plan-capture-settings-ml/M8_02_requirements.md` — requirements

---

## Team Lead Instructions

You are Team Lead for M8_02. Read CLAUDE.md and the design doc before decomposing.

Create a team of 3 specialists. Spawn them in parallel once shared interfaces
are confirmed. Dependencies:

```
Collector Agent ──► builds plans/capture.go, settings/snapshot.go, ml/ package
API Agent       ──► builds plans/store.go, settings/diff.go, api/plans.go, api/settings.go, migrations
QA Agent        ──► writes tests for all three features as code lands

Merge order:
  1. Migrations (API Agent) — schema must exist before any store code is tested
  2. Collector Agent — capture + ML (pure logic, no API deps)
  3. API Agent — store + endpoints (depend on schema)
  4. QA Agent — tests (depend on both)
  5. main.go wiring (Team Lead coordinates)
```

Merge only after QA Agent confirms all tests pass and `go test ./internal/... ./cmd/...` is clean.

---

## Collector Agent

**Your scope:** `internal/plans/capture.go`, `internal/settings/snapshot.go`,
`internal/ml/baseline.go`, `internal/ml/detector.go`, `internal/ml/config.go`

**Do NOT touch:** `internal/api/`, `internal/auth/`, `migrations/`

---

### Task 1: Plan Capture Collector

Create `internal/plans/capture.go`:

```go
package plans

import (
    "context"
    "crypto/md5"
    "crypto/sha256"
    "fmt"
    "strings"
    "sync"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/ios9000/PGPulse_01/internal/collector"
)

type TriggerType string
const (
    TriggerDuration  TriggerType = "duration_threshold"
    TriggerManual    TriggerType = "manual"
    TriggerScheduled TriggerType = "scheduled_topn"
    TriggerHashDiff  TriggerType = "hash_diff_signal"
)

type CaptureConfig struct {
    Enabled                bool
    DurationThresholdMs    int64
    DedupWindowSeconds     int
    ScheduledTopNCount     int
    ScheduledTopNInterval  time.Duration
    MaxPlanBytes           int
    RetentionDays          int
}

type PlanCapture struct {
    InstanceID       string
    DatabaseName     string
    QueryFingerprint string // md5 of normalized query
    PlanHash         string // sha256 of plan bytes
    PlanText         string
    TriggerType      TriggerType
    DurationMs       int64
    QueryText        string
    Truncated        bool
    Metadata         map[string]any
    CapturedAt       time.Time
}

type CaptureStore interface {
    SavePlan(ctx context.Context, p PlanCapture) error
    LatestPlanHash(ctx context.Context, instanceID, fingerprint string) (string, error)
}

type dedupCache struct {
    mu      sync.Mutex
    entries map[string]time.Time // key: "instanceID:fingerprint"
    window  time.Duration
}

func (c *dedupCache) seen(instanceID, fingerprint string) bool {
    c.mu.Lock()
    defer c.mu.Unlock()
    key := instanceID + ":" + fingerprint
    if t, ok := c.entries[key]; ok && time.Since(t) < c.window {
        return true
    }
    c.entries[key] = time.Now()
    return false
}

type Collector struct {
    config        CaptureConfig
    store         CaptureStore
    dedup         *dedupCache
    lastTopN      map[string]time.Time // instance_id → last topN run
    lastTopNMu    sync.Mutex
}

func NewCollector(cfg CaptureConfig, store CaptureStore) *Collector {
    return &Collector{
        config: cfg,
        store:  store,
        dedup: &dedupCache{
            entries: make(map[string]time.Time),
            window:  time.Duration(cfg.DedupWindowSeconds) * time.Second,
        },
        lastTopN: make(map[string]time.Time),
    }
}

func (c *Collector) Name() string { return "plan_capture" }

func (c *Collector) Collect(ctx context.Context, pool *pgxpool.Pool, ic collector.InstanceContext) error {
    if !c.config.Enabled { return nil }

    if err := c.collectDurationThreshold(ctx, pool, ic.InstanceID); err != nil {
        return fmt.Errorf("duration threshold capture: %w", err)
    }

    c.lastTopNMu.Lock()
    shouldRunTopN := time.Since(c.lastTopN[ic.InstanceID]) >= c.config.ScheduledTopNInterval
    c.lastTopNMu.Unlock()

    if shouldRunTopN {
        if err := c.collectScheduledTopN(ctx, pool, ic.InstanceID); err != nil {
            return fmt.Errorf("scheduled topN capture: %w", err)
        }
        c.lastTopNMu.Lock()
        c.lastTopN[ic.InstanceID] = time.Now()
        c.lastTopNMu.Unlock()
    }

    return nil
}
```

Implement `collectDurationThreshold`:

```go
func (c *Collector) collectDurationThreshold(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
    rows, err := pool.Query(ctx, `
        SELECT datname, query,
               EXTRACT(EPOCH FROM (now() - query_start)) * 1000 AS duration_ms
        FROM pg_stat_activity
        WHERE state = 'active'
          AND query NOT LIKE '%pg_stat_activity%'
          AND query NOT LIKE '%pgpulse%'
          AND now() - query_start > ($1::bigint * interval '1 millisecond')
          AND pid != pg_backend_pid()
    `, c.config.DurationThresholdMs)
    if err != nil { return err }
    defer rows.Close()

    type row struct { datname, query string; durationMs int64 }
    var hits []row
    for rows.Next() {
        var r row
        if err := rows.Scan(&r.datname, &r.query, &r.durationMs); err != nil { continue }
        hits = append(hits, r)
    }

    for _, h := range hits {
        fp := fingerprint(h.query)
        if c.dedup.seen(instanceID, fp) { continue }
        if isDDL(h.query) { continue }  // skip DDL for duration trigger
        plan, err := c.runExplain(ctx, pool, h.query)
        if err != nil { continue }
        plan.InstanceID = instanceID
        plan.DatabaseName = h.datname
        plan.QueryFingerprint = fp
        plan.TriggerType = TriggerDuration
        plan.DurationMs = h.durationMs
        plan.QueryText = truncateStr(h.query, 4096)
        plan.CapturedAt = time.Now()
        _ = c.checkAndSave(ctx, plan)
    }
    return nil
}
```

Implement `collectScheduledTopN`:

```go
func (c *Collector) collectScheduledTopN(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
    rows, err := pool.Query(ctx, `
        SELECT query, d.datname
        FROM pg_stat_statements s
        JOIN pg_database d ON d.oid = s.dbid
        WHERE query NOT LIKE '%pgpulse%'
          AND length(query) > 10
          AND query NOT LIKE '<%%'
        ORDER BY s.total_exec_time DESC
        LIMIT $1
    `, c.config.ScheduledTopNCount)
    if err != nil { return err }
    defer rows.Close()

    type row struct{ query, datname string }
    var hits []row
    for rows.Next() {
        var r row
        if err := rows.Scan(&r.query, &r.datname); err != nil { continue }
        hits = append(hits, r)
    }

    for i, h := range hits {
        if isDDLStrict(h.query) { continue }
        plan, err := c.runExplain(ctx, pool, h.query)
        if err != nil { continue }
        plan.InstanceID = instanceID
        plan.DatabaseName = h.datname
        plan.QueryFingerprint = fingerprint(h.query)
        plan.TriggerType = TriggerScheduled
        plan.QueryText = truncateStr(h.query, 4096)
        plan.Metadata = map[string]any{"rank": i + 1}
        plan.CapturedAt = time.Now()
        _ = c.checkAndSave(ctx, plan)
    }
    return nil
}
```

Implement `runExplain`, `checkAndSave`, `fingerprint`, `isDDL`, `truncateStr`:

```go
func (c *Collector) runExplain(ctx context.Context, pool *pgxpool.Pool, query string) (PlanCapture, error) {
    var planBytes []byte
    err := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+query).Scan(&planBytes)
    if err != nil { return PlanCapture{}, err }

    truncated := false
    if len(planBytes) > c.config.MaxPlanBytes {
        planBytes = planBytes[:c.config.MaxPlanBytes]
        truncated = true
    }

    h := sha256.Sum256(planBytes)
    return PlanCapture{
        PlanText:  string(planBytes),
        PlanHash:  fmt.Sprintf("%x", h),
        Truncated: truncated,
    }, nil
}

func (c *Collector) checkAndSave(ctx context.Context, plan PlanCapture) error {
    prevHash, _ := c.store.LatestPlanHash(ctx, plan.InstanceID, plan.QueryFingerprint)
    if prevHash != "" && prevHash != plan.PlanHash {
        // Emit plan regression signal (stored as additional row with TriggerHashDiff)
        regression := plan
        regression.TriggerType = TriggerHashDiff
        regression.Metadata = map[string]any{"prev_hash": prevHash, "new_hash": plan.PlanHash}
        _ = c.store.SavePlan(ctx, regression)
    }
    return c.store.SavePlan(ctx, plan)
}

func fingerprint(query string) string {
    h := md5.Sum([]byte(normalizeQuery(query)))
    return fmt.Sprintf("%x", h)
}

func normalizeQuery(q string) string {
    // Simple normalization: lowercase, collapse whitespace
    return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}

func isDDL(q string) bool {
    upper := strings.ToUpper(strings.TrimSpace(q))
    for _, kw := range []string{"DROP ", "CREATE ", "TRUNCATE ", "ALTER ", "REINDEX "} {
        if strings.HasPrefix(upper, kw) { return true }
    }
    return false
}

func isDDLStrict(q string) bool {
    // Stricter: also skip INSERT/UPDATE/DELETE for scheduled topN EXPLAIN
    upper := strings.ToUpper(strings.TrimSpace(q))
    for _, kw := range []string{"DROP ", "CREATE ", "TRUNCATE ", "ALTER ", "REINDEX ",
                                "INSERT ", "UPDATE ", "DELETE "} {
        if strings.HasPrefix(upper, kw) { return true }
    }
    return false
}

func truncateStr(s string, maxLen int) string {
    if len(s) <= maxLen { return s }
    return s[:maxLen]
}
```

---

### Task 2: Settings Snapshot Collector

Create `internal/settings/snapshot.go`:

```go
package settings

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type SettingValue struct {
    Setting        string
    Unit           string
    Source         string
    PendingRestart bool
}

type Snapshot struct {
    InstanceID  string
    CapturedAt  time.Time
    TriggerType string // "startup" | "scheduled" | "manual"
    PGVersion   string
    Settings    map[string]SettingValue
}

type SnapshotStore interface {
    SaveSnapshot(ctx context.Context, s Snapshot) error
    GetSnapshot(ctx context.Context, id int64) (*Snapshot, error)
    ListSnapshots(ctx context.Context, instanceID string, limit int) ([]SnapshotMeta, error)
}

type SnapshotMeta struct {
    ID          int64
    InstanceID  string
    CapturedAt  time.Time
    TriggerType string
    PGVersion   string
}

type SnapshotConfig struct {
    Enabled            bool
    ScheduledInterval  time.Duration
    CaptureOnStartup   bool
    RetentionDays      int
}

type SnapshotCollector struct {
    config           SnapshotConfig
    store            SnapshotStore
    mu               sync.Mutex
    capturedOnStart  map[string]bool
    lastScheduled    map[string]time.Time
}

func NewSnapshotCollector(cfg SnapshotConfig, store SnapshotStore) *SnapshotCollector {
    return &SnapshotCollector{
        config:          cfg,
        store:           store,
        capturedOnStart: make(map[string]bool),
        lastScheduled:   make(map[string]time.Time),
    }
}

func (c *SnapshotCollector) Collect(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
    if !c.config.Enabled { return nil }

    c.mu.Lock()
    needsStartup := c.config.CaptureOnStartup && !c.capturedOnStart[instanceID]
    needsScheduled := time.Since(c.lastScheduled[instanceID]) >= c.config.ScheduledInterval
    c.mu.Unlock()

    if needsStartup {
        if err := c.capture(ctx, pool, instanceID, "startup"); err != nil {
            return fmt.Errorf("startup settings snapshot: %w", err)
        }
        c.mu.Lock()
        c.capturedOnStart[instanceID] = true
        c.mu.Unlock()
    } else if needsScheduled {
        if err := c.capture(ctx, pool, instanceID, "scheduled"); err != nil {
            return fmt.Errorf("scheduled settings snapshot: %w", err)
        }
        c.mu.Lock()
        c.lastScheduled[instanceID] = time.Now()
        c.mu.Unlock()
    }

    return nil
}

func (c *SnapshotCollector) CaptureManual(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
    return c.capture(ctx, pool, instanceID, "manual")
}

func (c *SnapshotCollector) capture(ctx context.Context, pool *pgxpool.Pool, instanceID, trigger string) error {
    rows, err := pool.Query(ctx, `
        SELECT name, setting, COALESCE(unit,''), source, pending_restart
        FROM pg_catalog.pg_settings
        ORDER BY name
    `)
    if err != nil { return err }
    defer rows.Close()

    settings := make(map[string]SettingValue)
    for rows.Next() {
        var name, setting, unit, source string
        var pendingRestart bool
        if err := rows.Scan(&name, &setting, &unit, &source, &pendingRestart); err != nil { continue }
        settings[name] = SettingValue{Setting: setting, Unit: unit, Source: source, PendingRestart: pendingRestart}
    }

    var version string
    _ = pool.QueryRow(ctx, "SELECT version()").Scan(&version)

    return c.store.SaveSnapshot(ctx, Snapshot{
        InstanceID:  instanceID,
        CapturedAt:  time.Now(),
        TriggerType: trigger,
        PGVersion:   version,
        Settings:    settings,
    })
}
```

---

### Task 3: ML Baseline + Detector

Create `internal/ml/config.go`:

```go
package ml

import "time"

type MetricConfig struct {
    Key     string
    Period  int  // seasonal period in data points
    Enabled bool
}

type DetectorConfig struct {
    Enabled              bool
    ZScoreWarn           float64
    ZScoreCrit           float64
    AnomalyLogic         string // "or" | "and"
    Metrics              []MetricConfig
    CollectionInterval   time.Duration // needed for Bootstrap time range calc
}

func DefaultConfig() DetectorConfig {
    return DetectorConfig{
        Enabled:            true,
        ZScoreWarn:         3.0,
        ZScoreCrit:         5.0,
        AnomalyLogic:       "or",
        CollectionInterval: 60 * time.Second,
        Metrics: []MetricConfig{
            {Key: "connections.utilization_pct", Period: 1440, Enabled: true},
            {Key: "cache.hit_ratio",             Period: 1440, Enabled: true},
            {Key: "transactions.commit_rate",    Period: 1440, Enabled: true},
            {Key: "replication.lag_bytes",       Period: 1440, Enabled: true},
            {Key: "locks.blocking_count",        Period: 1440, Enabled: true},
        },
    }
}
```

Create `internal/ml/baseline.go` — full implementation using gonum:

```go
package ml

import (
    "math"
    "sort"

    "gonum.org/v1/gonum/stat"
)

type STLBaseline struct {
    MetricKey   string
    Period      int
    windowSize  int
    ring        []float64
    rHead       int
    rCount      int
    residuals   []float64
    resHead     int
    resCount    int
    ewma        float64
    ewmaAlpha   float64
    seasonal    []float64
    seasonN     []int
    totalSeen   int
    sumAll      float64
}

func NewSTLBaseline(key string, period int) *STLBaseline {
    size := max(3*period, 1000)
    alpha := 2.0 / float64(size+1)
    return &STLBaseline{
        MetricKey:  key,
        Period:     period,
        windowSize: size,
        ring:       make([]float64, size),
        residuals:  make([]float64, size),
        ewmaAlpha:  alpha,
        seasonal:   make([]float64, period),
        seasonN:    make([]int, period),
    }
}

func (b *STLBaseline) Update(value float64) {
    // EWMA trend
    if b.totalSeen == 0 {
        b.ewma = value
    } else {
        b.ewma = b.ewmaAlpha*value + (1-b.ewmaAlpha)*b.ewma
    }

    // Online seasonal mean per bucket
    bucket := b.totalSeen % b.Period
    n := float64(b.seasonN[bucket] + 1)
    b.seasonal[bucket] = (b.seasonal[bucket]*(n-1) + value) / n
    b.seasonN[bucket]++

    b.sumAll += value
    b.totalSeen++

    // Overall mean for seasonal de-centering
    overallMean := b.sumAll / float64(b.totalSeen)
    seasonal := b.seasonal[bucket] - overallMean

    // Residual
    residual := value - b.ewma - seasonal

    // Store residual in ring
    b.residuals[b.resHead] = residual
    b.resHead = (b.resHead + 1) % b.windowSize
    if b.resCount < b.windowSize { b.resCount++ }

    // Store value in ring
    b.ring[b.rHead] = value
    b.rHead = (b.rHead + 1) % b.windowSize
    if b.rCount < b.windowSize { b.rCount++ }
}

func (b *STLBaseline) Score(value float64) (zScore float64, isIQR bool) {
    if b.resCount < b.Period*2 { return 0, false }

    bucket := b.totalSeen % b.Period
    overallMean := b.sumAll / float64(b.totalSeen)
    seasonal := b.seasonal[bucket] - overallMean
    residual := value - b.ewma - seasonal

    resSlice := b.residuals[:b.resCount]
    mean := stat.Mean(resSlice, nil)
    stddev := stat.StdDev(resSlice, nil)
    if stddev > 1e-10 {
        zScore = (residual - mean) / stddev
    }

    // IQR on residuals
    sorted := make([]float64, len(resSlice))
    copy(sorted, resSlice)
    sort.Float64s(sorted)
    q1 := stat.Quantile(0.25, stat.Empirical, sorted, nil)
    q3 := stat.Quantile(0.75, stat.Empirical, sorted, nil)
    iqr := q3 - q1
    isIQR = residual < q1-1.5*iqr || residual > q3+1.5*iqr

    return zScore, isIQR
}

func (b *STLBaseline) ResidualStddev() float64 {
    if b.resCount < 2 { return 0 }
    return stat.StdDev(b.residuals[:b.resCount], nil)
}

func (b *STLBaseline) Ready() bool {
    return b.resCount >= b.Period*2
}

func max(a, b int) int {
    if a > b { return a }
    return b
}

func absF(v float64) float64 { return math.Abs(v) }
```

Create `internal/ml/detector.go`:

```go
package ml

import (
    "context"
    "fmt"
    "log/slog"
    "math"
    "strconv"
    "sync"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

type AnomalyResult struct {
    InstanceID string
    Metric     string
    Value      float64
    ZScore     float64
    IsIQR      bool
    IsAnomaly  bool
    Timestamp  time.Time
}

type Detector struct {
    config    DetectorConfig
    baselines map[string]*STLBaseline // "instanceID:metricKey"
    mu        sync.RWMutex
    store     collector.MetricStore
    evaluator collector.AlertEvaluator
}

func NewDetector(cfg DetectorConfig, store collector.MetricStore, evaluator collector.AlertEvaluator) *Detector {
    return &Detector{
        config:    cfg,
        baselines: make(map[string]*STLBaseline),
        store:     store,
        evaluator: evaluator,
    }
}

func (d *Detector) Bootstrap(ctx context.Context) error {
    instances, err := d.store.ListInstances(ctx)
    if err != nil { return fmt.Errorf("listing instances for ML bootstrap: %w", err) }

    for _, instanceID := range instances {
        for _, mc := range d.config.Metrics {
            if !mc.Enabled { continue }

            lookback := time.Duration(max(3*mc.Period, 1000)) * d.config.CollectionInterval
            points, err := d.store.Query(ctx, collector.MetricQuery{
                InstanceID: instanceID,
                Metric:     mc.Key,
                Start:      time.Now().Add(-lookback),
                End:        time.Now(),
                Limit:      max(3*mc.Period, 1000),
            })
            if err != nil || len(points) < mc.Period*2 {
                slog.Warn("insufficient history for ML baseline",
                    "instance", instanceID,
                    "metric", mc.Key,
                    "have", len(points),
                    "need", mc.Period*2)
                continue
            }

            b := NewSTLBaseline(mc.Key, mc.Period)
            for _, p := range points { b.Update(p.Value) }

            key := instanceID + ":" + mc.Key
            d.mu.Lock()
            d.baselines[key] = b
            d.mu.Unlock()

            slog.Info("ML baseline fitted",
                "instance", instanceID,
                "metric", mc.Key,
                "points", len(points),
                "residual_stddev", b.ResidualStddev())
        }
    }
    return nil
}

func (d *Detector) Evaluate(ctx context.Context, points []collector.MetricPoint) ([]AnomalyResult, error) {
    var results []AnomalyResult

    for _, p := range points {
        key := p.InstanceID + ":" + p.Metric
        d.mu.RLock()
        b, ok := d.baselines[key]
        d.mu.RUnlock()
        if !ok || !b.Ready() { continue }

        zScore, isIQR := b.Score(p.Value)

        d.mu.Lock()
        b.Update(p.Value)
        d.mu.Unlock()

        isAnomaly := false
        az := math.Abs(zScore)
        switch d.config.AnomalyLogic {
        case "and":
            isAnomaly = az >= d.config.ZScoreWarn && isIQR
        default: // "or"
            isAnomaly = az >= d.config.ZScoreWarn || isIQR
        }

        if !isAnomaly { continue }

        result := AnomalyResult{
            InstanceID: p.InstanceID,
            Metric:     p.Metric,
            Value:      p.Value,
            ZScore:     zScore,
            IsIQR:      isIQR,
            IsAnomaly:  true,
            Timestamp:  p.Timestamp,
        }
        results = append(results, result)

        method := "zscore"
        if isIQR && az < d.config.ZScoreWarn { method = "iqr" } else if isIQR { method = "both" }

        labels := map[string]string{
            "instance_id":     p.InstanceID,
            "original_metric": p.Metric,
            "method":          method,
            "original_value":  strconv.FormatFloat(p.Value, 'f', -1, 64),
        }
        if err := d.evaluator.Evaluate(ctx, "anomaly."+p.Metric, az, labels); err != nil {
            slog.Warn("anomaly alert dispatch failed", "err", err, "metric", p.Metric)
        }
    }
    return results, nil
}
```

---

## API & Security Agent

**Your scope:** `internal/plans/store.go`, `internal/settings/diff.go`,
`internal/settings/store.go`, `internal/api/plans.go`, `internal/api/settings.go`,
`migrations/007_plan_capture.sql`, `migrations/008_settings_snapshots.sql`,
`internal/plans/retention.go`

**Do NOT touch:** `internal/ml/`, `internal/collector/`

---

### Task 1: Migrations

Create `migrations/007_plan_capture.sql` exactly as specified in design doc section 5.
Create `migrations/008_settings_snapshots.sql` exactly as specified in design doc section 5.

---

### Task 2: Plan Store

Create `internal/plans/store.go`:

```go
package plans

import (
    "context"
    "encoding/json"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type PGPlanStore struct {
    pool *pgxpool.Pool
}

func NewPGPlanStore(pool *pgxpool.Pool) *PGPlanStore {
    return &PGPlanStore{pool: pool}
}

func (s *PGPlanStore) SavePlan(ctx context.Context, p PlanCapture) error {
    meta, _ := json.Marshal(p.Metadata)
    _, err := s.pool.Exec(ctx, `
        INSERT INTO query_plans
            (instance_id, database_name, query_fingerprint, plan_hash,
             plan_text, plan_json, trigger_type, duration_ms, query_text,
             truncated, metadata, captured_at)
        VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12)
        ON CONFLICT (instance_id, query_fingerprint, plan_hash)
        DO UPDATE SET captured_at = EXCLUDED.captured_at,
                      metadata = EXCLUDED.metadata
    `, p.InstanceID, p.DatabaseName, p.QueryFingerprint, p.PlanHash,
        p.PlanText, p.PlanText, // plan_json = same text cast to jsonb
        string(p.TriggerType), nullInt64(p.DurationMs), p.QueryText,
        p.Truncated, string(meta), p.CapturedAt)
    return err
}

func (s *PGPlanStore) LatestPlanHash(ctx context.Context, instanceID, fingerprint string) (string, error) {
    var hash string
    err := s.pool.QueryRow(ctx, `
        SELECT plan_hash FROM query_plans
        WHERE instance_id=$1 AND query_fingerprint=$2
        ORDER BY captured_at DESC LIMIT 1
    `, instanceID, fingerprint).Scan(&hash)
    if err != nil { return "", nil } // not found is OK
    return hash, nil
}

// ListPlans, GetPlan, ListRegressions — implement with standard SELECT queries
// ListPlans: SELECT id, instance_id, query_fingerprint, plan_hash, trigger_type,
//                   duration_ms, query_text, truncated, captured_at
//            FROM query_plans WHERE instance_id=$1
//            AND ($2::text IS NULL OR query_fingerprint=$2)
//            AND ($3::timestamptz IS NULL OR captured_at >= $3)
//            ORDER BY captured_at DESC LIMIT 100

// ListRegressions: SELECT * FROM query_plans WHERE trigger_type='hash_diff_signal'
//                  AND instance_id=$1 ORDER BY captured_at DESC LIMIT 50
```

Create `internal/plans/retention.go`:

```go
package plans

import (
    "context"
    "log/slog"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type RetentionWorker struct {
    pool      *pgxpool.Pool
    planDays  int
    checkHour time.Duration
}

func NewRetentionWorker(pool *pgxpool.Pool, planDays int) *RetentionWorker {
    return &RetentionWorker{pool: pool, planDays: planDays, checkHour: time.Hour}
}

func (w *RetentionWorker) Run(ctx context.Context) {
    ticker := time.NewTicker(w.checkHour)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            cutoff := time.Now().AddDate(0, 0, -w.planDays)
            result, err := w.pool.Exec(ctx,
                "DELETE FROM query_plans WHERE captured_at < $1", cutoff)
            if err != nil {
                slog.Warn("plan retention cleanup failed", "err", err)
                continue
            }
            if result.RowsAffected() > 0 {
                slog.Info("plan retention: deleted old plans", "count", result.RowsAffected())
            }
        }
    }
}
```

---

### Task 3: Settings Store + Diff

Create `internal/settings/store.go`:

```go
package settings

import (
    "context"
    "encoding/json"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type PGSnapshotStore struct {
    pool *pgxpool.Pool
}

func NewPGSnapshotStore(pool *pgxpool.Pool) *PGSnapshotStore {
    return &PGSnapshotStore{pool: pool}
}

func (s *PGSnapshotStore) SaveSnapshot(ctx context.Context, snap Snapshot) error {
    settingsJSON, err := json.Marshal(snap.Settings)
    if err != nil { return err }
    _, err = s.pool.Exec(ctx, `
        INSERT INTO settings_snapshots (instance_id, captured_at, trigger_type, pg_version, settings)
        VALUES ($1, $2, $3, $4, $5)
    `, snap.InstanceID, snap.CapturedAt, snap.TriggerType, snap.PGVersion, settingsJSON)
    return err
}

func (s *PGSnapshotStore) GetSnapshot(ctx context.Context, id int64) (*Snapshot, error) {
    var snap Snapshot
    var settingsJSON []byte
    err := s.pool.QueryRow(ctx, `
        SELECT instance_id, captured_at, trigger_type, pg_version, settings
        FROM settings_snapshots WHERE id=$1
    `, id).Scan(&snap.InstanceID, &snap.CapturedAt, &snap.TriggerType, &snap.PGVersion, &settingsJSON)
    if err != nil { return nil, err }
    snap.Settings = make(map[string]SettingValue)
    if err := json.Unmarshal(settingsJSON, &snap.Settings); err != nil { return nil, err }
    return &snap, nil
}

// ListSnapshots: SELECT id, instance_id, captured_at, trigger_type, pg_version
//               FROM settings_snapshots WHERE instance_id=$1
//               ORDER BY captured_at DESC LIMIT $2
```

Create `internal/settings/diff.go` — implement `DiffSnapshots` exactly as
specified in design doc section 2 (Go-side diff, returns `SettingsDiff` struct
with `Changed`, `Added`, `Removed`, `PendingRestart` slices).

---

### Task 4: API Handlers

Create `internal/api/plans.go` — wire up routes to chi router:

```go
// Routes to register on the chi router:
// POST   /api/v1/instances/{id}/plans/capture
// GET    /api/v1/instances/{id}/plans
// GET    /api/v1/instances/{id}/plans/{plan_id}
// GET    /api/v1/instances/{id}/plans/regressions
```

- All routes require JWT middleware (existing).
- Manual capture handler: reads `{query, database, label}` from request body,
  calls `plansCollector.CaptureManual(ctx, pool, instanceID, query, database)`.
  Add `CaptureManual` method to `internal/plans/capture.go` (Collector Agent to implement,
  or API Agent if Collector Agent has already committed).
- Return 404 if plan_id not found. Return 200 + JSON on success.

Create `internal/api/settings.go` — wire up routes:

```go
// Routes:
// POST   /api/v1/instances/{id}/settings/snapshot
// GET    /api/v1/instances/{id}/settings/history
// GET    /api/v1/instances/{id}/settings/diff?from={id}&to={id}
// GET    /api/v1/instances/{id}/settings/latest
// GET    /api/v1/instances/{id}/settings/pending-restart
```

- `pending-restart`: query latest snapshot, filter settings where PendingRestart=true.
- `diff`: parse `from` and `to` query params as int64, call `GetSnapshot` for each,
  call `DiffSnapshots`, return JSON.
- All handlers: 400 on bad input, 404 on missing resource, 200 + JSON on success.

---

### Task 5: Default ML Alert Rules

In `internal/alert/rules.go`, add to the default rules seed slice:

```go
{
    Name:      "ML Anomaly Warning",
    Metric:    "anomaly.*",
    Operator:  ">",
    Threshold: 3.0,
    Severity:  "warning",
    Source:    "yaml",
    Message:   "ML anomaly: Z-score {{.Value}} on {{.Labels.original_metric}} (instance: {{.Labels.instance_id}})",
},
{
    Name:      "ML Anomaly Critical",
    Metric:    "anomaly.*",
    Operator:  ">",
    Threshold: 5.0,
    Severity:  "critical",
    Source:    "yaml",
    Message:   "Critical ML anomaly: Z-score {{.Value}} on {{.Labels.original_metric}}",
},
```

Use the existing `INSERT ON CONFLICT DO NOTHING` pattern. Match the exact
field names used by the existing rules seed code.

---

### Task 6: configs/pgpulse.example.yml

Add the following top-level sections to the existing example config:

```yaml
plan_capture:
  enabled: true
  duration_threshold_ms: 1000
  dedup_window_seconds: 60
  scheduled_topn_count: 10
  scheduled_topn_interval: "1h"
  max_plan_bytes: 65536
  retention_days: 30

settings_snapshot:
  enabled: true
  scheduled_interval: "24h"
  capture_on_startup: true
  retention_days: 90

ml:
  enabled: true
  zscore_threshold_warning: 3.0
  zscore_threshold_critical: 5.0
  anomaly_logic: "or"
  collection_interval: "60s"
  metrics:
    - key: "connections.utilization_pct"
      period: 1440
      enabled: true
    - key: "cache.hit_ratio"
      period: 1440
      enabled: true
    - key: "transactions.commit_rate"
      period: 1440
      enabled: true
    - key: "replication.lag_bytes"
      period: 1440
      enabled: true
    - key: "locks.blocking_count"
      period: 1440
      enabled: true
```

---

## QA Agent

**Your scope:** All `*_test.go` files for packages created in this iteration.
Start writing test stubs immediately. Fill assertions as Collector + API agents commit code.

---

### Task 1: Plan Capture Tests

Create `internal/plans/capture_test.go`:

- `TestFingerprint_Stable`: same query → same fingerprint; whitespace-normalized version → same fingerprint
- `TestIsDDL`: known DDL prefixes return true; SELECT returns false
- `TestTruncation`: PlanCapture with plan > 64KB → truncated=true, len(PlanText) <= 64KB
- `TestDedupCache_BlocksRecentCapture`: second call within window → seen=true
- `TestDedupCache_AllowsAfterWindow`: sleep past window → seen=false (use short window in test)

Create `internal/plans/store_test.go` (testcontainers PG 16):

- `TestSavePlan_Dedup`: save same plan_hash twice → one row in DB, captured_at updated
- `TestSavePlan_NewHash`: save two different hashes for same fingerprint → two rows
- `TestLatestPlanHash_NoRows`: returns ("", nil) when no plans exist
- `TestLatestPlanHash_Returns`: returns correct hash after save

Create `internal/plans/capture_integration_test.go` (testcontainers PG 16):

- Tag: `//go:build integration`
- `TestDurationThresholdCapture`: run `SELECT pg_sleep(2)` in a goroutine;
  tick the collector; assert at least one plan captured with trigger=duration_threshold
- `TestScheduledTopNCaptureNoop`: if pg_stat_statements not installed, no error

---

### Task 2: Settings Tests

Create `internal/settings/diff_test.go`:

- `TestDiffSnapshots_Changed`: older={max_connections:100}, newer={max_connections:200} → 1 changed
- `TestDiffSnapshots_Added`: newer has a key not in older → 1 added
- `TestDiffSnapshots_Removed`: older has a key not in newer → 1 removed
- `TestDiffSnapshots_PendingRestart`: newer has pending_restart=true → name in PendingRestart list
- `TestDiffSnapshots_NoChange`: identical snapshots → all slices empty

Create `internal/settings/store_test.go` (testcontainers PG 16):

- `TestSaveAndGetSnapshot`: save snapshot, retrieve by ID, assert settings match
- `TestListSnapshots_OrderedByTime`: two snapshots, list returns newest first

---

### Task 3: ML Tests

Create `internal/ml/baseline_test.go`:

- `TestSTLBaseline_NeedsMinPoints`: Score returns (0, false) with fewer than Period*2 points
- `TestSTLBaseline_DetectsOutlier`: feed 3*period synthetic points with known mean=100, stddev=5;
  then Score(200) → |zScore| > 3 and IsAnomaly=true via Detector
- `TestSTLBaseline_StableSignal`: feed constant value → residuals near 0, zScore near 0
- `TestSTLBaseline_SeasonalPattern`: feed synthetic daily pattern (period=10);
  assert seasonal component non-zero and |zScore| near 0 for in-pattern values
- `TestSTLBaseline_Ready`: Ready()=false before Period*2 points, true after

Create `internal/ml/detector_test.go`:

- `TestDetector_EvaluateNoBaseline`: point with no baseline → empty results, no panic
- `TestDetector_EvaluateAnomaly`: inject baseline with known residuals; call Evaluate
  with outlier value → AnomalyResult.IsAnomaly=true, evaluator.Evaluate called once
- `TestDetector_AnomalyLogicOr`: |zScore| > warn, isIQR=false → isAnomaly=true (OR mode)
- `TestDetector_AnomalyLogicAnd`: |zScore| > warn, isIQR=false → isAnomaly=false (AND mode)
- Use a mock AlertEvaluator that records calls

---

### Task 4: API Handler Tests

Create `internal/api/plans_test.go` (httptest):

- `TestPlanCaptureManual_RequiresAuth`: POST without JWT → 401
- `TestPlanCaptureManual_Success`: POST with valid JWT + body → 200, plan stored
- `TestListPlans_Empty`: GET with no plans → 200, empty array
- `TestGetPlan_NotFound`: GET unknown ID → 404

Create `internal/api/settings_test.go` (httptest):

- `TestSettingsDiff_BadParams`: missing from/to → 400
- `TestSettingsDiff_Success`: two snapshots in DB → 200, diff JSON has expected shape
- `TestSettingsPendingRestart_Empty`: latest snapshot has no pending_restart → empty array

---

### Task 5: Lint + Security Scan

After all code is committed:

1. Run `golangci-lint run ./internal/plans/... ./internal/settings/... ./internal/ml/... ./internal/api/...`
2. Grep for string concatenation in SQL: `grep -r 'fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE' internal/`
   → must return 0 results (the single exception is `runExplain` which uses Sprintf to prepend
   `EXPLAIN (FORMAT JSON)` — verify this is explicitly noted in a comment and the query
   comes from pg_stat_activity, not user input)
3. Run `go test -race ./internal/ml/... ./internal/plans/... ./internal/settings/...`
4. Verify no test imports production packages from outside their own module

---

## Shared: main.go Wiring

Team Lead: after all agents complete, update `cmd/pgpulse-server/main.go`:

1. Initialize `PGPlanStore`, `PGSnapshotStore` with the storage pool
2. Initialize `plans.Collector`, `settings.SnapshotCollector` and register with orchestrator
3. Initialize `ml.Detector` and call `Bootstrap(ctx)` with 30s timeout before collector loop
4. Start `plans.RetentionWorker` as a goroutine with the storage pool
5. Wire `detector.Evaluate()` into the post-collection step (called after each metric batch)
6. Pass `snapshotCollector.CaptureManual` to the API handler for the manual-snapshot endpoint
7. Update `.claude/CLAUDE.md` current iteration section to reflect M8_02 completion

---

## Build Verification Sequence

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

All agents: do NOT commit if `go build ./cmd/pgpulse-server` fails.
QA Agent: report final test count and pass rate before Team Lead merges to main.
