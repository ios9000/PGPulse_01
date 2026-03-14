# Session Log: REM_01a — Rule-Based Remediation Engine (Backend)

**Date:** 2026-03-13
**Duration:** ~2 hours (single session, agent team crashed and recovered manually)
**Commits:** ab5336d, 1b12266

---

## Goal

Build a rule-based remediation engine that evaluates metrics and generates actionable recommendations. 25 compiled-in rules (17 PG + 8 OS) fire in two modes: alert-triggered (via RemediationProvider interface in the alert pipeline) and on-demand Diagnose (full snapshot scan). Persist recommendations to PostgreSQL. Expose 5 new API endpoints.

## Agent Configuration

- **Planning:** Claude.ai (Opus 4.6) — design doc, requirements, team prompt, checklist
- **Implementation:** Claude Code (Opus 4.6) — single-agent recovery after OOM crash
- **Original plan:** 2-agent team (API & Security + QA & Review)
- **What happened:** Agent team spawned but crashed with `panic(main thread): Out of memory while copying request body`. All work was done in a single-agent session to avoid repeating the OOM.

## What Was Built

### New Package: internal/remediation/

| File | Lines | Purpose |
|------|-------|---------|
| `rule.go` | 77 | Core types: Priority, Category, Rule, EvalContext, MetricSnapshot, RuleResult, Recommendation |
| `engine.go` | 88 | Engine struct — EvaluateMetric(), Diagnose(), Rules() |
| `rules_pg.go` | 546 | 17 PostgreSQL rules (connections, cache, replication, locks, transactions, bloat, vacuum, checkpoints, statements, temp files, deadlocks, WAL) |
| `rules_os.go` | 224 | 8 OS rules (CPU, memory, swap, disk usage, disk I/O, load average, network errors, OOM) |
| `store.go` | 27 | RecommendationStore interface and ListOpts |
| `pgstore.go` | 184 | PGStore — Write, ListByInstance, ListAll, ListByAlertEvent, Acknowledge, CleanOld |
| `nullstore.go` | 39 | NullStore — no-op implementation for live mode |
| `adapter.go` | 47 | AlertAdapter — bridges Engine to alert.RemediationProvider interface |
| `metricsource.go` | 37 | StoreMetricSource — queries MetricStore for last 2 minutes to build snapshot |

### New Files in Other Packages

| File | Lines | Purpose |
|------|-------|---------|
| `internal/alert/remediation.go` | 25 | RemediationResult struct + RemediationProvider interface (no import of internal/remediation) |
| `internal/api/remediation.go` | 160 | 5 HTTP handlers: list by instance, diagnose, list all, acknowledge, list rules |
| `internal/storage/migrations/013_remediation.sql` | 33 | remediation_recommendations table with 4 indexes |

### Test Files

| File | Tests | Lines |
|------|-------|-------|
| `internal/remediation/engine_test.go` | 7 | 136 |
| `internal/remediation/rules_test.go` | 27 (table-driven) | 441 |

Test coverage: matching rule, no-match, multiple rules, Diagnose with multiple issues, empty snapshot, partial snapshot, rule count verification, all 25 rules with positive/negative/boundary/missing-key cases, unique ID check, required fields validation, NullStore interface compliance.

### Modified Files

| File | Change |
|------|--------|
| `internal/alert/dispatcher.go` | Added `remediation RemediationProvider` field, `SetRemediationProvider()` setter, `runRemediation()` called after cooldown in processEvent() |
| `internal/api/server.go` | Added `remediationEngine`, `remediationStore`, `metricSource` fields, `SetRemediation()` setter, 5 routes in both auth-enabled and auth-disabled sections |
| `cmd/pgpulse-server/main.go` | Wired Engine, PGStore, StoreMetricSource, AlertAdapter; connected to dispatcher and API server |

### Stats

- **New files:** 14 (12 Go + 1 SQL + 1 test)
- **Modified files:** 4
- **Total new lines:** ~2,064 (remediation package) + 220 (other packages)
- **New API endpoints:** 5 (57 total)

## Architecture Decisions

| Decision | Rationale |
|----------|-----------|
| Interface in `internal/alert`, implementation in `internal/remediation` | Prevents import cycle: alert never imports remediation, wired in main.go |
| Setter pattern (`SetRemediationProvider`, `SetRemediation`) | Follows existing SetLiveMode, SetConnProvider precedent in the codebase |
| NullStore for live mode | Matches NullAlertHistoryStore pattern — no nil guards needed |
| Rules compiled-in, no DB config | V1 simplicity; rules change at code velocity, not runtime |
| Dual-mode evaluation (alert-triggered + Diagnose) | Alert mode is real-time single-metric; Diagnose scans full snapshot for advisory use |
| MetricSnapshot type (map[string]float64) | Simple lookup table — rules read sibling metrics via snapshot.Get() |
| Paired rules with non-overlapping ranges | e.g., rem_conn_high (80-99%) vs rem_conn_exhausted (>=99%) — no double-fire |

## Challenges & Fixes

| Issue | Resolution |
|-------|------------|
| Agent team OOM crash | Abandoned team spawn, implemented everything in single-agent session |
| Edit tool em-dash encoding mismatch in server.go | Multi-byte UTF-8 character (U+2014) couldn't be matched by Edit tool; used Python script to insert text at correct position |
| Missing `context` import in rules_test.go | Added `"context"` to import block after tests using `context.Background()` were added |

## Build & Test Results

| Check | Result |
|-------|--------|
| `go build ./cmd/... ./internal/...` | Clean |
| `go vet ./...` | Clean |
| `go test ./cmd/... ./internal/...` | All pass (34 new tests, 0 regressions) |
| `golangci-lint run` | 0 issues |
| `cd web && npm run build && npm run typecheck` | Clean |

## Commits

| Hash | Message |
|------|---------|
| `ab5336d` | feat(remediation): add rule-based remediation engine (REM_01a) |
| `1b12266` | docs: regenerate codebase digest for REM_01a |

## Not Done / Deferred to REM_01b

- Frontend Advisor page (recommendation display, Diagnose button)
- Alert detail panel with embedded recommendations
- Recommendations in email notification templates
- `internal/remediation/pgstore_test.go` (integration test — needs testcontainers)
- `internal/api/remediation_test.go` (handler tests)
- Embedding recommendations in alert API responses (`internal/api/alerts.go`)
