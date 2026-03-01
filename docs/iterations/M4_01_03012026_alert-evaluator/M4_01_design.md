# M4_01 Design — Alert Evaluator & Rules Engine

**Iteration:** M4_01
**Date:** 2026-03-01
**Input:** M4_01_requirements.md, PGAM_FEATURE_AUDIT.md §6, HANDOFF_M3_to_M4.md

---

## 1. Package Layout

```
internal/alert/
├── alert.go           ← Data model: Rule, AlertEvent, Severity, Operator, AlertState, StateEntry
├── evaluator.go       ← Evaluator struct, Evaluate(), state machine, hysteresis, cooldown
├── rules.go           ← BuiltinRules() []Rule — 20 hardcoded rule definitions
├── store.go           ← AlertRuleStore, AlertHistoryStore interfaces
├── pgstore.go         ← PGAlertRuleStore, PGAlertHistoryStore implementations
├── seed.go            ← SeedBuiltinRules() — upsert builtins to DB on startup
├── alert_test.go      ← Data model unit tests
├── evaluator_test.go  ← State machine, hysteresis, cooldown tests
├── rules_test.go      ← Builtin rule validation tests
├── pgstore_test.go    ← Store tests (mock-based unit + integration)
└── seed_test.go       ← Seeding logic tests
```

New migration:

```
internal/storage/migrations/
└── 004_alerts.sql     ← alert_rules + alert_history tables
```

Config addition:

```
internal/config/config.go  ← Add AlertingConfig struct to Config
internal/config/load.go    ← Add alerting section parsing + validation
```

---

## 2. Data Model (`internal/alert/alert.go`)

```go
package alert

import "time"

// Severity levels for alert rules and events.
type Severity string

const (
    SeverityInfo     Severity = "info"
    SeverityWarning  Severity = "warning"
    SeverityCritical Severity = "critical"
)

// severityLevel returns numeric level for comparison (higher = more severe).
func severityLevel(s Severity) int {
    switch s {
    case SeverityInfo:
        return 1
    case SeverityWarning:
        return 2
    case SeverityCritical:
        return 3
    default:
        return 0
    }
}

// Operator defines comparison operations for threshold checks.
type Operator string

const (
    OpGreater      Operator = ">"
    OpGreaterEqual Operator = ">="
    OpLess         Operator = "<"
    OpLessEqual    Operator = "<="
    OpEqual        Operator = "=="
    OpNotEqual     Operator = "!="
)

// Compare evaluates: value <op> threshold.
func (op Operator) Compare(value, threshold float64) bool {
    switch op {
    case OpGreater:
        return value > threshold
    case OpGreaterEqual:
        return value >= threshold
    case OpLess:
        return value < threshold
    case OpLessEqual:
        return value <= threshold
    case OpEqual:
        return value == threshold
    case OpNotEqual:
        return value != threshold
    default:
        return false
    }
}

// AlertState represents the current state of a rule+instance combination.
type AlertState string

const (
    StateOK      AlertState = "ok"
    StatePending AlertState = "pending"
    StateFiring  AlertState = "firing"
)

// RuleSource indicates where a rule originated.
type RuleSource string

const (
    SourceBuiltin RuleSource = "builtin"
    SourceCustom  RuleSource = "custom"
)

// Rule defines a threshold-based alert check.
type Rule struct {
    ID               string            `json:"id"`
    Name             string            `json:"name"`
    Description      string            `json:"description,omitempty"`
    Metric           string            `json:"metric"`
    Operator         Operator          `json:"operator"`
    Threshold        float64           `json:"threshold"`
    Severity         Severity          `json:"severity"`
    Labels           map[string]string `json:"labels,omitempty"`
    ConsecutiveCount int               `json:"consecutive_count"`
    CooldownMinutes  int               `json:"cooldown_minutes"`
    Channels         []string          `json:"channels,omitempty"`
    Source           RuleSource        `json:"source"`
    Enabled          bool              `json:"enabled"`
}

// AlertEvent represents a state transition that requires action (notification).
type AlertEvent struct {
    RuleID     string     `json:"rule_id"`
    RuleName   string     `json:"rule_name"`
    InstanceID string     `json:"instance_id"`
    Severity   Severity   `json:"severity"`
    Value      float64    `json:"value"`
    Threshold  float64    `json:"threshold"`
    Operator   Operator   `json:"operator"`
    Metric     string     `json:"metric"`
    Labels     map[string]string `json:"labels,omitempty"`
    Channels   []string   `json:"channels,omitempty"`
    FiredAt    time.Time  `json:"fired_at"`
    ResolvedAt *time.Time `json:"resolved_at,omitempty"`
    IsResolution bool     `json:"is_resolution"`
}

// stateEntry tracks per (rule_id, instance_id) evaluation state.
// This is internal to the evaluator, not persisted directly.
type stateEntry struct {
    State            AlertState
    ConsecutiveCount int        // current consecutive breach count
    FiredAt          time.Time  // when alert transitioned to FIRING
    LastNotifiedAt   time.Time  // when last notification was sent
    Severity         Severity   // severity of the firing rule
}
```

---

## 3. Evaluator (`internal/alert/evaluator.go`)

```go
package alert

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

// Evaluator processes metric points against alert rules and tracks state.
type Evaluator struct {
    ruleStore    AlertRuleStore
    historyStore AlertHistoryStore
    logger       *slog.Logger

    mu    sync.Mutex
    rules []Rule                       // cached rules (refreshed via LoadRules)
    state map[string]*stateEntry       // key: "ruleID:instanceID"
}

// NewEvaluator creates an evaluator with the given stores.
func NewEvaluator(ruleStore AlertRuleStore, historyStore AlertHistoryStore, logger *slog.Logger) *Evaluator {
    return &Evaluator{
        ruleStore:    ruleStore,
        historyStore: historyStore,
        logger:       logger,
        state:        make(map[string]*stateEntry),
    }
}

// LoadRules refreshes the cached rule set from the database.
// Call on startup and after rule changes via API.
func (e *Evaluator) LoadRules(ctx context.Context) error {
    rules, err := e.ruleStore.ListEnabled(ctx)
    if err != nil {
        return fmt.Errorf("load alert rules: %w", err)
    }
    e.mu.Lock()
    e.rules = rules
    e.mu.Unlock()
    e.logger.Info("alert rules loaded", "count", len(rules))
    return nil
}

// RestoreState seeds the state machine from unresolved alerts in history.
// Call once on startup after LoadRules.
func (e *Evaluator) RestoreState(ctx context.Context) error {
    unresolved, err := e.historyStore.ListUnresolved(ctx)
    if err != nil {
        return fmt.Errorf("restore alert state: %w", err)
    }
    e.mu.Lock()
    for _, evt := range unresolved {
        key := stateKey(evt.RuleID, evt.InstanceID)
        e.state[key] = &stateEntry{
            State:          StateFiring,
            FiredAt:        evt.FiredAt,
            LastNotifiedAt: evt.FiredAt,
            Severity:       evt.Severity,
        }
    }
    e.mu.Unlock()
    e.logger.Info("alert state restored", "unresolved_count", len(unresolved))
    return nil
}

// Evaluate processes a batch of metric points from one instance against all rules.
// Returns AlertEvents that need notification (new fires + resolutions).
func (e *Evaluator) Evaluate(ctx context.Context, instanceID string, points []collector.MetricPoint) ([]AlertEvent, error) {
    e.mu.Lock()
    rules := e.rules
    e.mu.Unlock()

    now := time.Now()
    var events []AlertEvent

    // Index points by metric name for efficient lookup.
    pointsByMetric := indexPoints(points)

    // Track which (rule, instance) pairs were evaluated this cycle.
    // Rules that had NO matching metrics are skipped (no state change).
    evaluated := make(map[string]bool)

    for _, rule := range rules {
        matchingPoints := findMatchingPoints(rule, pointsByMetric)
        if len(matchingPoints) == 0 {
            continue
        }

        // For rules with label filters, each unique label set is a separate evaluation.
        // For rules without label filters, aggregate: use worst (max for >, min for <) value.
        for _, mp := range matchingPoints {
            key := stateKey(rule.ID, instanceID)
            if len(rule.Labels) > 0 {
                // Per-label evaluation: include label identity in key
                key = stateKeyWithLabels(rule.ID, instanceID, mp.Labels)
            }
            evaluated[key] = true

            breached := rule.Operator.Compare(mp.Value, rule.Threshold)
            event := e.updateState(key, &rule, instanceID, mp, breached, now)
            if event != nil {
                events = append(events, *event)
            }
        }
    }

    // Check for resolutions: rules in FIRING state that were evaluated and not breached
    // are handled within updateState. Rules in FIRING state that had NO matching metrics
    // this cycle are left as-is (data gap, not resolution).

    // Persist events to history.
    for i := range events {
        if events[i].IsResolution {
            if err := e.historyStore.Resolve(ctx, events[i].RuleID, events[i].InstanceID, now); err != nil {
                e.logger.Error("failed to record alert resolution", "rule", events[i].RuleID, "error", err)
            }
        } else {
            if err := e.historyStore.Record(ctx, &events[i]); err != nil {
                e.logger.Error("failed to record alert event", "rule", events[i].RuleID, "error", err)
            }
        }
    }

    return events, nil
}

// updateState applies the state machine transition for one (rule, instance) evaluation.
func (e *Evaluator) updateState(key string, rule *Rule, instanceID string, mp collector.MetricPoint, breached bool, now time.Time) *AlertEvent {
    entry, exists := e.state[key]
    if !exists {
        entry = &stateEntry{State: StateOK}
        e.state[key] = entry
    }

    switch entry.State {
    case StateOK:
        if breached {
            entry.ConsecutiveCount++
            if entry.ConsecutiveCount >= rule.ConsecutiveCount {
                // Transition: OK → FIRING
                entry.State = StateFiring
                entry.FiredAt = now
                entry.LastNotifiedAt = now
                entry.Severity = rule.Severity
                entry.ConsecutiveCount = 0
                return &AlertEvent{
                    RuleID:     rule.ID,
                    RuleName:   rule.Name,
                    InstanceID: instanceID,
                    Severity:   rule.Severity,
                    Value:      mp.Value,
                    Threshold:  rule.Threshold,
                    Operator:   rule.Operator,
                    Metric:     rule.Metric,
                    Labels:     mp.Labels,
                    Channels:   rule.Channels,
                    FiredAt:    now,
                }
            }
            // Still in pending zone (counting up)
            entry.State = StatePending
        }
        // else: still OK, no action

    case StatePending:
        if breached {
            entry.ConsecutiveCount++
            if entry.ConsecutiveCount >= rule.ConsecutiveCount {
                // Transition: PENDING → FIRING
                entry.State = StateFiring
                entry.FiredAt = now
                entry.LastNotifiedAt = now
                entry.Severity = rule.Severity
                entry.ConsecutiveCount = 0
                return &AlertEvent{
                    RuleID:     rule.ID,
                    RuleName:   rule.Name,
                    InstanceID: instanceID,
                    Severity:   rule.Severity,
                    Value:      mp.Value,
                    Threshold:  rule.Threshold,
                    Operator:   rule.Operator,
                    Metric:     rule.Metric,
                    Labels:     mp.Labels,
                    Channels:   rule.Channels,
                    FiredAt:    now,
                }
            }
            // Still counting
        } else {
            // Metric recovered while pending — reset to OK
            entry.State = StateOK
            entry.ConsecutiveCount = 0
        }

    case StateFiring:
        if !breached {
            // Transition: FIRING → OK (resolution)
            resolvedAt := now
            entry.State = StateOK
            entry.ConsecutiveCount = 0
            return &AlertEvent{
                RuleID:       rule.ID,
                RuleName:     rule.Name,
                InstanceID:   instanceID,
                Severity:     entry.Severity,
                Value:        mp.Value,
                Threshold:    rule.Threshold,
                Operator:     rule.Operator,
                Metric:       rule.Metric,
                Labels:       mp.Labels,
                Channels:     rule.Channels,
                FiredAt:      entry.FiredAt,
                ResolvedAt:   &resolvedAt,
                IsResolution: true,
            }
        }
        // Still breached — check cooldown for repeat notification
        // (Repeat notifications are for M4_02 dispatcher; evaluator just returns
        // events for new fires and resolutions. The dispatcher handles cooldown
        // for repeat sends.)
    }

    return nil
}

// stateKey builds the map key for state tracking.
func stateKey(ruleID, instanceID string) string {
    return ruleID + ":" + instanceID
}

// stateKeyWithLabels builds a map key that includes label identity.
func stateKeyWithLabels(ruleID, instanceID string, labels map[string]string) string {
    // For label-filtered rules, include the label values that the rule filters on
    // to track state per unique label combination.
    // Simple approach: append sorted label values to key.
    key := ruleID + ":" + instanceID
    // Deterministic ordering handled by sorted map iteration.
    // For MVP, use fmt.Sprintf which is good enough for map keys.
    if len(labels) > 0 {
        key += ":" + fmt.Sprintf("%v", labels)
    }
    return key
}

// indexPoints builds a map from metric name to list of points.
func indexPoints(points []collector.MetricPoint) map[string][]collector.MetricPoint {
    idx := make(map[string][]collector.MetricPoint, len(points))
    for _, p := range points {
        idx[p.Metric] = append(idx[p.Metric], p)
    }
    return idx
}

// findMatchingPoints returns points that match a rule's metric name and label filter.
func findMatchingPoints(rule Rule, pointsByMetric map[string][]collector.MetricPoint) []collector.MetricPoint {
    candidates, ok := pointsByMetric[rule.Metric]
    if !ok {
        return nil
    }
    if len(rule.Labels) == 0 {
        return candidates
    }
    var matched []collector.MetricPoint
    for _, p := range candidates {
        if labelsMatch(rule.Labels, p.Labels) {
            matched = append(matched, p)
        }
    }
    return matched
}

// labelsMatch returns true if all required labels are present in point labels with matching values.
func labelsMatch(required, actual map[string]string) bool {
    for k, v := range required {
        if actual[k] != v {
            return false
        }
    }
    return true
}
```

### Design Notes — Evaluator

1. **Mutex scope**: The mutex protects `rules` and `state`. Evaluate() copies the rules slice reference under lock, then works lock-free. State updates happen under the same Evaluate() call — since the orchestrator calls Evaluate() per-instance sequentially within an interval group, there's no contention for the same instance. The mutex protects against concurrent LoadRules() or RestoreState().

2. **Cooldown in evaluator vs dispatcher**: The evaluator tracks state transitions and emits AlertEvents. Cooldown for repeat notifications is the dispatcher's responsibility (M4_02). The evaluator always returns a fire event on the first FIRING transition and a resolution event when resolved. It does NOT re-emit fire events on subsequent evaluations while in FIRING state — that's the dispatcher's "heartbeat/reminder" feature if desired.

3. **Missing metrics**: If a rule matches no metrics in a cycle (collector didn't produce that metric), the state is unchanged. This handles partial collector failures without false resolutions.

4. **Label-keyed state**: For rules with label filters (e.g. per-replication-slot), each unique label set gets its own state entry. Rule 9 (replication_slot_inactive) will track state per slot_name.

---

## 4. Store Interfaces (`internal/alert/store.go`)

```go
package alert

import (
    "context"
    "time"
)

// AlertRuleStore manages persistent alert rule storage.
type AlertRuleStore interface {
    // List returns all rules (enabled and disabled).
    List(ctx context.Context) ([]Rule, error)

    // ListEnabled returns only enabled rules.
    ListEnabled(ctx context.Context) ([]Rule, error)

    // Get returns a single rule by ID.
    Get(ctx context.Context, id string) (*Rule, error)

    // Create inserts a new rule. Returns error if ID already exists.
    Create(ctx context.Context, rule *Rule) error

    // Update modifies an existing rule.
    Update(ctx context.Context, rule *Rule) error

    // Delete removes a rule by ID.
    Delete(ctx context.Context, id string) error

    // UpsertBuiltin inserts or updates a builtin rule, preserving user-modified fields.
    // Specifically: updates name, description, metric, operator, source
    // but preserves: threshold, consecutive_count, cooldown_minutes, enabled, channels
    // if the rule already exists.
    UpsertBuiltin(ctx context.Context, rule *Rule) error
}

// AlertHistoryStore manages alert event history.
type AlertHistoryStore interface {
    // Record stores a new alert firing event.
    Record(ctx context.Context, event *AlertEvent) error

    // Resolve marks an existing unresolved alert as resolved.
    Resolve(ctx context.Context, ruleID, instanceID string, resolvedAt time.Time) error

    // ListUnresolved returns all currently firing (unresolved) alerts.
    ListUnresolved(ctx context.Context) ([]AlertEvent, error)

    // Query returns alert history filtered by parameters.
    Query(ctx context.Context, q AlertHistoryQuery) ([]AlertEvent, error)

    // Cleanup deletes resolved alerts older than the given duration.
    Cleanup(ctx context.Context, olderThan time.Duration) (int64, error)
}

// AlertHistoryQuery defines filters for querying alert history.
type AlertHistoryQuery struct {
    InstanceID   string
    RuleID       string
    Severity     Severity
    Start        time.Time
    End          time.Time
    UnresolvedOnly bool
    Limit        int
}
```

---

## 5. PG Store Implementation (`internal/alert/pgstore.go`)

### alert_rules table schema

```sql
-- in 004_alerts.sql

CREATE TABLE IF NOT EXISTS alert_rules (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    metric           TEXT NOT NULL,
    operator         TEXT NOT NULL CHECK (operator IN ('>', '>=', '<', '<=', '==', '!=')),
    threshold        DOUBLE PRECISION NOT NULL,
    severity         TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    labels           JSONB NOT NULL DEFAULT '{}',
    consecutive_count INTEGER NOT NULL DEFAULT 3,
    cooldown_minutes  INTEGER NOT NULL DEFAULT 15,
    channels         JSONB NOT NULL DEFAULT '[]',
    source           TEXT NOT NULL CHECK (source IN ('builtin', 'custom')),
    enabled          BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_history (
    id           BIGSERIAL PRIMARY KEY,
    rule_id      TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    instance_id  TEXT NOT NULL,
    severity     TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    metric       TEXT NOT NULL,
    value        DOUBLE PRECISION NOT NULL,
    threshold    DOUBLE PRECISION NOT NULL,
    operator     TEXT NOT NULL,
    labels       JSONB NOT NULL DEFAULT '{}',
    fired_at     TIMESTAMPTZ NOT NULL,
    resolved_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_history_unresolved
    ON alert_history (rule_id, instance_id) WHERE resolved_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_alert_history_fired_at
    ON alert_history (fired_at DESC);

CREATE INDEX IF NOT EXISTS idx_alert_history_instance
    ON alert_history (instance_id, fired_at DESC);
```

### PGAlertRuleStore

```go
// internal/alert/pgstore.go

package alert

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

// PGAlertRuleStore implements AlertRuleStore using PostgreSQL.
type PGAlertRuleStore struct {
    pool *pgxpool.Pool
}

func NewPGAlertRuleStore(pool *pgxpool.Pool) *PGAlertRuleStore {
    return &PGAlertRuleStore{pool: pool}
}
```

Key implementation details:

- **UpsertBuiltin**: Uses `INSERT ... ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description, metric=EXCLUDED.metric, operator=EXCLUDED.operator WHERE alert_rules.source='builtin'`. This preserves user-modified thresholds while updating rule definitions from code.
- **Labels/Channels**: Stored as JSONB. Marshal/unmarshal via `encoding/json`.
- **scanRule helper**: Scans a `pgx.Row` into a `Rule` struct, handling JSONB deserialization.

### PGAlertHistoryStore

```go
type PGAlertHistoryStore struct {
    pool *pgxpool.Pool
}

func NewPGAlertHistoryStore(pool *pgxpool.Pool) *PGAlertHistoryStore {
    return &PGAlertHistoryStore{pool: pool}
}
```

Key implementation details:

- **Record**: `INSERT INTO alert_history (rule_id, instance_id, severity, metric, value, threshold, operator, labels, fired_at)`
- **Resolve**: `UPDATE alert_history SET resolved_at = $3 WHERE rule_id = $1 AND instance_id = $2 AND resolved_at IS NULL`
- **ListUnresolved**: `SELECT ... FROM alert_history WHERE resolved_at IS NULL`
- **Query**: Dynamic WHERE construction using the same `buildQuery()` pattern from PGStore (but simpler — fewer filter dimensions)
- **Cleanup**: `DELETE FROM alert_history WHERE resolved_at IS NOT NULL AND resolved_at < now() - $1::interval`

---

## 6. Builtin Rules (`internal/alert/rules.go`)

```go
package alert

// BuiltinRules returns the default set of alert rules shipped with PGPulse.
// These are seeded to the database on startup via SeedBuiltinRules().
func BuiltinRules() []Rule {
    return []Rule{
        // --- Ported from PGAM (14 rules) ---
        {
            ID: "wraparound_warning", Name: "Transaction ID Wraparound Warning",
            Metric: "pgpulse.databases.wraparound_pct", Operator: OpGreater, Threshold: 20,
            Severity: SeverityWarning, Source: SourceBuiltin, Enabled: true,
            ConsecutiveCount: 3, CooldownMinutes: 15,
            Description: "Transaction ID wraparound approaching dangerous levels. Consider running VACUUM FREEZE on affected databases.",
        },
        {
            ID: "wraparound_critical", Name: "Transaction ID Wraparound Critical",
            Metric: "pgpulse.databases.wraparound_pct", Operator: OpGreater, Threshold: 50,
            Severity: SeverityCritical, Source: SourceBuiltin, Enabled: true,
            ConsecutiveCount: 1, CooldownMinutes: 5,
            Description: "Transaction ID wraparound at critical level. Immediate VACUUM FREEZE required to prevent database shutdown.",
        },
        // ... (remaining 18 rules follow same pattern)
        // Full list in requirements §FR-6

        // --- Deferred rules (defined but disabled) ---
        {
            ID: "wal_spike_warning", Name: "WAL Generation Spike",
            Metric: "pgpulse.wal.spike_ratio", Operator: OpGreater, Threshold: 3,
            Severity: SeverityWarning, Source: SourceBuiltin, Enabled: false,
            ConsecutiveCount: 3, CooldownMinutes: 30,
            Description: "WAL generation rate exceeds 3x baseline. Requires ML baseline (M8).",
        },
        // ... disk_forecast_critical, query_regression_warning (enabled: false)
    }
}
```

Note on **rule 17 (connection_pool_warning)**: After review, this overlaps with rule 5
(connections_warning at 80%) and rule 6 (connections_critical at 99%). Remove rule 17
and keep the PGAM pair. The "pool saturation" concept applies when PGPulse monitors
pgBouncer pools (future scope). Final count: **19 rules** (14 PGAM + 2 new replication + 3 deferred).

---

## 7. Seeding Logic (`internal/alert/seed.go`)

```go
package alert

import (
    "context"
    "fmt"
    "log/slog"
)

// SeedBuiltinRules upserts all builtin rules to the database.
// Safe to call on every startup — preserves user modifications to thresholds.
func SeedBuiltinRules(ctx context.Context, store AlertRuleStore, logger *slog.Logger) error {
    rules := BuiltinRules()
    var created, updated int
    for _, rule := range rules {
        if err := store.UpsertBuiltin(ctx, &rule); err != nil {
            return fmt.Errorf("seed rule %s: %w", rule.ID, err)
        }
        // UpsertBuiltin returns whether it was insert vs update (or we count separately)
    }
    logger.Info("builtin alert rules seeded", "total", len(rules), "created", created, "updated", updated)
    return nil
}
```

Called from `main.go` after migration, before starting orchestrator:

```go
// In main.go, after storage.Migrate():
if cfg.Alerting.Enabled {
    ruleStore := alert.NewPGAlertRuleStore(pool)
    historyStore := alert.NewPGAlertHistoryStore(pool)
    if err := alert.SeedBuiltinRules(ctx, ruleStore, logger); err != nil {
        logger.Error("failed to seed alert rules", "error", err)
        os.Exit(1)
    }
    evaluator := alert.NewEvaluator(ruleStore, historyStore, logger)
    if err := evaluator.LoadRules(ctx); err != nil { ... }
    if err := evaluator.RestoreState(ctx); err != nil { ... }
    // evaluator passed to orchestrator in M4_03
}
```

---

## 8. Config Addition (`internal/config/config.go`)

```go
// Add to Config struct:
type AlertingConfig struct {
    Enabled               bool   `koanf:"enabled"`
    DefaultConsecutiveCount int  `koanf:"default_consecutive_count"`
    DefaultCooldownMinutes  int  `koanf:"default_cooldown_minutes"`
    EvaluationTimeoutSec    int  `koanf:"evaluation_timeout_seconds"`
    HistoryRetentionDays    int  `koanf:"history_retention_days"`
}

// Add to Config:
type Config struct {
    Server   ServerConfig   `koanf:"server"`
    Storage  StorageConfig  `koanf:"storage"`
    Auth     AuthConfig     `koanf:"auth"`
    Alerting AlertingConfig `koanf:"alerting"`  // NEW
    Instances []InstanceConfig `koanf:"instances"`
}
```

Default values (set in validate/defaults):

```go
func alertingDefaults(c *AlertingConfig) {
    if c.DefaultConsecutiveCount == 0 {
        c.DefaultConsecutiveCount = 3
    }
    if c.DefaultCooldownMinutes == 0 {
        c.DefaultCooldownMinutes = 15
    }
    if c.EvaluationTimeoutSec == 0 {
        c.EvaluationTimeoutSec = 5
    }
    if c.HistoryRetentionDays == 0 {
        c.HistoryRetentionDays = 30
    }
}
```

Validation: `alerting.enabled=true` requires `storage.dsn` to be set (same pattern as auth).

Example YAML:

```yaml
alerting:
  enabled: true
  default_consecutive_count: 3
  default_cooldown_minutes: 15
  evaluation_timeout_seconds: 5
  history_retention_days: 30
```

---

## 9. Migration (`internal/storage/migrations/004_alerts.sql`)

```sql
-- Alert rules: stores both builtin and custom rules
CREATE TABLE IF NOT EXISTS alert_rules (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    metric            TEXT NOT NULL,
    operator          TEXT NOT NULL CHECK (operator IN ('>', '>=', '<', '<=', '==', '!=')),
    threshold         DOUBLE PRECISION NOT NULL,
    severity          TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    labels            JSONB NOT NULL DEFAULT '{}',
    consecutive_count INTEGER NOT NULL DEFAULT 3,
    cooldown_minutes  INTEGER NOT NULL DEFAULT 15,
    channels          JSONB NOT NULL DEFAULT '[]',
    source            TEXT NOT NULL CHECK (source IN ('builtin', 'custom')),
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Alert history: records fired and resolved events
CREATE TABLE IF NOT EXISTS alert_history (
    id            BIGSERIAL PRIMARY KEY,
    rule_id       TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    instance_id   TEXT NOT NULL,
    severity      TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    metric        TEXT NOT NULL,
    value         DOUBLE PRECISION NOT NULL,
    threshold     DOUBLE PRECISION NOT NULL,
    operator      TEXT NOT NULL,
    labels        JSONB NOT NULL DEFAULT '{}',
    fired_at      TIMESTAMPTZ NOT NULL,
    resolved_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fast lookup for unresolved alerts (state restoration on startup)
CREATE INDEX IF NOT EXISTS idx_alert_history_unresolved
    ON alert_history (rule_id, instance_id) WHERE resolved_at IS NULL;

-- Time-based queries for alert history API
CREATE INDEX IF NOT EXISTS idx_alert_history_fired_at
    ON alert_history (fired_at DESC);

-- Per-instance history queries
CREATE INDEX IF NOT EXISTS idx_alert_history_instance
    ON alert_history (instance_id, fired_at DESC);
```

---

## 10. Dependency Graph

```
internal/alert/alert.go          ← no imports (pure data model)
internal/alert/rules.go          ← imports alert.go only
internal/alert/store.go          ← imports alert.go only (interfaces)
internal/alert/pgstore.go        ← imports store.go, alert.go, pgx/pgxpool
internal/alert/seed.go           ← imports store.go, rules.go, slog
internal/alert/evaluator.go      ← imports alert.go, store.go, collector.MetricPoint, slog

internal/config/config.go        ← adds AlertingConfig (no new imports)
internal/storage/migrations/     ← 004_alerts.sql (embedded)
```

The `alert` package imports from `collector` only for the `MetricPoint` type.
It does NOT import from `api`, `auth`, `orchestrator`, or `storage` (except pgxpool for its own stores).
This keeps the dependency direction clean: orchestrator → alert → collector (types only).

---

## 11. Test Plan

### Unit Tests (no DB required)

| File | Test | What it validates |
|------|------|-------------------|
| `alert_test.go` | TestOperatorCompare | All 6 operators with edge cases (equal values, zero, negative) |
| `alert_test.go` | TestSeverityLevel | Ordering: info < warning < critical |
| `evaluator_test.go` | TestEvaluate_OKToFiring | Metric breaches threshold for consecutive_count cycles → FIRING event |
| `evaluator_test.go` | TestEvaluate_PendingResetOnOK | Breach then OK resets counter, no event |
| `evaluator_test.go` | TestEvaluate_FiringToOK | Resolution emits event with IsResolution=true |
| `evaluator_test.go` | TestEvaluate_Hysteresis | Counter increments correctly, fires at exact threshold |
| `evaluator_test.go` | TestEvaluate_ConsecutiveCountOne | Fires immediately when consecutive_count=1 |
| `evaluator_test.go` | TestEvaluate_NoMatchingMetrics | No events, state unchanged |
| `evaluator_test.go` | TestEvaluate_LabelFiltering | Only metrics with matching labels trigger rule |
| `evaluator_test.go` | TestEvaluate_MultipleRules | Two rules on same metric (warning + critical) |
| `evaluator_test.go` | TestLabelsMatch | Required labels subset of actual labels |
| `rules_test.go` | TestBuiltinRulesValid | All rules have required fields, no duplicate IDs |
| `rules_test.go` | TestBuiltinRulesCount | Expected count (19) |
| `rules_test.go` | TestDeferredRulesDisabled | WAL spike, query regression, disk forecast are enabled=false |

### Store Tests (mock or integration)

| File | Test | What it validates |
|------|------|-------------------|
| `pgstore_test.go` | TestPGAlertRuleStore_CRUD | Create, Get, List, Update, Delete |
| `pgstore_test.go` | TestPGAlertRuleStore_UpsertBuiltin | Insert new + update existing preserving threshold |
| `pgstore_test.go` | TestPGAlertRuleStore_ListEnabled | Returns only enabled rules |
| `pgstore_test.go` | TestPGAlertHistoryStore_RecordAndResolve | Record firing, resolve, verify timestamps |
| `pgstore_test.go` | TestPGAlertHistoryStore_ListUnresolved | Returns only unresolved events |
| `pgstore_test.go` | TestPGAlertHistoryStore_Cleanup | Deletes resolved events older than retention |
| `seed_test.go` | TestSeedBuiltinRules | Seeds all rules, idempotent on re-run |

Note: pgstore integration tests require Docker (CI-only). Unit tests for evaluator use
mock stores (in-memory map implementations in test helpers).

### Test Helpers

```go
// internal/alert/evaluator_test.go

// mockRuleStore implements AlertRuleStore for testing.
type mockRuleStore struct {
    rules []Rule
}

func (m *mockRuleStore) ListEnabled(ctx context.Context) ([]Rule, error) {
    var enabled []Rule
    for _, r := range m.rules {
        if r.Enabled {
            enabled = append(enabled, r)
        }
    }
    return enabled, nil
}
// ... other methods

// mockHistoryStore implements AlertHistoryStore for testing.
type mockHistoryStore struct {
    events []AlertEvent
}
// ... Record, Resolve, ListUnresolved, Query, Cleanup
```

---

## 12. Files to Create/Modify Summary

### New Files

| File | Lines (est.) | Owner |
|------|-------------|-------|
| `internal/alert/alert.go` | ~120 | Domain model |
| `internal/alert/evaluator.go` | ~250 | Core engine |
| `internal/alert/rules.go` | ~200 | 19 rule definitions |
| `internal/alert/store.go` | ~60 | Interfaces |
| `internal/alert/pgstore.go` | ~300 | PG implementations |
| `internal/alert/seed.go` | ~40 | Startup seeding |
| `internal/storage/migrations/004_alerts.sql` | ~40 | Schema |
| `internal/alert/alert_test.go` | ~80 | Data model tests |
| `internal/alert/evaluator_test.go` | ~350 | State machine tests |
| `internal/alert/rules_test.go` | ~60 | Validation tests |
| `internal/alert/pgstore_test.go` | ~200 | Store tests |
| `internal/alert/seed_test.go` | ~80 | Seeding tests |

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add AlertingConfig struct, add to Config |
| `internal/config/load.go` | Add alertingDefaults(), validateAlerting() |
| `internal/config/config_test.go` | Add test for alerting config validation |
| `configs/pgpulse.example.yml` | Add alerting section |
| `internal/storage/migrate.go` | Embed 004_alerts.sql (automatic via go:embed glob) |

### NOT Modified (M4_02 / M4_03)

| File | Why not |
|------|---------|
| `cmd/pgpulse-server/main.go` | Wiring happens in M4_03 |
| `internal/orchestrator/*` | Post-collect hook in M4_03 |
| `internal/api/*` | Alert API endpoints in M4_03 |
