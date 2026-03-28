# M15_01 — Pre-Flight Corrections

**Date:** 2026-03-27
**Source:** CODEBASE_DIGEST.md (generated 2026-03-26, commit 5e5bf2f6)
**Method:** Project Knowledge search against the digest. Live codebase grep (Step 5 in checklist) should CONFIRM or OVERRIDE each finding below.

---

## C1 — ConnForDB May Already Exist

**Finding:** The CODEBASE_DIGEST interface table shows:

```
| InstanceConnProvider | internal/api/connprovider.go | ConnFor(), ConnForDB() | api/, orchestrator |
```

The file is only 14 lines. The design doc (Section 9) treats `ConnForDB` as something M15_01 must ADD to the interface. If it already exists, the Backend agent must NOT re-declare it — just use it.

**Action:** Run `grep -n "ConnForDB" internal/api/connprovider.go` on the live codebase.

- If **present**: Remove `connprovider.go` from the "Modified Files" list. Update the team-prompt to say "ConnForDB already exists — use it, do not re-add."
- If **absent**: The digest may be stale or aspirational. Proceed with the design as written — add `ConnForDB` to the interface.

**Severity:** 🔴 High — adding a duplicate method to an interface causes a compile error.

---

## C2 — BaselineProvider Interface Mismatch

**Finding:** The design doc defines:

```go
type BaselineProvider interface {
    GetBaselineStats(instanceID, metricKey string) (trend []float64, residualStdDev float64, ok bool)
}
```

The CODEBASE_DIGEST shows `ml/detector.go` (292 lines) and `ml/baseline.go` (242 lines, "Rolling baseline with seasonal decomposition"). There is NO evidence that `ml.Detector` exports a method with the exact signature `GetBaselineStats(instanceID, metricKey) ([]float64, float64, bool)`.

The existing ML integration pattern uses `ForecastProvider` in `internal/alert/forecast.go` (23 lines) to bridge ML to alerts without direct import. The same adapter pattern should be used here.

**Action:** Run on live codebase:

```bash
grep -rn "func.*Detector.*Get\|func.*Detector.*Baseline\|func.*Detector.*Trend\|func.*Detector.*Stats" internal/ml/detector.go
grep -rn "func.*Baseline.*Get\|func.*RollingBaseline" internal/ml/baseline.go
```

- If a compatible method exists (even with different name/signature): write an adapter in `internal/forecast/` that wraps it to satisfy `BaselineProvider`.
- If no compatible method exists: the Backend agent must build a thin adapter or add a method to `ml.Detector` that extracts the trend slope and residual stddev from the existing `RollingBaseline` struct. Do NOT rewrite the ML detector — write a wrapper.

**Severity:** 🟡 Medium — the design explicitly says `BaselineProvider` can be nil if ML is disabled, so this won't block the build. But it WILL block ML-enhanced forecasting.

---

## C3 — ActiveInstanceIDs May Not Exist on Orchestrator

**Finding:** The design and main.go wire-up assume `orchestrator` satisfies the `InstanceLister` interface:

```go
type InstanceLister interface {
    ActiveInstanceIDs(ctx context.Context) ([]string, error)
}
```

The CODEBASE_DIGEST shows `orchestrator.go` (301 lines) and `runner.go` (223 lines). There is no evidence of an `ActiveInstanceIDs()` method.

**Action:** Run:

```bash
grep -rn "func.*Orchestrator.*Instance\|func.*Orchestrator.*Active\|func.*Orchestrator.*IDs" internal/orchestrator/orchestrator.go
grep -rn "ActiveInstance\|InstanceIDs\|instanceRunners" internal/orchestrator/
```

- If a method like `RunningInstances()` or `InstanceIDs()` exists: write a thin adapter or rename the interface method to match.
- If no method exists: the Backend agent must add `ActiveInstanceIDs() ([]string, error)` to the orchestrator. This should return the keys of the internal `instanceRunners` map. Minimal change — one method, ~5 lines.
- **Fallback alternative:** Use `InstanceStore.List()` from `internal/storage/instances.go` instead, filtering to `Enabled == true`. This avoids modifying the orchestrator.

**Severity:** 🟡 Medium — easy to solve either way, but the agent needs to know WHICH approach before coding the NeedEvaluator.

---

## C4 — WAL Rate Metric Key May Not Exist

**Finding:** The design uses `pg.wal.bytes_rate` for basebackup need forecasting. The CODEBASE_DIGEST metric catalog does NOT show this key in the visible sections. The WAL-related metrics I can confirm:

- `pg.checkpoint.*` (from CheckpointCollector) — includes `buffers_written`, `write_time_ms`
- No `pg.wal.bytes_rate` or `pg.wal.bytes_per_second` visible

The M1 strategy doc mentions `collectWAL() — WAL generation rate (version-gated: xlog vs wal)` but the exact metric key emitted is not visible in the digest.

**Action:** Run:

```bash
grep -rn "pg.wal\." internal/collector/ | grep -v "_test.go"
```

- If `pg.wal.bytes_rate` or similar exists: confirm the exact key and update the design/team-prompt if different.
- If NO WAL rate metric exists: the basebackup forecast feature cannot work without it. Options:
  1. Add a WAL rate metric to an existing collector (CheckpointCollector or a new WalCollector). This is a scope expansion — document it.
  2. Defer basebackup forecasting to M15_02. The design already says `basebackup_interval: 0s` (disabled by default), so this is safe.
  3. Compute WAL rate from `pg_stat_wal.wal_bytes` deltas between collection cycles (PG 14+).

**Severity:** 🟡 Medium — basebackup forecasting is the least critical of the four operations. Deferring it is acceptable.

---

## C5 — Basebackup Progress Labels Differ from Other Operations

**Finding:** The CODEBASE_DIGEST metric catalog shows label asymmetry:

| Collector | Labels |
|-----------|--------|
| VacuumProgressCollector | `pid, datname, relname, phase` |
| AnalyzeProgressCollector | `pid, datname, relname, phase` |
| CreateIndexProgressCollector | `pid, datname, relname, phase` |
| **BasebackupProgressCollector** | **`pid, phase`** (NO `datname`, NO `relname`) |

The OperationTracker's `startOp()` builds keys as `"{instance}:{pid}:{operation}"` and records `Database` and `Table` from labels. For basebackup operations, `datname` and `relname` will be absent.

**Action:** The Backend agent must handle this in `tracker.go`:

- When the operation is `basebackup`, set `Database = ""` and `Table = ""` (already the zero values, but document explicitly).
- The `maintenance_operations` table already allows `database = ''` and `table_name = ''` (DEFAULT ''), so no schema change needed.
- The ETA calculator must not panic on empty database/table strings.

**Severity:** 🟢 Low — just needs defensive handling, no design change.

---

## C6 — Config Defaults Mechanism

**Finding:** `internal/config/load.go` is 286 lines. The exact mechanism for setting defaults (koanf `Set()`, struct defaults, or a `Defaults()` function) needs to be confirmed.

**Action:** Run:

```bash
grep -n "playbooks\.\|rca\.\|remediation\.\|ml\." internal/config/load.go | head -20
```

The Backend agent must follow the EXACT same pattern used for `playbooks.*`, `rca.*`, and `remediation.*` defaults. Do not invent a new pattern.

**Severity:** 🟢 Low — pattern-matching, not a design issue.

---

## C7 — Analyze Progress Work Unit

**Finding:** The AnalyzeProgressCollector emits `sample_blks_scanned` and `sample_blks_total` as the work-done metrics, unlike vacuum which uses `heap_blks_vacuumed` / `heap_blks_total`.

The OperationTracker's `tick()` must use the correct "work done" and "work total" fields per operation type:

| Operation | WorkDone metric | WorkTotal metric |
|-----------|----------------|-----------------|
| vacuum | `pg.progress.vacuum.heap_blks_vacuumed` | `pg.progress.vacuum.heap_blks_total` |
| analyze | `pg.progress.analyze.sample_blks_scanned` | `pg.progress.analyze.sample_blks_total` |
| create_index | `pg.progress.create_index.blocks_done` | `pg.progress.create_index.blocks_total` |
| basebackup | `pg.progress.basebackup.backup_streamed` | `pg.progress.basebackup.backup_total` |

All four also emit `completion_pct` which can be used as the universal progress indicator, but for WMA rate calculation, the raw work units give better precision.

**Action:** The Backend agent should use `completion_pct` for the `PctDone` field and the operation-specific work metrics for the WMA `WorkDone`/`WorkTotal` fields. Document the mapping in `tracker.go`.

**Severity:** 🟢 Low — not a design change, just implementation detail the agent needs.

---

## Summary for Team-Prompt Appendix

Append the following to the team-prompt before agent spawn:

```
--- CORRECTIONS (from pre-flight analysis, M15_01_corrections.md) ---

C1: ConnForDB may already exist in connprovider.go. CHECK FIRST.
    If present, do not re-add. If absent, add it.

C2: BaselineProvider.GetBaselineStats may not exist on ml.Detector.
    Write an adapter in internal/forecast/ that wraps whatever method
    the Detector exposes. Check ml/detector.go and ml/baseline.go for
    available methods. If no compatible method exists, leave BaselineProvider
    as nil — ML enhancement degrades gracefully.

C3: ActiveInstanceIDs may not exist on orchestrator.
    Check internal/orchestrator/orchestrator.go. If missing, either:
    (a) add it (~5 lines, returns keys of instanceRunners map), or
    (b) use InstanceStore.List() filtered to Enabled == true.

C4: pg.wal.bytes_rate metric key may not exist.
    Check internal/collector/ for pg.wal.* metrics. If missing, set
    forecast.basebackup_interval = 0s (disabled) and skip basebackup
    forecasting. Document as known limitation for M15_02.

C5: Basebackup progress has NO datname/relname labels (only pid, phase).
    Handle defensively: Database="" and Table="" for basebackup operations.

C6: Match the EXACT config defaults pattern used by playbooks/rca/remediation
    in internal/config/load.go. Do not invent a new pattern.

C7: Each operation type uses different work-unit metrics for WMA calculation.
    Use completion_pct for PctDone, and operation-specific metrics for
    WorkDone/WorkTotal. See corrections doc for the full mapping table.
```
