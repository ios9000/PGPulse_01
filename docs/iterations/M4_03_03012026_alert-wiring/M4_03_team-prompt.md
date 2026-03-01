# M4_03 Team Prompt — Alert API, Orchestrator Wiring & Integration

**Paste this into Claude Code after placing M4_03_requirements.md and M4_03_design.md
in `docs/iterations/M4_03_03012026_alert-wiring/`.**

---

Build the final M4 iteration: wire the alert evaluator and dispatcher into the orchestrator
and API, integrate everything in main.go.
Read CLAUDE.md for project context.
Read docs/iterations/M4_03_03012026_alert-wiring/M4_03_design.md for the full design.

This iteration connects M4_01 (evaluator, rules, stores) and M4_02 (email notifier, dispatcher)
to the rest of the system: orchestrator post-collect hook, alert REST API endpoints, main.go
startup/shutdown, and history cleanup. After this, the alert pipeline works end-to-end.

Create a team of 2 specialists:

---

SPECIALIST 1 — WIRING & API (production code):

**Important context:** Read existing code before modifying. Key files to understand first:
- `internal/api/server.go` — current APIServer struct and Routes()
- `internal/api/helpers_test.go` — existing test helper patterns (mockStore, mockPinger, newTestServer)
- `internal/orchestrator/orchestrator.go` — current Orchestrator struct and New()
- `internal/orchestrator/runner.go` — instanceRunner and buildCollectors
- `internal/orchestrator/group.go` — intervalGroup collect()
- `cmd/pgpulse-server/main.go` — current startup/shutdown flow
- `internal/alert/evaluator.go` — Evaluator struct (from M4_01)
- `internal/alert/dispatcher.go` — Dispatcher struct (from M4_02)

Create and modify the following:

1. **`internal/api/alerts.go`** — NEW FILE. Alert API handlers:

   Request/response types:
   - createRuleRequest struct: ID, Name, Description, Metric, Operator, Threshold, Severity, Labels, ConsecutiveCount, CooldownMinutes, Channels, Enabled (*bool pointer to distinguish missing vs false)
   - validateRuleRequest(req, cfg) error — validate required fields, slug format ID, valid operator/severity
   - slugPattern regexp: `^[a-z0-9][a-z0-9_-]*$`
   - validOperator(op) bool, validSeverity(s) bool helpers

   Handlers (all follow existing response envelope pattern from response.go):
   - handleGetActiveAlerts: GET /api/v1/alerts → alertHistoryStore.ListUnresolved() → writeJSON with count
   - handleGetAlertHistory: GET /api/v1/alerts/history → parse query params (instance_id, rule_id, severity, start, end, unresolved, limit), validate time format (RFC3339), cap limit at 1000, default 100 → alertHistoryStore.Query() → writeJSON
   - handleGetAlertRules: GET /api/v1/alerts/rules → alertRuleStore.List() → writeJSON with count
   - handleCreateAlertRule: POST /api/v1/alerts/rules → decode JSON, validate, set source="custom", apply defaults from alertingCfg, alertRuleStore.Create() → evaluator.LoadRules() → writeJSON 201
   - handleUpdateAlertRule: PUT /api/v1/alerts/rules/{id} → alertRuleStore.Get(), decode JSON, merge fields (builtin: only threshold/consecutive/cooldown/enabled/channels; custom: all fields), alertRuleStore.Update() → evaluator.LoadRules() → writeJSON 200
   - handleDeleteAlertRule: DELETE /api/v1/alerts/rules/{id} → check source, reject builtin with 409, alertRuleStore.Delete() → evaluator.LoadRules() → 204
   - handleTestNotification: POST /api/v1/alerts/test → decode {channel, message}, look up notifier in registry, create synthetic AlertEvent (severity=info, metric="pgpulse.test"), call notifier.Send() directly → 200 or 502

2. **`internal/api/server.go`** — MODIFY. Add alert fields to APIServer:
   - alertRuleStore alert.AlertRuleStore
   - alertHistoryStore alert.AlertHistoryStore  
   - evaluator (needs to be an interface or concrete *alert.Evaluator — use concrete pointer since api already imports alert)
   - dispatcher *alert.Dispatcher (only used for reference, not called directly from API)
   - notifierRegistry *alert.NotifierRegistry
   - alertingCfg config.AlertingConfig

   Update New() constructor to accept these additional parameters. All may be nil when alerting disabled.

   Update Routes():
   - Add alert route group `/api/v1/alerts` ONLY when alertRuleStore != nil
   - GET endpoints: viewer+ auth (same middleware as instances)
   - POST/PUT/DELETE endpoints: admin only (same pattern as existing admin routes)
   - Follow the exact chi middleware ordering pattern from M3 (Use() before method handlers)

3. **`internal/orchestrator/group.go`** — MODIFY. Add post-collect evaluator hook:
   - Add evaluator and dispatcher fields to intervalGroup struct
   - After store.Write(points) in collect(), add:
     ```
     if g.evaluator != nil && len(allPoints) > 0 {
         g.evaluateAlerts(ctx, allPoints)
     }
     ```
   - New method evaluateAlerts(ctx, points):
     * context.WithTimeout(ctx, 5s)
     * evaluator.Evaluate(ctx, instanceID, points)
     * For each returned event: dispatcher.Dispatch(event)
     * Log errors but do NOT abort collect cycle
     * Log event count if > 0

   IMPORTANT: The evaluator and dispatcher here should be interfaces defined in the orchestrator
   package to keep dependency direction clean. Define:
   ```go
   // AlertEvaluator evaluates metrics against alert rules.
   type AlertEvaluator interface {
       Evaluate(ctx context.Context, instanceID string, points []collector.MetricPoint) ([]alert.AlertEvent, error)
   }
   
   // AlertDispatcher dispatches alert events for notification delivery.
   type AlertDispatcher interface {
       Dispatch(event alert.AlertEvent) bool
   }
   ```
   The concrete alert.Evaluator and alert.Dispatcher already satisfy these interfaces.
   Import alert.AlertEvent as a data type (this is acceptable — it's a struct, not a behavior).

4. **`internal/orchestrator/orchestrator.go`** — MODIFY:
   - Add evaluator AlertEvaluator and dispatcher AlertDispatcher fields
   - Update New() to accept evaluator and dispatcher (both may be nil)
   - Pass them through to instanceRunner

5. **`internal/orchestrator/runner.go`** — MODIFY:
   - Add evaluator and dispatcher fields to instanceRunner
   - Pass to intervalGroup during group creation in buildCollectors() or start()

6. **`internal/alert/evaluator.go`** — MODIFY. Add cleanup methods:
   - StartCleanup(ctx context.Context, retentionDays int):
     * Default retentionDays to 30 if <= 0
     * Launch goroutine with 1-hour ticker
     * Run cleanup once immediately on startup
     * Stop via ctx.Done()
   - runCleanup(ctx, retention time.Duration):
     * 30s timeout context
     * Call historyStore.Cleanup(ctx, retention)
     * Log deleted count (only if > 0)
     * Log errors

7. **`cmd/pgpulse-server/main.go`** — MODIFY. Full alert pipeline wiring:

   After auth setup, before API server creation:
   ```
   var (evaluator, dispatcher, notifierRegistry, alertRuleStore, alertHistoryStore)
   if cfg.Alerting.Enabled:
       alertRuleStore = alert.NewPGAlertRuleStore(pool)
       alertHistoryStore = alert.NewPGAlertHistoryStore(pool)
       alert.SeedBuiltinRules(ctx, alertRuleStore, logger)
       evaluator = alert.NewEvaluator(alertRuleStore, alertHistoryStore, logger)
       evaluator.LoadRules(ctx)
       evaluator.RestoreState(ctx)
       evaluator.StartCleanup(ctx, cfg.Alerting.HistoryRetentionDays)
       notifierRegistry = alert.NewNotifierRegistry()
       if cfg.Alerting.Email != nil:
           emailNotifier = notifier.NewEmailNotifier(...)
           notifierRegistry.Register(emailNotifier)
       dispatcher = alert.NewDispatcher(registry, channels, cooldown, logger)
       dispatcher.Start()
   ```

   Update api.New() call to pass alert components.
   Update orchestrator.New() call to pass evaluator and dispatcher.

   Updated shutdown order:
   ```
   1. httpServer.Shutdown(ctx)    — stop HTTP
   2. orch.Stop()                 — stop collectors (no more evaluator calls)
   3. if dispatcher != nil: dispatcher.Stop()  — drain buffered events
   4. store.Close()               — close DB
   ```

   Import `internal/alert/notifier` package for NewEmailNotifier.

IMPORTANT RULES:
- Do NOT change any existing API response formats or behavior
- Alert routes registered ONLY when alertRuleStore != nil (alerting enabled)
- All alert rule mutations (create/update/delete) call evaluator.LoadRules() after success
- evaluator/dispatcher may be nil in api server — nil-check before calling
- Orchestrator interfaces (AlertEvaluator, AlertDispatcher) defined in orchestrator package
- All new query params validated before use (no raw string into SQL)
- Follow existing chi route patterns from server.go
- Follow existing writeJSON/writeError patterns from response.go
- Import alert package in api: "github.com/ios9000/PGPulse_01/internal/alert"
- Import notifier sub-package in main.go: "github.com/ios9000/PGPulse_01/internal/alert/notifier"

---

SPECIALIST 2 — TESTS:

Create and update tests for all wiring code:

1. **`internal/api/alerts_test.go`** — NEW FILE. Alert API handler tests:

   Add mock types (or add to helpers_test.go if more appropriate):
   ```go
   type mockAlertRuleStore struct {
       rules    []alert.Rule
       getErr   error
       createErr error
       updateErr error
       deleteErr error
   }
   // Implement all AlertRuleStore methods
   
   type mockAlertHistoryStore struct {
       events     []alert.AlertEvent
       queryErr   error
       cleanupN   int64
   }
   // Implement all AlertHistoryStore methods
   
   type mockEvaluatorForAPI struct {
       loadRulesCalled int
   }
   func (m *mockEvaluatorForAPI) LoadRules(ctx context.Context) error {
       m.loadRulesCalled++
       return nil
   }
   ```

   Create a helper to build test APIServer with alert components:
   ```go
   func newAlertTestServer(t *testing.T, ruleStore, historyStore, evaluator, registry) *httptest.Server
   ```

   Tests (use t.Run subtests):
   - TestGetActiveAlerts: 2 unresolved events → 200 with count=2
   - TestGetActiveAlerts_Empty: no events → 200 with empty array and count=0
   - TestGetAlertHistory: with query params → verify filters passed to store
   - TestGetAlertHistory_InvalidStartTime: bad format → 400
   - TestGetAlertHistory_LimitCapped: limit=5000 → capped to 1000
   - TestGetAlertRules: 19 rules → 200 with count=19
   - TestCreateAlertRule_Valid: full valid body → 201, source="custom"
   - TestCreateAlertRule_Defaults: missing consecutive_count/cooldown → defaults applied
   - TestCreateAlertRule_InvalidOperator: operator=">>>" → 400
   - TestCreateAlertRule_MissingID: no id → 400
   - TestCreateAlertRule_MissingMetric: no metric → 400
   - TestCreateAlertRule_DuplicateID: store returns error → 409
   - TestCreateAlertRule_RefreshesRules: verify evaluator.LoadRules called after create
   - TestUpdateAlertRule_Success: update threshold → 200
   - TestUpdateAlertRule_NotFound: unknown id → 404
   - TestUpdateAlertRule_BuiltinLimited: builtin rule → only threshold/enabled changed, metric unchanged
   - TestDeleteAlertRule_Custom: source=custom → 204
   - TestDeleteAlertRule_Builtin: source=builtin → 409
   - TestDeleteAlertRule_NotFound: unknown id → 404
   - TestTestNotification_Success: mock notifier → 200, sent=true
   - TestTestNotification_UnknownChannel: channel="sms" → 400
   - TestTestNotification_MissingChannel: no channel → 400

   Auth tests (if auth enabled in test server):
   - TestAlertRules_ViewerCanRead: GET /alerts/rules with viewer token → 200
   - TestAlertRules_ViewerCannotCreate: POST /alerts/rules with viewer token → 403

2. **`internal/orchestrator/group_test.go`** — MODIFY. Add evaluator integration tests:

   Add mock types:
   ```go
   type mockAlertEvaluator struct {
       mu     sync.Mutex
       calls  []evaluateCall
       events []alert.AlertEvent
       err    error
   }
   type evaluateCall struct {
       instanceID string
       pointCount int
   }
   
   type mockAlertDispatcher struct {
       mu         sync.Mutex
       dispatched []alert.AlertEvent
   }
   ```

   Tests:
   - TestGroupCollect_WithEvaluator: mock evaluator returns 1 event → dispatcher.Dispatch called
   - TestGroupCollect_EvaluatorNil: evaluator=nil → no panic, collect works
   - TestGroupCollect_EvaluatorError: evaluator returns error → logged, collect still succeeds
   - TestGroupCollect_DispatcherNil: evaluator set but dispatcher nil → no panic
   - TestGroupCollect_NoPoints: empty collect → evaluator NOT called

3. **`internal/alert/evaluator_test.go`** — MODIFY. Add cleanup test:
   - TestEvaluator_RunCleanup: mock historyStore, call runCleanup directly, verify Cleanup called with correct duration

4. **`internal/config/config_test.go`** — verify existing alerting tests still pass (no changes needed)

TESTING RULES:
- Follow existing test patterns from helpers_test.go, instances_test.go, auth_test.go
- Use httptest.NewServer for API tests
- All mock types implement full interface (return stored values or errors)
- Use t.Run() for subtests
- Do NOT modify existing test files beyond adding new test functions and mocks
- Run `go test -race ./internal/api/... ./internal/orchestrator/... ./internal/alert/...`
- Run `go test -race ./...` for full regression
- Run `golangci-lint run` — 0 issues
- Fix any compilation errors or lint issues before declaring done

---

COORDINATION:

- Specialist 1 starts with orchestrator changes (group.go, orchestrator.go, runner.go) since they're smaller
- Then API handlers (alerts.go, server.go modifications)
- Then main.go wiring
- Then evaluator cleanup addition
- Specialist 2 starts writing mock types and test structure immediately, fills assertions as code lands
- After both are done:
  1. Run `go build ./...` — must compile cleanly
  2. Run `go vet ./...` — must pass
  3. Run `go test -race ./...` — ALL tests must pass (full regression)
  4. Run `golangci-lint run` — 0 issues
  5. Verify alert routes appear only when alerting is enabled (check Routes() logic)
- Fix any issues before declaring done
- Commit: `git add . && git commit -m "feat(alert): wire evaluator and dispatcher into orchestrator and API (M4_03)"`
