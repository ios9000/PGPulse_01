# M4_01 Team Prompt — Alert Evaluator & Rules Engine

**Paste this into Claude Code after placing M4_01_requirements.md and M4_01_design.md
in `docs/iterations/M4_01_03012026_alert-evaluator/`.**

---

Build the alert evaluation engine for PGPulse.
Read CLAUDE.md for project context.
Read docs/iterations/M4_01_03012026_alert-evaluator/M4_01_design.md for the full design.

This iteration builds the core alert domain logic: data model, evaluator with state machine,
19 builtin rule definitions, DB storage for rules and alert history, startup seeding,
and config extension. No notifiers, no API endpoints, no orchestrator wiring — those are M4_02 and M4_03.

Create a team of 2 specialists:

---

SPECIALIST 1 — ALERT ENGINE (domain logic + stores):

Create the following files:

1. `internal/alert/alert.go` — Data model types:
   - Severity (info, warning, critical) with severityLevel() helper
   - Operator (>, >=, <, <=, ==, !=) with Compare(value, threshold) method
   - AlertState (ok, pending, firing)
   - RuleSource (builtin, custom)
   - Rule struct: id, name, description, metric, operator, threshold, severity, labels (map[string]string), consecutive_count, cooldown_minutes, channels ([]string), source, enabled
   - AlertEvent struct: rule_id, rule_name, instance_id, severity, value, threshold, operator, metric, labels, channels, fired_at, resolved_at (*time.Time), is_resolution (bool)
   - stateEntry struct (unexported): state, consecutive_count, fired_at, last_notified_at, severity

2. `internal/alert/evaluator.go` — Core evaluation engine:
   - Evaluator struct with: ruleStore AlertRuleStore, historyStore AlertHistoryStore, logger *slog.Logger, mu sync.Mutex, rules []Rule, state map[string]*stateEntry
   - NewEvaluator(ruleStore, historyStore, logger) constructor
   - LoadRules(ctx) — fetch enabled rules from store, cache in memory
   - RestoreState(ctx) — query unresolved alerts from history store, seed state map with FIRING entries
   - Evaluate(ctx, instanceID string, points []collector.MetricPoint) ([]AlertEvent, error):
     * Index points by metric name
     * For each enabled rule, find matching points (by metric name + label filter)
     * For each match, run state machine: OK→PENDING→FIRING on breach, FIRING→OK on resolution
     * Hysteresis: increment counter on breach, reset on OK, fire when counter >= rule.ConsecutiveCount
     * Return AlertEvents for: new fires (PENDING→FIRING) and resolutions (FIRING→OK)
     * Persist events to history store (Record for fires, Resolve for resolutions)
   - Helper functions: stateKey(), stateKeyWithLabels(), indexPoints(), findMatchingPoints(), labelsMatch()

3. `internal/alert/rules.go` — Builtin rule definitions:
   - BuiltinRules() []Rule returning 19 rules:
     * 14 from PGAM thresholds (see design §6 for full list)
     * 2 new: replication_lag_warning (>1MB), replication_lag_critical (>100MB)
     * 3 deferred (enabled: false): wal_spike_warning, query_regression_warning, disk_forecast_critical
   - Each rule has: descriptive Name, metric matching existing collector output, appropriate ConsecutiveCount (1 for criticals like wraparound_critical, 3 for most warnings), CooldownMinutes
   - Critical severity rules: consecutive_count=1 (fire immediately), cooldown=5 min
   - Warning severity rules: consecutive_count=3 (hysteresis), cooldown=15 min

4. `internal/alert/store.go` — Interfaces:
   - AlertRuleStore: List, ListEnabled, Get, Create, Update, Delete, UpsertBuiltin
   - AlertHistoryStore: Record, Resolve, ListUnresolved, Query (with AlertHistoryQuery struct), Cleanup
   - AlertHistoryQuery struct: InstanceID, RuleID, Severity, Start, End, UnresolvedOnly, Limit

5. `internal/alert/pgstore.go` — PostgreSQL implementations:
   - PGAlertRuleStore using pgxpool.Pool
   - PGAlertHistoryStore using pgxpool.Pool
   - UpsertBuiltin: INSERT ON CONFLICT DO UPDATE preserving user-modified threshold/consecutive_count/cooldown_minutes/enabled/channels
   - All queries use parameterized args ($1, $2, etc.)
   - JSONB marshal/unmarshal for labels and channels fields
   - scanRule and scanEvent helper functions

6. `internal/alert/seed.go` — Startup seeding:
   - SeedBuiltinRules(ctx, store AlertRuleStore, logger) error
   - Iterates BuiltinRules(), calls store.UpsertBuiltin for each
   - Logs count of rules seeded

7. `internal/storage/migrations/004_alerts.sql` — Migration:
   - alert_rules table with CHECK constraints on operator, severity, source
   - alert_history table with FK to alert_rules, partial index on unresolved
   - 3 indexes: unresolved lookup, fired_at desc, instance+fired_at

8. Config changes:
   - Add AlertingConfig struct to internal/config/config.go:
     * Enabled bool, DefaultConsecutiveCount int, DefaultCooldownMinutes int, EvaluationTimeoutSec int, HistoryRetentionDays int
   - Add `Alerting AlertingConfig` field to Config struct
   - Add alertingDefaults() in load.go (defaults: consecutive=3, cooldown=15, timeout=5, retention=30)
   - Add validateAlerting(): if enabled=true, require storage.dsn
   - Update configs/pgpulse.example.yml with alerting section

IMPORTANT RULES:
- Import collector.MetricPoint from "github.com/ios9000/PGPulse_01/internal/collector"
- Import pgxpool from "github.com/jackc/pgx/v5/pgxpool"
- All SQL uses parameterized queries — never fmt.Sprintf for SQL
- JSONB fields: use json.Marshal/json.Unmarshal for labels (map[string]string) and channels ([]string)
- The alert package must NOT import from api, auth, orchestrator, or storage (except pgxpool)
- Follow existing patterns: see internal/auth/store.go for PG store pattern, internal/config/config.go for config pattern

---

SPECIALIST 2 — TESTS:

Create comprehensive tests for all alert engine code:

1. `internal/alert/alert_test.go`:
   - TestOperatorCompare: table-driven, all 6 operators with edge cases (zero, negative, equal boundary)
   - TestSeverityLevel: info=1, warning=2, critical=3, unknown=0

2. `internal/alert/evaluator_test.go`:
   - Create mock implementations at top of file:
     * mockRuleStore (in-memory map, implements AlertRuleStore)
     * mockHistoryStore (in-memory slice, implements AlertHistoryStore)
   - TestEvaluate_OKToFiring: feed 3 consecutive breaching metrics, verify event returned on 3rd
   - TestEvaluate_PendingResetOnOK: breach twice then OK, verify counter resets, no event
   - TestEvaluate_FiringToOK: fire an alert, then feed OK metric, verify resolution event
   - TestEvaluate_Hysteresis_ExactThreshold: verify fires at exactly consecutive_count, not before
   - TestEvaluate_ConsecutiveCountOne: rule with consecutive_count=1 fires immediately
   - TestEvaluate_NoMatchingMetrics: rule for metric X, feed only metric Y, verify no events
   - TestEvaluate_LabelFiltering: rule with labels filter, verify only matching labels trigger
   - TestEvaluate_MultipleRules: warning at 80% + critical at 99% on same metric
   - TestEvaluate_ResolutionAlwaysEmits: verify resolution event regardless of cooldown
   - TestLabelsMatch: required subset matching, empty required matches all, missing key fails
   - TestRestoreState: mock unresolved alerts, verify evaluator enters FIRING state

3. `internal/alert/rules_test.go`:
   - TestBuiltinRulesValid: all rules have non-empty ID, Name, Metric, valid Operator, valid Severity, positive ConsecutiveCount
   - TestBuiltinRulesNoDuplicateIDs: verify no two rules share an ID
   - TestBuiltinRulesCount: exactly 19 rules
   - TestDeferredRulesDisabled: rules with "wal_spike", "query_regression", "disk_forecast" are enabled=false

4. `internal/alert/pgstore_test.go`:
   - Use //go:build integration tag (requires Docker/testcontainers)
   - TestPGAlertRuleStore_CreateAndGet
   - TestPGAlertRuleStore_List
   - TestPGAlertRuleStore_Update
   - TestPGAlertRuleStore_Delete
   - TestPGAlertRuleStore_UpsertBuiltin_NewRule
   - TestPGAlertRuleStore_UpsertBuiltin_PreservesUserThreshold
   - TestPGAlertRuleStore_ListEnabled
   - TestPGAlertHistoryStore_RecordAndResolve
   - TestPGAlertHistoryStore_ListUnresolved
   - TestPGAlertHistoryStore_Cleanup

5. `internal/alert/seed_test.go`:
   - TestSeedBuiltinRules: uses mockRuleStore, verify all 19 rules upserted
   - TestSeedBuiltinRules_Idempotent: seed twice, verify no errors

6. `internal/config/config_test.go` — ADD tests (don't replace existing):
   - TestAlertingConfig_Defaults: empty alerting config gets defaults
   - TestAlertingConfig_EnabledRequiresStorage: alerting.enabled + no storage.dsn = error

TESTING RULES:
- All evaluator tests use mock stores (no DB dependency)
- Use table-driven tests where applicable
- Use t.Run() for subtests
- pgstore tests tagged //go:build integration
- Run `go test -race ./internal/alert/...` to verify
- Run `go test -race ./internal/config/...` to verify config tests
- Run `golangci-lint run` on all new code — fix any issues

---

COORDINATION:

- Specialist 1 creates all production code first
- Specialist 2 creates tests (can start mock implementations immediately, fill assertions once production code exists)
- After both are done:
  1. Run `go build ./...` — must compile cleanly
  2. Run `go vet ./...` — must pass
  3. Run `go test -race ./internal/alert/... ./internal/config/...` — all tests must pass
  4. Run `golangci-lint run` — 0 issues
  5. Run `go test -race ./...` — full regression, all prior tests must still pass
- Fix any issues before declaring done
- Commit: `git add . && git commit -m "feat(alert): add evaluator engine, rules, and stores (M4_01)"`
