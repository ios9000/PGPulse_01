# Session: 2026-03-01 — M4_03 Alert API, Orchestrator Wiring & Integration

## Goal

Wire the alert evaluator (M4_01) and dispatcher (M4_02) into the orchestrator post-collect
cycle, expose alert REST API endpoints, integrate everything in main.go with proper
startup/shutdown ordering, and add periodic alert history cleanup. Final M4 iteration.

## Agent Team Configuration

- Team Lead: Claude Code (Opus 4.6)
- Specialists: 2 (Wiring & API + Tests)
- Bash: Working (v2.1.63)

## What Was Built

### Orchestrator Wiring
- Post-collect evaluator hook in intervalGroup.collect()
- AlertEvaluator and AlertDispatcher interfaces defined in orchestrator package
- evaluator/dispatcher passed through Orchestrator → instanceRunner → intervalGroup
- Evaluation errors logged but don't abort collect cycle

### Alert API Endpoints (6 new)

| Method | Path | Purpose | Auth |
|--------|------|---------|------|
| GET | /api/v1/alerts | List active (unresolved) alerts | viewer+ |
| GET | /api/v1/alerts/history | Query alert history with filters | viewer+ |
| GET | /api/v1/alerts/rules | List all alert rules | viewer+ |
| POST | /api/v1/alerts/rules | Create custom rule | admin |
| PUT | /api/v1/alerts/rules/{id} | Update rule (builtin: limited fields) | admin |
| DELETE | /api/v1/alerts/rules/{id} | Delete custom rule (builtin: 409) | admin |
| POST | /api/v1/alerts/test | Send test notification | admin |

### main.go Integration
- Full alert pipeline wiring: stores → seed → evaluator → notifiers → dispatcher
- Updated shutdown order: HTTP → Orchestrator → Dispatcher → Store
- History cleanup goroutine (1-hour interval, configurable retention)

### History Cleanup
- Evaluator.StartCleanup(ctx, retentionDays) — periodic goroutine
- Runs once on startup, then every hour
- Deletes resolved alerts older than retention period

## Files Created/Modified

### New Files

| File | Description |
|------|-------------|
| internal/api/alerts.go | 7 alert API handlers + validation + request types |
| internal/api/alerts_test.go | Handler tests with mocks |

### Modified Files

| File | Change |
|------|--------|
| internal/api/server.go | Added alert fields, updated constructor, added alert routes |
| internal/api/helpers_test.go | Added mock alert stores, evaluator, dispatcher |
| internal/orchestrator/orchestrator.go | Added evaluator/dispatcher fields, AlertEvaluator/AlertDispatcher interfaces |
| internal/orchestrator/runner.go | Pass evaluator/dispatcher to interval groups |
| internal/orchestrator/group.go | Post-collect evaluateAlerts() hook |
| internal/orchestrator/group_test.go | Evaluator integration tests |
| internal/alert/evaluator.go | Added StartCleanup(), runCleanup() |
| internal/alert/evaluator_test.go | Cleanup test |
| cmd/pgpulse-server/main.go | Full alert pipeline wiring + updated shutdown |

## Build & Validation

| Check | Result |
|-------|--------|
| go build ./... | ✅ Pass |
| go vet ./... | ✅ Pass |
| go test ./... | ✅ All 8 packages pass, zero regressions |
| golangci-lint run | ✅ 0 issues |

## Commits

- `7c6ab36` — feat(alert): wire evaluator and dispatcher into orchestrator and API (M4_03)

## Architecture Decisions

- AlertEvaluator/AlertDispatcher interfaces defined in orchestrator package (not imported from alert) to keep dependency direction clean
- APIServer constructor extended with alert parameters (acknowledged as long, refactor deferred to M7)
- Builtin rules cannot be deleted via API (409 Conflict) — only disabled
- Rule mutations trigger evaluator.LoadRules() for immediate effect
- History cleanup runs in evaluator goroutine, stopped via context cancellation

## M4 Milestone Summary

| Iteration | Commit | Scope |
|-----------|--------|-------|
| M4_01 | 0371455 | Evaluator engine, 19 rules, stores, migration |
| M4_02 | eae52e1 | Email notifier, dispatcher, HTML templates |
| M4_03 | 7c6ab36 | API endpoints, orchestrator wiring, main.go |

**Total REST API endpoints after M4: 14**
- Health (1), Auth (3), Instances (3), Alerts (7)

**Total alert rules: 19** (14 PGAM + 2 new + 3 deferred)

## Not Done / Deferred

- [ ] Per-rule cooldown override in dispatcher (uses global default)
- [ ] Cooldown map periodic cleanup
- [ ] Telegram/Slack/webhook/messenger notifiers → M7+
- [ ] Alert acknowledgment/silencing → M7+
- [ ] Frontend alert UI → M5
