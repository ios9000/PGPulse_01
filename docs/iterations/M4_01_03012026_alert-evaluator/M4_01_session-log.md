# Session: 2026-03-01 — M4_01 Alert Evaluator & Rules Engine

## Goal

Build the core alert evaluation engine: data model, state machine with hysteresis and cooldown,
19 builtin rule definitions, DB-backed rule and history storage, startup seeding, and config extension.
Domain logic only — no notifiers, no API endpoints, no orchestrator wiring.

## Agent Team Configuration

- Team Lead: Claude Code (Opus 4.6)
- Specialists: 2 (Alert Engine + Tests)
- Bash: Working (v2.1.63, no hybrid workflow)

## Planning Decisions Made (Claude.ai)

| # | Decision | Choice |
|---|----------|--------|
| D48 | M4 split | 3 iterations: M4_01 (evaluator+rules) → M4_02 (email+dispatcher) → M4_03 (API+wiring) |
| D49 | MVP notifiers | Email (SMTP) only. Telegram/Slack/messenger/webhook deferred to M7+ |
| D50 | Rule storage | Both: YAML-defined builtins seeded to DB + custom rules via API |
| D51 | Alert history | DB table, 30-day default retention |
| D52 | Evaluator integration | Post-collect hook in orchestrator (M4_03) |
| D53 | State machine persistence | In-memory, seed from unresolved alert_history on restart |
| D54 | Notification channels | Global defaults in config, per-rule override optional |
| D55 | Hysteresis | Default 3 consecutive violations, configurable per rule |
| D56 | Cooldown | Default 15 min, configurable per rule. State transitions always notify immediately |
| D57 | Deferred rules | WAL spike, query regression, disk forecast defined but enabled=false |
| D58 | SMTP testing | Mock/mailhog in dev, real SMTP in production |

## PGAM Thresholds Ported

All 14 actionable PGAM thresholds from PGAM_FEATURE_AUDIT.md §6 ported as builtin rules:

| PGAM Metric | Rule ID | Severity |
|-------------|---------|----------|
| Wraparound > 20% | wraparound_warning | warning |
| Wraparound > 50% | wraparound_critical | critical |
| Multixact > 20% | multixact_warning | warning |
| Multixact > 50% | multixact_critical | critical |
| Connections > 80% | connections_warning | warning |
| Connections ≥ 99% | connections_critical | critical |
| Cache hit < 90% | cache_hit_warning | warning |
| Commit ratio < 90% | commit_ratio_warning | warning |
| Replication slot inactive | replication_slot_inactive | critical |
| Long tx > 1 min | long_tx_warning | warning |
| Long tx ≥ 5 min | long_tx_critical | critical |
| Bloat > 2× | bloat_warning | warning |
| Bloat > 50× | bloat_critical | critical |
| pgss fill ≥ 95% | pgss_fill_warning | warning |

Plus 2 new implementable rules (replication_lag_warning, replication_lag_critical)
and 3 deferred rules (wal_spike, query_regression, disk_forecast — enabled: false).

## Files Created

### Production Code

| File | Lines (est.) | Description |
|------|-------------|-------------|
| internal/alert/alert.go | ~120 | Severity, Operator, AlertState, RuleSource, Rule, AlertEvent, stateEntry |
| internal/alert/store.go | ~60 | AlertRuleStore (7 methods), AlertHistoryStore (5 methods) interfaces |
| internal/alert/evaluator.go | ~250 | State machine: OK→PENDING→FIRING→OK, hysteresis, label-aware keys |
| internal/alert/rules.go | ~200 | BuiltinRules() — 19 rule definitions |
| internal/alert/pgstore.go | ~300 | PGAlertRuleStore + PGAlertHistoryStore, JSONB, UpsertBuiltin |
| internal/alert/seed.go | ~40 | SeedBuiltinRules() — idempotent startup upsert |
| internal/storage/migrations/004_alerts.sql | ~40 | alert_rules + alert_history tables, 3 indexes |

### Config Changes

| File | Change |
|------|--------|
| internal/config/config.go | Added AlertingConfig struct (enabled, consecutive_count, cooldown, timeout, retention) |
| internal/config/load.go | Added alertingDefaults(), validateAlerting() |
| configs/pgpulse.example.yml | Added alerting section |

### Tests

| File | Tests | Description |
|------|-------|-------------|
| internal/alert/alert_test.go | ~8 | Operator Compare (all 6 ops + edge cases), severityLevel |
| internal/alert/evaluator_test.go | 16 | Full state machine coverage, hysteresis, labels, restore |
| internal/alert/rules_test.go | ~4 | Validation, no duplicate IDs, count=19, deferred disabled |
| internal/alert/pgstore_test.go | 12 | Integration tests (testcontainers) |
| internal/alert/seed_test.go | ~2 | Seed + idempotent re-seed |
| internal/config/config_test.go | +2 | Alerting defaults, enabled-requires-DSN |

**Total: 18 files, ~2409 lines**

## Build & Validation

| Check | Result |
|-------|--------|
| go build ./... | ✅ Pass |
| go vet ./... | ✅ Pass |
| go test ./... | ✅ All pass, zero regressions |
| golangci-lint run | ✅ 0 issues |

## Commit

- `0371455` — feat(alert): add evaluator engine, rules, and stores (M4_01)

## Architecture Decisions (Made During Implementation)

- Rule 17 (connection_pool_warning) dropped — overlaps with connections_warning. Final count: 19 rules.
- Critical rules use consecutive_count=1, cooldown=5min. Warnings use consecutive_count=3, cooldown=15min.
- Evaluator emits events only on state transitions (fire + resolve), not on every cycle while FIRING. Repeat notification logic deferred to dispatcher (M4_02).
- State keys include label identity for label-filtered rules (per-slot tracking for replication).

## Not Done / Next Iteration

- [ ] M4_02: Email (SMTP) notifier + dispatcher with retry/backoff
- [ ] M4_03: Alert API endpoints + orchestrator post-collect wiring + main.go integration
- [ ] History cleanup goroutine (periodic deletion of old resolved alerts)
