# PGPulse M14_03 — Session Log

**Date:** 2026-03-22
**Iteration:** M14_03 — Expansion, Calibration, and Knowledge Integration
**Duration:** ~1 session
**Tool:** Claude Code (Opus 4.6, 1M context) — Agent Teams (2 agents)
**Commit:** d0920cf

---

## Goal

Harden the RCA engine for production: activate all 20 causal chains (Tier B included), improve anomaly detection reliability, bridge RCA findings into the Adviser dashboard, add review instrumentation, and refine confidence scoring. The most complex M14 sub-iteration.

---

## Decisions Confirmed

All decisions D400–D508 and Q1–Q3 were locked before implementation:

| ID | Decision | Status |
|----|----------|--------|
| D505 | RCA→Adviser bridge uses Unified Upsert | Confirmed — implemented via `remediation.Engine.EvaluateHook()` |
| D506 | Schema uses `BIGINT[]` + `urgency_score FLOAT8` | Confirmed — migration 017 |
| D507 | `EvaluateHook()` lives on `remediation.Engine` | Confirmed — engine.go extended |
| D508 | Migration 017 includes partial unique index + GIN index | Confirmed |
| Q1 | Urgency soft cap at 10.0 via `LEAST()` | Confirmed — in Upsert SQL |
| Q2 | `Write()` initializes `urgency_score` from priority | Confirmed |
| Q3 | No recurrence lineage from resolved recommendations | Confirmed — deferred |
| D503 | Threshold fallback: 4h window + 15min calm period | Confirmed |
| D501 | Comprehensive ML metric mapping | Confirmed — 35+ metrics in demo YAML |
| D504 | JSON tags on CausalNode/CausalEdge with seconds conversion | Confirmed |
| D502 | 2 agents (Backend + Frontend) | Confirmed |

---

## Agent Team

| Agent | Role | Files Created | Files Modified |
|-------|------|---------------|----------------|
| Agent 1 — Backend | All Go changes: W2/W3/W4/W5/W6/W7/W8/W9/W10, migration 017 | 8 | 25 |
| Agent 2 — Frontend | TypeScript: types, components, hooks, pages | 3 | 9 |

**Execution:** Both agents ran in parallel. Backend: ~17 min (131 tool calls). Frontend: ~4.5 min (40 tool calls). Total wall-clock: ~17 minutes.

---

## Issues Encountered

1. **DiffEntry shape mismatch (Correction 3):** Design doc assumed `MeanExecTimeRatio` field and `Regressions` slice. Actual `DiffEntry` has `AvgExecTimePerCall` and `DiffResult` has flat `Entries[]` + `NewQueries[]`. Agent 1 adapted: computes regression ratio from old/new snapshot entries, iterates `Entries` to find regressions where avg time increased >2×.

2. **Hook→Rule ID namespace (Correction 1):** Hook constants use `remediation.*` prefix, rule IDs use `rem_*` prefix. Agent 1 built semantic `HookToRuleID` map matching hooks to closest rule (e.g., `HookCheckpointTuning` → `rem_wraparound_warn`, `HookConnectionPooling` → `rem_conn_high`). Hooks without matching rules return nil from `EvaluateHook()`.

3. **RCAConfig duplication (Correction 2):** New threshold fields added to both `internal/rca/config.go` (koanf tags) and `internal/config/config.go` (koanf tags). `main.go` converts between them.

4. **WhileEffective settings adapter:** `settings.PGSnapshotStore` is a concrete struct, not an interface. The `SnapshotSettingsProvider` adapter works directly with `*settings.PGSnapshotStore`. Live query fallback via `InstanceConnProvider` was wired but is optional.

5. **Review column name:** Migration 016 has `review_comment` (not `review_notes`). Backend correctly uses `review_comment` in UPDATE SQL.

---

## Files Created (11 new files, 647 lines)

| File | Lines | Purpose |
|------|-------|---------|
| `internal/rca/settings.go` | 23 | SettingsProvider interface + SettingChange struct |
| `internal/rca/settings_adapter.go` | 72 | SnapshotSettingsProvider — diffs settings snapshots for WhileEffective |
| `internal/rca/settings_adapter_test.go` | 54 | Tests for settings adapter |
| `internal/rca/statement_source.go` | 136 | StatementDiffSource — detects query regressions + new queries from PGSS diffs |
| `internal/rca/statement_source_test.go` | 139 | Tests for regression detection, new query detection, insufficient snapshots |
| `internal/remediation/hooks.go` | 23 | HookToRuleID map — 15 ontology hooks mapped to remediation rule IDs |
| `internal/remediation/urgency.go` | 23 | UrgencyFromPriority() + urgency constants (Critical=3.0, Warning=2.0, Info=1.0) |
| `internal/storage/migrations/017_recommendation_rca_bridge.sql` | 31 | ALTER TABLE: source, urgency_score, incident_ids BIGINT[], last_incident_at + indexes + backfill |
| `web/src/components/advisor/RCABadge.tsx` | 29 | Purple badge: "Linked to N incident(s)" with navigation |
| `web/src/components/rca/ReviewWidget.tsx` | 102 | Three-button review (Confirmed/False Positive/Inconclusive) + notes textarea |
| `web/src/hooks/useRecommendationsByIncident.ts` | 15 | React Query hook for recommendations filtered by incident_id |

## Files Modified (33 files, ~3,700 lines changed)

### Backend (25 files)

| File | Change |
|------|--------|
| `internal/rca/config.go` | +ThresholdBaselineWindow (4h), ThresholdCalmPeriod (15m), ThresholdCalmSigma (1.5) |
| `internal/rca/anomaly.go` | Configurable baseline window, isBaselineCalm() method, unreliable source marking |
| `internal/rca/anomaly_test.go` | +3 tests: unreliable baseline, configurable window, calm period |
| `internal/rca/graph.go` | JSON tags on CausalNode/CausalEdge (snake_case), MinLagSeconds/MaxLagSeconds fields |
| `internal/rca/engine.go` | SettingsProvider + StatementDiffSource + RemediationHookEvaluator fields; WhileEffective impl; Tier B filter removed; fireRemediationHooks(); temporal weight formula; instanceID threading |
| `internal/rca/incident.go` | Improved generateSummary() with metric values, timestamps, direction verbs |
| `internal/rca/chains.go` | Updated query_regression/new_query nodes with synthetic metric keys |
| `internal/rca/store.go` | +UpdateReview() to IncidentStore interface |
| `internal/rca/pgstore.go` | +UpdateReview() implementation (review_status, review_comment, reviewed_at) |
| `internal/rca/nullstore.go` | +UpdateReview() no-op |
| `internal/remediation/rule.go` | +Source, UrgencyScore, IncidentIDs, LastIncidentAt on Recommendation |
| `internal/remediation/store.go` | +Upsert(), ListByIncident() on interface; +Source, IncidentID, OrderBy on ListOpts |
| `internal/remediation/pgstore.go` | Upsert with ON CONFLICT + LEAST() cap; ListByIncident GIN query; Write urgency init; updated all SELECT/Scan |
| `internal/remediation/nullstore.go` | +Upsert(), ListByIncident() no-ops |
| `internal/remediation/engine.go` | +SetStore(), SetMetricSource(), EvaluateHook() — hook lookup → rule eval → Upsert |
| `internal/config/config.go` | +RCA threshold fields + RemediationConfig urgency deltas |
| `internal/config/load.go` | +Defaults for all new config fields |
| `internal/api/rca.go` | +handleRCAReviewIncident PUT handler; graph response with seconds conversion |
| `internal/api/remediation.go` | +incident_id, source, order_by query params |
| `internal/api/server.go` | +PUT /rca/incidents/{incidentId}/review route |
| `internal/api/remediation_test.go` | +Upsert(), ListByIncident() on mock store |
| `cmd/pgpulse-server/main.go` | Wire remEngine.SetStore/SetMetricSource, settings/statement providers into RCA engine |

### Frontend (9 files)

| File | Change |
|------|--------|
| `web/src/types/rca.ts` | PascalCase → snake_case for graph types; +review_status, review_comment on Incident |
| `web/src/types/models.ts` | +source, urgency_score, incident_ids, last_incident_at on Recommendation |
| `web/src/components/rca/CausalGraphView.tsx` | All field references updated to snake_case |
| `web/src/hooks/useRCA.ts` | +useReviewIncident mutation |
| `web/src/hooks/useRecommendations.ts` | +source, order_by params |
| `web/src/pages/Advisor.tsx` | +source filter dropdown; default sort by urgency_score |
| `web/src/components/advisor/AdvisorFilters.tsx` | +source/onSourceChange props + SOURCE_OPTIONS |
| `web/src/components/advisor/AdvisorRow.tsx` | +RCABadge when incident_ids present |
| `web/src/pages/RCAIncidentDetail.tsx` | +ReviewWidget below header; +Recommended Actions section after timeline |

---

## Test Results

| Check | Result |
|-------|--------|
| `go test ./cmd/... ./internal/...` | All PASS (21 packages) |
| `golangci-lint run` | 0 issues |
| `go vet` | Clean |
| `go build ./cmd/pgpulse-server` | OK |
| `npm run build` | OK |
| `npm run typecheck` | OK (0 errors) |
| `npm run lint` | 0 errors (1 pre-existing warning) |

---

## Demo Verification Results

Demo VM deployment deferred to a separate session. The M14_03 code changes are committed and verified locally. Demo deployment steps documented in the design doc (Section 2.3) include:

1. Cross-compile Linux binary
2. SCP to demo VM (185.159.111.139)
3. Update `/opt/pgpulse/pgpulse.yml` with comprehensive ML metrics config (35+ keys)
4. Add RCA threshold config and remediation urgency deltas
5. Run migration 017 on restart
6. Wait for ML baseline warm-up (2 min)
7. Verify incidents generated after chaos test

---

## What's Next

M14 is functionally complete. All three sub-iterations delivered:
- **M14_01:** RCA engine (20 chains, 9-step algorithm, 5 API endpoints)
- **M14_02:** RCA UI (incidents page, timeline visualization, alert integration)
- **M14_03:** Production hardening (threshold reliability, Tier B activation, RCA→Adviser bridge, review workflow)

Potential follow-ups:
- Demo VM deployment + chaos testing validation
- Incident review analytics (confirmed vs false positive rate)
- M15 — Maintenance Forecasting (will use same EvaluateHook path with `source: "forecast"`)

---

## Stats

- **Total lines changed:** 4,353 insertions, 131 deletions (48 files)
- **New files:** 11 (647 lines)
- **Modified files:** 33 (backend: 25, frontend: 9)
- **New migration:** 017_recommendation_rca_bridge.sql
- **New API endpoint:** PUT /rca/incidents/{id}/review
- **Chains activated:** 20 (was 16 Tier A only)
- **Hook mappings:** 15 ontology hooks → remediation rules
