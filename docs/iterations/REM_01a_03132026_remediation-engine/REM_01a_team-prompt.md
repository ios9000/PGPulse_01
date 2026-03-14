# REM_01a — Team Prompt

**Paste this into Claude Code to spawn the agent team.**

---

Build the rule-based remediation engine for PGPulse.
Read CLAUDE.md for project context, then read docs/iterations/REM_01a_03132026_remediation-engine/design.md for the full design.

Create a team of 2 specialists:

## API & SECURITY AGENT

### New Package: internal/remediation/

Create the entire `internal/remediation/` package from scratch:

**rule.go** (~80 lines):
- `Priority` type (string): `"info"`, `"suggestion"`, `"action_required"`
- `Category` type (string): `"performance"`, `"capacity"`, `"configuration"`, `"replication"`, `"maintenance"`
- `Rule` struct: ID, Priority, Category, Evaluate func(EvalContext) *RuleResult
- `EvalContext` struct: InstanceID, MetricKey, Value, Labels, Severity, Snapshot
- `MetricSnapshot` type (map[string]float64) with `Get(key) (float64, bool)` method
- `RuleResult` struct: Title, Description, DocURL
- `Recommendation` struct with JSON tags: id, rule_id, instance_id, alert_event_id (omitempty), metric_key, metric_value, priority, category, title, description, doc_url, created_at, acknowledged_at, acknowledged_by

**engine.go** (~90 lines):
- `Engine` struct holding `[]Rule`
- `NewEngine()` — creates engine with all rules from `pgRules()` and `osRules()`
- `EvaluateMetric(ctx, instanceID, metricKey, value, labels, severity, snapshot) []Recommendation` — runs rules matching the metric key
- `Diagnose(ctx, instanceID, snapshot) []Recommendation` — runs ALL rules against snapshot
- `Rules() []Rule` — returns all registered rules for introspection

**rules_pg.go** (~350 lines):
- `pgRules() []Rule` — returns 17 PostgreSQL remediation rules
- Each rule handles BOTH alert-triggered mode (checks ctx.MetricKey) and Diagnose mode (checks ctx.Snapshot)
- Rules (see design.md Section 3.1 for full table):
  1. `rem_conn_high` — connections > 80% of max → suggestion/capacity
  2. `rem_conn_exhausted` — connections ≥ 99% of max → action_required/capacity
  3. `rem_cache_low` — cache hit < 90% → suggestion/performance
  4. `rem_commit_ratio_low` — commit ratio < 90% → suggestion/performance
  5. `rem_repl_lag_bytes` — replay lag > 10MB → suggestion/replication
  6. `rem_repl_lag_critical` — replay lag > 100MB → action_required/replication
  7. `rem_repl_slot_inactive` — inactive slot count > 0 → action_required/replication
  8. `rem_long_txn_warn` — oldest active > 60s → suggestion/performance
  9. `rem_long_txn_crit` — oldest active > 300s → action_required/performance
  10. `rem_locks_blocking` — blocked count > 0 → suggestion/performance
  11. `rem_pgss_fill` — statements fill ≥ 95% → suggestion/maintenance
  12. `rem_wraparound_warn` — wraparound > 20% → suggestion/maintenance
  13. `rem_wraparound_crit` — wraparound > 50% → action_required/maintenance
  14. `rem_track_io` — track_io_timing = 0 → info/configuration
  15. `rem_deadlocks` — deadlocks > 0 → suggestion/performance
  16. `rem_bloat_high` — bloat ratio > 2x → suggestion/maintenance
  17. `rem_bloat_extreme` — bloat ratio > 50x → action_required/maintenance
- Every rule Description should be 2-3 sentences with actionable advice
- Metric keys use the pg.* prefix (post-MN_01 naming)

**rules_os.go** (~200 lines):
- `osRules() []Rule` — returns 8 OS remediation rules
- OS rules ALWAYS use Snapshot.Get() since they may need composite values
- Rules (see design.md Section 3.2):
  18. `rem_cpu_high` — user + system > 80% → suggestion/performance
  19. `rem_cpu_iowait` — iowait > 20% → action_required/performance
  20. `rem_mem_pressure` — available < 10% of total → action_required/capacity
  21. `rem_mem_overcommit` — committed > commit_limit → suggestion/capacity
  22. `rem_load_high` — load 1m > 4.0 → suggestion/performance
  23. `rem_disk_util` — util > 80% → action_required/capacity
  24. `rem_disk_read_latency` — read await > 20ms → suggestion/performance
  25. `rem_disk_write_latency` — write await > 20ms → suggestion/performance

**store.go** (~40 lines):
- `ListOpts` struct: InstanceID, Priority, Category, Acknowledged (*bool), AlertEventID (*int64), Limit, Offset
- `RecommendationStore` interface: Write, ListByInstance, ListAll, ListByAlertEvent, Acknowledge, CleanOld

**pgstore.go** (~200 lines):
- `PGStore` struct with pgxpool
- Implements `RecommendationStore`
- Write: INSERT with RETURNING to get IDs
- ListByInstance: SELECT with dynamic WHERE clauses from ListOpts, returns ([]Recommendation, totalCount, error)
- ListAll: same but no instance filter
- ListByAlertEvent: SELECT WHERE alert_event_id = $1
- Acknowledge: UPDATE SET acknowledged_at = NOW(), acknowledged_by = $2 WHERE id = $1
- CleanOld: DELETE WHERE created_at < NOW() - $1 AND acknowledged_at IS NOT NULL

**nullstore.go** (~40 lines):
- `NullStore` — all methods return empty/nil, matching NullAlertHistoryStore pattern

**adapter.go** (~40 lines):
- `AlertAdapter` wraps Engine to implement `alert.RemediationProvider`
- `MetricSource` interface: `CurrentSnapshot(ctx, instanceID) (MetricSnapshot, error)`
- `NewAlertAdapter(engine, metricSource)` constructor
- `EvaluateForAlert()` method: gets snapshot, calls engine, converts to `alert.RemediationResult`
- IMPORTANT: This file imports `internal/alert` — verify no cycle exists

**metricsource.go** (~40 lines):
- `StoreMetricSource` implements `MetricSource` using `collector.MetricStore`
- `CurrentSnapshot()`: queries store for last 2 minutes of metrics, takes latest value per key

### New File: internal/alert/remediation.go (~20 lines)

- `RemediationResult` struct: RuleID, Title, Description, Priority, Category, DocURL (all string, with JSON tags)
- `RemediationProvider` interface: `EvaluateForAlert(ctx, instanceID, metricKey string, value float64, labels map[string]string, severity string) []RemediationResult`
- This file must NOT import anything from internal/remediation

### New Migration: internal/storage/migrations/013_remediation.sql

- Create `remediation_recommendations` table (see design.md Section 4)
- NO foreign key on alert_event_id (soft reference only)
- Indexes: by instance, by priority, by alert_event, unacknowledged
- All statements use IF NOT EXISTS

### New File: internal/api/remediation.go (~200 lines)

- `handleListRecommendations` — GET /instances/{id}/recommendations
- `handleDiagnose` — POST /instances/{id}/diagnose
- `handleListAllRecommendations` — GET /recommendations
- `handleAcknowledgeRecommendation` — PUT /recommendations/{id}/acknowledge
- `handleListRemediationRules` — GET /recommendations/rules (returns engine.Rules() as JSON)
- Parse query params: priority, category, acknowledged, limit (default 100, max 500), offset

### Modified: internal/alert/dispatcher.go

- Add `remediation RemediationProvider` field to Dispatcher
- Add `SetRemediationProvider(p RemediationProvider)` setter method
- In `fire()` method: after writing alert event to history store, if remediation != nil:
  - Call `d.remediation.EvaluateForAlert(...)`
  - If results non-empty, convert to recommendations and persist via recommendation store
  - The Dispatcher needs access to a RecommendationStore — add via setter or constructor
- CRITICAL: Check the actual method name and signature where alert events are written. Read dispatcher.go first.

### Modified: internal/api/server.go

- Add fields: remediationEngine, remediationStore, metricSource
- Add `SetRemediation(engine *remediation.Engine, store remediation.RecommendationStore, source remediation.MetricSource)` setter
- Add routes in Routes():
  ```
  r.Get("/instances/{id}/recommendations", s.handleListRecommendations)
  r.Post("/instances/{id}/diagnose", s.handleDiagnose)
  r.Get("/recommendations", s.handleListAllRecommendations)
  r.Put("/recommendations/{id}/acknowledge", s.handleAcknowledgeRecommendation)
  r.Get("/recommendations/rules", s.handleListRemediationRules)
  ```

### Modified: internal/api/alerts.go

- In alert history/active responses, include recommendations for each alert event
- Query remediationStore.ListByAlertEvent(alertEventID) and embed in response
- If remediationStore is nil (live mode), skip embedding

### Modified: cmd/pgpulse-server/main.go

- Import `internal/remediation`
- Create `remediation.NewEngine()`
- Create `remediation.NewStoreMetricSource(metricStore)`
- Create `remediation.NewPGStore(pool)` or `remediation.NewNullStore()` based on live mode
- Create `remediation.NewAlertAdapter(engine, metricSource)`
- Call `dispatcher.SetRemediationProvider(adapter)`
- Call `apiServer.SetRemediation(engine, store, metricSource)`

### Modified: internal/alert/template.go

- If alert event has recommendations, include them in the HTML email template
- Add a "Recommendations" section below the alert details
- Each recommendation shows: priority badge, title, description

### CRITICAL RULES FOR THIS AGENT:
- Read dispatcher.go BEFORE modifying it — understand the actual fire() flow
- Read server.go BEFORE adding routes — follow the exact pattern for route groups
- Read alerts.go BEFORE modifying — understand the response struct
- NO import cycles: internal/alert must NOT import internal/remediation
- Dependency direction: remediation → alert (for RemediationResult type), never reverse
- All SQL must use parameterized queries ($1, $2)
- Follow setter pattern for APIServer extensions (SetLiveMode, SetAuthMode precedent)
- Test scope: `go test ./cmd/... ./internal/...` (not `./...`)
- Build scope: `go build ./cmd/... ./internal/...`

---

## QA & REVIEW AGENT

### Test Files to Create

**internal/remediation/engine_test.go** (~300 lines):
- TestEvaluateMetric_MatchingRule — rule fires when condition met
- TestEvaluateMetric_NoMatch — returns empty when condition not met
- TestEvaluateMetric_MultipleRules — multiple rules can fire for same metric
- TestDiagnose_AllRules — Diagnose runs all rules against snapshot
- TestDiagnose_EmptySnapshot — gracefully returns empty, no panics
- TestDiagnose_PartialSnapshot — rules with missing metrics return nil

**internal/remediation/rules_test.go** (~400 lines):
- Table-driven tests for ALL 25 rules
- For each rule test: positive case, negative case, boundary case, missing keys case
- Test that rule IDs are unique across pgRules() and osRules()
- Test that all rules have non-empty ID, Priority, Category
- Verify descriptions contain actionable text (not empty)

**internal/remediation/pgstore_test.go** (~200 lines):
- Requires a test database (use testcontainers or build-tag guarded)
- TestWrite_ListByInstance round-trip
- TestListAll_WithFilters (priority, category, acknowledged)
- TestListByAlertEvent
- TestAcknowledge_SetsTimestamp
- TestCleanOld_RemovesAcknowledgedOnly
- TestNullStore_AllMethods — verify NullStore implements interface correctly

**internal/api/remediation_test.go** (~250 lines):
- HTTP handler tests using httptest.NewRecorder
- TestHandleListRecommendations — correct pagination
- TestHandleDiagnose — returns recommendations based on metric snapshot
- TestHandleListAllRecommendations — fleet-wide response
- TestHandleAcknowledgeRecommendation — 200 on success, 404 on missing
- TestHandleListRemediationRules — returns all compiled-in rules

### Integration Checks

- Verify no import cycles: `go build ./cmd/... ./internal/...` must pass
- Verify ALL existing tests still pass (full regression)
- Run golangci-lint on new and modified files
- Verify migration 013 applies cleanly on a fresh database
- Verify NullStore works correctly for live mode path

### Build Verification Sequence

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd ..
go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

### CRITICAL RULES FOR THIS AGENT:
- Write test stubs immediately, fill assertions as API agent's code lands
- Table-driven tests for rules — one test function with 25+ subtests
- Test both alert-triggered and Diagnose evaluation paths
- Test NullStore carefully — it must satisfy the interface without panics
- Every test must be independent (no shared state between subtests)
- Use `t.Parallel()` where safe
- Run full regression before declaring done

---

## COORDINATION NOTES

- API Agent creates all files first, QA Agent writes tests in parallel
- QA Agent should check that new files compile before writing tests
- Both agents must run `go build ./cmd/... ./internal/...` before committing
- Final merge only when ALL tests pass and golangci-lint is clean
- After completion: regenerate docs/CODEBASE_DIGEST.md per .claude/rules/codebase-digest.md
