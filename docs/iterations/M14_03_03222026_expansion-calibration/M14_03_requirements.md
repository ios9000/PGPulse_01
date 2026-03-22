# M14_03 — Requirements: Expansion, Calibration, and Knowledge Integration

**Iteration:** M14_03
**Date:** 2026-03-22
**Predecessor:** M14_02 (RCA UI — complete)
**Locked Decisions:** D500–D508, Q1–Q3

---

## 1. Objective

Make the RCA engine operationally useful. M14_01 built the algorithm, M14_02 built the UI — M14_03 makes chains actually fire on real PostgreSQL workloads and connects RCA findings to the Adviser subsystem so DBAs can act on them.

---

## 2. Scope Summary

| # | Work Item | Tier | Dependency |
|---|-----------|------|------------|
| W1 | ML metric key alignment — comprehensive mapping | 1 | None |
| W2 | Threshold fallback hardening — 4h window + 15min calm period | 1 | None |
| W3 | WhileEffective temporal semantics — chain 19 | 1 | None |
| W4 | Statement snapshot diff integration — chains 12, 13 | 1 | None |
| W5 | Activate Tier B chains — remove filter, wire data sources | 1 | W3, W4 |
| W6 | Improve summary generation | 2 | W1 (needs chains to fire) |
| W7 | Strengthen confidence model | 2 | W1 (needs chains to fire) |
| W8 | RCA→Adviser bridge (Unified Upsert) | 2 | W5 |
| W9 | JSON tag cleanup on CausalNode/CausalEdge | 3 | None |
| W10 | Rule-quality instrumentation stubs | 3 | W8 |

---

## 3. Detailed Requirements

### W1 — ML Metric Key Alignment (Comprehensive)

**Decision:** D501 — comprehensive mapping, not minimum-to-fire.

**Problem:** The demo `pgpulse.yml` ML config references old metric key names (`connections_active`, `cache_hit_ratio`) that don't match actual collector output (`pg.connections.utilization_pct`, `pg.cache.hit_ratio`). The ML detector never sees the data the RCA chains need.

**Requirements:**

- R1.1: Audit all 20 causal chain node `MetricKeys` in `chains.go` against the metric key catalog (Section 3 of CODEBASE_DIGEST). Produce a mapping table: chain node → metric key(s) it references → collector that emits them → collection frequency.

- R1.2: Build a comprehensive `ml.metrics[]` YAML config block that covers every metric key referenced by any causal chain node. Group by collection tier (high/medium/low). Include seasonal period estimates appropriate for each metric type.

- R1.3: Update the demo `pgpulse.yml` on the demo VM (`185.159.111.139`) with the corrected config block. The ML detector must track all metrics that any chain could reference.

- R1.4: Verify that after restart with new config, the ML detector's `ListModels()` output includes all expected metric keys.

**Metric keys that MUST be in the ML config (minimum set for all 20 chains):**

Instance-level (high frequency):
- `pg.connections.utilization_pct`, `pg.connections.total`, `pg.connections.by_state` (state=active, idle, idle_in_transaction)
- `pg.cache.hit_ratio`
- `pg.locks.blocker_count`, `pg.locks.blocked_count`, `pg.locks.max_chain_depth`
- `pg.long_transactions.oldest_seconds`, `pg.long_transactions.count`
- `pg.wait_events.count` (by wait_event_type)

Instance-level (medium frequency):
- `pg.checkpoint.timed`, `pg.checkpoint.requested`, `pg.checkpoint.write_time_ms`, `pg.checkpoint.sync_time_ms`
- `pg.bgwriter.buffers_backend`, `pg.bgwriter.buffers_clean`
- `pg.replication.lag.total_bytes`, `pg.replication.lag.replay_seconds`
- `pg.replication.slot.retained_bytes`
- `pg.statements.fill_pct`

Instance-level (low frequency):
- `pg.server.wraparound_pct`
- `pg.database.size_bytes`

OS-level:
- `os.cpu.user_pct`, `os.cpu.iowait_pct`
- `os.memory.available_kb`, `os.memory.used_kb`
- `os.disk.util_pct`, `os.disk.free_bytes`
- `os.load.1m`, `os.load.5m`

Transaction metrics:
- `pg.transactions.commit_ratio`, `pg.transactions.deadlocks`

**Note:** The above is the minimum-required subset. Since D501 requires comprehensive mapping, agents must also include any additional metrics from the full catalog (~163 keys in CODEBASE_DIGEST) that would add value for anomaly detection, even if no chain currently references them. Use judgment — not every metric benefits from ML tracking (e.g., label-type metrics like `pg.server.hostname` are excluded).

### W2 — Threshold Fallback Hardening

**Decision:** D503 — option (a) + (b): 4h baseline window + 15min calm period.

**Problem:** The `ThresholdAnomalySource` computes baselines from the last hour, which may include chaos-level values from prior test runs. When the ML source has no data, the fallback produces unreliable baselines.

**Requirements:**

- R2.1: Change the threshold fallback's stats query window from 1h to 4h. This should be a configurable value in `RCAConfig`, e.g., `threshold_baseline_window: 4h`.

- R2.2: Implement a "calm period" check: before trusting the baseline, verify that the most recent 15 minutes of data for the metric are within 1.5σ of the rolling mean. If the check fails, mark the metric's anomaly detection as "baseline unreliable" and reflect this in the `QualityStatus.AnomalySourceMode` field.

- R2.3: Both values (calm period duration and sigma threshold) must be configurable in `RCAConfig`:
  ```yaml
  rca:
    threshold_baseline_window: 4h
    threshold_calm_period: 15m
    threshold_calm_sigma: 1.5
  ```

- R2.4: When the calm period check fails, the engine must still attempt analysis using the ML source. Only if both sources are unreliable should the quality banner show "Limited telemetry confidence."

### W3 — WhileEffective Temporal Semantics

**Problem:** Chain 19 (config change → behavioral shift) requires knowing whether a settings change is still in effect. The `WhileEffective` temporal mode is currently a stub that returns `(0, nil, false)` in `evaluateEdge()`.

**Requirements:**

- R3.1: Implement `WhileEffective` in `graph.go`'s `evaluateEdge()`. Logic: the edge score is non-zero if the source node's condition (a settings change) is currently still in effect — meaning the setting's current value differs from its `boot_val` or `reset_val`, indicating a deliberate change.

- R3.2: The implementation must query the settings snapshot store (`internal/settings/`) to find the most recent snapshot for the instance within the analysis window. If a relevant setting changed in the last N hours and the new value is still active, the edge fires.

- R3.3: The engine needs access to an `InstanceConnProvider` or equivalent to query current settings values via SQL if no snapshot is recent enough. Prefer snapshot data over live queries.

### W4 — Statement Snapshot Diff Integration

**Problem:** Chains 12 (query regression) and 13 (new query pattern) need statement snapshot diff data. The `internal/statements/diff.go` engine exists but isn't connected to RCA.

**Requirements:**

- R4.1: Create a new anomaly source type (or adapter) that wraps the statement diff engine. It should detect:
  - **Query regression** (chain 12): a queryid whose `mean_exec_time` increased by >2× between the two most recent snapshots
  - **New query** (chain 13): a queryid present in the latest snapshot but absent from the previous one, with significant total_time contribution

- R4.2: The adapter must implement the `AnomalySource` interface or provide data through a compatible path. The RCA engine calls it during the "detect anomalies" step.

- R4.3: The adapter must handle the case where `statement_snapshots.enabled = false` or no snapshots exist — return empty results, not an error. Reflect missing statement snapshots in `QualityStatus.UnavailableDeps`.

### W5 — Activate Tier B Chains

**Problem:** Chains 3, 12, 13, 19 are currently Tier B (stubbed). M14_03 provides their missing data sources.

**Requirements:**

- R5.1: Remove the Tier B filter from the engine's chain selection logic. All 20 chains participate in analysis.

- R5.2: Chain 3 — verify its data source requirements are met by existing collectors. If it only needs metric keys that collectors already emit, simply removing the filter is sufficient.

- R5.3: Chain 12 — wire to statement diff adapter (W4). Must detect query regression anomalies.

- R5.4: Chain 13 — wire to statement diff adapter (W4). Must detect new query pattern anomalies.

- R5.5: Chain 19 — wire to settings effective-state reasoning (W3). Must detect config-change-driven behavioral shifts.

### W6 — Improve Summary Generation

**Requirements:**

- R6.1: The incident summary must include specific metric values and timestamps. Instead of "Connection pressure detected," produce "Connection utilization rose to 94.2% at 14:23 UTC (baseline: 45.3%), indicating connection pool saturation."

- R6.2: The summary must name the root cause chain in plain language: "Primary chain: Checkpoint I/O Storm → Buffer Backend Writes → Connection Queueing (confidence: high, score: 0.82)."

- R6.3: If alternative chains exist, the summary must note them: "Alternative explanation: Memory Pressure → OS Swapping → I/O Latency (confidence: medium, score: 0.54)."

- R6.4: Timeline events in the `Description` field must include the actual value and baseline, e.g., "pg.cache.hit_ratio dropped to 0.71 (baseline: 0.98, z-score: 4.2) at 14:21 UTC."

### W7 — Strengthen Confidence Model

**Requirements:**

- R7.1: Review and tune edge `BaseConfidence` values in `chains.go` based on domain knowledge. Document the reasoning for each value (e.g., "checkpoint_write_time → buffer_backend_writes has BaseConfidence=0.85 because this is a near-deterministic causal link in PostgreSQL").

- R7.2: Refine temporal proximity weighting. Currently anomaly events are scored by temporal distance from the trigger. Add a weighting curve that accounts for the expected propagation delay defined in the edge's `MinLag`/`MaxLag` — anomalies arriving within the expected lag window get full weight, those outside get exponentially decaying weight.

- R7.3: Add an evidence-strength multiplier: when an anomaly's z-score is very high (>5σ), the edge score should get a small boost (capped at 1.1×). This prevents weak anomalies from contributing as much as strong ones.

### W8 — RCA→Adviser Bridge (Unified Upsert)

**Decision:** D505 — Option (c) via Unified Upsert. D506–D508 + Q1–Q3 all locked.

**Full specification in:** `D506_recommendation_schema_evolution.md` and `Architecture Decision Record RCA - Adviser Bridge (M14_03).txt`

**Requirements:**

- R8.1: Migration 017 — add `source`, `urgency_score`, `incident_ids`, `last_incident_at` columns to `recommendations` table. Add partial unique index `(rule_id, instance_id) WHERE status = 'active'`. Add GIN index on `incident_ids`. Backfill `urgency_score` from priority.

- R8.2: Update `Recommendation` Go struct and frontend TypeScript type with new fields.

- R8.3: Add `Upsert()` and `ListByIncident()` methods to `RecommendationStore` interface and `PGRecommendationStore` implementation. Update `NullRecommendationStore` accordingly.

- R8.4: Implement `EvaluateHook()` on `remediation.Engine`. Create `hookToRuleID` registry in `internal/remediation/hooks.go`. Create urgency scoring in `internal/remediation/urgency.go`.

- R8.5: Call `EvaluateHook()` from the RCA engine's `Analyze()` method after a chain fires with a `RemediationHook`.

- R8.6: `Write()` must initialize `urgency_score = UrgencyFromPriority(priority)` for all new recommendations (Q2 — mandatory).

- R8.7: `Upsert()` must use `LEAST(urgency + delta, 10.0)` for soft cap (Q1).

- R8.8: Update `ListOpts` with `Source` and `OrderBy` fields.

- R8.9: Add `incident_id` query parameter to `GET /api/v1/recommendations`.

- R8.10: Frontend — add `RCABadge` component, inline recommendations on Incident Detail page, sort Adviser feed by `urgency_score` by default.

- R8.11: Add `rca_urgency_delta` and `forecast_urgency_delta` to `RemediationConfig`.

### W9 — JSON Tag Cleanup

**Decision:** D504 — do in M14_03.

**Requirements:**

- R9.1: Add `json:"..."` tags to all fields of `CausalNode` and `CausalEdge` structs in `graph.go` for lowercase consistency. Use snake_case matching the existing pattern: `json:"id"`, `json:"name"`, `json:"metric_keys"`, `json:"from_node"`, `json:"to_node"`, etc.

- R9.2: Update `web/src/types/rca.ts` to match the new lowercase field names.

- R9.3: Verify `CausalGraphView.tsx` renders correctly with the updated field names.

- R9.4: The `MinLag`/`MaxLag` fields should serialize as seconds (float64), not nanoseconds. Add a custom JSON marshaler or use `time.Duration` → seconds conversion in the API handler. This fixes the "Edge MinLag/MaxLag serialized as nanoseconds" known issue from the handoff.

### W10 — Rule-Quality Instrumentation Stubs

**Requirements:**

- R10.1: The `rca_incidents` table already has `review_status` columns. Wire a simple API endpoint: `PUT /api/v1/rca/incidents/{id}/review` accepting `{ "status": "confirmed" | "false_positive" | "inconclusive", "notes": "..." }`.

- R10.2: Frontend: add a small review widget on the Incident Detail page — three buttons (Confirmed / False Positive / Inconclusive) + optional notes text area.

- R10.3: No analytics or feedback loop in M14_03 — this is purely instrumentation for future use.

---

## 4. Acceptance Criteria

| # | Criterion | Validates |
|---|-----------|-----------|
| AC1 | After demo VM restart with updated config, `GET /api/v1/ml/models` returns entries for all metric keys listed in W1 | W1 |
| AC2 | Running chaos script on demo VM produces at least one RCA incident with a non-empty primary chain within 5 minutes | W1, W2, W5 |
| AC3 | The fired chain's timeline events contain specific metric values (not just "anomaly detected") | W6 |
| AC4 | The incident summary includes metric values, timestamps, and chain name in plain English | W6 |
| AC5 | If the fired chain has a RemediationHook, a recommendation appears in both the Incident Detail page and the global Adviser feed | W8 |
| AC6 | The Adviser feed sorts by urgency_score DESC; RCA-linked items show an incident badge | W8 |
| AC7 | A second incident for the same root cause bumps the existing recommendation's urgency (no duplicate) | W8 |
| AC8 | The causal graph API returns lowercase JSON field names | W9 |
| AC9 | Chain 19 fires when a settings change is detected within the analysis window | W3, W5 |
| AC10 | Chains 12/13 fire when statement snapshots show query regression / new query | W4, W5 |
| AC11 | Threshold fallback with 4h window + calm period check produces "baseline unreliable" quality note when recent data is volatile | W2 |
| AC12 | Review buttons on Incident Detail page successfully update `review_status` | W10 |
| AC13 | Full build verification passes: `cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...` | All |

---

## 5. Out of Scope

- Full feedback loop (learning from confirmed/false-positive reviews) — deferred to future iteration
- Time-decay cron for urgency_score — identified as future mitigation in ADR
- Enrichment history audit log (`enrichment_history JSONB`) — future iteration
- Resolved recommendation recurrence lineage (Q3 — deferred)
- New causal chains beyond the existing 20
- Frontend Settings Diff page 404 fix (pre-existing, not M14)
