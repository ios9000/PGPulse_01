# PGPulse — M8_01 Design
**Milestone:** M8_01 — P1 Features: Session Kill + On-Demand Query Plans + Cross-Instance Settings Diff
**Date:** 2026-03-08

---

## 1. Migration: session_audit_log

```sql
-- migrations/006_session_audit_log.sql
CREATE TABLE IF NOT EXISTS session_audit_log (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     TEXT        NOT NULL,
    operator_user   TEXT        NOT NULL,   -- username from JWT
    target_pid      INT         NOT NULL,
    operation       TEXT        NOT NULL CHECK (operation IN ('cancel','terminate')),
    result          TEXT        NOT NULL CHECK (result IN ('ok','error')),
    error_message   TEXT,
    executed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON session_audit_log (instance_id, executed_at DESC);
CREATE INDEX ON session_audit_log (operator_user, executed_at DESC);

-- Retention: auto-delete records older than 90 days
-- (can be a pg_cron job or handled in Go via a cleanup ticker)
```

Migration number: 006 (verify sequence against existing migrations in repo before finalizing).

---

## 2. New API Files

### internal/api/sessions.go

Handles the two kill endpoints. Key implementation notes:

```go
// Route registration (in server.go):
// r.Post("/instances/{id}/sessions/{pid}/cancel",    s.handleCancelSession)
// r.Post("/instances/{id}/sessions/{pid}/terminate", s.handleTerminateSession)

func (s *Server) handleCancelSession(w http.ResponseWriter, r *http.Request) {
    // 1. Auth: require dba or super_admin role (403 otherwise)
    // 2. Parse instance ID + pid from URL
    // 3. Get pool for instance from orchestrator (read-only — no new pools created here)
    // 4. Execute: SELECT pg_cancel_backend($1)  — parameterized, $1 = pid
    // 5. Write to session_audit_log
    // 6. Return {"ok": true} or {"ok": false, "error": "..."}
}

func (s *Server) handleTerminateSession(w http.ResponseWriter, r *http.Request) {
    // Same pattern, uses pg_terminate_backend($1)
}
```

**SQL (parameterized):**
```sql
SELECT pg_cancel_backend($1::int)
SELECT pg_terminate_backend($1::int)
```

Both return a boolean. If false, it means the PID no longer exists (session already gone) —
treat as success with a note, not an error.

**Audit log insert:**
```sql
INSERT INTO session_audit_log
    (instance_id, operator_user, target_pid, operation, result, error_message, executed_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
```

Connection: use the existing pgxpool for PGPulse's own storage DB for the audit insert.
Use the monitored instance pool for the `pg_cancel_backend` / `pg_terminate_backend` call.

### internal/api/plans.go

Handles on-demand EXPLAIN.

```go
// Route registration:
// r.Post("/instances/{id}/explain", s.handleExplainQuery)

type ExplainRequest struct {
    Database string `json:"database" validate:"required"`
    Query    string `json:"query"    validate:"required,min=1,max=65535"`
    Analyze  bool   `json:"analyze"`
    Buffers  bool   `json:"buffers"`
}

type ExplainResponse struct {
    PlanJSON        []map[string]any `json:"plan_json"`
    ExecutionTimeMs *float64         `json:"execution_time_ms,omitempty"`
    PlanningTimeMs  *float64         `json:"planning_time_ms,omitempty"`
}

func (s *Server) handleExplainQuery(w http.ResponseWriter, r *http.Request) {
    // 1. Auth: require dba or super_admin (403 otherwise)
    // 2. Decode + validate ExplainRequest
    // 3. Build EXPLAIN options string:
    //    opts := []string{"FORMAT JSON"}
    //    if req.Analyze { opts = append(opts, "ANALYZE") }
    //    if req.Buffers && req.Analyze { opts = append(opts, "BUFFERS") }
    //    sql := fmt.Sprintf("EXPLAIN (%s) %s", strings.Join(opts, ", "), req.Query)
    //    NOTE: req.Query is NOT parameterized — it IS the SQL. This is intentional.
    //    The whole point is to EXPLAIN arbitrary SQL. Auth gate protects this.
    // 4. Acquire a connection from the instance pool
    //    Set statement_timeout = '30s' on that connection before running.
    //    Set application_name = 'pgpulse_explain'.
    //    Connect to req.Database (need DSN substitution to change dbname).
    // 5. Execute EXPLAIN, scan result rows into []string, join, parse JSON.
    // 6. Extract ExecutionTime/PlanningTime from top-level plan node if Analyze=true.
    // 7. Return ExplainResponse.
}
```

**Database connection for EXPLAIN:**
The instance's pgxpool connects to a fixed database (usually `postgres`). EXPLAIN must
run against `req.Database`. Two options:
- **Option A:** Derive a new DSN from the instance config by substituting the dbname,
  create a temporary `*pgx.Conn` (not pool) for this request, close after use.
- **Option B:** Reuse DBRunner's per-database pool map.

Use **Option A** for M8_01 — simplest, no coupling to DBRunner. Create a one-shot
`pgx.Connect()` with the instance DSN, substitute `dbname=req.Database`, set
`statement_timeout=30s` and `application_name=pgpulse_explain`, run EXPLAIN, close.

**DSN substitution helper** (new function in internal/config or internal/api):
```go
// SubstituteDatabase returns a copy of dsn with the dbname replaced.
func SubstituteDatabase(dsn, dbName string) (string, error)
// Handles both key=value and postgres:// URL formats.
```

### internal/api/settings_diff.go

Handles cross-instance comparison.

```go
// Route registration:
// r.Get("/settings/compare", s.handleSettingsCompare)

// Noise-filter list (excluded by default):
var settingsNoiseFilter = map[string]bool{
    "server_version": true, "server_version_num": true,
    "data_directory": true, "hba_file": true, "ident_file": true,
    "config_file": true, "log_directory": true,
    "unix_socket_directories": true,
    "lc_collate": true, "lc_ctype": true, "lc_messages": true,
    "lc_monetary": true, "lc_numeric": true, "lc_time": true,
}

type SettingEntry struct {
    Name     string `json:"name"`
    Category string `json:"category"`
    ValueA   string `json:"value_a,omitempty"`
    ValueB   string `json:"value_b,omitempty"`
    Unit     string `json:"unit,omitempty"`
}

type SettingsDiffResponse struct {
    InstanceA     InstanceRef    `json:"instance_a"`
    InstanceB     InstanceRef    `json:"instance_b"`
    Changed       []SettingEntry `json:"changed"`
    OnlyInA       []SettingEntry `json:"only_in_a"`
    OnlyInB       []SettingEntry `json:"only_in_b"`
    MatchingCount int            `json:"matching_count"`
}

func (s *Server) handleSettingsCompare(w http.ResponseWriter, r *http.Request) {
    // 1. Auth: any authenticated user (viewer OK)
    // 2. Parse ?instance_a=X&instance_b=Y query params
    // 3. Fetch pg_settings from both instances concurrently (errgroup, timeout 10s each)
    // 4. Build map[name]setting for each instance
    // 5. Iterate union of keys, skip noise filter (unless ?show_all=true)
    // 6. Categorize: changed / only_in_a / only_in_b / matching
    // 7. Return SettingsDiffResponse
}
```

**SQL for settings fetch:**
```sql
SELECT name, setting, unit, category
FROM pg_catalog.pg_settings
ORDER BY category, name
```

Run on both instances concurrently using `errgroup.WithContext`.

---

## 3. New Frontend Files

### web/src/components/SessionKillButtons.tsx

```typescript
// Props: pid: number, applicationName: string, instanceId: string, onSuccess: () => void
// Renders two icon buttons: Cancel (✕, gray) and Terminate (⚡, red on hover)
// Both open a confirmation Dialog before calling the API
// Uses existing api client (fetch + auth header)
// On success: calls onSuccess() to trigger activity list refresh
// On error: shows inline error toast
```

Integrate into the existing active sessions / pg_stat_activity table in ServerDetail.tsx
(or ActivitySection component if extracted).

### web/src/pages/QueryPlanViewer.tsx

```typescript
// Accessible as a tab or section on ServerDetail, route: /instances/:id/explain
// State: database (string), query (string), analyze (bool), buffers (bool), result, loading, error
//
// PlanNode component — recursive:
//   Props: node: PlanNodeJSON, depth: number
//   Renders: node type + relation + cost/rows/time
//   Highlights: actual_rows / plan_rows ratio > 10x → yellow; > 100x → red
//              shared_read_blocks > 1000 → yellow
//   Expandable children
//
// Raw JSON toggle: show full plan_json in <pre> block
```

### web/src/pages/SettingsDiff.tsx

```typescript
// Route: /settings/diff (add to main navigation under Administration)
// State: instanceA, instanceB (selected from instance list), result, loading
//
// Renders:
//   - Two instance selector dropdowns
//   - [Compare] button
//   - Results accordion grouped by category:
//       "Changed (N)" — expanded by default
//       "Only in [A name] (N)"
//       "Only in [B name] (N)"  
//       "Matching (N)" — collapsed by default
//   - Each row: setting name | value_a | value_b | unit
//   - Changed rows: value_a in red, value_b in green (or side-by-side comparison styling)
//   - CSV export button
```

### web/src/types/models.ts additions

```typescript
// Session kill
interface SessionKillResult { ok: boolean; error?: string; }

// Query plans
interface ExplainRequest {
  database: string; query: string; analyze: boolean; buffers: boolean;
}
interface ExplainResponse {
  plan_json: PlanNode[];
  execution_time_ms?: number;
  planning_time_ms?: number;
}
interface PlanNode {
  "Node Type": string;
  "Relation Name"?: string;
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
interface SettingEntry {
  name: string; category: string;
  value_a?: string; value_b?: string; unit?: string;
}
interface SettingsDiffResponse {
  instance_a: { id: string; name: string };
  instance_b: { id: string; name: string };
  changed: SettingEntry[];
  only_in_a: SettingEntry[];
  only_in_b: SettingEntry[];
  matching_count: number;
}
```

---

## 4. Modified Files

| File | Change |
|------|--------|
| `internal/api/server.go` | Register 4 new routes (cancel, terminate, explain, settings/compare) |
| `web/src/pages/ServerDetail.tsx` | Add SessionKillButtons to activity rows; add Query Plan tab/link |
| `web/src/App.tsx` (or router file) | Add `/settings/diff` route; add `/instances/:id/explain` route |
| `web/src/types/models.ts` | Append 8 new types listed above |
| `web/src/components/Navigation.tsx` | Add "Settings Diff" link under Administration |

---

## 5. Role Enforcement Summary

| Endpoint | viewer | app_admin | dba | super_admin |
|----------|--------|-----------|-----|-------------|
| POST /sessions/:pid/cancel | 403 | 403 | ✅ | ✅ |
| POST /sessions/:pid/terminate | 403 | 403 | ✅ | ✅ |
| POST /instances/:id/explain | 403 | 403 | ✅ | ✅ |
| GET /settings/compare | ✅ | ✅ | ✅ | ✅ |

---

## 6. Error Handling

| Scenario | HTTP | Body |
|----------|------|------|
| PID already gone (cancel/terminate returns false) | 200 | `{"ok":true,"note":"session no longer exists"}` |
| Permission denied by PG (`pg_cancel_backend` returns error) | 500 | `{"ok":false,"error":"..."}` |
| EXPLAIN timeout (30s exceeded) | 408 | standard error body |
| EXPLAIN syntax error (PG returns error) | 400 | `{"error":"<pg error message>"}` |
| Settings compare: instance not found | 404 | standard error body |
| Settings compare: instance unreachable | 502 | standard error body |

---

## 7. File List Summary

**New backend files:**
- `internal/api/sessions.go`
- `internal/api/plans.go`
- `internal/api/settings_diff.go`
- `migrations/006_session_audit_log.sql`

**New frontend files:**
- `web/src/components/SessionKillButtons.tsx`
- `web/src/pages/QueryPlanViewer.tsx`
- `web/src/pages/SettingsDiff.tsx`

**Modified files:**
- `internal/api/server.go`
- `web/src/pages/ServerDetail.tsx`
- `web/src/App.tsx` (or equivalent router)
- `web/src/types/models.ts`
- `web/src/components/Navigation.tsx` (if it exists)

**No new collector files** — all three features are API-layer only.
**No new orchestrator changes** — no new collection loops.

---

## 8. Build Verification (unchanged sequence)

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

**Never** `go test ./...` — hits web/node_modules.
