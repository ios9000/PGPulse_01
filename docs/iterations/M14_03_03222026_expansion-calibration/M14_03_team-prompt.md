# M14_03 — Team Prompt: Expansion, Calibration, and Knowledge Integration

**Iteration:** M14_03
**Date:** 2026-03-22
**Model:** `claude --model claude-opus-4-6`
**Agents:** 2 (Backend + Frontend/Config)

---

## Pre-Read (MANDATORY)

Before ANY code changes, each agent MUST read:

```
docs/CODEBASE_DIGEST.md          — file inventory, interfaces, metric keys, API endpoints
docs/iterations/M14_03_*/design.md — full technical specification
docs/iterations/M14_03_*/requirements.md — acceptance criteria
docs/iterations/M14_03_*/corrections.md — pre-flight grep findings
CLAUDE.md                         — project rules and current state
```

---

## DO NOT RE-DISCUSS

All decisions D400–D508 and Q1–Q3 are locked. Specifically:

- The RCA→Adviser bridge uses Unified Upsert (D505), not a separate bridge type
- Schema uses `BIGINT[]` + `urgency_score FLOAT8`, not JSONB + boolean (D506)
- `EvaluateHook()` lives on `remediation.Engine`, not a new type (D507)
- Migration 017 includes partial unique index + GIN index (D508)
- Urgency soft cap at 10.0 via `LEAST()` in SQL (Q1)
- `Write()` MUST initialize `urgency_score` from priority (Q2)
- No recurrence lineage from resolved recommendations (Q3)
- Threshold fallback: 4h window + 15min calm period (D503)
- Comprehensive ML metric mapping, not minimum-to-fire (D501)
- JSON tags on CausalNode/CausalEdge with seconds conversion for lag fields (D504)
- 2 agents (D502)

Do NOT propose alternatives, suggest "improvements," or ask clarifying questions about these decisions. They are final.

---

## Agent 1: BACKEND AGENT

### Ownership

All Go code changes. Migration. Tests. Specifically: W2, W3, W4, W5, W7, W8 (backend), W9 (Go), W10 (backend), and W1 (metric key verification in chains.go).

### Phase 1 — Parallel Foundation (no dependencies between items)

#### Task 1A: Threshold Fallback Hardening (W2)

**Files:**
- MODIFY `internal/rca/config.go` — add `ThresholdBaselineWindow`, `ThresholdCalmPeriod`, `ThresholdCalmSigma` to RCAConfig with defaults (4h, 15m, 1.5)
- MODIFY `internal/rca/anomaly.go` — change stats query window from hardcoded 1h to `t.baselineWindow`; add `isBaselineCalm()` method; when calm check fails, set anomaly Source to `"threshold_unreliable"` and append limitation to quality
- MODIFY `internal/rca/anomaly_test.go` — add tests for 4h window, calm period pass/fail, insufficient data case

**Implementation:**
```go
// isBaselineCalm: query recent CalmPeriod of data, check if recent mean
// is within CalmSigma stddevs of the full baseline mean.
// Return false if fewer than 3 recent data points.
// When false, append to QualityStatus.ScopeLimitations.
```

#### Task 1B: WhileEffective Temporal Semantics (W3)

**Files:**
- CREATE `internal/rca/settings.go` — `SettingsProvider` interface with `GetSettingValue()` and `GetSettingChanges()`, plus `SettingChange` struct
- CREATE `internal/rca/settings_adapter.go` — `SnapshotSettingsProvider` implementing `SettingsProvider` using `settings.SnapshotStore` + optional `InstanceConnProvider` fallback
- CREATE `internal/rca/settings_adapter_test.go` — test with mock store
- MODIFY `internal/rca/graph.go` — implement `WhileEffective` case in `evaluateEdge()`. The engine must have `settingsProvider` field. When WhileEffective: query `GetSettingChanges()` for the analysis window, check if any relevant setting changed and is still in effect via `GetSettingValue()`
- MODIFY `internal/rca/engine.go` — accept `SettingsProvider` in constructor, pass to graph evaluator

**Relevant settings mapping** (use MechanismKey from edge context):
```go
var settingsByMechanism = map[string][]string{
    MechCheckpointStorm:    {"checkpoint_completion_target", "max_wal_size", "checkpoint_timeout"},
    MechMemoryPressure:     {"shared_buffers", "work_mem", "maintenance_work_mem"},
    MechConnectionExhaust:  {"max_connections", "superuser_reserved_connections"},
    MechVacuumFailing:      {"autovacuum_max_workers", "autovacuum_naptime"},
    MechWALBloat:           {"max_wal_size", "wal_keep_size"},
}
```

**IMPORTANT:** First grep `internal/rca/graph.go` for the existing `WhileEffective` case statement. It currently returns `(0, nil, false)`. Replace that specific block.

#### Task 1C: Statement Snapshot Diff Integration (W4)

**Files:**
- CREATE `internal/rca/statement_source.go` — `StatementDiffSource` struct with `GetStatementAnomalies()` method
- CREATE `internal/rca/statement_source_test.go` — test with mock SnapshotStore
- MODIFY `internal/rca/engine.go` — accept optional `StatementDiffSource` in constructor; call during detect step (step 4 of 9-step algorithm)

**Before implementing:** Grep `internal/statements/diff.go` to find the exact shape of `DiffEntry` and what fields are available. The source needs:
- Query regression: a queryid whose mean exec time increased >2× between snapshots
- New query: a queryid present in latest snapshot but absent from previous, with >5% total_time contribution

**Handle gracefully:** If `statement_snapshots.enabled = false` or no snapshots exist, return empty slice (not error). Add `"statement_snapshots"` to `QualityStatus.UnavailableDeps`.

#### Task 1D: JSON Tag Cleanup (W9 — Go side)

**Files:**
- MODIFY `internal/rca/graph.go` — add `json:"..."` tags to ALL fields of `CausalNode` and `CausalEdge`. Use snake_case: `json:"id"`, `json:"name"`, `json:"metric_keys"`, `json:"from_node"`, `json:"to_node"`, `json:"base_confidence"`, etc.
- MODIFY `internal/api/rca.go` — in the graph handler, convert `MinLag`/`MaxLag` from `time.Duration` to seconds (float64) in the response. Use a response wrapper struct with `min_lag_seconds`/`max_lag_seconds` fields. Do NOT expose nanosecond values.

**CRITICAL:** After adding json tags, the API response field names change from PascalCase to snake_case. Agent 2 must update frontend types to match. Coordinate via the existing field name list in the design doc Section 10.

### Phase 2 — Dependent Work

#### Task 2A: Activate Tier B Chains (W5)

**Depends on:** Task 1B (WhileEffective), Task 1C (StatementDiffSource)

**Files:**
- MODIFY `internal/rca/engine.go` or `internal/rca/ontology.go` — find and REMOVE the Tier B filter. Grep for `TierB`, `Tier B`, `tierB`, or any chain-skipping logic that checks tier classification.

**Before implementing:** Grep `chains.go` for chains 3, 12, 13, 19 to identify their exact chain IDs, node names, and MetricKeys. Verify:
- Chain 3: check what data source it needs. If all MetricKeys are emitted by existing collectors, removing the filter is sufficient.
- Chain 12: needs `pg.statements.regression` anomalies from StatementDiffSource
- Chain 13: needs `pg.statements.new_query` anomalies from StatementDiffSource
- Chain 19: needs WhileEffective temporal semantics working

#### Task 2B: Confidence Model Refinement (W7)

**Files:**
- MODIFY `internal/rca/engine.go` (or wherever edge scoring happens) — replace flat temporal scoring with lag-window-aware `temporalWeight()` function. Add `evidenceMultiplier()` for z-score strength.
- MODIFY `internal/rca/chains.go` — review and adjust `BaseConfidence` values with documented reasoning. Add comments to each edge explaining the confidence level.

**Temporal weight formula:**
```
Within [MinLag, MaxLag] window → weight = 1.0
Outside window → weight = exp(-0.693 × distance / 2min)  (half-life = 2 minutes)
```

**Evidence multiplier:**
```
z > 5.0 → min(1.0 + (z-5)*0.02, 1.1)
z < 2.0 → max(0.5 + z*0.25, 0.5)
else    → 1.0
```

#### Task 2C: RCA→Adviser Bridge Backend (W8)

**Files:**
- CREATE `migrations/017_recommendation_rca_bridge.sql` — exact SQL from D506 doc Section 3.2, including partial unique index, GIN index, urgency backfill
- CREATE `internal/remediation/hooks.go` — `hookToRuleID` map from ontology Hook constants to remediation rule IDs
- CREATE `internal/remediation/urgency.go` — `UrgencyFromPriority()`, constants `UrgencyBaseCritical=3.0`, `UrgencyBaseWarning=2.0`, `UrgencyBaseInfo=1.0`, `UrgencyDeltaRCAIncident=1.0`
- MODIFY `internal/remediation/rule.go` — add `Source string`, `UrgencyScore float64`, `IncidentIDs []int64`, `LastIncidentAt *time.Time` to Recommendation struct with json tags
- MODIFY `internal/remediation/store.go` — add `Upsert()` and `ListByIncident()` to interface; add `Source` and `OrderBy` to ListOpts
- MODIFY `internal/remediation/pgstore.go` — implement `Upsert()` with INSERT ON CONFLICT + LEAST() cap; implement `ListByIncident()` with GIN query; update `Write()` to set `urgency_score = UrgencyFromPriority(priority)` for all new recommendations; update all SELECT queries to include new columns; update column lists in scan calls
- MODIFY `internal/remediation/nullstore.go` — add no-op `Upsert()` returning populated struct, `ListByIncident()` returning nil
- MODIFY `internal/remediation/engine.go` — add `EvaluateHook(ctx, hookID, instanceID, incidentID, incidentTime)` method. Logic: lookup hookToRuleID → evaluate rule → build Recommendation with source="rca" → call store.Upsert()
- MODIFY `internal/config/config.go` — add `RCAUrgencyDelta float64` and `ForecastUrgencyDelta float64` to RemediationConfig with yaml tags and defaults
- MODIFY `internal/rca/engine.go` — after incident is stored and primary chain has RemediationHook, call `e.remediationEngine.EvaluateHook()`
- MODIFY `internal/api/remediation.go` — add `incident_id`, `source`, `order_by` query params to `handleListAllRecommendations` and `handleListRecommendations`

**Before implementing:** Grep `internal/remediation/pgstore.go` for the existing `Write()` method to see the exact column list and scan pattern. The new columns must be added to ALL queries that return Recommendation rows.

**CRITICAL pgx note:** `incident_ids BIGINT[]` maps to `[]int64` in Go. pgx/v5 handles this natively. If scan errors occur, use `pgtype.FlatArray[int64]` as an intermediary.

**Tests:**
- MODIFY `internal/remediation/pgstore_test.go` — add `TestUpsert_New`, `TestUpsert_Existing`, `TestUpsert_DuplicateIncident`, `TestUpsert_SoftCap`, `TestListByIncident`, `TestListByIncident_Empty`
- MODIFY `internal/remediation/engine_test.go` — add `TestEvaluateHook_Match`, `TestEvaluateHook_NoRule`, `TestEvaluateHook_UpsertExisting`

#### Task 2D: Review Instrumentation Backend (W10)

**Files:**
- MODIFY `internal/rca/store.go` — add `UpdateReview(ctx, id int64, status, notes string) error` to IncidentStore interface
- MODIFY `internal/rca/pgstore.go` — implement `UpdateReview`: `UPDATE rca_incidents SET review_status = $2, review_notes = $3, updated_at = NOW() WHERE id = $1`
- MODIFY `internal/rca/nullstore.go` — add no-op `UpdateReview`
- MODIFY `internal/api/rca.go` — add `PUT /api/v1/rca/incidents/{id}/review` handler

**Before implementing:** Grep `migrations/016_*.sql` to confirm `review_status` and `review_notes` columns exist on `rca_incidents`. Also grep `internal/rca/incident.go` for the Incident struct to see if these fields are already defined.

### Phase 3 — Summary + Metric Verification

#### Task 3A: Summary Generation (W6)

**Depends on:** Phase 2 (needs chains to actually fire for validation)

**Files:**
- MODIFY `internal/rca/incident.go` — rewrite `BuildSummary()` to include specific metric values, timestamps, chain names, and alternative chain mentions. Each timeline event Description must include metric value, baseline, z-score, and timestamp.

**Format:**
```
"Primary chain: {ChainName} (confidence: {bucket}, score: {score}).
 {RootNodeName} {rose|dropped} to {value} at {HH:MM UTC} (baseline: {baseline}, z-score: {zscore}).
 Triggered by {metric} = {value} at {HH:MM UTC}.
 Alternative explanation: {AltChainName} (confidence: {bucket}, score: {score})."
```

#### Task 3B: Metric Key Verification (W1 — Go side)

**Files:**
- MODIFY `internal/rca/chains.go` — verify every node's MetricKeys against collector output. Fix any mismatches found during pre-flight greps.

**Produce a verification table as a code comment at the top of chains.go:**
```go
// Metric Key Verification (M14_03):
// Chain 1 "..." — all MetricKeys confirmed against collector catalog ✓
// Chain 2 "..." — all MetricKeys confirmed ✓
// ...
```

### Build Verification

After ALL changes:
```bash
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...
```

Use `./cmd/... ./internal/...` not `./...` — avoids scanning `web/node_modules`.

---

## Agent 2: FRONTEND + CONFIG AGENT

### Ownership

All TypeScript/React changes. Demo VM config deployment. Specifically: W8 (frontend), W9 (TypeScript types), W10 (frontend widget), W1 (demo YAML + deployment), W6 (summary display verification).

### Phase 1 — Type Updates + Foundation

#### Task 1E: JSON Tag Type Updates (W9 — Frontend side)

**Files:**
- MODIFY `web/src/types/rca.ts` — update ALL field names from PascalCase to snake_case:

```typescript
// RCACausalNode fields:
//   ID → id, Name → name, MetricKeys → metric_keys, Layer → layer,
//   SymptomKey → symptom_key, MechanismKey → mechanism_key

// RCACausalEdge fields:
//   FromNode → from_node, ToNode → to_node,
//   MinLag/MaxLag → min_lag_seconds/max_lag_seconds (now float64 seconds, not nanoseconds),
//   Temporal → temporal, Evidence → evidence,
//   BaseConfidence → base_confidence, ChainID → chain_id,
//   RemediationHook → remediation_hook
```

- MODIFY `web/src/components/rca/CausalGraphView.tsx` — update all field references to use new snake_case names. Grep for `\.ID`, `\.Name`, `\.MetricKeys`, `\.FromNode`, `\.ToNode` etc and replace with `.id`, `.name`, `.metric_keys`, `.from_node`, `.to_node`.

**Before implementing:** Grep `web/src/` for every usage of the old PascalCase field names from the RCA types. Every single reference must be updated.

#### Task 1F: Recommendation Type Updates (W8 — Frontend types)

**Files:**
- MODIFY `web/src/types/remediation.ts` (or `web/src/types/models.ts` — grep to find where `Recommendation` interface lives) — add:
```typescript
source: 'background' | 'rca' | 'alert' | 'forecast';
urgency_score: number;
incident_ids: number[];
last_incident_at?: string;
```

### Phase 2 — Components + Pages

#### Task 2E: RCABadge Component (W8)

**Files:**
- CREATE `web/src/components/advisor/RCABadge.tsx`

```typescript
// Props: incidentIds: number[], lastIncidentAt?: string
// Renders: link icon + "Linked to N incident(s) (latest: Mar 21)"
// On click: navigate to most recent incident detail page
// Only renders when incidentIds.length > 0
```

Use Tailwind classes consistent with existing badge components (grep `ConfidenceBadge.tsx` or `PriorityBadge.tsx` for patterns).

#### Task 2F: Inline Recommendations on Incident Detail (W8)

**Files:**
- CREATE `web/src/hooks/useRecommendationsByIncident.ts`
```typescript
export function useRecommendationsByIncident(incidentId: number) {
  return useQuery({
    queryKey: ['recommendations', 'incident', incidentId],
    queryFn: () => api.get(`/recommendations?incident_id=${incidentId}`).then(r => r.data),
    enabled: incidentId > 0,
  });
}
```

- MODIFY `web/src/pages/RCAIncidentDetail.tsx` — add a "Recommended Actions" section below the timeline:
  - Query recommendations by incident ID
  - If results exist, render each using the same card component used in Advisor page (grep for `AdvisorRow` usage)
  - If no results, show subtle text: "No automated remediation available for this root cause"
  - Place AFTER timeline section, BEFORE quality banner

#### Task 2G: Adviser Dashboard Updates (W8)

**Files:**
- MODIFY `web/src/pages/Advisor.tsx` — change default sort from `created_at` to `urgency_score DESC`; add `source` filter dropdown (All / Background / RCA / Alert)
- MODIFY `web/src/components/advisor/AdvisorRow.tsx` — render `RCABadge` when `recommendation.incident_ids.length > 0`
- MODIFY `web/src/hooks/useRecommendations.ts` — pass `order_by=urgency_score` and optional `source` param in API calls

#### Task 2H: Review Widget (W10)

**Files:**
- CREATE `web/src/components/rca/ReviewWidget.tsx`
```typescript
// Three buttons in a button group: Confirmed | False Positive | Inconclusive
// Below buttons: collapsible textarea for notes (hidden by default, toggle with "Add notes" link)
// Submit calls PUT /api/v1/rca/incidents/{id}/review
// After success, show current review status as a badge
// If incident already has review_status set, show it and allow change
```

- MODIFY `web/src/pages/RCAIncidentDetail.tsx` — add ReviewWidget at the top of the page (below header card, above chain summary)
- MODIFY `web/src/hooks/useRCA.ts` — add `useReviewIncident` mutation hook

### Phase 3 — Demo Deployment + Validation

#### Task 3C: Demo VM Configuration (W1)

**Steps:**
1. Build the updated binary locally:
```bash
cd ~/Projects/PGPulse_01
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED
```

2. Deploy to demo VM:
```bash
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
```

3. Update the ML config on the demo VM. SSH in and edit `/opt/pgpulse/pgpulse.yml`:
   - Replace the entire `ml:` block with the comprehensive config from the design doc Section 2.2
   - Add new RCA config fields under `rca:`:
     ```yaml
     rca:
       threshold_baseline_window: 4h
       threshold_calm_period: 15m
       threshold_calm_sigma: 1.5
     ```
   - Add new remediation config fields under `remediation:`:
     ```yaml
     remediation:
       rca_urgency_delta: 1.0
       forecast_urgency_delta: 0.5
     ```

4. Deploy and restart:
```bash
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

5. Wait 2 minutes for ML baseline warm-up, then verify:
```bash
ssh ml4dbs@185.159.111.139 'curl -s http://localhost:8989/api/v1/ml/models | python3 -m json.tool | wc -l'
# Expect 30+ entries (one per tracked metric)
```

6. Run chaos test on the chaos instance (:5434) to generate load that should trigger chains.

7. After 5 minutes, check for incidents:
```bash
ssh ml4dbs@185.159.111.139 'curl -s http://localhost:8989/api/v1/rca/incidents | python3 -m json.tool'
# Expect at least one incident with non-empty primary_chain
```

### Build Verification

After ALL frontend changes:
```bash
cd web && npm run build && npm run lint && npm run typecheck
```

---

## Coordination Notes

1. **Agent 1 finishes Phase 1 before Agent 2 needs the API changes.** Agent 2 can work on types and components (Phase 1-2) in parallel since it knows the field names from the design doc.

2. **JSON tag change (W9) is a breaking API change.** Both agents must update their respective sides. Agent 2 should grep ALL frontend files for old PascalCase field references.

3. **The partial unique index in migration 017 is CRITICAL.** Without it, the upsert SQL will fail. Agent 1 must include it in the migration, not as a separate step.

4. **pgx array scanning:** If `[]int64` doesn't scan cleanly from `BIGINT[]`, use `pgtype.FlatArray[int64]` and convert. Test this early.

5. **Demo VM config editing:** Use `head -n` to truncate to the last known-good line before the `ml:` block, then append the new block. Do NOT use nano for large edits — YAML indentation errors have caused deployment issues before.

---

## Expected New Files (Watch List)

```
internal/rca/settings.go
internal/rca/settings_adapter.go
internal/rca/settings_adapter_test.go
internal/rca/statement_source.go
internal/rca/statement_source_test.go
internal/remediation/hooks.go
internal/remediation/urgency.go
migrations/017_recommendation_rca_bridge.sql
web/src/components/advisor/RCABadge.tsx
web/src/components/rca/ReviewWidget.tsx
web/src/hooks/useRecommendationsByIncident.ts
```

## Expected Modified Files

```
internal/rca/config.go
internal/rca/anomaly.go
internal/rca/anomaly_test.go
internal/rca/graph.go
internal/rca/graph_test.go
internal/rca/engine.go
internal/rca/engine_test.go
internal/rca/incident.go
internal/rca/chains.go
internal/rca/ontology.go
internal/rca/store.go
internal/rca/pgstore.go
internal/rca/nullstore.go
internal/rca/nullstore_test.go
internal/remediation/rule.go
internal/remediation/store.go
internal/remediation/pgstore.go
internal/remediation/pgstore_test.go
internal/remediation/nullstore.go
internal/remediation/engine.go
internal/remediation/engine_test.go
internal/config/config.go
internal/api/rca.go
internal/api/remediation.go
web/src/types/rca.ts
web/src/types/remediation.ts (or models.ts)
web/src/components/rca/CausalGraphView.tsx
web/src/pages/RCAIncidentDetail.tsx
web/src/pages/Advisor.tsx
web/src/components/advisor/AdvisorRow.tsx
web/src/hooks/useRecommendations.ts
web/src/hooks/useRCA.ts
```
