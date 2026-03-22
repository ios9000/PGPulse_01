# PGPulse — Iteration Handoff: M14_02 → M14_03

**Date:** 2026-03-21
**From:** M14_02 (RCA UI — complete)
**To:** M14_03 (Expansion, Calibration, and Knowledge Integration)

---

## DO NOT RE-DISCUSS

All D400–D408 decisions remain locked. Additionally:

### M14_01 — RCA Engine (COMPLETE)

- `internal/rca/` package: 12 production files, 2,580 lines
- 20 causal chains (16 Tier A active, 4 Tier B stubbed), 47 shared nodes
- Core 9-step Analyze() algorithm: trigger → scope → query → detect → traverse+prune → rank → build → store
- Required-evidence pruning (D407): branches die when mandatory intermediate evidence is absent
- Dual anomaly source: `MLAnomalySource` wraps ml.Detector, `ThresholdAnomalySource` fallback with fuzzy window
- Basic confidence scoring: edge scores × anomaly strength × temporal proximity → chain score → bucket (high/medium/low)
- State-aware temporal semantics: `BoundedLag`, `PersistentState` implemented; `WhileEffective` stubbed (Tier B)
- Shared RCA↔Adviser ontology: `ontology.go` with stable `Sym*`, `Mech*`, `RC*`, `Chain*`, `Hook*` constants
- AutoTrigger: OnAlert hook on dispatcher, severity check, 15-min cooldown, async 30s-timeout analysis
- 5 API endpoints: POST analyze, GET incidents (instance + fleet), GET incident detail, GET graph
- Migration 016: `rca_incidents` table with JSONB timeline + normalized summary columns
- MetricStatsProvider on PGMetricStore (SQL aggregation) and MemoryStore (in-memory)
- 30 tests across 6 test files

### M14_02 — RCA UI (COMPLETE)

- RCA Incidents list page (fleet-wide with instance/confidence/trigger filters)
- Per-instance RCA Incidents page (sidebar nav under each server)
- Incident Detail page: header card, summary banner, quality banner, analysis metadata
- Timeline visualization ready (IncidentTimeline component) — renders when causal chain events exist
- Causal Graph page: interactive ECharts force-directed graph showing all 47 nodes and 20 chains
- "Investigate Root Cause" button in AlertDetailPanel — triggers on-demand RCA, navigates to result
- ConfidenceBadge, QualityBanner, ChainSummaryCard, RemediationHooks components
- React Query hooks for all 5 RCA API endpoints
- TypeScript interfaces matching API response shapes
- Sidebar: "RCA Incidents" at fleet level + per-server level

### Bug Fixed During M14_02

- `CausalGraphView.tsx`: types file had PascalCase (`Nodes`, `Edges`) but API returns lowercase (`nodes`, `edges`). Fixed types to match API response. Inner field names remain PascalCase (Go default JSON serialization without tags).

### Current State: RCA Returns "No Causal Chain" on Demo

The engine works correctly end-to-end. The reason chains don't fire on the demo is **ML calibration**:

1. The demo `pgpulse.yml` ML config uses old metric key names (`connections_active`, `cache_hit_ratio`) that don't match collector output (`pg.connections.utilization_pct`, `pg.cache.hit_ratio`)
2. The ML detector isn't tracking the metrics the RCA chains reference
3. The threshold fallback computes baselines from the last hour — which may already include chaos-level values from previous test runs
4. A healthy system correctly produces "No probable causal chain identified"

This is the primary focus of M14_03.

---

## What M14_03 Needs to Build On

### Scope: Expansion, Calibration, and Knowledge Integration

From M14_requirements_v2.md:

1. **Integrate settings effective-state reasoning** — activate chain 19 (config change → behavioral shift) by implementing `WhileEffective` temporal semantics. Requires checking current settings values, not just change timestamps.

2. **Integrate statement snapshot diffs** — activate chains 12 (query regression) and 13 (new query) by connecting to `internal/statements/` diff engine. Detect new queryids via set-difference between snapshots.

3. **Activate Tier B chains** — chains 3, 12, 13, 19 currently stubbed. Provide the data sources they need and remove the Tier B filter.

4. **Improve summary generation** — more descriptive narratives with specific metric values and timestamps in the summary text.

5. **Strengthen confidence model** — tune edge base confidence values based on testing; add temporal proximity weighting refinement.

6. **ML metric key alignment** — fix the demo config to use correct metric keys so the ML detector actually tracks what RCA needs.

7. **Rule-quality instrumentation stubs** — prepare incident feedback capture path (the `review_status` columns exist in the schema).

8. **RCA→Adviser bridge** — when an incident identifies a root cause with a remediation hook, surface it as a focused adviser recommendation.

### Key Interfaces for M14_03

```go
// internal/rca/engine.go
type Engine struct { ... }
func (e *Engine) Analyze(ctx context.Context, req AnalyzeRequest) (*Incident, error)

// internal/rca/graph.go — WhileEffective needs implementation in evaluateEdge()
case WhileEffective:
    // Currently returns (0, nil, false) — needs real implementation

// internal/rca/anomaly.go — may need new source type for statement diffs
type AnomalySource interface {
    GetAnomalies(ctx context.Context, instanceID string, from, to time.Time) ([]AnomalyEvent, error)
    GetMetricAnomaly(ctx context.Context, instanceID, metricKey string, from, to time.Time) (*AnomalyEvent, error)
}

// internal/rca/ontology.go — Hook* constants link to adviser rules
const HookCheckpointTuning = "remediation.checkpoint_completion_target"
```

### Causal Graph JSON Serialization Note

The Go `CausalGraph` struct serializes with mixed casing:
- Top-level fields: lowercase (`nodes`, `edges`, `chain_ids`) — from `json:"..."` tags on the handler response wrapper
- Inner struct fields: PascalCase (`ID`, `Name`, `MetricKeys`, `FromNode`, `ToNode`) — Go default, no json tags on `CausalNode`/`CausalEdge`

The frontend `types/rca.ts` has been corrected to match this. If M14_03 adds json tags to the Go structs, the types file must be updated.

---

## Codebase Scale (Post M14_02)

- **Go files:** ~250 (~41,000 lines)
- **TypeScript files:** ~155 (~14,500 lines)
- **Metric keys:** ~220
- **API endpoints:** ~70
- **Collectors:** 27
- **Frontend pages:** 17
- **React components:** ~70
- **RCA package:** 12 files (~2,580 lines), 20 chains, 47 nodes
- **RCA UI:** 15 new components/pages (~1,200 lines)

---

## Build & Deploy

```bash
# Full verification
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...

# Cross-compile + deploy
export GOOS=linux && export GOARCH=amd64 && export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

---

## Known Issues

| Issue | Severity | Notes |
|-------|----------|-------|
| RCA chains don't fire on demo | Medium | ML metric keys misaligned; threshold fallback baseline includes prior chaos data. M14_03 calibration. |
| WhileEffective temporal semantics not implemented | Info | Tier B stub — chain 19 won't fire until M14_03 |
| Statement diff integration missing | Info | Tier B — chains 12, 13 won't fire until M14_03 |
| CausalGraph JSON casing mixed | Low | Top-level lowercase, inner PascalCase. Types file corrected. Add json tags in M14_03 for consistency. |
| Settings Diff page 404 | Pre-existing | Not M14 related |
| Timeline visualization untested with real events | Info | No chain has fired yet; timeline component ready but showing "No causal chain" |
| Edge MinLag/MaxLag serialized as nanoseconds | Low | JSON shows `30000000000` instead of `30s`. Frontend should convert for display. |

---

## Roadmap

| Milestone | Status |
|-----------|--------|
| M14_01 — RCA Engine | ✅ Done |
| M14_02 — RCA UI | ✅ Done |
| **M14_03 — Expansion/Calibration** | **🔲 Next** |
| M15 — Maintenance Forecasting | 🔲 |
| M13 — Prometheus Exporter | 🔲 |
