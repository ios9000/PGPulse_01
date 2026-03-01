# M4_03 Design — Alert API, Orchestrator Wiring & Integration

**Iteration:** M4_03
**Date:** 2026-03-01
**Input:** M4_03_requirements.md, M4_01/M4_02 codebase

---

## 1. Changes Overview

This iteration modifies existing files more than it creates new ones. The core work is
plumbing — connecting M4_01 (evaluator) and M4_02 (dispatcher) to the orchestrator and API.

```
Modified files:
├── internal/api/server.go           ← Add alert-related fields + routes
├── internal/orchestrator/group.go   ← Post-collect evaluator hook
├── internal/orchestrator/orchestrator.go ← Accept evaluator + dispatcher
├── internal/orchestrator/runner.go  ← Pass evaluator/dispatcher to groups
├── cmd/pgpulse-server/main.go      ← Full wiring + updated shutdown
├── internal/alert/evaluator.go      ← Add StartCleanup(), StopCleanup()

New files:
├── internal/api/alerts.go           ← 6 alert API handlers
├── internal/api/alerts_test.go      ← Handler tests
```

---

## 2. Orchestrator Wiring

### 2.1 Orchestrator Struct Changes (`internal/orchestrator/orchestrator.go`)

```go
// Add to Orchestrator struct:
type Orchestrator struct {
    cfg        config.Config
    store      collector.MetricStore
    logger     *slog.Logger
    evaluator  AlertEvaluator     // NEW — nil when alerting disabled
    dispatcher AlertDispatcher    // NEW — nil when alerting disabled
    runners    []*instanceRunner
    ctx        context.Context
    cancel     context.CancelFunc
}

// AlertEvaluator is the interface the orchestrator uses.
// Defined here to avoid importing the alert package directly.
type AlertEvaluator interface {
    Evaluate(ctx context.Context, instanceID string, points []collector.MetricPoint) ([]alert.AlertEvent, error)
}

// AlertDispatcher is the interface for dispatching alert events.
type AlertDispatcher interface {
    Dispatch(event alert.AlertEvent) bool
}
```

**Design Decision — Interface in orchestrator package:**

To keep the dependency direction clean (orchestrator should not import alert's concrete types
beyond what's necessary), define thin interfaces in the orchestrator package. The alert.Evaluator
and alert.Dispatcher already satisfy these interfaces — no adapter needed.

However, AlertEvent must be imported from the alert package since it's a data type passed through.
This is acceptable: orchestrator imports `alert.AlertEvent` (a struct, not a behavior).

Alternative: define the event type in the collector package (shared data layer). But AlertEvent
is alert-domain-specific, so importing from alert is the cleaner choice.

```go
// Updated constructor:
func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger,
    evaluator AlertEvaluator, dispatcher AlertDispatcher) *Orchestrator {
    // evaluator and dispatcher may be nil (alerting disabled)
}
```

### 2.2 Instance Runner Changes (`internal/orchestrator/runner.go`)

Pass evaluator and dispatcher through to interval groups:

```go
type instanceRunner struct {
    // ... existing fields ...
    evaluator  AlertEvaluator
    dispatcher AlertDispatcher
}
```

In `buildCollectors()` / group creation, pass evaluator/dispatcher to each intervalGroup.

### 2.3 Interval Group Hook (`internal/orchestrator/group.go`)

The key change — after collecting and writing metrics, evaluate alerts:

```go
func (g *intervalGroup) run(ctx context.Context) {
    ticker := time.NewTicker(g.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            g.collect(ctx)
        }
    }
}

func (g *intervalGroup) collect(ctx context.Context) {
    // ... existing: query InstanceContext, run collectors, write to store ...

    // NEW: post-collect alert evaluation
    if g.evaluator != nil && len(allPoints) > 0 {
        g.evaluateAlerts(ctx, allPoints)
    }
}

func (g *intervalGroup) evaluateAlerts(ctx context.Context, points []collector.MetricPoint) {
    evalCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    events, err := g.evaluator.Evaluate(evalCtx, g.instanceID, points)
    if err != nil {
        g.logger.Error("alert evaluation failed",
            "instance", g.instanceID,
            "error", err,
        )
        return
    }

    for _, event := range events {
        if g.dispatcher != nil {
            if !g.dispatcher.Dispatch(event) {
                g.logger.Warn("alert event dropped (dispatcher buffer full)",
                    "rule", event.RuleID,
                    "instance", event.InstanceID,
                )
            }
        }
    }

    if len(events) > 0 {
        g.logger.Info("alert evaluation produced events",
            "instance", g.instanceID,
            "event_count", len(events),
        )
    }
}
```

**Key design points:**

1. Evaluation runs AFTER store.Write() — metrics are persisted first
2. 5-second timeout for evaluation (matches config default)
3. Evaluation errors don't abort the collect cycle
4. Dispatcher.Dispatch() is non-blocking (designed in M4_02)
5. When evaluator is nil, no evaluation happens (zero cost)
6. All points from all collectors in the group are passed together — the evaluator
   indexes by metric name and matches against rules

---

## 3. API Server Changes

### 3.1 APIServer Struct (`internal/api/server.go`)

```go
type APIServer struct {
    // ... existing fields ...
    store       collector.MetricStore
    instances   []config.InstanceConfig
    serverCfg   config.ServerConfig
    authCfg     config.AuthConfig
    jwtService  *auth.JWTService
    userStore   auth.UserStore
    rateLimiter *auth.RateLimiter
    logger      *slog.Logger
    startTime   time.Time
    pool        Pinger

    // NEW — alert fields (all nil when alerting disabled)
    alertRuleStore    alert.AlertRuleStore
    alertHistoryStore alert.AlertHistoryStore
    evaluator         *alert.Evaluator
    dispatcher        *alert.Dispatcher
    notifierRegistry  *alert.NotifierRegistry
    alertingCfg       config.AlertingConfig
}
```

Updated constructor:

```go
func New(cfg config.Config, store collector.MetricStore, pool Pinger,
    jwtSvc *auth.JWTService, userStore auth.UserStore, logger *slog.Logger,
    alertRuleStore alert.AlertRuleStore, alertHistoryStore alert.AlertHistoryStore,
    evaluator *alert.Evaluator, dispatcher *alert.Dispatcher,
    registry *alert.NotifierRegistry,
) *APIServer
```

**Design Note:** The constructor is getting long. Accepted for now — a builder pattern
or options struct can be introduced in M7 refactoring if it grows further.

### 3.2 Route Registration (`internal/api/server.go` — Routes())

```go
func (s *APIServer) Routes() http.Handler {
    r := chi.NewRouter()

    // ... existing middleware and routes ...

    // Alert routes (only registered when alerting is enabled)
    if s.alertRuleStore != nil {
        r.Route("/api/v1/alerts", func(r chi.Router) {
            // Apply auth middleware if enabled
            if s.jwtService != nil {
                r.Use(auth.RequireAuth(s.jwtService, writeErrorRaw))
            }

            // Reader endpoints (viewer+)
            r.Get("/", s.handleGetActiveAlerts)
            r.Get("/history", s.handleGetAlertHistory)
            r.Get("/rules", s.handleGetAlertRules)

            // Writer endpoints (admin only)
            r.Group(func(r chi.Router) {
                if s.jwtService != nil {
                    r.Use(auth.RequireRole(auth.RoleAdmin, writeErrorRaw))
                }
                r.Post("/rules", s.handleCreateAlertRule)
                r.Put("/rules/{id}", s.handleUpdateAlertRule)
                r.Delete("/rules/{id}", s.handleDeleteAlertRule)
                r.Post("/test", s.handleTestNotification)
            })
        })
    }

    return r
}
```

---

## 4. Alert API Handlers (`internal/api/alerts.go`)

### GET /api/v1/alerts

```go
func (s *APIServer) handleGetActiveAlerts(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    events, err := s.alertHistoryStore.ListUnresolved(ctx)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "internal_error", "failed to fetch active alerts")
        s.logger.Error("fetch active alerts failed", "error", err)
        return
    }

    writeJSON(w, http.StatusOK, events, map[string]interface{}{"count": len(events)})
}
```

### GET /api/v1/alerts/history

```go
func (s *APIServer) handleGetAlertHistory(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    q := r.URL.Query()

    query := alert.AlertHistoryQuery{
        InstanceID:     q.Get("instance_id"),
        RuleID:         q.Get("rule_id"),
        Severity:       alert.Severity(q.Get("severity")),
        UnresolvedOnly: q.Get("unresolved") == "true",
        Limit:          100, // default
    }

    // Parse time filters
    if start := q.Get("start"); start != "" {
        t, err := time.Parse(time.RFC3339, start)
        if err != nil {
            writeError(w, http.StatusBadRequest, "invalid_param", "invalid start time format, use RFC3339")
            return
        }
        query.Start = t
    }
    if end := q.Get("end"); end != "" {
        t, err := time.Parse(time.RFC3339, end)
        if err != nil {
            writeError(w, http.StatusBadRequest, "invalid_param", "invalid end time format, use RFC3339")
            return
        }
        query.End = t
    }
    if limit := q.Get("limit"); limit != "" {
        l, err := strconv.Atoi(limit)
        if err != nil || l < 1 {
            writeError(w, http.StatusBadRequest, "invalid_param", "limit must be a positive integer")
            return
        }
        if l > 1000 {
            l = 1000
        }
        query.Limit = l
    }

    // Validate severity if provided
    if query.Severity != "" && query.Severity != alert.SeverityInfo &&
        query.Severity != alert.SeverityWarning && query.Severity != alert.SeverityCritical {
        writeError(w, http.StatusBadRequest, "invalid_param", "severity must be info, warning, or critical")
        return
    }

    events, err := s.alertHistoryStore.Query(ctx, query)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "internal_error", "failed to query alert history")
        s.logger.Error("query alert history failed", "error", err)
        return
    }

    writeJSON(w, http.StatusOK, events, map[string]interface{}{"count": len(events)})
}
```

### GET /api/v1/alerts/rules

```go
func (s *APIServer) handleGetAlertRules(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    rules, err := s.alertRuleStore.List(ctx)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "internal_error", "failed to fetch alert rules")
        s.logger.Error("fetch alert rules failed", "error", err)
        return
    }

    writeJSON(w, http.StatusOK, rules, map[string]interface{}{"count": len(rules)})
}
```

### POST /api/v1/alerts/rules

```go
// createRuleRequest is the JSON body for creating a custom alert rule.
type createRuleRequest struct {
    ID               string            `json:"id"`
    Name             string            `json:"name"`
    Description      string            `json:"description"`
    Metric           string            `json:"metric"`
    Operator         alert.Operator    `json:"operator"`
    Threshold        float64           `json:"threshold"`
    Severity         alert.Severity    `json:"severity"`
    Labels           map[string]string `json:"labels"`
    ConsecutiveCount int               `json:"consecutive_count"`
    CooldownMinutes  int               `json:"cooldown_minutes"`
    Channels         []string          `json:"channels"`
    Enabled          *bool             `json:"enabled"` // pointer to detect missing vs false
}

func (s *APIServer) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
    var req createRuleRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
        return
    }

    // Validate
    if err := validateRuleRequest(req, s.alertingCfg); err != nil {
        writeError(w, http.StatusBadRequest, "validation_error", err.Error())
        return
    }

    rule := alert.Rule{
        ID:               req.ID,
        Name:             req.Name,
        Description:      req.Description,
        Metric:           req.Metric,
        Operator:         req.Operator,
        Threshold:        req.Threshold,
        Severity:         req.Severity,
        Labels:           req.Labels,
        ConsecutiveCount: req.ConsecutiveCount,
        CooldownMinutes:  req.CooldownMinutes,
        Channels:         req.Channels,
        Source:           alert.SourceCustom, // forced
        Enabled:          true,
    }
    if req.Enabled != nil {
        rule.Enabled = *req.Enabled
    }
    if rule.ConsecutiveCount == 0 {
        rule.ConsecutiveCount = s.alertingCfg.DefaultConsecutiveCount
    }
    if rule.CooldownMinutes == 0 {
        rule.CooldownMinutes = s.alertingCfg.DefaultCooldownMinutes
    }

    if err := s.alertRuleStore.Create(r.Context(), &rule); err != nil {
        // Check for duplicate ID
        writeError(w, http.StatusConflict, "duplicate_id", "rule with this ID already exists")
        s.logger.Error("create alert rule failed", "id", req.ID, "error", err)
        return
    }

    // Refresh evaluator rules cache
    if s.evaluator != nil {
        if err := s.evaluator.LoadRules(r.Context()); err != nil {
            s.logger.Error("failed to reload rules after create", "error", err)
        }
    }

    writeJSON(w, http.StatusCreated, rule, nil)
}
```

### PUT /api/v1/alerts/rules/{id}

```go
func (s *APIServer) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
    ruleID := chi.URLParam(r, "id")
    ctx := r.Context()

    // Fetch existing
    existing, err := s.alertRuleStore.Get(ctx, ruleID)
    if err != nil {
        writeError(w, http.StatusNotFound, "not_found", "alert rule not found")
        return
    }

    var req createRuleRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
        return
    }

    // Apply updates to existing rule
    // For builtin rules: only allow threshold, consecutive_count, cooldown_minutes, enabled, channels
    // For custom rules: allow all fields
    if existing.Source == alert.SourceCustom {
        if req.Name != "" { existing.Name = req.Name }
        if req.Description != "" { existing.Description = req.Description }
        if req.Metric != "" { existing.Metric = req.Metric }
        if req.Operator != "" { existing.Operator = req.Operator }
        if req.Severity != "" { existing.Severity = req.Severity }
        if req.Labels != nil { existing.Labels = req.Labels }
    }
    // Both builtin and custom can modify these:
    existing.Threshold = req.Threshold
    if req.ConsecutiveCount > 0 { existing.ConsecutiveCount = req.ConsecutiveCount }
    if req.CooldownMinutes > 0 { existing.CooldownMinutes = req.CooldownMinutes }
    if req.Enabled != nil { existing.Enabled = *req.Enabled }
    if req.Channels != nil { existing.Channels = req.Channels }

    if err := s.alertRuleStore.Update(ctx, existing); err != nil {
        writeError(w, http.StatusInternalServerError, "internal_error", "failed to update rule")
        s.logger.Error("update alert rule failed", "id", ruleID, "error", err)
        return
    }

    // Refresh evaluator rules cache
    if s.evaluator != nil {
        if err := s.evaluator.LoadRules(ctx); err != nil {
            s.logger.Error("failed to reload rules after update", "error", err)
        }
    }

    writeJSON(w, http.StatusOK, existing, nil)
}
```

### DELETE /api/v1/alerts/rules/{id}

```go
func (s *APIServer) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
    ruleID := chi.URLParam(r, "id")
    ctx := r.Context()

    existing, err := s.alertRuleStore.Get(ctx, ruleID)
    if err != nil {
        writeError(w, http.StatusNotFound, "not_found", "alert rule not found")
        return
    }

    if existing.Source == alert.SourceBuiltin {
        writeError(w, http.StatusConflict, "builtin_rule",
            "cannot delete builtin rules; disable it instead by setting enabled=false")
        return
    }

    if err := s.alertRuleStore.Delete(ctx, ruleID); err != nil {
        writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete rule")
        s.logger.Error("delete alert rule failed", "id", ruleID, "error", err)
        return
    }

    // Refresh evaluator rules cache
    if s.evaluator != nil {
        if err := s.evaluator.LoadRules(ctx); err != nil {
            s.logger.Error("failed to reload rules after delete", "error", err)
        }
    }

    w.WriteHeader(http.StatusNoContent)
}
```

### POST /api/v1/alerts/test

```go
func (s *APIServer) handleTestNotification(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Channel string `json:"channel"`
        Message string `json:"message"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
        return
    }

    if req.Channel == "" {
        writeError(w, http.StatusBadRequest, "validation_error", "channel is required")
        return
    }

    if s.notifierRegistry == nil {
        writeError(w, http.StatusServiceUnavailable, "alerting_disabled", "alerting is not enabled")
        return
    }

    n := s.notifierRegistry.Get(req.Channel)
    if n == nil {
        writeError(w, http.StatusBadRequest, "unknown_channel",
            fmt.Sprintf("channel '%s' is not registered", req.Channel))
        return
    }

    testEvent := alert.AlertEvent{
        RuleID:     "test",
        RuleName:   "Test Notification",
        InstanceID: "test-instance",
        Severity:   alert.SeverityInfo,
        Metric:     "pgpulse.test",
        Value:      0,
        Threshold:  0,
        Operator:   alert.OpGreater,
        FiredAt:    time.Now(),
    }
    if req.Message != "" {
        testEvent.RuleName = req.Message
    }

    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()

    if err := n.Send(ctx, testEvent); err != nil {
        writeError(w, http.StatusBadGateway, "send_failed",
            fmt.Sprintf("failed to send test notification: %s", err.Error()))
        s.logger.Error("test notification failed", "channel", req.Channel, "error", err)
        return
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "sent":    true,
        "channel": req.Channel,
    }, nil)
}
```

### Validation Helper

```go
func validateRuleRequest(req createRuleRequest, cfg config.AlertingConfig) error {
    if req.ID == "" {
        return fmt.Errorf("id is required")
    }
    // Slug format: lowercase alphanumeric + hyphens + underscores
    if !slugPattern.MatchString(req.ID) {
        return fmt.Errorf("id must be lowercase alphanumeric with hyphens or underscores")
    }
    if req.Name == "" {
        return fmt.Errorf("name is required")
    }
    if req.Metric == "" {
        return fmt.Errorf("metric is required")
    }
    if !validOperator(req.Operator) {
        return fmt.Errorf("operator must be one of: >, >=, <, <=, ==, !=")
    }
    if req.Severity != "" && !validSeverity(req.Severity) {
        return fmt.Errorf("severity must be info, warning, or critical")
    }
    if req.Severity == "" {
        return fmt.Errorf("severity is required")
    }
    return nil
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func validOperator(op alert.Operator) bool {
    switch op {
    case alert.OpGreater, alert.OpGreaterEqual, alert.OpLess,
        alert.OpLessEqual, alert.OpEqual, alert.OpNotEqual:
        return true
    }
    return false
}

func validSeverity(s alert.Severity) bool {
    switch s {
    case alert.SeverityInfo, alert.SeverityWarning, alert.SeverityCritical:
        return true
    }
    return false
}
```

---

## 5. History Cleanup (`internal/alert/evaluator.go` addition)

```go
// StartCleanup launches a periodic goroutine that deletes old resolved alerts.
func (e *Evaluator) StartCleanup(ctx context.Context, retentionDays int) {
    if retentionDays <= 0 {
        retentionDays = 30
    }
    retention := time.Duration(retentionDays) * 24 * time.Hour

    go func() {
        ticker := time.NewTicker(1 * time.Hour)
        defer ticker.Stop()

        // Run once immediately on startup
        e.runCleanup(ctx, retention)

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                e.runCleanup(ctx, retention)
            }
        }
    }()

    e.logger.Info("alert history cleanup started", "retention_days", retentionDays, "interval", "1h")
}

func (e *Evaluator) runCleanup(ctx context.Context, retention time.Duration) {
    cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    deleted, err := e.historyStore.Cleanup(cleanupCtx, retention)
    if err != nil {
        e.logger.Error("alert history cleanup failed", "error", err)
        return
    }
    if deleted > 0 {
        e.logger.Info("alert history cleanup completed", "deleted", deleted)
    }
}
```

---

## 6. main.go Integration

### Updated Startup Sequence

```go
func main() {
    // ... existing: config, logger, storage, pool, migrate, auth setup ...

    // --- Alert pipeline setup ---
    var (
        evaluator         *alert.Evaluator
        dispatcher        *alert.Dispatcher
        notifierRegistry  *alert.NotifierRegistry
        alertRuleStore    alert.AlertRuleStore
        alertHistoryStore alert.AlertHistoryStore
    )

    if cfg.Alerting.Enabled {
        if cfg.Storage.DSN == "" {
            logger.Error("alerting.enabled requires storage.dsn to be configured")
            os.Exit(1)
        }

        alertRuleStore = alert.NewPGAlertRuleStore(pool)
        alertHistoryStore = alert.NewPGAlertHistoryStore(pool)

        // Seed builtin rules
        if err := alert.SeedBuiltinRules(ctx, alertRuleStore, logger); err != nil {
            logger.Error("failed to seed alert rules", "error", err)
            os.Exit(1)
        }

        // Create evaluator
        evaluator = alert.NewEvaluator(alertRuleStore, alertHistoryStore, logger)
        if err := evaluator.LoadRules(ctx); err != nil {
            logger.Error("failed to load alert rules", "error", err)
            os.Exit(1)
        }
        if err := evaluator.RestoreState(ctx); err != nil {
            logger.Error("failed to restore alert state", "error", err)
            os.Exit(1)
        }

        // Start history cleanup
        evaluator.StartCleanup(ctx, cfg.Alerting.HistoryRetentionDays)

        // Create notifier registry + email notifier
        notifierRegistry = alert.NewNotifierRegistry()
        if cfg.Alerting.Email != nil {
            emailNotifier := notifier.NewEmailNotifier(
                *cfg.Alerting.Email, cfg.Alerting.DashboardURL, logger,
            )
            notifierRegistry.Register(emailNotifier)
        }

        // Create and start dispatcher
        dispatcher = alert.NewDispatcher(
            notifierRegistry,
            cfg.Alerting.DefaultChannels,
            cfg.Alerting.DefaultCooldownMinutes,
            logger,
        )
        dispatcher.Start()

        logger.Info("alerting pipeline started",
            "rules", len(cfg.Alerting.DefaultChannels),
            "channels", cfg.Alerting.DefaultChannels,
        )
    }

    // --- API server ---
    apiServer := api.New(cfg, store, pool, jwtSvc, userStore, logger,
        alertRuleStore, alertHistoryStore, evaluator, dispatcher, notifierRegistry)

    // --- Orchestrator (with evaluator + dispatcher) ---
    orch := orchestrator.New(cfg, store, logger, evaluator, dispatcher)

    // ... existing: HTTP server, orch.Start(), signal handling ...

    // --- Updated shutdown sequence ---
    // 1. HTTP server
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer shutdownCancel()
    httpServer.Shutdown(shutdownCtx)

    // 2. Orchestrator (stops collect cycles, no more evaluator calls)
    orch.Stop()

    // 3. Dispatcher (drains buffered events)
    if dispatcher != nil {
        dispatcher.Stop()
    }

    // 4. Store (close DB connections)
    store.Close()
}
```

---

## 7. Dependency Graph (After M4_03)

```
cmd/pgpulse-server/main.go
  ├── imports: config, storage, auth, alert, alert/notifier, api, orchestrator
  │
  └── wires:
      config.Load()
      → storage.NewPool(), storage.Migrate(), storage.NewPGStore()
      → auth.NewPGUserStore(), auth.NewJWTService()
      → alert.NewPGAlertRuleStore(), alert.NewPGAlertHistoryStore()
      → alert.SeedBuiltinRules()
      → alert.NewEvaluator().LoadRules().RestoreState().StartCleanup()
      → alert.NewNotifierRegistry() + notifier.NewEmailNotifier()
      → alert.NewDispatcher().Start()
      → api.New(...all dependencies...)
      → orchestrator.New(...evaluator, dispatcher...)

internal/orchestrator/
  ├── imports: collector (MetricPoint), alert (AlertEvent) — data types only
  └── uses: AlertEvaluator, AlertDispatcher interfaces (defined locally)

internal/api/
  ├── imports: alert (types, stores, Evaluator, Dispatcher, NotifierRegistry)
  ├── imports: auth, config, collector
  └── new handlers in alerts.go

internal/alert/
  ├── (unchanged from M4_01 + M4_02)
  └── evaluator.go: +StartCleanup(), +runCleanup()
```

---

## 8. Test Plan

### API Handler Tests (`internal/api/alerts_test.go`)

Follows existing patterns from helpers_test.go — use mockAlertRuleStore, mockAlertHistoryStore,
mockEvaluator, mockDispatcher, mockNotifierRegistry.

```go
// Test mocks (add to helpers_test.go or alerts_test.go)

type mockAlertRuleStore struct {
    rules []alert.Rule
    err   error
}
// implements: List, ListEnabled, Get, Create, Update, Delete, UpsertBuiltin

type mockAlertHistoryStore struct {
    events []alert.AlertEvent
    err    error
}
// implements: Record, Resolve, ListUnresolved, Query, Cleanup

type mockEvaluator struct {
    loadRulesCalled int
}
func (m *mockEvaluator) LoadRules(ctx context.Context) error {
    m.loadRulesCalled++
    return nil
}

type mockDispatcher struct {
    dispatched []alert.AlertEvent
}

type mockNotifier struct {
    name     string
    sendErr  error
    called   int
}
```

| Test | Handler | Validates |
|------|---------|-----------|
| TestGetActiveAlerts | GET /alerts | Returns unresolved events, correct envelope |
| TestGetActiveAlerts_Empty | GET /alerts | Empty array [], not null |
| TestGetAlertHistory | GET /alerts/history | Filters applied, limit respected |
| TestGetAlertHistory_InvalidTime | GET /alerts/history?start=bad | 400 error |
| TestGetAlertHistory_MaxLimit | GET /alerts/history?limit=5000 | Capped to 1000 |
| TestGetAlertRules | GET /alerts/rules | Returns all rules with count |
| TestCreateAlertRule | POST /alerts/rules | 201, source="custom", defaults applied |
| TestCreateAlertRule_InvalidOperator | POST /alerts/rules | 400 validation error |
| TestCreateAlertRule_MissingRequired | POST /alerts/rules | 400 for missing id/name/metric/severity |
| TestCreateAlertRule_DuplicateID | POST /alerts/rules | 409 conflict |
| TestCreateAlertRule_RefreshesRules | POST /alerts/rules | evaluator.LoadRules called |
| TestCreateAlertRule_ViewerForbidden | POST /alerts/rules | 403 when viewer role |
| TestUpdateAlertRule | PUT /alerts/rules/{id} | 200, threshold updated |
| TestUpdateAlertRule_BuiltinLimited | PUT /alerts/rules/{id} | Builtin: only threshold/enabled/etc changeable |
| TestUpdateAlertRule_NotFound | PUT /alerts/rules/{id} | 404 |
| TestDeleteAlertRule_Custom | DELETE /alerts/rules/{id} | 204 |
| TestDeleteAlertRule_Builtin | DELETE /alerts/rules/{id} | 409 conflict |
| TestDeleteAlertRule_NotFound | DELETE /alerts/rules/{id} | 404 |
| TestTestNotification | POST /alerts/test | 200, notifier.Send called |
| TestTestNotification_UnknownChannel | POST /alerts/test | 400 |

### Orchestrator Tests (update existing)

| Test | Validates |
|------|-----------|
| TestGroupCollect_WithEvaluator | evaluator.Evaluate called after store.Write |
| TestGroupCollect_EvaluatorNil | No panic, collect works normally |
| TestGroupCollect_EvaluatorError | Error logged, collect cycle continues |
| TestGroupCollect_DispatchesEvents | Events from evaluator passed to dispatcher |

### Cleanup Tests

| Test | Validates |
|------|-----------|
| TestEvaluator_RunCleanup | historyStore.Cleanup called with correct retention |

---

## 9. Files to Create/Modify Summary

### New Files

| File | Lines (est.) | Description |
|------|-------------|-------------|
| `internal/api/alerts.go` | ~350 | 7 handlers + validation + request types |
| `internal/api/alerts_test.go` | ~400 | ~20 handler tests with mocks |

### Modified Files

| File | Change | Lines changed (est.) |
|------|--------|---------------------|
| `internal/api/server.go` | Add alert fields, update constructor, add alert routes | ~40 |
| `internal/api/helpers_test.go` | Add mock alert stores, evaluator, dispatcher | ~80 |
| `internal/orchestrator/orchestrator.go` | Add evaluator/dispatcher fields, update New() | ~15 |
| `internal/orchestrator/runner.go` | Pass evaluator/dispatcher to groups | ~10 |
| `internal/orchestrator/group.go` | Add evaluateAlerts() post-collect hook | ~40 |
| `internal/orchestrator/group_test.go` | Add evaluator integration tests | ~60 |
| `internal/alert/evaluator.go` | Add StartCleanup(), runCleanup() | ~40 |
| `internal/alert/evaluator_test.go` | Add cleanup test | ~20 |
| `cmd/pgpulse-server/main.go` | Full alert pipeline wiring + updated shutdown | ~60 |

### NOT Modified

| File | Why |
|------|-----|
| `internal/alert/alert.go` | Data model stable |
| `internal/alert/dispatcher.go` | Dispatcher complete from M4_02 |
| `internal/alert/template.go` | Templates complete from M4_02 |
| `internal/alert/notifier/email.go` | Email notifier complete from M4_02 |
| `internal/alert/pgstore.go` | Stores complete from M4_01 |
| `internal/alert/rules.go` | Rules complete from M4_01 |
