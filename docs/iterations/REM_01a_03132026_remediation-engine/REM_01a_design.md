# REM_01a — Design Document

**Iteration:** REM_01a — Rule-Based Remediation Engine (Backend)
**Date:** 2026-03-13
**Follows:** MN_01 (commit 2f96bed)

---

## 1. Architecture Overview

```
┌─────────────────────┐       ┌──────────────────────┐
│  alert/evaluator.go │       │  api/remediation.go   │
│  (on alert fire)    │       │  (Diagnose endpoint)  │
│                     │       │                       │
│ calls interface:    │       │ calls directly:       │
│ RemediationProvider │       │ Engine.Diagnose()     │
└────────┬────────────┘       └──────────┬────────────┘
         │                               │
         ▼                               ▼
┌─────────────────────────────────────────────────────┐
│              internal/remediation/                    │
│                                                      │
│  engine.go    — Engine.EvaluateMetric()              │
│              — Engine.Diagnose()                     │
│  rule.go     — Rule, Priority, Category types        │
│  rules_pg.go — ~17 PostgreSQL rules                  │
│  rules_os.go — ~8 OS rules                           │
│  store.go    — RecommendationStore interface          │
│  pgstore.go  — PostgreSQL implementation              │
│  nullstore.go — NoOp for live mode                    │
└─────────────────┬───────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────┐
│  remediation_recommendations table (013 migration)   │
└─────────────────────────────────────────────────────┘
```

### Dependency Direction (No Import Cycles)

```
internal/alert     defines  RemediationProvider interface
internal/remediation  implements  RemediationProvider
cmd/pgpulse-server/main.go  wires them together

internal/remediation  imports NOTHING from internal/alert
internal/alert        imports NOTHING from internal/remediation
```

---

## 2. Types & Interfaces

### 2.1 internal/remediation/rule.go

```go
package remediation

import "time"

// Priority levels for recommendations (decoupled from alert severity).
type Priority string

const (
    PriorityInfo           Priority = "info"
    PrioritySuggestion     Priority = "suggestion"
    PriorityActionRequired Priority = "action_required"
)

// Category groups recommendations for filtering.
type Category string

const (
    CategoryPerformance   Category = "performance"
    CategoryCapacity      Category = "capacity"
    CategoryConfiguration Category = "configuration"
    CategoryReplication   Category = "replication"
    CategoryMaintenance   Category = "maintenance"
)

// Rule defines a compiled-in remediation rule.
type Rule struct {
    ID       string
    Priority Priority
    Category Category
    // Evaluate checks whether this rule fires given the context.
    // Returns nil if the rule does not match.
    Evaluate func(ctx EvalContext) *RuleResult
}

// EvalContext provides metric context to a rule's Evaluate function.
type EvalContext struct {
    InstanceID string
    MetricKey  string            // the specific metric that triggered (empty for Diagnose)
    Value      float64           // current value of the triggering metric
    Labels     map[string]string // metric labels (e.g., database, schema, table)
    Severity   string            // "warning", "critical", or "" for Diagnose
    // Snapshot gives rules access to ALL current metric values for the instance.
    // Used by composite rules (e.g., CPU = user + system) and Diagnose mode.
    Snapshot MetricSnapshot
}

// MetricSnapshot provides read access to current metric values.
type MetricSnapshot map[string]float64

// Get returns a metric value and whether it exists.
func (s MetricSnapshot) Get(key string) (float64, bool) {
    v, ok := s[key]
    return v, ok
}

// RuleResult is what a matched rule returns.
type RuleResult struct {
    Title       string
    Description string
    DocURL      string
}

// Recommendation is the output persisted to the database and returned via API.
type Recommendation struct {
    ID             int64      `json:"id"`
    RuleID         string     `json:"rule_id"`
    InstanceID     string     `json:"instance_id"`
    AlertEventID   *int64     `json:"alert_event_id,omitempty"`
    MetricKey      string     `json:"metric_key"`
    MetricValue    float64    `json:"metric_value"`
    Priority       Priority   `json:"priority"`
    Category       Category   `json:"category"`
    Title          string     `json:"title"`
    Description    string     `json:"description"`
    DocURL         string     `json:"doc_url,omitempty"`
    CreatedAt      time.Time  `json:"created_at"`
    AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
    AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
}
```

### 2.2 internal/remediation/engine.go

```go
package remediation

import "context"

// Engine evaluates compiled-in remediation rules.
type Engine struct {
    rules []Rule
}

// NewEngine creates an Engine with all compiled-in rules.
func NewEngine() *Engine {
    rules := make([]Rule, 0, 30)
    rules = append(rules, pgRules()...)
    rules = append(rules, osRules()...)
    return &Engine{rules: rules}
}

// EvaluateMetric runs rules that match a specific metric key.
// Called when an alert fires. Returns zero or more recommendations.
func (e *Engine) EvaluateMetric(
    ctx context.Context,
    instanceID, metricKey string,
    value float64,
    labels map[string]string,
    severity string,
    snapshot MetricSnapshot,
) []Recommendation {
    var recs []Recommendation
    evalCtx := EvalContext{
        InstanceID: instanceID,
        MetricKey:  metricKey,
        Value:      value,
        Labels:     labels,
        Severity:   severity,
        Snapshot:   snapshot,
    }
    for _, rule := range e.rules {
        result := rule.Evaluate(evalCtx)
        if result != nil {
            recs = append(recs, Recommendation{
                RuleID:      rule.ID,
                InstanceID:  instanceID,
                MetricKey:   metricKey,
                MetricValue: value,
                Priority:    rule.Priority,
                Category:    rule.Category,
                Title:       result.Title,
                Description: result.Description,
                DocURL:      result.DocURL,
            })
        }
    }
    return recs
}

// Diagnose runs ALL rules against a full metric snapshot for an instance.
// Called on-demand via the Diagnose API endpoint.
func (e *Engine) Diagnose(
    ctx context.Context,
    instanceID string,
    snapshot MetricSnapshot,
) []Recommendation {
    var recs []Recommendation
    for _, rule := range e.rules {
        evalCtx := EvalContext{
            InstanceID: instanceID,
            Snapshot:   snapshot,
        }
        result := rule.Evaluate(evalCtx)
        if result != nil {
            recs = append(recs, Recommendation{
                RuleID:     rule.ID,
                InstanceID: instanceID,
                Priority:   rule.Priority,
                Category:   rule.Category,
                Title:      result.Title,
                Description: result.Description,
                DocURL:     result.DocURL,
            })
        }
    }
    return recs
}

// Rules returns all registered rules (for introspection/listing).
func (e *Engine) Rules() []Rule {
    return e.rules
}
```

### 2.3 internal/remediation/store.go

```go
package remediation

import (
    "context"
    "time"
)

// ListOpts controls filtering and pagination for recommendation queries.
type ListOpts struct {
    InstanceID     string   // filter by instance (empty = all)
    Priority       string   // filter by priority (empty = all)
    Category       string   // filter by category (empty = all)
    Acknowledged   *bool    // nil = all, true = only acknowledged, false = only unacknowledged
    AlertEventID   *int64   // filter by alert event
    Limit          int      // 0 = default (100)
    Offset         int
}

// RecommendationStore persists recommendations to the database.
type RecommendationStore interface {
    // Write persists one or more recommendations. Returns the saved records with IDs.
    Write(ctx context.Context, recs []Recommendation) ([]Recommendation, error)

    // ListByInstance returns recommendations for a specific instance.
    ListByInstance(ctx context.Context, instanceID string, opts ListOpts) ([]Recommendation, int, error)

    // ListAll returns recommendations across all instances (fleet-wide).
    ListAll(ctx context.Context, opts ListOpts) ([]Recommendation, int, error)

    // ListByAlertEvent returns recommendations attached to a specific alert event.
    ListByAlertEvent(ctx context.Context, alertEventID int64) ([]Recommendation, error)

    // Acknowledge marks a recommendation as acknowledged by a user.
    Acknowledge(ctx context.Context, id int64, username string) error

    // CleanOld removes recommendations older than the given duration.
    CleanOld(ctx context.Context, olderThan time.Duration) error
}
```

### 2.4 internal/alert — RemediationProvider Interface (NEW FILE)

```go
// internal/alert/remediation.go  (~20 lines)
package alert

import "context"

// RemediationResult is a recommendation returned by the remediation engine.
// Defined here to avoid importing internal/remediation from internal/alert.
type RemediationResult struct {
    RuleID      string `json:"rule_id"`
    Title       string `json:"title"`
    Description string `json:"description"`
    Priority    string `json:"priority"`
    Category    string `json:"category"`
    DocURL      string `json:"doc_url,omitempty"`
}

// RemediationProvider evaluates remediation rules for a fired alert.
type RemediationProvider interface {
    EvaluateForAlert(
        ctx context.Context,
        instanceID, metricKey string,
        value float64,
        labels map[string]string,
        severity string,
    ) []RemediationResult
}
```

---

## 3. Rule Definitions (~25 Rules)

### 3.1 PostgreSQL Rules (rules_pg.go)

Each rule's `Evaluate` function checks `EvalContext.MetricKey` (for alert-triggered mode)
or inspects `EvalContext.Snapshot` (for Diagnose mode). Rules handle BOTH modes.

| # | Rule ID | Metric Key(s) | Condition | Priority | Category | Title |
|---|---------|---------------|-----------|----------|----------|-------|
| 1 | `rem_conn_high` | `pg.connections.active` | > 80% of max_connections | suggestion | capacity | Consider connection pooling |
| 2 | `rem_conn_exhausted` | `pg.connections.active` | ≥ 99% of max_connections | action_required | capacity | Connections near limit |
| 3 | `rem_cache_low` | `pg.cache.hit_ratio` | < 90% | suggestion | performance | Review shared_buffers sizing |
| 4 | `rem_commit_ratio_low` | `pg.transactions.commit_ratio` | < 90% | suggestion | performance | High rollback rate detected |
| 5 | `rem_repl_lag_bytes` | `pg.replication.replay_lag_bytes` | > 10 MB | suggestion | replication | Check replica load and network |
| 6 | `rem_repl_lag_critical` | `pg.replication.replay_lag_bytes` | > 100 MB | action_required | replication | Replica severely lagging |
| 7 | `rem_repl_slot_inactive` | `pg.replication.slot_inactive` | > 0 | action_required | replication | Inactive replication slots detected |
| 8 | `rem_long_txn_warn` | `pg.transactions.oldest_active_sec` | > 60s | suggestion | performance | Long-running transactions detected |
| 9 | `rem_long_txn_crit` | `pg.transactions.oldest_active_sec` | > 300s | action_required | performance | Stale transactions require intervention |
| 10 | `rem_locks_blocking` | `pg.locks.blocked_count` | > 0 | suggestion | performance | Blocking lock chains detected |
| 11 | `rem_pgss_fill` | `pg.statements.fill_pct` | ≥ 95% | suggestion | maintenance | pg_stat_statements nearing capacity |
| 12 | `rem_wraparound_warn` | `pg.server.wraparound_pct` | > 20% | suggestion | maintenance | Transaction wraparound approaching |
| 13 | `rem_wraparound_crit` | `pg.server.wraparound_pct` | > 50% | action_required | maintenance | Wraparound imminent — vacuum urgently |
| 14 | `rem_track_io` | `pg.settings.track_io_timing` | = 0 | info | configuration | Enable track_io_timing for I/O analysis |
| 15 | `rem_deadlocks` | `pg.transactions.deadlocks` | > 0 (delta) | suggestion | performance | Deadlocks occurring |
| 16 | `rem_bloat_high` | `pg.db.bloat.ratio` | > 2x | suggestion | maintenance | Table bloat exceeding threshold |
| 17 | `rem_bloat_extreme` | `pg.db.bloat.ratio` | > 50x | action_required | maintenance | Severe bloat — schedule pg_repack |

### 3.2 OS Rules (rules_os.go)

OS rules use `Snapshot.Get()` to read composite values. They fire in both
alert-triggered (if OS alert rules exist) and Diagnose modes.

| # | Rule ID | Metric Key(s) | Condition | Priority | Category | Title |
|---|---------|---------------|-----------|----------|----------|-------|
| 18 | `rem_cpu_high` | `os.cpu.user_pct` + `os.cpu.system_pct` | combined > 80% | suggestion | performance | High CPU utilization |
| 19 | `rem_cpu_iowait` | `os.cpu.iowait_pct` | > 20% | action_required | performance | I/O wait bottleneck detected |
| 20 | `rem_mem_pressure` | `os.memory.available_kb`, `os.memory.total_kb` | available < 10% of total | action_required | capacity | Memory pressure detected |
| 21 | `rem_mem_overcommit` | `os.memory.committed_as_kb`, `os.memory.commit_limit_kb` | committed > limit | suggestion | capacity | Memory overcommit detected |
| 22 | `rem_load_high` | `os.load.1m` | > 4.0 (configurable in rule) | suggestion | performance | System load elevated |
| 23 | `rem_disk_util` | `os.disk.util_pct` | > 80% | action_required | capacity | Disk saturation approaching |
| 24 | `rem_disk_read_latency` | `os.disk.read_await_ms` | > 20ms | suggestion | performance | Storage read latency elevated |
| 25 | `rem_disk_write_latency` | `os.disk.write_await_ms` | > 20ms | suggestion | performance | Storage write latency elevated |

### 3.3 Rule Implementation Pattern

Every rule follows this pattern — handle both alert-triggered and Diagnose modes:

```go
{
    ID:       "rem_conn_high",
    Priority: PrioritySuggestion,
    Category: CategoryCapacity,
    Evaluate: func(ctx EvalContext) *RuleResult {
        // Mode 1: Alert-triggered (specific metric key known)
        if ctx.MetricKey == "pg.connections.active" {
            maxConn, ok := ctx.Snapshot.Get("pg.connections.max_connections")
            if !ok || maxConn == 0 {
                return nil
            }
            pct := (ctx.Value / maxConn) * 100
            if pct > 80 && pct < 99 {
                return &RuleResult{
                    Title:       "Consider connection pooling",
                    Description: fmt.Sprintf(
                        "Connection utilization at %.0f%% (%v/%v). "+
                            "Consider adding PgBouncer or increasing max_connections. "+
                            "Review application connection pool settings for idle connections.",
                        pct, ctx.Value, maxConn),
                    DocURL: "https://www.pgbouncer.org/",
                }
            }
            return nil
        }

        // Mode 2: Diagnose (check snapshot directly)
        active, ok1 := ctx.Snapshot.Get("pg.connections.active")
        maxConn, ok2 := ctx.Snapshot.Get("pg.connections.max_connections")
        if !ok1 || !ok2 || maxConn == 0 {
            return nil
        }
        pct := (active / maxConn) * 100
        if pct > 80 && pct < 99 {
            return &RuleResult{
                Title:       "Consider connection pooling",
                Description: fmt.Sprintf(
                    "Connection utilization at %.0f%% (%v/%v). "+
                        "Consider adding PgBouncer or increasing max_connections.",
                    pct, active, maxConn),
                DocURL: "https://www.pgbouncer.org/",
            }
        }
        return nil
    },
},
```

**Composite OS rule example (CPU):**

```go
{
    ID:       "rem_cpu_high",
    Priority: PrioritySuggestion,
    Category: CategoryPerformance,
    Evaluate: func(ctx EvalContext) *RuleResult {
        user, ok1 := ctx.Snapshot.Get("os.cpu.user_pct")
        sys, ok2 := ctx.Snapshot.Get("os.cpu.system_pct")
        if !ok1 || !ok2 {
            return nil
        }
        total := user + sys
        if total > 80 {
            return &RuleResult{
                Title:       "High CPU utilization",
                Description: fmt.Sprintf(
                    "CPU usage at %.0f%% (user: %.0f%%, system: %.0f%%). "+
                        "Investigate CPU-intensive queries with pg_stat_statements. "+
                        "Check for missing indexes causing sequential scans.",
                    total, user, sys),
            }
        }
        return nil
    },
},
```

---

## 4. Database Migration

### 4.1 migrations/013_remediation.sql

```sql
-- 013_remediation.sql — Remediation recommendations table

CREATE TABLE IF NOT EXISTS remediation_recommendations (
    id              BIGSERIAL PRIMARY KEY,
    rule_id         TEXT NOT NULL,
    instance_id     TEXT NOT NULL,
    alert_event_id  BIGINT,
    metric_key      TEXT NOT NULL DEFAULT '',
    metric_value    DOUBLE PRECISION NOT NULL DEFAULT 0,
    priority        TEXT NOT NULL CHECK (priority IN ('info', 'suggestion', 'action_required')),
    category        TEXT NOT NULL CHECK (category IN ('performance', 'capacity', 'configuration', 'replication', 'maintenance')),
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    doc_url         TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by TEXT NOT NULL DEFAULT ''
);

-- Query patterns: by instance (Server detail), fleet-wide (Advisor page), by alert event
CREATE INDEX IF NOT EXISTS idx_remediation_instance
    ON remediation_recommendations (instance_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_remediation_priority
    ON remediation_recommendations (priority, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_remediation_alert_event
    ON remediation_recommendations (alert_event_id)
    WHERE alert_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_remediation_unacknowledged
    ON remediation_recommendations (created_at DESC)
    WHERE acknowledged_at IS NULL;
```

**NOTE:** The `alert_event_id` column does NOT have a foreign key constraint.
The alert history table may use a different ID type or may be cleaned up
independently. A soft reference (application-level join) is safer and avoids
cascade issues with retention cleanup.

---

## 5. API Endpoints

### 5.1 New Routes (internal/api/remediation.go)

| Method | Path | Handler | Auth | Permission | Notes |
|--------|------|---------|------|------------|-------|
| GET | /instances/{id}/recommendations | handleListRecommendations | token | — | Paginated, filterable |
| POST | /instances/{id}/diagnose | handleDiagnose | token | — | On-demand evaluation |
| GET | /recommendations | handleListAllRecommendations | token | — | Fleet-wide for Advisor page |
| PUT | /recommendations/{id}/acknowledge | handleAcknowledgeRecommendation | token | alert_management | Mark reviewed |
| GET | /recommendations/rules | handleListRemediationRules | token | — | List compiled-in rules |

### 5.2 Query Parameters

**GET /instances/{id}/recommendations** and **GET /recommendations**:
- `priority` — filter: `info`, `suggestion`, `action_required`
- `category` — filter: `performance`, `capacity`, `configuration`, `replication`, `maintenance`
- `acknowledged` — filter: `true`, `false`
- `limit` — default 100, max 500
- `offset` — pagination offset

**POST /instances/{id}/diagnose**:
- No request body. Fetches current metrics from MetricStore internally.
- Response: `{ "recommendations": [...], "metrics_evaluated": 157, "rules_evaluated": 25 }`

### 5.3 Alert Detail Enrichment

Modify `handleGetAlertHistory` and `handleGetActiveAlerts` in `internal/api/alerts.go`
to include recommendations for each alert event:

```go
// In the response struct, add:
type AlertEventResponse struct {
    // ... existing fields ...
    Recommendations []Recommendation `json:"recommendations,omitempty"`
}
```

Look up recommendations by `alert_event_id` and embed in response.
In live mode (NullRecommendationStore), the field is empty.

---

## 6. Integration Points

### 6.1 Alert Dispatcher (internal/alert/dispatcher.go)

When `dispatcher.fire()` creates an alert event, it calls `RemediationProvider.EvaluateForAlert()`
if the provider is non-nil:

```go
// In Dispatcher struct, add field:
type Dispatcher struct {
    // ... existing fields ...
    remediation RemediationProvider // nil = disabled
}

// In fire() method, after writing alert event to history:
if d.remediation != nil {
    results := d.remediation.EvaluateForAlert(ctx, event.InstanceID, event.Metric, event.Value, event.Labels, string(event.Severity))
    // Convert results to recommendations, set alert_event_id, persist via store
}
```

The `Dispatcher` receives the `RemediationProvider` via a new setter:
```go
func (d *Dispatcher) SetRemediationProvider(p RemediationProvider) {
    d.remediation = p
}
```

### 6.2 Remediation Adapter (bridges remediation.Engine → alert.RemediationProvider)

Create `internal/remediation/adapter.go` (~30 lines):

```go
package remediation

import "context"
import "github.com/ios9000/PGPulse_01/internal/alert"

// AlertAdapter wraps Engine to implement alert.RemediationProvider.
type AlertAdapter struct {
    engine       *Engine
    metricSource MetricSource
}

// MetricSource provides a snapshot of current metrics for an instance.
type MetricSource interface {
    CurrentSnapshot(ctx context.Context, instanceID string) (MetricSnapshot, error)
}

func NewAlertAdapter(engine *Engine, source MetricSource) *AlertAdapter {
    return &AlertAdapter{engine: engine, metricSource: source}
}

func (a *AlertAdapter) EvaluateForAlert(
    ctx context.Context,
    instanceID, metricKey string,
    value float64,
    labels map[string]string,
    severity string,
) []alert.RemediationResult {
    snapshot, _ := a.metricSource.CurrentSnapshot(ctx, instanceID)
    recs := a.engine.EvaluateMetric(ctx, instanceID, metricKey, value, labels, severity, snapshot)
    results := make([]alert.RemediationResult, len(recs))
    for i, r := range recs {
        results[i] = alert.RemediationResult{
            RuleID:      r.RuleID,
            Title:       r.Title,
            Description: r.Description,
            Priority:    string(r.Priority),
            Category:    string(r.Category),
            DocURL:      r.DocURL,
        }
    }
    return results
}
```

**Import direction:** `internal/remediation` imports `internal/alert` for the
`RemediationResult` type only. This is safe because `internal/alert` does NOT
import `internal/remediation`.

### 6.3 MetricSource Implementation

The API layer or a thin adapter provides `MetricSource` by querying the
`MetricStore.Query()` method for current metric values of an instance:

```go
// internal/remediation/metricsource.go (~40 lines)
package remediation

import (
    "context"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

// StoreMetricSource implements MetricSource using a collector.MetricStore.
type StoreMetricSource struct {
    store collector.MetricStore
}

func NewStoreMetricSource(store collector.MetricStore) *StoreMetricSource {
    return &StoreMetricSource{store: store}
}

func (s *StoreMetricSource) CurrentSnapshot(ctx context.Context, instanceID string) (MetricSnapshot, error) {
    snap := make(MetricSnapshot)
    // Query recent metrics (last 2 minutes to catch all collector intervals)
    points, err := s.store.Query(ctx, collector.MetricQuery{
        InstanceID: instanceID,
        Start:      time.Now().Add(-2 * time.Minute),
        End:        time.Now(),
    })
    if err != nil {
        return snap, err
    }
    // Take the latest value for each metric key
    for _, p := range points {
        snap[p.Metric] = p.Value
    }
    return snap, nil
}
```

### 6.4 Wiring in main.go

```go
// In cmd/pgpulse-server/main.go, after creating MetricStore and AlertDispatcher:

// Create remediation engine (always available — pure Go, no config needed)
remEngine := remediation.NewEngine()

// Create metric source adapter
metricSource := remediation.NewStoreMetricSource(metricStore)

// Create recommendation store (persistent or null)
var remStore remediation.RecommendationStore
if !liveMode {
    remStore = remediation.NewPGStore(pool)
} else {
    remStore = remediation.NewNullStore()
}

// Wire remediation into alert dispatcher
remAdapter := remediation.NewAlertAdapter(remEngine, metricSource)
dispatcher.SetRemediationProvider(remAdapter)

// Wire into API server
apiServer.SetRemediation(remEngine, remStore, metricSource)
```

---

## 7. Live Mode Behavior

| Feature | Live Mode | Persistent Mode |
|---------|-----------|-----------------|
| Engine available | Yes | Yes |
| Diagnose endpoint | Works (queries MemoryStore) | Works (queries PG store) |
| Auto-attach to alerts | Disabled | Enabled |
| Recommendation persistence | NullStore (no-op) | PGStore |
| Fleet-wide listing | Returns empty | Returns from DB |
| Acknowledge | No-op | Persists |

`NullRecommendationStore` implements all methods, matching the `NullAlertHistoryStore` pattern.

---

## 8. Files Created / Modified

### New Files (11)

| File | Lines (est.) | Owner |
|------|-------------|-------|
| `internal/remediation/rule.go` | ~80 | API & Security Agent |
| `internal/remediation/engine.go` | ~90 | API & Security Agent |
| `internal/remediation/rules_pg.go` | ~350 | API & Security Agent |
| `internal/remediation/rules_os.go` | ~200 | API & Security Agent |
| `internal/remediation/store.go` | ~40 | API & Security Agent |
| `internal/remediation/pgstore.go` | ~200 | API & Security Agent |
| `internal/remediation/nullstore.go` | ~40 | API & Security Agent |
| `internal/remediation/adapter.go` | ~40 | API & Security Agent |
| `internal/remediation/metricsource.go` | ~40 | API & Security Agent |
| `internal/api/remediation.go` | ~200 | API & Security Agent |
| `internal/storage/migrations/013_remediation.sql` | ~25 | API & Security Agent |

### New Test Files (4)

| File | Lines (est.) | Owner |
|------|-------------|-------|
| `internal/remediation/engine_test.go` | ~300 | QA Agent |
| `internal/remediation/rules_test.go` | ~400 | QA Agent |
| `internal/remediation/pgstore_test.go` | ~200 | QA Agent |
| `internal/api/remediation_test.go` | ~250 | QA Agent |

### Modified Files (6)

| File | Change | Owner |
|------|--------|-------|
| `internal/alert/dispatcher.go` | Add `RemediationProvider` field + `SetRemediationProvider()` + call in `fire()` | API & Security Agent |
| `internal/alert/remediation.go` | NEW: `RemediationProvider` interface + `RemediationResult` type | API & Security Agent |
| `internal/api/server.go` | Add remediation dependencies + routes in `Routes()` | API & Security Agent |
| `internal/api/alerts.go` | Embed recommendations in alert event responses | API & Security Agent |
| `cmd/pgpulse-server/main.go` | Wire remediation engine, store, adapter | API & Security Agent |
| `internal/alert/template.go` | Include recommendations in alert email notifications | API & Security Agent |

---

## 9. Pre-Flight Issue Checklist

Before spawning agents, verify these manually:

| # | Check | Risk if Skipped |
|---|-------|-----------------|
| 1 | Verify `alert_history` table has a usable ID column for soft-reference from `remediation_recommendations.alert_event_id` | Migration fails or join breaks |
| 2 | Verify `Dispatcher.fire()` method signature and where alert events are created | Integration point may not exist as expected |
| 3 | Verify `MetricStore.Query()` accepts time-range queries (Start/End) | MetricSource adapter won't work |
| 4 | Verify `MetricQuery` struct has Start/End fields | Compile error |
| 5 | Check for existing migration 013 in `internal/storage/migrations/` | Migration number collision |
| 6 | Verify APIServer setter pattern (`SetLiveMode`, `SetAuthMode`) to follow for `SetRemediation` | Inconsistent API surface |

---

## 10. Test Strategy

### Unit Tests (internal/remediation/)

- **engine_test.go**: Test `EvaluateMetric()` and `Diagnose()` with mock snapshots
  - Each rule fires when condition is met
  - Each rule returns nil when condition is not met
  - Boundary values (exactly at threshold)
  - Missing metrics in snapshot → rule gracefully returns nil
  - Alert-triggered mode vs Diagnose mode produce equivalent results

- **rules_test.go**: Table-driven tests for all 25 rules
  - Positive case (condition met → recommendation returned)
  - Negative case (condition not met → nil)
  - Edge cases (zero values, missing keys, extreme values)

- **pgstore_test.go**: Database integration tests
  - Write + ListByInstance round-trip
  - ListAll with filters (priority, category, acknowledged)
  - Acknowledge updates timestamp and username
  - CleanOld removes old records
  - ListByAlertEvent returns correct subset

### API Tests (internal/api/)

- **remediation_test.go**: HTTP handler tests
  - GET /instances/{id}/recommendations — pagination, filters
  - POST /instances/{id}/diagnose — returns recommendations
  - GET /recommendations — fleet-wide listing
  - PUT /recommendations/{id}/acknowledge — 200 on success, 404 on missing
  - GET /recommendations/rules — returns compiled-in rule list
  - Live mode: diagnose works, fleet-wide returns empty

### Integration Tests

- Alert fires → recommendations auto-attached and persisted
- Alert detail response includes embedded recommendations
- Diagnose on instance with no metrics → empty list (not error)
