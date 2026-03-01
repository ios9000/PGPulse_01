# M4_03 Requirements — Alert API, Orchestrator Wiring & Integration

**Iteration:** M4_03
**Milestone:** M4 — Alerting (final iteration)
**Date:** 2026-03-01
**Depends on:** M4_01 (evaluator, rules, stores), M4_02 (email notifier, dispatcher, templates)

---

## Goal

Wire all alert components together: hook the evaluator into the orchestrator's post-collect
cycle, expose alert REST API endpoints for rule management and alert history, integrate
everything in main.go with proper startup/shutdown ordering, and add periodic alert history
cleanup. After M4_03, the complete alerting pipeline is operational end-to-end.

## Scope

### In Scope

1. **Orchestrator wiring** — after each collector group writes metrics, call evaluator.Evaluate() and pipe AlertEvents to dispatcher.Dispatch()
2. **Alert API endpoints** (6 new endpoints):
   - GET /api/v1/alerts — list active (unresolved) alerts
   - GET /api/v1/alerts/history — query alert history with filters
   - GET /api/v1/alerts/rules — list all alert rules
   - POST /api/v1/alerts/rules — create custom rule (admin only)
   - PUT /api/v1/alerts/rules/{id} — update rule (admin only)
   - DELETE /api/v1/alerts/rules/{id} — delete custom rule (admin only)
   - POST /api/v1/alerts/test — send test notification (admin only)
3. **main.go integration** — wire evaluator, dispatcher, email notifier, rule seeding, pass to orchestrator and API server
4. **Graceful shutdown ordering** — HTTP server → Orchestrator → Dispatcher → Store
5. **History cleanup** — periodic goroutine in evaluator or standalone, delete resolved alerts older than retention
6. **Config validation** — alerting.enabled requires storage.dsn (already done in M4_01, verify still holds)
7. **Tests** — API handler tests, orchestrator integration test, cleanup test

### Out of Scope

- Frontend UI for alerts → M5
- Additional notifier channels → M7+
- Alert rule import/export → M7+
- Alert acknowledgment/silencing → M7+
- Prometheus /metrics exposition of alert counts → M7+

## Functional Requirements

### FR-1: Orchestrator Post-Collect Hook

After each interval group completes its collector cycle and writes metrics to storage,
the orchestrator calls the evaluator:

```
intervalGroup.run()
  → for each collector: Collect() → points
  → store.Write(points)
  → if evaluator != nil:
      events, err := evaluator.Evaluate(ctx, instanceID, points)
      → for each event: dispatcher.Dispatch(event)
```

The evaluator is optional — when alerting is disabled (evaluator=nil), the orchestrator
works exactly as before (zero behavioral change to existing code paths).

Design constraints:
- Evaluator runs synchronously within the collect cycle (it's fast — just threshold comparisons)
- Evaluation errors are logged but do not abort the collect cycle
- Dispatcher.Dispatch() is non-blocking (already designed in M4_02)
- Points passed to evaluator are the same []MetricPoint batch from collectors (no re-query)

### FR-2: Alert API Endpoints

All endpoints under `/api/v1/alerts/`. Auth follows existing patterns (viewer+ for reads, admin for writes).

#### GET /api/v1/alerts

List currently active (unresolved) alerts.

Response:
```json
{
  "data": [
    {
      "rule_id": "wraparound_critical",
      "rule_name": "Transaction ID Wraparound Critical",
      "instance_id": "prod-db-01",
      "severity": "critical",
      "metric": "pgpulse.databases.wraparound_pct",
      "value": 55.3,
      "threshold": 50,
      "operator": ">",
      "fired_at": "2026-03-01T14:30:00Z",
      "labels": {"database": "mydb"}
    }
  ],
  "meta": {"count": 1}
}
```

Auth: viewer+

#### GET /api/v1/alerts/history

Query alert history with optional filters.

Query params:
- `instance_id` — filter by instance
- `rule_id` — filter by rule
- `severity` — filter by severity (info, warning, critical)
- `start` — ISO 8601 datetime (fired_at >= start)
- `end` — ISO 8601 datetime (fired_at <= end)
- `unresolved` — boolean, if true only unresolved alerts
- `limit` — max results (default 100, max 1000)

Response: same envelope as GET /alerts but with resolved_at field present for resolved alerts.

Auth: viewer+

#### GET /api/v1/alerts/rules

List all alert rules (enabled and disabled, builtin and custom).

Response:
```json
{
  "data": [
    {
      "id": "wraparound_warning",
      "name": "Transaction ID Wraparound Warning",
      "description": "...",
      "metric": "pgpulse.databases.wraparound_pct",
      "operator": ">",
      "threshold": 20,
      "severity": "warning",
      "labels": {},
      "consecutive_count": 3,
      "cooldown_minutes": 15,
      "channels": [],
      "source": "builtin",
      "enabled": true
    }
  ],
  "meta": {"count": 19}
}
```

Auth: viewer+

#### POST /api/v1/alerts/rules

Create a custom alert rule.

Request body:
```json
{
  "id": "custom_disk_usage",
  "name": "Disk Usage Warning",
  "description": "Alert when disk usage exceeds threshold",
  "metric": "pgpulse.os.disk_usage_pct",
  "operator": ">",
  "threshold": 85,
  "severity": "warning",
  "labels": {},
  "consecutive_count": 3,
  "cooldown_minutes": 15,
  "channels": ["email"],
  "enabled": true
}
```

Validation:
- id: required, slug format (lowercase, hyphens/underscores, no spaces)
- name: required
- metric: required
- operator: must be one of >, >=, <, <=, ==, !=
- threshold: required (any float64)
- severity: must be info, warning, or critical
- consecutive_count: positive integer, defaults to config default
- cooldown_minutes: positive integer, defaults to config default
- source: automatically set to "custom" (cannot create builtin via API)

Response: 201 Created with the created rule.

Auth: admin only

#### PUT /api/v1/alerts/rules/{id}

Update an existing rule (builtin or custom). For builtin rules, this modifies the
user-overridable fields (threshold, consecutive_count, cooldown_minutes, enabled, channels).
For custom rules, all fields are modifiable.

After successful update, call evaluator.LoadRules() to refresh cached rules.

Response: 200 OK with the updated rule.

Auth: admin only

#### DELETE /api/v1/alerts/rules/{id}

Delete a rule. Only custom rules can be deleted. Attempting to delete a builtin rule
returns 409 Conflict with error message.

After successful delete, call evaluator.LoadRules() to refresh cached rules.

Response: 204 No Content.

Auth: admin only

#### POST /api/v1/alerts/test

Send a test notification through configured channels. Useful for verifying email setup.

Request body:
```json
{
  "channel": "email",
  "message": "Test alert from PGPulse"
}
```

Creates a synthetic AlertEvent (severity=info, metric="pgpulse.test", value=0) and
sends it through the specified notifier directly (bypasses evaluator and dispatcher).

Response: 200 OK with `{"data": {"sent": true, "channel": "email"}}` or error details.

Auth: admin only

### FR-3: main.go Integration

Update main.go startup sequence:

```
main.go
  → config.Load(path)
  → storage setup (existing)
  → migration (existing — now includes 004_alerts.sql)
  → auth setup (existing)
  → if cfg.Alerting.Enabled && cfg.Storage.DSN != "":
      alertRuleStore := alert.NewPGAlertRuleStore(pool)
      alertHistoryStore := alert.NewPGAlertHistoryStore(pool)
      alert.SeedBuiltinRules(ctx, alertRuleStore, logger)
      evaluator := alert.NewEvaluator(alertRuleStore, alertHistoryStore, logger)
      evaluator.LoadRules(ctx)
      evaluator.RestoreState(ctx)
      registry := alert.NewNotifierRegistry()
      if cfg.Alerting.Email != nil:
          emailNotifier := notifier.NewEmailNotifier(*cfg.Alerting.Email, cfg.Alerting.DashboardURL, logger)
          registry.Register(emailNotifier)
      dispatcher := alert.NewDispatcher(registry, cfg.Alerting.DefaultChannels, cfg.Alerting.DefaultCooldownMinutes, logger)
      dispatcher.Start()
      // Pass evaluator + dispatcher to orchestrator
      // Pass alertRuleStore + alertHistoryStore + evaluator + dispatcher + registry to API server
    else:
      evaluator = nil, dispatcher = nil  // alerting disabled
  → api.New(cfg, store, pool, jwtSvc, userStore, logger, alertRuleStore, alertHistoryStore, evaluator, dispatcher, registry)
  → orchestrator.New(cfg, store, logger, evaluator, dispatcher)
  → ... (existing startup)
  → shutdown: HTTP → Orchestrator → Dispatcher.Stop() → Store.Close()
```

### FR-4: Graceful Shutdown Order

Updated sequence:

```
1. httpServer.Shutdown(ctx)         — stop accepting requests, drain in-flight
2. orchestrator.Stop()              — stop collect cycles, no more evaluator calls
3. dispatcher.Stop()                — drain buffered events, send remaining notifications
4. store.Close()                    — close DB connections
```

Dispatcher must stop AFTER orchestrator to ensure all final evaluation events are dispatched.

### FR-5: History Cleanup

Periodic goroutine that deletes resolved alerts older than `alerting.history_retention_days`.

Options:
- Run inside evaluator (it already has historyStore reference)
- Standalone goroutine started from main.go

Recommendation: Method on Evaluator — `StartCleanup(ctx, interval)` that launches a goroutine
running every 1 hour, calling `historyStore.Cleanup(retentionDays)`. Stops via context cancellation.

### FR-6: Evaluator Rule Refresh

When rules are modified via API (create, update, delete), the handler calls
`evaluator.LoadRules(ctx)` to refresh the in-memory rule cache. This ensures new/modified
rules take effect on the next evaluation cycle without requiring a restart.

## Non-Functional Requirements

- Evaluator.Evaluate() call in orchestrator must not slow the collect cycle by more than 10ms
- Alert API endpoints follow existing response envelope pattern (data + meta, or error)
- All new API endpoints covered by handler unit tests
- Zero regressions on existing tests
- History cleanup: 1-hour interval, log count of deleted records

## Test Requirements

### API Handler Tests (`internal/api/alerts_test.go`)

| Test | What it validates |
|------|-------------------|
| TestGetActiveAlerts | Returns unresolved alerts from mock history store |
| TestGetActiveAlerts_Empty | Returns empty array (not null) when no active alerts |
| TestGetAlertHistory | Returns filtered history, respects query params |
| TestGetAlertHistory_DefaultLimit | Default limit 100 applied when not specified |
| TestGetAlertRules | Returns all rules from mock rule store |
| TestCreateAlertRule | Valid custom rule → 201, source set to "custom" |
| TestCreateAlertRule_InvalidOperator | Bad operator → 400 |
| TestCreateAlertRule_MissingFields | Missing required fields → 400 |
| TestCreateAlertRule_ViewerForbidden | Viewer role → 403 |
| TestUpdateAlertRule | Update threshold → 200, evaluator.LoadRules called |
| TestUpdateAlertRule_NotFound | Unknown ID → 404 |
| TestDeleteAlertRule_Custom | Custom rule → 204 |
| TestDeleteAlertRule_Builtin | Builtin rule → 409 Conflict |
| TestTestNotification | Sends test event through notifier → 200 |
| TestTestNotification_UnknownChannel | Unknown channel → 400 |

### Orchestrator Integration Tests

| Test | What it validates |
|------|-------------------|
| TestOrchestratorWithEvaluator | Collect cycle → evaluator.Evaluate called with correct points |
| TestOrchestratorWithoutEvaluator | Evaluator nil → collect works normally (no panic) |
| TestOrchestratorEvaluatorError | Evaluator returns error → logged, cycle continues |

### Cleanup Tests

| Test | What it validates |
|------|-------------------|
| TestEvaluatorCleanup | Old resolved alerts deleted, recent ones preserved |

## Deliverables

| File | Description |
|------|-------------|
| `internal/api/alerts.go` | Alert API handlers (6 endpoints) |
| `internal/api/server.go` | Updated — add alert stores, evaluator, dispatcher, registry fields |
| `internal/api/alerts_test.go` | Handler tests with mocks |
| `internal/orchestrator/group.go` | Updated — post-collect evaluator hook |
| `internal/orchestrator/orchestrator.go` | Updated — accept evaluator + dispatcher |
| `internal/orchestrator/orchestrator_test.go` | Updated — evaluator integration tests |
| `cmd/pgpulse-server/main.go` | Updated — full alert pipeline wiring + shutdown order |
| `internal/alert/evaluator.go` | Updated — add StartCleanup() method |
| `internal/alert/evaluator_test.go` | Updated — cleanup test |
