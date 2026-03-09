# M8_02 Design
## Auto-Capture Plans + Temporal Settings Diff + ML Anomaly Detection

**Iteration:** M8_02
**Date:** 2026-03-09
**Depends on:** M8_01 (gonum integration, MetricStore query pipeline)

---

## 1. Plan Capture Architecture

### Collector-Side: How Triggers Fire

The plan capture collector runs as a separate goroutine within the collection
loop, not as part of the instance collector. It has its own connection with
`application_name = 'pgpulse_plan_capture'` and `statement_timeout = '10s'`.

```
Collection loop tick (10s)
    │
    ├─► PlanCaptureCollector.Collect(ctx, pool, ic)
    │       │
    │       ├─ 1. DurationThreshold: scan pg_stat_activity
    │       │       SELECT pid, query, datname, now()-query_start AS duration, usename
    │       │       FROM pg_stat_activity
    │       │       WHERE state='active'
    │       │         AND query NOT LIKE '%pg_stat_activity%'
    │       │         AND now()-query_start > $1::interval
    │       │       -- $1 = config.DurationThresholdMs
    │       │
    │       │   For each row:
    │       │     fingerprint = md5(normalizeQuery(query))
    │       │     if dedupCache.recent(fingerprint, 60s): skip
    │       │     plan = runExplain(conn_to_datname, query)
    │       │     store.SavePlan(plan, trigger=DurationThreshold)
    │       │
    │       ├─ 2. ScheduledTopN: check if topNTimer fired
    │       │       SELECT queryid, query, dbid, total_exec_time/calls AS mean_ms
    │       │       FROM pg_stat_statements
    │       │       ORDER BY total_exec_time DESC LIMIT $1
    │       │
    │       │   For each row:
    │       │     plan = runExplain(conn_to_db(dbid), query)
    │       │     store.SavePlan(plan, trigger=ScheduledTopN)
    │       │
    │       └─ 3. HashDiff: after every SavePlan
    │               prev = store.LatestPlanHash(instance_id, fingerprint)
    │               if prev != "" && prev != newHash:
    │                 store.EmitRegression(instance_id, fingerprint, prev, newHash)
```

### runExplain Function

```go
func runExplain(ctx context.Context, conn *pgxpool.Pool, dbname, query string) (PlanCapture, error) {
    explainSQL := fmt.Sprintf("EXPLAIN (FORMAT JSON) %s", query)
    // NOTE: query is NOT user-controlled in duration-threshold path —
    // it comes from pg_stat_activity on the monitored server.
    // Still: cap at 4096 chars before sending, skip if contains DDL keywords.
    var planJSON []byte
    err := conn.QueryRow(ctx, explainSQL).Scan(&planJSON)
    ...
    // Truncate if > 64KB
    if len(planJSON) > 65536 {
        planJSON = planJSON[:65536]
        truncated = true
    }
    return PlanCapture{
        PlanJSON:  planJSON,
        PlanHash:  sha256hex(planJSON),
        Truncated: truncated,
    }, nil
}
```

**Safety note:** `EXPLAIN` on a query text from `pg_stat_activity` runs a *new*
planning pass against the target database. It does not affect the running query.
The planner may produce a different plan if statistics changed since the active query
began, but this is acceptable for monitoring purposes.

**DDL guard:** Before running EXPLAIN, check if the query starts with INSERT/UPDATE/
DELETE/DROP/CREATE/TRUNCATE/ALTER. If yes, for duration-threshold trigger, still
run EXPLAIN (planner handles DML). For scheduled-topN, skip DDL queries entirely.

### Dedup Cache

In-memory `sync.Map` with TTL:

```go
type dedupEntry struct {
    capturedAt time.Time
}

// key: "instance_id:fingerprint"
// Before capture: check if entry exists and age < dedupWindowSeconds
// After capture: set entry with current time
```

This is ephemeral — resets on server restart. Acceptable: worst case is one
extra capture per metric per instance on restart.

### Storage Layer: Plan Dedup via Upsert

```sql
INSERT INTO query_plans (
    instance_id, database_name, query_fingerprint, plan_hash,
    plan_text, plan_json, trigger_type, duration_ms, query_text,
    truncated, metadata, captured_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
ON CONFLICT (instance_id, query_fingerprint, plan_hash)
DO UPDATE SET captured_at = now(), metadata = EXCLUDED.metadata
```

Add unique constraint: `UNIQUE (instance_id, query_fingerprint, plan_hash)`

This means identical plan shapes for the same query are stored once and simply
refreshed. New plan hashes (regressions) always produce a new row.

---

## 2. Temporal Settings Diff Architecture

### Snapshot Collector

```go
// internal/settings/snapshot.go

type SettingsCollector struct {
    lastSnapshot map[string]time.Time  // instance_id → last scheduled capture
    capturedOnStartup map[string]bool
    store        SettingsStore
    config       SettingsConfig
}

func (c *SettingsCollector) Collect(ctx context.Context, pool *pgxpool.Pool, ic InstanceContext) error {
    instanceID := ic.InstanceID

    // Startup capture: once per instance per server lifetime
    if !c.capturedOnStartup[instanceID] {
        if err := c.captureAndStore(ctx, pool, instanceID, "startup"); err != nil {
            return err
        }
        c.capturedOnStartup[instanceID] = true
    }

    // Scheduled capture
    last := c.lastSnapshot[instanceID]
    if time.Since(last) >= c.config.ScheduledInterval {
        if err := c.captureAndStore(ctx, pool, instanceID, "scheduled"); err != nil {
            return err
        }
        c.lastSnapshot[instanceID] = time.Now()
    }

    return nil
}
```

### Settings Query

```sql
SELECT name,
       setting,
       unit,
       source,
       pending_restart
FROM pg_catalog.pg_settings
ORDER BY name
```

Assembled into a `map[string]SettingValue` before storing as JSONB.

### Diff Algorithm

```go
func DiffSnapshots(older, newer map[string]SettingValue) SettingsDiff {
    var diff SettingsDiff
    for name, newVal := range newer {
        if oldVal, ok := older[name]; !ok {
            diff.Added = append(diff.Added, SettingChange{Name: name, NewValue: newVal.Setting})
        } else if oldVal.Setting != newVal.Setting {
            diff.Changed = append(diff.Changed, SettingChange{
                Name: name, OldValue: oldVal.Setting, NewValue: newVal.Setting,
                Unit: newVal.Unit, Source: newVal.Source,
            })
        }
        if newVal.PendingRestart {
            diff.PendingRestart = append(diff.PendingRestart, name)
        }
    }
    for name := range older {
        if _, ok := newer[name]; !ok {
            diff.Removed = append(diff.Removed, SettingChange{Name: name, OldValue: older[name].Setting})
        }
    }
    sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Name < diff.Changed[j].Name })
    return diff
}
```

### API Response Shape

`GET /api/v1/instances/:id/settings/diff?from=42&to=47`:

```json
{
  "from": {"id": 42, "captured_at": "2026-03-08T00:00:00Z", "trigger_type": "scheduled"},
  "to":   {"id": 47, "captured_at": "2026-03-09T10:00:00Z", "trigger_type": "scheduled"},
  "summary": {"changed": 2, "added": 0, "removed": 0, "pending_restart": 1},
  "changed": [
    {"name": "max_connections", "old_value": "100", "new_value": "200", "unit": null, "source": "configuration file"},
    {"name": "shared_buffers",  "old_value": "2048", "new_value": "4096", "unit": "8kB", "source": "configuration file"}
  ],
  "added": [],
  "removed": [],
  "pending_restart": ["shared_buffers"]
}
```

---

## 3. ML Anomaly Detection Architecture

### Package Structure

```
internal/ml/
├── baseline.go      ← STLBaseline: ring buffer, trend/seasonal/residual
├── detector.go      ← Detector: per-instance-metric baseline map, Bootstrap, Evaluate
├── config.go        ← DetectorConfig, MetricConfig structs
└── baseline_test.go ← unit tests
```

### STLBaseline Implementation Detail

```go
type STLBaseline struct {
    MetricKey   string
    Period      int       // seasonal period in # of data points
    windowSize  int       // = max(3*Period, 1000)
    ring        []float64 // circular buffer, size = windowSize
    head        int       // next write position
    count        int       // points seen (capped at windowSize)
    ewmaAlpha  float64   // EWMA smoothing factor = 2/(windowSize+1)
    ewma       float64   // current EWMA value (trend)
    seasonBuf  []float64 // length = Period; seasonal[i] = period-folded mean
    seasonCount []int    // how many observations contributed to each bucket
    residuals  []float64 // last windowSize residuals (ring buffer parallel)
    rHead      int
    rCount     int
}

func (b *STLBaseline) Update(value float64) {
    // 1. Update EWMA (trend)
    b.ewma = b.ewmaAlpha*value + (1-b.ewmaAlpha)*b.ewma

    // 2. Update seasonal bucket
    bucket := b.count % b.Period
    // Running mean per bucket (online update)
    n := float64(b.seasonCount[bucket] + 1)
    b.seasonBuf[bucket] = b.seasonBuf[bucket]*(n-1)/n + value/n
    b.seasonCount[bucket]++

    // 3. Compute residual
    seasonal := b.seasonBuf[bucket] - b.overallMean()
    residual := value - b.ewma - seasonal

    // 4. Store residual in ring
    b.residuals[b.rHead] = residual
    b.rHead = (b.rHead + 1) % b.windowSize
    if b.rCount < b.windowSize { b.rCount++ }

    // 5. Store value in ring
    b.ring[b.head] = value
    b.head = (b.head + 1) % b.windowSize
    if b.count < b.windowSize { b.count++ }
}

func (b *STLBaseline) Score(value float64) (zScore float64, isIQR bool) {
    // Needs at least Period*2 observations to be meaningful
    if b.rCount < b.Period*2 { return 0, false }

    // Get residual for this value
    bucket := b.count % b.Period
    seasonal := b.seasonBuf[bucket] - b.overallMean()
    residual := value - b.ewma - seasonal

    // Z-score
    mean, stddev := stats(b.residuals[:b.rCount])
    if stddev > 0 { zScore = (residual - mean) / stddev }

    // IQR
    sorted := sortedCopy(b.residuals[:b.rCount])
    q1 := percentile(sorted, 0.25)
    q3 := percentile(sorted, 0.75)
    iqr := q3 - q1
    isIQR = residual < q1-1.5*iqr || residual > q3+1.5*iqr

    return zScore, isIQR
}
```

**gonum usage:** `gonum.org/v1/gonum/stat` for `stat.Mean`, `stat.StdDev`,
`stat.Quantile`. gonum's `stat.Quantile` requires a sorted slice — use
`gonum.org/v1/gonum/floats.Argsort` for in-place sort on a copy.

### Detector Bootstrap

```go
func (d *Detector) Bootstrap(ctx context.Context) error {
    for _, instanceID := range d.store.ListInstances(ctx) {
        for _, mc := range d.config.Metrics {
            if !mc.Enabled { continue }

            points, err := d.store.Query(ctx, MetricQuery{
                InstanceID: instanceID,
                Metric:     mc.Key,
                Start:      time.Now().Add(-time.Duration(max(3*mc.Period, 1000)) * collectionInterval),
                Limit:      max(3*mc.Period, 1000),
            })
            if err != nil || len(points) < mc.Period {
                slog.Warn("insufficient history for ML baseline",
                    "instance", instanceID, "metric", mc.Key, "points", len(points))
                continue
            }

            key := instanceID + ":" + mc.Key
            b := NewSTLBaseline(mc.Key, mc.Period)
            for _, p := range points {
                b.Update(p.Value)
            }
            d.baselines[key] = b
            slog.Info("ML baseline fitted", "instance", instanceID, "metric", mc.Key,
                "points", len(points), "residual_stddev", b.ResidualStddev())
        }
    }
    return nil
}
```

### Evaluate + Alert Dispatch

```go
func (d *Detector) Evaluate(ctx context.Context, points []MetricPoint) ([]AnomalyResult, error) {
    var results []AnomalyResult
    for _, p := range points {
        key := p.InstanceID + ":" + p.Metric
        b, ok := d.baselines[key]
        if !ok { continue }

        zScore, isIQR := b.Score(p.Value)
        b.Update(p.Value)  // online update after scoring

        isAnomaly := false
        switch d.config.AnomalyLogic {
        case "or":
            isAnomaly = math.Abs(zScore) > d.config.ZScoreWarn || isIQR
        case "and":
            isAnomaly = math.Abs(zScore) > d.config.ZScoreWarn && isIQR
        }

        if isAnomaly {
            r := AnomalyResult{
                InstanceID: p.InstanceID, Metric: p.Metric,
                Value: p.Value, ZScore: zScore, IsIQR: isIQR,
                IsAnomaly: true, Timestamp: p.Timestamp,
            }
            results = append(results, r)

            // Dispatch to alert evaluator
            severity := math.Abs(zScore)
            labels := map[string]string{
                "instance_id":    p.InstanceID,
                "original_metric": p.Metric,
                "method":          anomalyMethod(zScore, isIQR, d.config),
                "original_value":  strconv.FormatFloat(p.Value, 'f', -1, 64),
            }
            _ = d.evaluator.Evaluate(ctx, "anomaly."+p.Metric, severity, labels)
        }
    }
    return results, nil
}
```

### Default ML Alert Rules (seeded in rules.go)

```go
var defaultMLAlertRules = []AlertRule{
    {
        Name:     "ML Anomaly Warning",
        Metric:   "anomaly.*",  // wildcard match on metric prefix
        Operator: ">",
        Threshold: 3.0,
        Severity:  "warning",
        Message:   "ML anomaly detected: Z-score {{.Value}} on {{.Labels.original_metric}}",
    },
    {
        Name:     "ML Anomaly Critical",
        Metric:   "anomaly.*",
        Operator: ">",
        Threshold: 5.0,
        Severity:  "critical",
        Message:   "Critical ML anomaly: Z-score {{.Value}} on {{.Labels.original_metric}}",
    },
}
```

These are seeded via the existing `INSERT ON CONFLICT DO NOTHING` pattern with `source='yaml'`.

---

## 4. main.go Wiring

```go
// cmd/pgpulse-server/main.go — bootstrap sequence

// 1. Load config
// 2. Connect to storage DB
// 3. Run migrations (including 007, 008)
// 4. Start orchestrator (instance connections)

// 5. Bootstrap ML Detector (BEFORE collector loop)
if cfg.ML.Enabled {
    detector := ml.NewDetector(cfg.ML, metricStore, alertEvaluator)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    if err := detector.Bootstrap(ctx); err != nil {
        slog.Warn("ML bootstrap incomplete", "err", err)
    }
    cancel()
    // Pass detector to collector loop for online Evaluate calls
}

// 6. Start collector loop (now includes PlanCaptureCollector, SettingsCollector)
// 7. Start HTTP server
```

---

## 5. Migration Files

### migrations/007_plan_capture.sql

```sql
CREATE TABLE IF NOT EXISTS query_plans (
    id                BIGSERIAL    PRIMARY KEY,
    instance_id       TEXT         NOT NULL,
    database_name     TEXT         NOT NULL DEFAULT '',
    query_fingerprint TEXT         NOT NULL,
    plan_hash         TEXT         NOT NULL,
    plan_text         TEXT,
    plan_json         JSONB,
    trigger_type      TEXT         NOT NULL,
    duration_ms       BIGINT,
    query_text        TEXT,
    truncated         BOOLEAN      NOT NULL DEFAULT FALSE,
    metadata          JSONB,
    captured_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_query_plans_dedup
    ON query_plans(instance_id, query_fingerprint, plan_hash);
CREATE INDEX IF NOT EXISTS idx_query_plans_instance_time
    ON query_plans(instance_id, captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_query_plans_fingerprint
    ON query_plans(instance_id, query_fingerprint, captured_at DESC);

-- Retention cleanup (called by background worker, or pg_cron if available)
-- DELETE FROM query_plans WHERE captured_at < now() - INTERVAL '30 days';
```

### migrations/008_settings_snapshots.sql

```sql
CREATE TABLE IF NOT EXISTS settings_snapshots (
    id           BIGSERIAL    PRIMARY KEY,
    instance_id  TEXT         NOT NULL,
    captured_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    trigger_type TEXT         NOT NULL,
    pg_version   TEXT         NOT NULL DEFAULT '',
    settings     JSONB        NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_settings_snapshots_instance
    ON settings_snapshots(instance_id, captured_at DESC);
```

---

## 6. Retention Background Worker

A lightweight goroutine that runs once per hour and deletes expired rows:

```go
// internal/plans/retention.go

func (s *PlanStore) RunRetentionLoop(ctx context.Context, retentionDays int) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            cutoff := time.Now().AddDate(0, 0, -retentionDays)
            _, _ = s.pool.Exec(ctx,
                "DELETE FROM query_plans WHERE captured_at < $1", cutoff)
        }
    }
}
```

Same pattern for `settings_snapshots` (retention_days from config, default 90).

---

## 7. Module Ownership

| Package | Agent |
|---------|-------|
| `internal/plans/` | Collector Agent (capture.go) + API Agent (store.go) |
| `internal/settings/` | Collector Agent (snapshot.go) + API Agent (diff.go) |
| `internal/api/plans.go` | API Agent |
| `internal/api/settings.go` | API Agent |
| `internal/ml/` | Collector Agent |
| `migrations/007_*.sql`, `migrations/008_*.sql` | API Agent |
| `*_test.go` | QA Agent |

Cross-package dependency: `internal/ml/detector.go` imports `MetricStore` and
`AlertEvaluator` interfaces from `internal/collector/collector.go` (no new
circular deps introduced).

---

## 8. Open Questions (Resolved)

| Question | Decision |
|----------|----------|
| EXPLAIN ANALYZE vs EXPLAIN on active queries | EXPLAIN only — never ANALYZE on a running query |
| Plan storage: delta vs full | Full plan JSON per unique hash; dedup by plan_hash |
| Settings storage: delta vs full | Full snapshot per event |
| ML model persistence | Recompute on startup from TimescaleDB |
| Forecast horizon | Out of scope for M8_02 |
| STL implementation | Simplified (EWMA trend + period-folded mean seasonal) using gonum stat primitives |
| Anomaly logic | OR (flag if Z-score OR IQR exceeds threshold); configurable |
