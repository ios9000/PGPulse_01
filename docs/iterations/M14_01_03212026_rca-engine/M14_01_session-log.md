# PGPulse M14_01 — Session Log

**Date:** 2026-03-21
**Iteration:** M14_01 — Causal Graph + Reliable Correlation Engine
**Duration:** ~1 session
**Tool:** Claude Code (Opus 4.6, 1M context) — Agent Teams (3 agents)
**Commit:** 4e2e7e2

---

## Goal

Build the backend Root Cause Analysis engine that correlates metric anomalies across PostgreSQL and OS layers to produce incident timelines explaining *why* an issue occurred.

This is the most architecturally complex feature PGPulse has built. M14_01 covers the backend engine; M14_02 will add the UI.

---

## Agent Team

| Agent | Role | Files Created | Files Modified |
|-------|------|---------------|----------------|
| Agent 1 — RCA Engine | All `internal/rca/` production code | 12 | 0 |
| Agent 2 — API + Integration | Migration, API handlers, config, storage, wiring | 2 | 6 |
| Agent 3 — QA | All `*_test.go`, full build verification | 6 | 0 |

**Execution:** Agents 1 and 2 ran in parallel (~7.5 min each). Agent 3 ran after both completed (~6.3 min). Total wall-clock: ~20 minutes.

---

## What Was Built

### New Package: `internal/rca/` (2,580 production lines + 982 test lines)

| File | Lines | Purpose |
|------|-------|---------|
| `ontology.go` | 158 | Shared RCA↔Adviser knowledge constants (symptoms, mechanisms, root causes, 20 chain IDs, hooks) |
| `graph.go` | 199 | CausalNode, CausalEdge, CausalGraph types + traversal methods (IncomingEdges, ChainsForTrigger, ReachableNodes, MetricKeysForChains) |
| `chains.go` | 486 | 20 chain definitions: 16 Tier A (active), 4 Tier B (stubbed). 47 shared nodes with exact metric keys from collector catalog |
| `engine.go` | 518 | Core 9-step Analyze() algorithm: window → scope → query → detect → traverse+prune → rank → build → store → return |
| `anomaly.go` | 280 | AnomalySource interface + ThresholdAnomalySource (2σ + rate-of-change) + MLAnomalySource (wraps ml.Detector with fallback) |
| `statsource.go` | 21 | MetricStatsProvider interface for batch statistics |
| `incident.go` | 239 | Incident, CausalChainResult, TimelineEvent, QualityStatus types. IncidentBuilder with qualified summary generation |
| `config.go` | 39 | RCAConfig struct with production-safe defaults |
| `trigger.go` | 94 | AutoTrigger: OnAlert hook, severity check, 15-min cooldown per metric/instance, async 30s-timeout analysis |
| `store.go` | 21 | IncidentStore interface (Create, Get, ListByInstance, ListAll, Cleanup) |
| `pgstore.go` | 284 | PostgreSQL-backed store: JSONB serialization for timeline, normalized summary columns for filtering |
| `nullstore.go` | 34 | No-op store for live mode |

### API Endpoints (5 new)

| Method | Path | Handler |
|--------|------|---------|
| POST | `/instances/{id}/rca/analyze` | On-demand RCA analysis |
| GET | `/instances/{id}/rca/incidents` | Paginated incidents for instance |
| GET | `/instances/{id}/rca/incidents/{incidentId}` | Full incident with timeline |
| GET | `/rca/incidents` | All incidents across fleet |
| GET | `/rca/graph` | Causal graph definition (nodes + edges) |

### Migration

`016_rca_incidents.sql` — `rca_incidents` table with JSONB timeline column, 4 indexes (instance+time, trigger, chain, review status).

### Modified Files

| File | Change |
|------|--------|
| `cmd/pgpulse-server/main.go` | +52 lines: RCA engine init, anomaly source selection (ML preferred, threshold fallback), auto-trigger hook on dispatcher |
| `internal/api/server.go` | +27 lines: rcaEngine/rcaStore fields, SetRCA() setter, route registration in both auth paths |
| `internal/config/config.go` | +18 lines: RCAConfig struct added to Config |
| `internal/config/load.go` | +32 lines: RCA defaults (lookback 30m, max 10/hr, retention 90d, traversal depth 5, min chain score 0.40) |
| `internal/storage/pgstore.go` | +45 lines: MetricStatsProvider via SQL aggregation (avg, stddev_samp, min, max, count) |
| `internal/storage/memory.go` | +72 lines: MetricStatsProvider with in-memory sample variance computation |

### Tests (30 tests, 982 lines)

| File | Tests | Coverage Focus |
|------|-------|----------------|
| `graph_test.go` | 7 | Graph construction, edge lookup, reachability, deduplication |
| `chains_test.go` | 9 | All 20 chains: integrity, tiers (16A/4B), evidence, confidence, metric keys |
| `engine_test.go` | 10 | Full chain traversal, required-evidence pruning (D407), confidence buckets, max depth, empty results |
| `anomaly_test.go` | 3 | Threshold detection, no-data handling, stable-data baseline |
| `nullstore_test.go` | 5 | Null store contract |
| `pgstore_test.go` | 3 | Integration stubs (build-tagged) |

---

## Key Design Decisions Implemented

| ID | Decision | Implementation |
|----|----------|----------------|
| D400 | Incident Timeline + Lightweight Causal Graph | `Incident` struct with `Timeline []TimelineEvent` + `PrimaryChain`/`AlternativeChain` |
| D401 | Auto CRITICAL + on-demand any | `AutoTrigger.RegisterHook()` on dispatcher; POST `/rca/analyze` for manual |
| D402 | 30-min lookback, configurable to 2h | `RCAConfig.LookbackWindow` default 30m |
| D404 | ML-enhanced, not required | `MLAnomalySource` wraps detector; `ThresholdAnomalySource` fallback |
| D407 | Negative evidence pruning required | `EvidenceRequired` kills branch in `evaluateEdge()` |
| D408 | Shared ontology with stable identifiers | `ontology.go` with `Sym*`, `Mech*`, `RC*`, `Chain*`, `Hook*` constants |

---

## Verification Results

| Check | Result |
|-------|--------|
| `go test ./cmd/... ./internal/... -count=1` | All PASS (21 packages) |
| `golangci-lint run ./cmd/... ./internal/...` | 0 issues |
| `npm run build` | OK |
| `npm run typecheck` | OK |
| `npm run lint` | OK |
| `go build ./cmd/pgpulse-server` | OK |
| `go build -tags desktop ./cmd/pgpulse-server` | OK |

---

## Notable Observations

1. **Temporal window alignment:** Engine evaluates ALL edge temporal windows relative to the original `triggerTime` (not relative to intermediate event timestamps). This means anomaly evidence for upstream nodes must fall within `[triggerTime - MaxLag - jitter, triggerTime - MinLag + jitter]` for each edge. Discovered during test writing.

2. **MetricStatsProvider duck typing:** To avoid circular imports between `internal/rca` and `internal/storage`, both packages define their own `MetricStats` struct. The RCA engine accepts `MetricStatsProvider` as a constructor parameter, and `main.go` adapts between the types.

3. **Tier B filtering:** Tier B chains (3, 12, 13, 19) exist in the graph with full node/edge definitions but are filtered out during chain selection in the engine. This allows M14_03 to activate them without graph changes.

4. **No frontend changes:** Zero web/src/ files modified, as designed. UI comes in M14_02.

---

## What's Next: M14_02

- RCA Incident List page (fleet-wide and per-instance)
- Incident Detail page with timeline visualization
- Causal graph visualization (interactive node/edge diagram)
- Integration with existing Alert Detail Panel (link to RCA incidents)
- Confidence badge and quality banner components

---

## Stats

- **Total lines added:** 3,808 (26 files)
- **Production code:** 2,580 lines (12 new files in `internal/rca/` + 1 API handler + 1 migration)
- **Test code:** 982 lines (6 test files, 30 tests)
- **Modified files:** 6 existing files (~246 lines added)
- **New API endpoints:** 5
- **Causal chains:** 20 (16 active, 4 stubbed)
- **Shared nodes:** 47
