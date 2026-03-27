# PGPulse — Iteration Handoff: M14_04 → M15

**Date:** 2026-03-27
**From:** M14_04 (Guided Remediation Playbooks — complete)
**To:** M15 (Maintenance Operation Forecasting)

---

## DO NOT RE-DISCUSS

All D400–D609 decisions remain locked. The entire M14 milestone (M14_01 through M14_04) is complete and deployed.

---

## M14 Retrospective — Full Milestone

### M14_01 — RCA Engine (COMPLETE)

- `internal/rca/` package: 12 production files, ~2,580 lines
- 20 causal chains (16 Tier A + 4 Tier B), 47 shared nodes
- Core 9-step Analyze() algorithm: trigger → scope → query → detect → traverse+prune → rank → build → store
- Required-evidence pruning: branches die when mandatory intermediate evidence is absent
- Dual anomaly source: MLAnomalySource + ThresholdAnomalySource fallback
- AutoTrigger: OnAlert hook, severity check, 15-min cooldown, async 30s-timeout analysis
- 5 API endpoints, migration 016, 30 tests across 6 test files

### M14_02 — RCA UI (COMPLETE)

- RCA Incidents list page (fleet-wide with instance/confidence/trigger filters)
- Per-instance RCA Incidents page (sidebar nav under each server)
- Incident Detail page: header card, summary banner, quality banner, analysis metadata
- Causal Graph page: interactive ECharts force-directed graph, 47 nodes, 20 chains
- "Investigate Root Cause" button in AlertDetailPanel
- 15 new components/pages (~1,200 lines)

### M14_03 — Expansion, Calibration, Knowledge Integration (COMPLETE)

- **ML metric key alignment:** comprehensive mapping — 30+ metrics tracked, baselines fitted (75 across 3 instances)
- **Threshold fallback hardening:** 4h baseline window + 15min calm period check
- **WhileEffective temporal semantics:** chain 19 (config change → behavioral shift) implemented
- **Statement snapshot diff integration:** chains 12/13 (query regression, new query) wired
- **All 20 chains active:** Tier B filter removed
- **Improved summaries:** specific metric values, timestamps, direction verbs
- **Confidence model refined:** temporal proximity weighting with lag-window decay, evidence strength multiplier
- **RCA→Adviser bridge (Unified Upsert):** migration 017, `source`, `urgency_score`, `incident_ids BIGINT[]`, `last_incident_at`. `EvaluateHook()` on `remediation.Engine`. hookToRuleID registry. Soft cap at 10.0 via `LEAST()`. `Write()` initializes urgency_score.
- **JSON tag cleanup:** snake_case on CausalNode/CausalEdge, MinLag/MaxLag as seconds
- **Review instrumentation:** PUT /rca/incidents/{id}/review with confirmed/false_positive/inconclusive
- 11 new files (647 lines), 33 modified files (~3,700 lines changed)
- First real RCA chain fired on demo VM: `chain.longtx_bloat_conn`, z-score 41.9

### M14_04 — Guided Remediation Playbooks (COMPLETE)

- **New subsystem:** `internal/playbook/` — executor, resolver, interpreter, store, seed, feedback worker
- **Four-tier execution safety:** diagnostic (READ ONLY), remediate (confirm), dangerous (RBAC approval), external (manual instructions)
- **Transaction-scoped execution:** BEGIN + SET LOCAL + ROLLBACK pattern (C1 correction)
- **Multi-statement injection guard:** semicolon check before execution (C2)
- **Concurrency guard:** atomic UPDATE ... RETURNING before step execution (C5)
- **Explicit error state machine:** failed steps halt run, offer retry (C6)
- **Lightweight approval flow:** pending_approval status, DBA approves in-run (C7)
- **Playbook Resolver:** 5-level priority (hook > root_cause > metric > adviser_rule > manual)
- **Result interpretation:** static declarative rules with green/yellow/red verdicts. Scope: "first" only for MVP.
- **Bounded conditional branching:** guarded decision trees, no loops/nesting
- **PlaybookRun persistence:** database-stored with resume capability across browser sessions
- **Implicit feedback worker:** auto-detects alert resolution after playbook completion
- **Core 10 seed playbooks:** WAL archive failure, replication lag, connection saturation, lock contention, long transactions, checkpoint storm, disk full, autovacuum failing, wraparound risk, heavy query diagnostics. All seeded as stable. 
- **Migration 018:** playbooks, playbook_steps, playbook_runs, playbook_run_steps (4 tables + GIN index)
- **Integration:** Alert → Playbook, RCA → Playbook, Adviser → Playbook (via Resolver)
- **Frontend:** PlaybookCatalog, PlaybookDetail, PlaybookEditor, PlaybookWizard (step-by-step), PlaybookRunHistory, 18 new components, 5 new pages
- ~25 new Go files, ~23 new React files

---

## Demo VM Validation

### RCA + Playbook End-to-End (Validated 2026-03-25)

1. Connection flood chaos → Alert fires (81% utilization)
2. Manual RCA trigger → chain `chain.longtx_bloat_conn` fires (score 0.49, z-score 41.9)
3. Adviser shows recommendation with RCA badge + "View full analysis" link
4. Playbook Resolver matches `remediation.wal_archive` → WAL Archive Failure playbook
5. Step 1 auto-executes pg_stat_archiver → green verdict "No archive failures" in 3ms
6. Tier 4 step shows manual escalation instructions
7. Run state persists across requests (resume verified)

### Smoke Test Results (2026-03-25)

| Test | Result |
|------|--------|
| Resolver: hook match | ✅ `rca_hook: wal-archive-failure` |
| Resolver: metric match | ✅ `metric: connection-saturation` |
| Resolver: no match | ✅ `playbook: None` |
| Tier 1 auto-execute | ✅ green verdict, real SQL, 3ms |
| Tier 4 external | ✅ manual instructions, no SQL |
| Resume | ✅ step history preserved |
| Injection guard | ✅ "multi-statement SQL is forbidden" |
| READ ONLY sandbox | ⚠️ See known issues |

---

## Known Issues

| Issue | Severity | Notes |
|-------|----------|-------|
| Tier 1 READ ONLY transaction not enforced | 🟡 Medium | `transaction_read_only = off` during Tier 1 execution. pg_monitor role provides equivalent protection (blocks DDL/DML on real tables). C1 correction specified but implementation needs verification/patching. Defense-in-depth incomplete. |
| `pg_stat_statements` not loaded on chaos instance | 🟢 Low | `shared_preload_libraries` missing pgss on port 5434. Statements collectors warn every 60s. Not a PGPulse bug — demo VM config issue. |
| `pg_largeobject` permission denied | 🟢 Low | Database collector's large_object_sizes sub-collector fails. Monitor user needs SELECT on pg_largeobject. Not blocking. |
| Settings Diff page 404 | 🟢 Low | Pre-existing from M14_02. Not M14 related. |
| Interpreter scope "any"/"all" not implemented | 🟢 Info | Deferred per final review. Struct field present, evaluation falls back to "first". All seed playbooks use aggregated SQL (single-row rule). |
| Approval queue (notifications, queue page, delegation) | 🟢 Info | Deferred per D609. Lightweight in-run approval works. |
| Parameterized inputs (PID passing between steps) | 🟢 Info | Deferred per ADR Section 6. Schema supports future implementation. |

---

## Codebase Scale (Post M14)

- **Go files:** ~280 (~46,000 lines)
- **TypeScript files:** ~195 (~18,000 lines)
- **Metric keys:** ~220
- **API endpoints:** ~85
- **Collectors:** 27
- **Frontend pages:** 22
- **React components:** ~110
- **RCA package:** 15 files (~3,300 lines), 20 chains, 47 nodes
- **Playbook package:** ~25 files (~3,000 lines), 10 seed playbooks
- **Migrations:** 018

---

## What M15 Needs to Build On

### M15 — Maintenance Operation Forecasting

From the roadmap: vacuum/analyze/reindex/basebackup scheduling, maintenance window planning ("will this finish by 6am?").

**Integration points already built:**

1. **EvaluateHook() path:** M15 forecast engine can call `remediation.Engine.EvaluateHook()` with `source: "forecast"` to push predictions into the Adviser feed. The `forecast_urgency_delta` config (default 0.5) is already wired.

2. **Playbook Resolver:** M15 can create maintenance-specific playbooks (e.g., "Pre-Maintenance Vacuum Check") bound to forecast hooks. The Resolver will surface them when a forecast predicts maintenance window overrun.

3. **ML baseline data:** 30+ metrics with fitted baselines provide the foundation for time-series forecasting. The `ml.forecast` config section already exists with `horizon`, `confidence_z`, and `alert_min_consecutive`.

4. **Existing collectors:** VacuumProgressCollector, AnalyzeProgressCollector, BasebackupProgressCollector already capture real-time maintenance progress. M15 needs to build the predictive layer on top.

5. **Per-database vacuum stats:** `pg.db.vacuum.*` metrics from DatabaseCollector (dead_tuples, dead_pct, autovacuum_age_sec, last_vacuum timestamps) provide the inputs for vacuum forecasting.

**Key question for M15:** Does forecasting need its own package (`internal/forecast/`) or can it extend `internal/ml/`? The ML detector already has `forecast.go` with basic horizon projection. M15 may need a more sophisticated model (workload-aware, table-size-aware) that justifies a dedicated package.

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

**Demo VM config path:** `/opt/pgpulse/configs/pgpulse.yml` (NOT `/opt/pgpulse/pgpulse.yml`)

---

## Roadmap

| Milestone | Status |
|-----------|--------|
| M14_01 — RCA Engine | ✅ Done |
| M14_02 — RCA UI | ✅ Done |
| M14_03 — Expansion/Calibration | ✅ Done |
| M14_04 — Guided Remediation Playbooks | ✅ Done |
| **M15 — Maintenance Forecasting** | **🔲 Next** |
| M13 — Prometheus Exporter | 🔲 |
| M12_02 — UX + NSIS Installer | 🔲 |

---

## First Customer Request

Email alert notifications for replication issues (from DBA colleague's client demo). This is a configuration task (enable email notifier in `pgpulse.yml`, add alert channel) rather than a feature gap — the notification infrastructure exists from M4.
