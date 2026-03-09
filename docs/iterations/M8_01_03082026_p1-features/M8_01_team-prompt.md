# PGPulse — M8_01 Team Prompt
**Milestone:** M8_01 — P1 Features: Session Kill + On-Demand Query Plans + Cross-Instance Settings Diff

---

## Your checklist to start:

```bash
# 1. Copy docs to iteration folder
mkdir -p docs/iterations/M8_01_03082026_p1-features
cp M8_01_requirements.md docs/iterations/M8_01_03082026_p1-features/requirements.md
cp M8_01_design.md docs/iterations/M8_01_03082026_p1-features/design.md
cp M8_01_team-prompt.md docs/iterations/M8_01_03082026_p1-features/team-prompt.md

# 2. Update CLAUDE.md current iteration section
#    Set: M8_01 — P1 Features (Session Kill, Query Plans, Settings Diff)
#    See: docs/iterations/M8_01_03082026_p1-features/

# 3. Paste the prompt below into Claude Code
```

---

Paste this entire prompt into Claude Code to spawn the agent team.

---

## PROMPT (paste into Claude Code)

```
Implement M8_01 for PGPulse. Read CLAUDE.md for project context.
Read docs/iterations/M8_01_03082026_p1-features/requirements.md and design.md before starting.

This milestone adds three P1 features:
1. Session Kill (cancel/terminate pg backends)
2. On-demand Query Plans (EXPLAIN via API)
3. Cross-Instance Settings Diff (stateless pg_settings comparison)

Create a team of 3 specialists:

═══════════════════════════════════════════════════════
API AGENT — owns: internal/api/*, migrations/
═══════════════════════════════════════════════════════

Create migrations/006_session_audit_log.sql:
- Table: session_audit_log (id BIGSERIAL PK, instance_id TEXT NOT NULL,
  operator_user TEXT NOT NULL, target_pid INT NOT NULL,
  operation TEXT NOT NULL CHECK (operation IN ('cancel','terminate')),
  result TEXT NOT NULL CHECK (result IN ('ok','error')),
  error_message TEXT, executed_at TIMESTAMPTZ NOT NULL DEFAULT now())
- Indexes: (instance_id, executed_at DESC), (operator_user, executed_at DESC)
- Use CREATE TABLE IF NOT EXISTS

Create internal/api/sessions.go:
- handleCancelSession(w, r):
  - Require dba or super_admin role — 403 otherwise
  - Parse instance_id from URL param {id}, pid from URL param {pid}
  - Get pgxpool for the instance from orchestrator
  - Run: SELECT pg_cancel_backend($1::int)
  - If pg returns false (pid gone), return {"ok":true,"note":"session no longer exists"}
  - Insert to session_audit_log (use PGPulse storage pool, not instance pool)
  - Return {"ok": true} or {"ok": false, "error": "..."}
- handleTerminateSession(w, r):
  - Same pattern, uses pg_terminate_backend($1::int)
  - Confirm modal text difference handled in frontend only — backend is identical logic

Create internal/api/plans.go:
- ExplainRequest struct: Database string (required), Query string (required, max 65535),
  Analyze bool, Buffers bool
- ExplainResponse struct: PlanJSON []map[string]any, ExecutionTimeMs *float64, PlanningTimeMs *float64
- handleExplainQuery(w, r):
  - Require dba or super_admin role — 403 otherwise
  - Decode + validate ExplainRequest
  - Build EXPLAIN options: always "FORMAT JSON"; add "ANALYZE" if req.Analyze;
    add "BUFFERS" only if req.Analyze && req.Buffers
  - IMPORTANT: do NOT parameterize req.Query — EXPLAIN cannot use $1 placeholders.
    The query string is submitted as-is. Auth gate is the protection here.
  - Build DSN for req.Database: take instance's DSN, substitute the database name.
    Write a helper func SubstituteDatabase(dsn, dbName string) (string, error) in
    internal/api/plans.go (handles both "key=value" and "postgres://" DSN formats).
  - Open a *pgx.Conn (not pool) for this one request:
    SET statement_timeout = '30s'
    SET application_name = 'pgpulse_explain'
    Then run the EXPLAIN statement.
  - Scan all result rows (EXPLAIN returns multiple rows), join into single JSON string,
    parse into []map[string]any.
  - Extract "Execution Time" and "Planning Time" from plan_json[0] if present.
  - Close the connection after use (defer conn.Close).
  - On timeout: return 408. On PG error: return 400 with PG error message.
  - Return ExplainResponse as JSON.

Create internal/api/settings_diff.go:
- settingsNoiseFilter map[string]bool — exclude these by default:
  server_version, server_version_num, data_directory, hba_file, ident_file,
  config_file, log_directory, unix_socket_directories,
  lc_collate, lc_ctype, lc_messages, lc_monetary, lc_numeric, lc_time
- SettingEntry struct: Name, Category, ValueA, ValueB, Unit
- SettingsDiffResponse struct: InstanceA, InstanceB (name+id), Changed, OnlyInA,
  OnlyInB []SettingEntry, MatchingCount int
- handleSettingsCompare(w, r):
  - Any authenticated user (viewer OK) — just require valid JWT
  - Parse ?instance_a=X&instance_b=Y
  - Fetch pg_settings from both instances CONCURRENTLY using errgroup:
    SELECT name, setting, unit, category FROM pg_catalog.pg_settings ORDER BY category, name
    10s timeout per fetch.
  - Build map[name]→setting for each instance.
  - Compute union of all keys. Skip noise filter unless ?show_all=true in query params.
  - Categorize: changed / only_in_a / only_in_b / matching
  - Sort each category by category then name.
  - Return SettingsDiffResponse.

Modify internal/api/server.go:
- Register new routes inside the authenticated router group:
  r.Post("/instances/{id}/sessions/{pid}/cancel",    s.handleCancelSession)
  r.Post("/instances/{id}/sessions/{pid}/terminate", s.handleTerminateSession)
  r.Post("/instances/{id}/explain",                  s.handleExplainQuery)
  r.Get("/settings/compare",                         s.handleSettingsCompare)

All SQL MUST use parameterized queries ($1, $2) where parameters exist.
The EXPLAIN query body is the one exception — it cannot be parameterized by design.
Document this exception with a comment in the code.

═══════════════════════════════════════════════════════
FRONTEND AGENT — owns: web/src/*
═══════════════════════════════════════════════════════

Add new types to web/src/types/models.ts (append, do not replace existing):

```typescript
// Session kill
export interface SessionKillResult { ok: boolean; error?: string; note?: string; }

// Query plans
export interface ExplainRequest {
  database: string; query: string; analyze: boolean; buffers: boolean;
}
export interface ExplainResponse {
  plan_json: PlanNode[];
  execution_time_ms?: number;
  planning_time_ms?: number;
}
export interface PlanNode {
  "Node Type": string;
  "Relation Name"?: string;
  "Alias"?: string;
  "Startup Cost": number;
  "Total Cost": number;
  "Plan Rows": number;
  "Actual Rows"?: number;
  "Actual Total Time"?: number;
  "Shared Hit Blocks"?: number;
  "Shared Read Blocks"?: number;
  Plans?: PlanNode[];
  [key: string]: unknown;
}

// Settings diff
export interface SettingEntry {
  name: string; category: string;
  value_a?: string; value_b?: string; unit?: string;
}
export interface SettingsDiffResponse {
  instance_a: { id: string; name: string };
  instance_b: { id: string; name: string };
  changed: SettingEntry[];
  only_in_a: SettingEntry[];
  only_in_b: SettingEntry[];
  matching_count: number;
}
```

Create web/src/components/SessionKillButtons.tsx:
- Props: pid: number, applicationName: string, instanceId: string, onSuccess: () => void
- Two icon buttons: Cancel (✕ icon, neutral/gray style) and Terminate (⚡ or skull icon, red on hover)
- Both open a confirmation Dialog/Modal:
  - Cancel dialog: "Cancel query for PID {pid} ({applicationName})? The connection will remain open."
    Confirm button: "Cancel Query" (not destructive styling)
  - Terminate dialog: "Terminate session PID {pid} ({applicationName})? This will close the connection."
    Confirm button: "Terminate" (destructive red styling)
- On confirm: POST to /api/v1/instances/{instanceId}/sessions/{pid}/cancel (or /terminate)
  with Authorization header from auth context
- Show loading state on buttons during request
- On success: call onSuccess(), show success toast
- On error: show error message inline or toast

Create web/src/pages/QueryPlanViewer.tsx:
- Accessible at route /instances/:id/explain
- State: selectedDatabase (string), query (string), analyze (bool, default true),
  buffers (bool, default true), result (ExplainResponse|null), loading, error
- Layout:
  - Database selector: dropdown populated from instance's database list
    (fetch from /api/v1/instances/:id — use existing databases field if available,
    or add a separate fetch to /api/v1/instances/:id/databases if needed)
  - Multi-line SQL textarea (min 6 rows)
  - Checkboxes: "ANALYZE" (default checked), "BUFFERS" (default checked, disabled if ANALYZE unchecked)
  - [Run EXPLAIN] button — disabled if no database selected or query empty
  - Loading spinner during request
  - Error display
- Result section (shown after successful response):
  - If analyze=true: show "Planning: {planning_time_ms}ms | Execution: {execution_time_ms}ms"
  - PlanNode recursive component:
    - Shows: Node Type, Relation Name/Alias, Startup Cost → Total Cost, Rows (planned vs actual if analyze)
    - Row count discrepancy highlight: actual/planned > 10x → yellow warning, > 100x → red warning
    - Shared Read Blocks > 1000 → yellow (cache miss indicator)
    - Expandable children (Plans array) — expanded by default up to depth 3, collapsed deeper
  - "Show Raw JSON" toggle — reveals full plan_json in a <pre> block

Create web/src/pages/SettingsDiff.tsx:
- Route: /settings/diff
- State: instanceAId, instanceBId (strings), result (SettingsDiffResponse|null), loading, error
- Layout:
  - Page title: "Settings Diff"
  - Two instance selector dropdowns side-by-side (Instance A, Instance B)
    Populated from the instances list (use existing hook or fetch /api/v1/instances)
  - [Compare] button — disabled if either instance not selected or same instance selected for both
  - Loading spinner during fetch
  - Error display
- Results section (accordion/collapsible groups):
  - "Changed ({n})" — expanded by default
    Each row: setting name | Instance A value | Instance B value | unit
    Value A in amber/yellow, Value B in blue (or clear visual diff)
    Grouped by category (sub-headings within this section)
  - "Only in {instanceA.name} ({n})" — collapsed by default
  - "Only in {instanceB.name} ({n})" — collapsed by default
  - "Matching ({matching_count})" — collapsed, just shows count (no rows to reduce noise)
  - CSV Export button: generates and downloads a CSV of changed + only_in_a + only_in_b rows
  - If result has 0 changed + 0 only_in_a + 0 only_in_b: show "Instances have identical settings ✓"

Modify web/src/pages/ServerDetail.tsx:
- Find the section that renders the active sessions / pg_stat_activity table
  (look for "pg_stat_activity" or "sessions" or "activity" in the file)
- Add a "Query Plan" link/button near the page header or the statements section:
  Link: "Explain Query →" → navigates to /instances/:id/explain
- Add SessionKillButtons component to each row in the activity table
  Pass pid, application_name, instanceId, and a refresh callback
  Only render the buttons if the current user has dba or super_admin role
  (check auth context for role)

Modify routing (web/src/App.tsx or equivalent router file):
- Add route: /instances/:id/explain → QueryPlanViewer
- Add route: /settings/diff → SettingsDiff

Modify navigation (web/src/components/Navigation.tsx or equivalent):
- Add "Settings Diff" link in the Administration section (or wherever Settings/Admin links are)
- Add "Explain" link accessible from instance pages (or let it be discoverable via ServerDetail only)

═══════════════════════════════════════════════════════
QA AGENT — owns: *_test.go files
═══════════════════════════════════════════════════════

Create internal/api/sessions_test.go:
- TestCancelSession_Success: POST /instances/:id/sessions/:pid/cancel with dba token
  Mock pg_cancel_backend returning true → expect 200 {"ok":true}
- TestCancelSession_PIDGone: Mock pg_cancel_backend returning false → expect 200 with "note"
- TestCancelSession_Forbidden: POST with app_admin or viewer token → expect 403
- TestCancelSession_Unauthenticated: POST with no token → expect 401
- TestTerminateSession_Success: POST with dba token → expect 200 {"ok":true}
- TestTerminateSession_Forbidden: viewer token → 403

Create internal/api/plans_test.go:
- TestExplainQuery_Success: POST /instances/:id/explain with valid query + dba token
  Use testcontainers-go with real PG instance
  Run: EXPLAIN (FORMAT JSON) SELECT 1
  Verify response contains plan_json with at least one node
- TestExplainQuery_WithAnalyze: EXPLAIN (ANALYZE, FORMAT JSON) SELECT 1
  Verify execution_time_ms present in response
- TestExplainQuery_Forbidden: app_admin token → 403
- TestExplainQuery_Unauthenticated: no token → 401
- TestExplainQuery_InvalidSQL: submit "NOT VALID SQL" → expect 400
- TestSubstituteDatabase: unit test the DSN substitution helper
  Test with key=value DSN format: "host=localhost dbname=postgres ..." → "dbname=mydb"
  Test with postgres:// URL format
  Test with missing dbname key (should add it)

Create internal/api/settings_diff_test.go:
- TestSettingsDiff_Changed: spin up two testcontainer PG instances
  SET work_mem = '64MB' on instance B; compare → expect work_mem in "changed"
- TestSettingsDiff_NoiseFiltered: verify server_version NOT in changed/only_a/only_b
  (unless ?show_all=true)
- TestSettingsDiff_ShowAll: with ?show_all=true verify server_version IS included
- TestSettingsDiff_ViewerAllowed: viewer token → 200 (not 403)
- TestSettingsDiff_Unauthenticated: no token → 401
- TestSettingsDiff_SameInstance: ?instance_a=X&instance_b=X → matching_count = all,
  changed is empty

Run all existing tests to verify no regressions.
Run golangci-lint.
Verify no string concatenation in SQL queries.
Confirm the EXPLAIN query body non-parameterization has a comment explaining why.

═══════════════════════════════════════════════════════
COORDINATION NOTES
═══════════════════════════════════════════════════════

Dependencies:
- API Agent and Frontend Agent can work in parallel from the start.
  Frontend can stub API calls while API Agent implements them.
- QA Agent writes test stubs immediately, fills in assertions once
  API Agent commits initial implementations.
- API Agent must commit sessions.go before QA Agent can finalize session tests.

Build verification (run by developer after agents finish — do NOT include bash steps):
  cd web && npm run build && npm run lint && npm run typecheck
  cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...

NEVER use: go test ./...  (hits web/node_modules/)

List all files created and modified when done so developer can run verification.
```
