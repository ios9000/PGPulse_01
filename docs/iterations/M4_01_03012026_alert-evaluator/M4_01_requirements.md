# M4_01 Requirements — Alert Evaluator & Rules Engine

**Iteration:** M4_01
**Milestone:** M4 — Alerting
**Date:** 2026-03-01
**Depends on:** M3_01 (auth), M2_02 (storage layer)

---

## Goal

Build the core alert evaluation engine: data model, state machine, threshold comparison,
hysteresis, cooldown tracking, rule definitions (20 rules), and persistent storage for
rules and alert history. This iteration produces the domain logic only — no notifiers,
no API endpoints, no orchestrator wiring.

## Scope

### In Scope

1. **Alert data model** — Rule, AlertEvent, Severity, Operator, AlertState types
2. **Evaluator engine** — accepts metric points, evaluates against rules, manages state machine
3. **State machine** — per rule+instance: OK → FIRING (WARNING or CRITICAL). Transitions on threshold breach/recovery
4. **Hysteresis** — configurable consecutive violation count before state transition (default: 3)
5. **Cooldown** — suppress repeat notifications for same rule+instance+severity (default: 15 min). State transitions (severity change or resolution) always notify immediately regardless of cooldown
6. **20 alert rule definitions** — 14 ported from PGAM thresholds + 3 new implementable + 3 deferred (defined but disabled)
7. **DB migration** — `alert_rules` table + `alert_history` table
8. **AlertRuleStore** — interface + PG implementation for rule CRUD (list, get, create, update, delete, upsert-builtins)
9. **AlertHistoryStore** — interface + PG implementation (record event, resolve event, query history, cleanup old)
10. **Builtin rule seeding** — on startup, upsert all builtin rules from code to DB (preserving user modifications to thresholds)
11. **Config section** — `alerting:` block in pgpulse.yml with defaults (hysteresis, cooldown, enabled flag)
12. **Unit tests** — evaluator state machine, hysteresis, cooldown, rule matching, store operations

### Out of Scope (M4_02 / M4_03)

- Email notifier / SMTP sender → M4_02
- Dispatcher (routing alerts to channels) → M4_02
- REST API endpoints for alerts → M4_03
- Orchestrator wiring (post-collect evaluation hook) → M4_03
- Telegram, Slack, Webhook notifiers → M7+
- Corporate messenger integration → M7+

## Functional Requirements

### FR-1: Alert Rule Model

A rule defines a threshold check against a named metric:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | string | auto | Unique identifier (slug format: `wraparound_warning`) |
| name | string | yes | Human-readable name |
| metric | string | yes | Metric name to match (e.g. `pgpulse.databases.wraparound_pct`) |
| operator | enum | yes | Comparison: `>`, `>=`, `<`, `<=`, `==`, `!=` |
| threshold | float64 | yes | Value to compare against |
| severity | enum | yes | `info`, `warning`, `critical` |
| labels | map | no | Optional label filter (metric must have ALL specified labels to match) |
| consecutive_count | int | yes | Violations before firing (default from config, typically 3) |
| cooldown_minutes | int | yes | Re-notification suppression (default from config, typically 15) |
| channels | []string | no | Override global notification channels (empty = use defaults) |
| source | enum | yes | `builtin` (seeded from code) or `custom` (user-created) |
| enabled | bool | yes | Active/inactive toggle |
| description | string | no | Explanation of what this rule detects and suggested remediation |

### FR-2: Evaluator State Machine

Per unique (rule_id, instance_id) combination:

```
         threshold breached              threshold breached
         count < consecutive       count >= consecutive
    ┌──────────────┐            ┌──────────────┐
    │              │            │              │
    ▼              │            ▼              │
 ┌──────┐   breach  ┌────────────┐  breach  ┌─────────┐
 │  OK  │ ────────► │ PENDING    │ ───────► │ FIRING  │
 │      │           │ (counting) │          │         │
 └──┬───┘           └─────┬──────┘          └────┬────┘
    │                     │                      │
    │    metric OK        │  metric OK           │  metric OK
    │    (reset count)    │  (reset count)       │  (consecutive OK)
    │                     │                      │
    └─────────────────────┘                      │
    ▲                                            │
    │              resolved                      │
    └────────────────────────────────────────────┘
```

- **OK**: No violation. Counter = 0.
- **PENDING**: Breach detected but consecutive count not yet met. Counter increments.
- **FIRING**: Alert is active. Notification sent (subject to cooldown). Recorded in alert_history.
- **Resolution**: When metric returns to OK while in FIRING state, mark resolved in alert_history, notify immediately (resolution notification ignores cooldown).

A single metric OK resets the pending counter. To return from FIRING to OK, the metric
must be OK for 1 consecutive evaluation (no hysteresis on recovery — immediate resolution).

### FR-3: Hysteresis

- Default consecutive count: 3 (configurable globally in config, overridable per rule)
- Counter increments on each evaluation cycle where the metric breaches the threshold
- Counter resets to 0 on any evaluation where the metric is within bounds
- Alert only fires (transitions to FIRING) when counter reaches consecutive_count

### FR-4: Cooldown

- Default cooldown: 15 minutes (configurable globally, overridable per rule)
- After a notification is sent for a FIRING alert, suppress identical notifications for the cooldown period
- Cooldown applies per (rule_id, instance_id, severity) tuple
- These events ALWAYS notify immediately regardless of cooldown:
  - Severity escalation (e.g. WARNING → CRITICAL)
  - Resolution (FIRING → OK)

### FR-5: Rule Evaluation

The evaluator receives a batch of MetricPoints (from a collector cycle) and:

1. For each enabled rule, find matching metrics (by metric name + label filter)
2. For each matching metric, run the operator comparison against the threshold
3. Update the state machine for that (rule, instance) pair
4. Return a list of AlertEvents that need notification (new fires + resolutions)

### FR-6: Builtin Rules (20 total)

#### Ported from PGAM (14 rules)

| # | Rule ID | Metric | Op | Threshold | Severity | Notes |
|---|---------|--------|----|-----------|----------|-------|
| 1 | `wraparound_warning` | `pgpulse.databases.wraparound_pct` | > | 20 | warning | |
| 2 | `wraparound_critical` | `pgpulse.databases.wraparound_pct` | > | 50 | critical | |
| 3 | `multixact_warning` | `pgpulse.databases.multixact_pct` | > | 20 | warning | |
| 4 | `multixact_critical` | `pgpulse.databases.multixact_pct` | > | 50 | critical | |
| 5 | `connections_warning` | `pgpulse.connections.utilization_pct` | > | 80 | warning | |
| 6 | `connections_critical` | `pgpulse.connections.utilization_pct` | >= | 99 | critical | |
| 7 | `cache_hit_warning` | `pgpulse.cache.hit_ratio` | < | 90 | warning | |
| 8 | `commit_ratio_warning` | `pgpulse.transactions.commit_ratio` | < | 90 | warning | |
| 9 | `replication_slot_inactive` | `pgpulse.replication.slot_active` | == | 0 | critical | label filter: checks per slot |
| 10 | `long_tx_warning` | `pgpulse.transactions.longest_active_seconds` | > | 60 | warning | |
| 11 | `long_tx_critical` | `pgpulse.transactions.longest_active_seconds` | >= | 300 | critical | |
| 12 | `bloat_warning` | `pgpulse.tables.bloat_ratio` | > | 2 | warning | Per-DB metric, deferred source |
| 13 | `bloat_critical` | `pgpulse.tables.bloat_ratio` | > | 50 | critical | Per-DB metric, deferred source |
| 14 | `pgss_fill_warning` | `pgpulse.statements.fill_pct` | >= | 95 | warning | |

Note: PGAM rules for `stats_reset_age`, `track_io_timing=off`, `object_size`, `system_catalog_size`,
`autovacuum_disabled`, and `event_trigger_disabled` are INFO-level visual indicators in PGAM, not
threshold alerts. They can be added as INFO rules later if desired. The 14 above are the
actionable threshold-based alerts.

#### New rules — implementable now (3)

| # | Rule ID | Metric | Op | Threshold | Severity |
|---|---------|--------|----|-----------|----------|
| 15 | `replication_lag_warning` | `pgpulse.replication.lag_bytes` | > | 1048576 (1MB) | warning |
| 16 | `replication_lag_critical` | `pgpulse.replication.lag_bytes` | > | 104857600 (100MB) | critical |
| 17 | `connection_pool_warning` | `pgpulse.connections.utilization_pct` | > | 90 | warning |

Note: Rule 17 overlaps with rule 5 but at a higher threshold — decide during design whether
to merge or keep separate (pool saturation vs. general connection warning).

#### New rules — deferred data source (3, defined but enabled=false)

| # | Rule ID | Metric | Op | Threshold | Severity | Why deferred |
|---|---------|--------|----|-----------|----------|-------------|
| 18 | `wal_spike_warning` | `pgpulse.wal.spike_ratio` | > | 3 | warning | Needs baseline (M8 ML) |
| 19 | `query_regression_warning` | `pgpulse.statements.regression_ratio` | > | 2 | warning | Needs historical comparison (M7/M8) |
| 20 | `disk_forecast_critical` | `pgpulse.os.disk_days_remaining` | < | 7 | critical | Needs OS metrics (M6) + regression (M8) |

### FR-7: Alert Rule Storage (DB)

- `alert_rules` table stores all rules (builtin + custom)
- `alert_history` table stores fired/resolved events
- AlertRuleStore interface with CRUD operations
- AlertHistoryStore interface with record/resolve/query/cleanup operations

### FR-8: Builtin Rule Seeding

On startup (when storage is available):
1. Load builtin rules from code (hardcoded Go structs, not YAML file)
2. For each builtin rule, upsert to DB:
   - If rule doesn't exist → insert
   - If rule exists with source="builtin" → update definition (name, metric, operator, description) BUT preserve user-modified fields (threshold, consecutive_count, cooldown_minutes, enabled, channels)
   - If rule exists with source="custom" and same ID → skip (user override takes precedence)
3. Log count of seeded/updated rules

### FR-9: Config

```yaml
alerting:
  enabled: true                    # master switch
  default_consecutive_count: 3     # hysteresis default
  default_cooldown_minutes: 15     # cooldown default
  default_channels: []             # notification channels (populated in M4_02)
  evaluation_timeout_seconds: 5    # max time for one evaluation cycle
```

### FR-10: Evaluator Interface

```go
// Called by orchestrator (M4_03) after each collect cycle
type Evaluator interface {
    Evaluate(ctx context.Context, instanceID string, points []MetricPoint) ([]AlertEvent, error)
    LoadRules(ctx context.Context) error      // refresh rules from DB
    RestoreState(ctx context.Context) error   // seed state from unresolved alerts on startup
}
```

## Non-Functional Requirements

- Evaluator.Evaluate() must complete in < 100ms for typical rule set (20 rules × 100 metrics)
- State machine must be goroutine-safe (evaluator may be called from multiple instance runners)
- All DB operations use parameterized queries (pgx)
- Alert history cleanup: configurable retention (default 30 days), run periodically
- Zero external dependencies beyond pgx (no alert framework libraries)

## Test Requirements

- State machine: test all transitions (OK→PENDING→FIRING→OK, direct OK→FIRING when consecutive=1)
- Hysteresis: test counter increment, reset on OK, fire on threshold
- Cooldown: test suppression, test immediate notify on severity change and resolution
- Rule matching: test metric name match, label filter match, operator comparisons
- Builtin seeding: test upsert behavior (new rule, existing rule, user-modified threshold preserved)
- AlertRuleStore: CRUD operations (unit tests with mock, integration with testcontainers if possible)
- AlertHistoryStore: record, resolve, query by time range, cleanup

## Deliverables

| File | Description |
|------|-------------|
| `internal/alert/alert.go` | Data model: Rule, AlertEvent, Severity, Operator, AlertState |
| `internal/alert/evaluator.go` | Evaluator implementation with state machine |
| `internal/alert/rules.go` | Builtin rule definitions (20 rules) |
| `internal/alert/store.go` | AlertRuleStore + AlertHistoryStore interfaces |
| `internal/alert/pgstore.go` | PG implementation of both stores |
| `internal/alert/seed.go` | Builtin rule seeding logic |
| `internal/storage/migrations/004_alerts.sql` | alert_rules + alert_history tables |
| `internal/config/config.go` | Add AlertingConfig struct |
| `internal/alert/alert_test.go` | Data model tests |
| `internal/alert/evaluator_test.go` | State machine, hysteresis, cooldown tests |
| `internal/alert/rules_test.go` | Builtin rule definition validation tests |
| `internal/alert/pgstore_test.go` | Store integration tests |
| `internal/alert/seed_test.go` | Seeding logic tests |
