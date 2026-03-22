# M14_03 — Pre-Flight Corrections

**Iteration:** M14_03
**Date:** 2026-03-22
**Based on:** 24 pre-flight greps from M14_03_checklist.md Step 5

---

## Summary

| Status | Count | Details |
|--------|-------|---------|
| Clean | 21 | All metric keys verified, types match, routes correct, Tier B filtering identified |
| Acceptable mismatch | 2 | Hook→Rule ID namespaces (5.13); RCAConfig duplication (5.19) |
| Needs investigation | 1 | DiffEntry type shape for StatementDiffSource (5.4) |

**No blocking issues.** The M14_01/02 implementation is structurally sound. Three items below require agent attention during implementation.

---

## Correction 1: Hook→Rule ID Namespace Mismatch (Grep 5.13)

**What the design doc assumes:** The `hookToRuleID` map in `internal/remediation/hooks.go` maps ontology `Hook*` constants (e.g., `"remediation.checkpoint_completion_target"`) directly to remediation rule IDs (e.g., `"pg_checkpoint_completion"`).

**What the codebase actually has:** The Hook constants in `internal/rca/ontology.go` and the rule IDs in `internal/remediation/rules_pg.go` / `rules_os.go` use different naming conventions. This is by design — the ontology owns the hook namespace, the remediation engine owns the rule ID namespace.

**Action for Agent 1:**
1. Before writing `hooks.go`, grep both files to extract the exact values:
   ```bash
   grep -n "Hook" internal/rca/ontology.go
   grep -n "func.*Rule\|ID.*=" internal/remediation/rules_pg.go | head -30
   grep -n "func.*Rule\|ID.*=" internal/remediation/rules_os.go | head -10
   ```
2. Build the `hookToRuleID` map using the **actual** constant values from both sides, not the example values in the design doc Section 5.2. The design doc values are illustrative — the real mapping must come from the code.
3. If a Hook constant has no corresponding rule ID, that's expected — not every hook has a rule yet. Log a warning and return nil from `EvaluateHook()`.

---

## Correction 2: RCAConfig Duplication Across Packages (Grep 5.19)

**What the design doc assumes:** New threshold config fields (`ThresholdBaselineWindow`, `ThresholdCalmPeriod`, `ThresholdCalmSigma`) are added to the `RCAConfig` struct in `internal/rca/config.go`.

**What the codebase actually has:** There are two config-related locations:
- `internal/rca/config.go` — contains `RCAConfig` struct used by the RCA engine directly
- `internal/config/config.go` — contains the top-level `Config` struct with an `RCA` field that gets parsed from YAML

This split exists to avoid circular imports (the `config` package can't import `rca`, and `rca` can't import `config`). The YAML is parsed into `config.Config.RCA`, which is then passed to the RCA engine constructor.

**Action for Agent 1:**
1. Add the new threshold fields to `RCAConfig` in `internal/rca/config.go` (where the engine reads them)
2. Verify that `internal/config/config.go` either embeds or mirrors the same struct. If it mirrors, add the same fields there with matching `yaml:"..."` tags
3. Verify the constructor in `engine.go` — confirm how `RCAConfig` flows from the parsed YAML into the engine. Follow the data path:
   ```bash
   grep -n "RCAConfig\|rca.Config\|rcaConfig\|rcaCfg" cmd/pgpulse-server/main.go
   ```
4. Same pattern applies for the new `RemediationConfig` fields (`RCAUrgencyDelta`, `ForecastUrgencyDelta`) — verify where `RemediationConfig` is defined and how it flows to the engine

---

## Correction 3: DiffEntry Type Shape for StatementDiffSource (Grep 5.4)

**What the design doc assumes:** `internal/statements/diff.go` provides a `DiffEntry` type with fields like `MeanExecTimeRatio`, `MeanExecTimeOld`, `MeanExecTimeNew`, `TotalTimePct`, and that there are separate `Regressions` and `NewQueries` slices.

**What the codebase actually has:** The exact field names and structure need to be confirmed by the agent at implementation time. The diff engine exists and works, but the RCA source adapter must use the real field names.

**Action for Agent 1:**
1. Before writing `statement_source.go`, read the actual types:
   ```bash
   cat internal/statements/diff.go | head -80
   cat internal/statements/types.go | grep -A 30 "type DiffEntry\|type Diff "
   ```
2. Adapt the `StatementDiffSource` implementation to match the real `DiffEntry` shape. The design doc's field names are illustrative — the agent must use whatever fields actually exist.
3. Specifically check:
   - How regression is quantified (ratio? absolute delta? percentage?)
   - How new queries are identified (set difference? flag field?)
   - Whether `ComputeDiff()` returns a struct with categorized entries or a flat slice
4. If the existing diff types don't expose what the RCA source needs (e.g., no regression ratio), add a small helper function in `statement_source.go` that computes it from the raw diff data. Do NOT modify `internal/statements/diff.go` unless absolutely necessary — that package is stable.

---

## Clean Checks (No Action Required)

The following assumptions from the design doc are confirmed correct:

| Grep | Check | Result |
|------|-------|--------|
| 5.1 | Chain MetricKeys exist in collector catalog | ✓ All verified |
| 5.2 | Tier B filter location identified | ✓ Found, removable |
| 5.3 | WhileEffective stub in graph.go | ✓ Returns (0, nil, false) as expected |
| 5.5 | Recommendation struct shape | ✓ Matches design doc Section 2.2 |
| 5.6 | RecommendationStore interface | ✓ Methods match |
| 5.7 | Write() column list | ✓ Identifiable, new columns can be appended |
| 5.8 | Scan patterns in pgstore | ✓ Consistent pattern, extendable |
| 5.9 | Engine constructor | ✓ Accepts config + dependencies |
| 5.10 | Analyze() method structure | ✓ 9-step algorithm, injection points identifiable |
| 5.11 | RemediationHook on edges | ✓ Present in chains.go |
| 5.12 | Hook constants in ontology | ✓ Present and prefixed |
| 5.14 | review_status in migration 016 | ✓ Columns exist |
| 5.15 | Frontend Recommendation type | ✓ Located |
| 5.16 | Frontend RCA types (PascalCase) | ✓ Confirmed PascalCase, needs snake_case update |
| 5.17 | CausalGraphView field access | ✓ All references identified for renaming |
| 5.18 | Settings SnapshotStore interface | ✓ Available for SettingsProvider adapter |
| 5.20 | Demo ML config | ✓ Old keys confirmed — needs replacement |
| 5.21 | Incident struct | ✓ Shape confirmed |
| 5.22 | RCA API routes | ✓ All registered in server.go |
| 5.23 | Adviser sort order | ✓ Current default identified, changeable |
| 5.24 | InstanceConnProvider in RCA | ✓ Accessible via engine dependencies |
