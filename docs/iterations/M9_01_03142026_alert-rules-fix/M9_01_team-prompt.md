# M9_01 — Alert & Advisor Polish (Metric Keys + UI Nav + Cosmetic Fixes)

Read `docs/iterations/M9_01_03142026_alert-rules-fix/design.md` for full technical design.
Read `docs/CODEBASE_DIGEST.md` Section 3 (Metric Key Catalog) as the canonical reference for all metric keys.

Create a team of 2 specialists:

## BACKEND AGENT

### Task A: Alert Rules Metric Key Audit

1. Read `internal/alert/rules.go` — all 23 alert rule definitions (19 standard + 3 forecast + 1 logical repl)
2. Read `docs/CODEBASE_DIGEST.md` Section 3 — canonical metric key catalog
3. For EVERY rule, compare its `Metric` field against the canonical key:

   **Expected canonical keys for standard rules:**
   - Wraparound: `pg.server.wraparound_pct`
   - Connections: `pg.connections.utilization_pct`
   - Cache hit: `pg.cache.hit_ratio`
   - Commit ratio: `pg.transactions.commit_ratio_pct`
   - Inactive repl slots: `pg.replication.slot.active`
   - Long transactions: `pg.long_transactions.oldest_seconds`
   - Blocking locks: `pg.locks.blocker_count`
   - Bloat: `pg.db.bloat.table_ratio`
   - PGSS fill: `pg.statements.fill_pct`
   - Track IO timing: `pg.settings.track_io_timing`
   - Replication lag: `pg.replication.lag.total_bytes`
   - Logical repl: `pg.db.logical_replication.pending_sync_tables`

4. Fix any mismatches IN PLACE — change the Metric string in the rule definition
5. For forecast rules, also verify the metric key used in `internal/alert/evaluator.go` forecast evaluation logic
6. If any rule references an OS metric, verify it handles dual-prefix (`os.*` + `pg.os.*`) — see `internal/remediation/rules_os.go` for the `getOS()`/`isOSMetric()` pattern
7. Update `internal/alert/rules_test.go` to assert correct metric keys
8. Check `internal/alert/seed.go` — verify seed creates rules with corrected keys

### Task C1 Backend: Port Display Parser Fix

1. Search codebase for where instance host:port display string is built
   - Check `internal/storage/instances.go` for DSN parsing helpers
   - Check `internal/api/instances.go` for display data construction
   - `grep -r "5432" internal/` to find hardcoded defaults
2. Fix the DSN port extraction to handle both formats:
   - URI: `postgres://host:port/db` — extract port from URL
   - Keyword/value: `host=x port=5433 dbname=z` — parse `port=` value
   - Default to 5432 only when no port specified
3. Add tests for both DSN formats

### Task C2 Backend: Diagnose Panel Metric Value Fix

1. Read `internal/remediation/rule.go` — check if `RuleResult` has MetricKey/MetricValue fields
2. Read `internal/remediation/engine.go` — look at `Diagnose()` method
3. **If RuleResult already has these fields:** Fix each rule's `Evaluate` function in `rules_pg.go` and `rules_os.go` to set MetricKey and MetricValue from the snapshot when returning a result
4. **If RuleResult does NOT have these fields:**
   a. Add `MetricKey string` and `MetricValue float64` to `RuleResult` in `rule.go`
   b. Update each rule's `Evaluate` function in `rules_pg.go` to set these fields
   c. Update each rule's `Evaluate` function in `rules_os.go` to set these fields
   d. Update `Recommendation` struct in `engine.go` if it needs MetricKey/MetricValue
   e. Update `PGStore` serialization in `pgstore.go` if Recommendation is persisted
   f. Update `NullStore` if needed
   g. Verify API response includes the new fields (JSON tags)
5. Update `engine_test.go` — add test that Diagnose mode returns non-zero MetricValue
6. Do NOT break alert-triggered mode (`EvaluateMetric`) — verify both paths work

**Critical: Run the full build after all changes:**
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

---

## FRONTEND AGENT

### Task B1: Alert Tab Bar

1. Create `web/src/components/alerts/AlertsTabBar.tsx`:
   - Three tabs: **Active** | **History** | **Rules**
   - Props: `activeTab: 'active' | 'history' | 'rules'`
   - "Active" links to `/alerts` (or `/alerts?view=active`)
   - "History" links to `/alerts?view=history` (or controls filter state)
   - "Rules" links to `/alerts/rules`
   - Use `react-router-dom` `Link` or `NavLink`
   - Active tab: bold text, colored bottom border (match Tailwind theme)
   - Inactive tabs: muted text, hover highlight
   - Match existing component patterns (check AlertFilters.tsx, AdvisorFilters.tsx for styling reference)
2. Modify `web/src/pages/AlertsDashboard.tsx`:
   - Import and render `AlertsTabBar` at the top of the page, below PageHeader
   - Determine activeTab from URL query params or component state
   - If "Active" and "History" are currently controlled by AlertFilters, integrate the tab bar with that filter state
3. Modify `web/src/pages/AlertRules.tsx`:
   - Import and render `AlertsTabBar` with `activeTab="rules"`
   - Tab bar should appear at the same position as on AlertsDashboard

### Task B2: Sidebar Expandable Alerts Group

1. Modify `web/src/components/layout/Sidebar.tsx`:
   - Change the "Alerts" item from a flat link to an expandable group
   - Group header: "Alerts" with Bell icon + ChevronDown/ChevronRight toggle
   - Sub-items (indented, smaller font):
     - "Dashboard" → `/alerts`
     - "Rules" → `/alerts/rules`
   - Auto-expand when current route matches `/alerts*`
   - Collapse/expand state: use layoutStore (Zustand) or local component state
   - Keep "Advisor" item (Lightbulb icon) positioned AFTER the Alerts group, BEFORE Admin
2. Import `ChevronDown` and `ChevronRight` from `lucide-react`
3. Ensure sidebar collapse (narrow mode) still works — collapsed sidebar should show just the Bell icon, expanded on hover/click

### Task C1 Frontend: Port Display (if needed)

1. Check if port parsing happens in the frontend:
   - Search `web/src/` for DSN parsing, port extraction, `:5432`
   - Check InstanceCard.tsx, HeaderCard.tsx, InstancesTab.tsx
2. If frontend parses DSN for display: fix the parser to handle keyword/value format
3. If backend now returns correct port: verify frontend displays it correctly

### Task C2 Frontend: Diagnose Panel Metric Value

1. Modify `web/src/components/server/DiagnosePanel.tsx`:
   - When `metric_value` is non-zero and `metric_key` is present, display: `metric_key = formatted_value`
   - When `metric_value` is 0 and `metric_key` is empty, hide the value line entirely
   - Use appropriate formatters from `web/src/lib/formatters.ts` based on metric key suffix:
     - `*_pct` → percentage with 1 decimal
     - `*_bytes` → byte formatter
     - `*_seconds` → duration formatter
     - default → 2 decimal places
2. If the TypeScript `Recommendation` type in `web/src/types/models.ts` doesn't have `metric_key`/`metric_value` fields, add them (optional, matching backend JSON)

**Critical: Run the full frontend build after all changes:**
```bash
cd web && npm run build && npm run typecheck && npm run lint
```

---

## Coordination

- Backend Agent and Frontend Agent work in PARALLEL
- Backend Agent: start with Task A (alert rules audit), then C1 backend, then C2 backend
- Frontend Agent: start with Task B1 (tab bar) + B2 (sidebar), then C1/C2 frontend
- Frontend Agent should wait for Backend Agent to confirm C2 backend changes (Recommendation struct) before updating TypeScript types
- Both agents commit independently
- Merge only when:
  - `go build ./cmd/... ./internal/...` passes
  - `go test ./cmd/... ./internal/... -count=1` passes
  - `golangci-lint run ./cmd/... ./internal/...` passes
  - `cd web && npm run build && npm run typecheck && npm run lint` passes
