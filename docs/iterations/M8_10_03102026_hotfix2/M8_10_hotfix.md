# M8_10 Hotfix — Remaining Production Bugs

**Iteration:** M8_10 (hotfix)
**Date:** 2026-03-10
**Priority:** HIGH — multiple demo features broken

---

## Context

PGPulse deployed on Ubuntu 24 with PG 16.13. Core dashboard works (connections, 
statements, settings, alerts) but several features are broken. This hotfix fixes
all remaining issues in one pass.

## Bug 1: HIGH — Explain endpoint missing (404)

**Symptom:** POST to `/api/v1/instances/{id}/explain` returns 404.

**Root cause:** The explain handler was created in M8_01 (`internal/api/explain.go`
or similar), deleted in M8_02 during unused handler cleanup, and never recreated.
No handler file exists and no route is registered in `server.go`.

**Fix:** Recreate `internal/api/explain.go` with:
- `handleExplain(w, r)` handler
- Accepts JSON body: `{ "database": "demo_app", "query": "SELECT ...", "analyze": true, "buffers": true }`
- Uses `SubstituteDatabase()` helper (exists somewhere in the codebase — find it) to
  connect to the specified database on the instance
- Opens a one-shot `pgx.Connect` with:
  - 30s statement_timeout
  - application_name = 'pgpulse_explain'
- Builds EXPLAIN query: `EXPLAIN (FORMAT JSON` + optional `, ANALYZE` + optional `, BUFFERS` + `) ` + user query
- Returns the JSON plan
- Register route: `r.Post("/instances/{id}/explain", s.handleExplain)` in the DBA/instance_management permission group

**Important:** The query body is intentionally NOT parameterized (you cannot use `$1` for
EXPLAIN targets). Auth gate is the protection. Document this in a code comment.

## Bug 2: MEDIUM — Breadcrumb "Servers" link goes to 404

**Symptom:** Clicking "Servers" in the breadcrumb navigates to `/servers` which shows 404.

**Fix:** In the breadcrumb component, change the "Servers" link to point to `/fleet`
(Fleet Overview). Search for the breadcrumb in `web/src/` — likely in a layout component
or ServerDetail.

## Bug 3: MEDIUM — Replication `client_addr` inet scan error

**Log:** `cannot scan inet (OID 869) in binary format into **string`

**Fix:** In `internal/api/replication.go`, the `client_addr` column is `inet` type.
pgx binary protocol can't auto-cast inet to string. Fix by casting in SQL:
```sql
client_addr::text
```
Or use `pgx.QueryExecModeSimpleProtocol` for this query. The SQL cast is simpler.

## Bug 4: MEDIUM — Progress query `command_desc` column missing

**Log:** `column c.command_desc does not exist (SQLSTATE 42703)`

**Fix:** In `internal/api/handler_progress.go`, check what column name is used.
PG 16 uses `command` not `command_desc` for `pg_stat_progress_*` views. Verify:
```sql
SELECT * FROM pg_stat_progress_vacuum LIMIT 0;
```
Fix the column name in the query.

## Bug 5: MEDIUM — Lock tree `datname` NULL scan

**Log:** `cannot scan NULL into *string`

**Fix:** In `internal/api/handler_locks.go`, the lock tree query joins `pg_database`
but `datname` can be NULL for non-database-specific locks. Change the Go struct field
from `string` to `*string`, or use `COALESCE(d.datname, '')` in SQL.

## Bug 6: LOW — Replication `received_lsn` column (already partially fixed)

**Log from earlier:** `column "received_lsn" does not exist`

**Status:** M8_09 changed it to `flushed_lsn` in `replication_status.go`. Verify
this fix is working. If still erroring, check the exact query.

---

## Team Prompt

Read CLAUDE.md for full project context.

This is a **hotfix iteration** — fix bugs 1-6. Use a **2-specialist team** 
(Backend Agent handles bugs 1,3,4,5,6 + Frontend Agent handles bug 2).

Create a team of 2 specialists:

### BACKEND AGENT

**Bug 1 (HIGH): Recreate Explain handler**

1. Find the `SubstituteDatabase()` helper — search: `grep -rn "SubstituteDatabase" internal/`
2. Create `internal/api/explain.go`:

```go
package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5"
)

type explainRequest struct {
    Database string `json:"database"`
    Query    string `json:"query"`
    Analyze  bool   `json:"analyze"`
    Buffers  bool   `json:"buffers"`
}

func (s *APIServer) handleExplain(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "id")

    var req explainRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
        return
    }
    if req.Database == "" || req.Query == "" {
        writeError(w, http.StatusBadRequest, "bad_request", "database and query are required")
        return
    }

    // Get instance DSN and substitute database
    inst := s.findInstance(instanceID) // or however the server looks up instance config
    if inst == nil {
        writeError(w, http.StatusNotFound, "not_found", "instance not found")
        return
    }

    dsn := SubstituteDatabase(inst.DSN, req.Database) // find actual function name

    // One-shot connection with timeout
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

    conn, err := pgx.Connect(ctx, dsn)
    if err != nil {
        writeError(w, http.StatusBadGateway, "connection_error", fmt.Sprintf("cannot connect: %v", err))
        return
    }
    defer conn.Close(ctx)

    // Set statement timeout and application name
    conn.Exec(ctx, "SET statement_timeout = '30s'")
    conn.Exec(ctx, "SET application_name = 'pgpulse_explain'")

    // Build EXPLAIN command
    // NOTE: Query is intentionally not parameterized — EXPLAIN cannot use $1.
    // Auth gate (DBA/instance_management permission) is the protection layer.
    opts := []string{"FORMAT JSON"}
    if req.Analyze {
        opts = append(opts, "ANALYZE")
    }
    if req.Buffers {
        opts = append(opts, "BUFFERS")
    }
    explainSQL := fmt.Sprintf("EXPLAIN (%s) %s", strings.Join(opts, ", "), req.Query)

    var planJSON []byte
    err = conn.QueryRow(ctx, explainSQL).Scan(&planJSON)
    if err != nil {
        writeError(w, http.StatusUnprocessableEntity, "explain_error", err.Error())
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.Write(planJSON)
}
```

3. Adapt the code to match actual patterns in the codebase:
   - Find how other handlers look up instance config (check `session_actions.go` or `logical_replication.go`)
   - Find `SubstituteDatabase` or equivalent helper
   - Use the same error response patterns (`writeError` etc.)

4. Register the route in `server.go` in the instance_management permission group:
   ```go
   r.Post("/instances/{id}/explain", s.handleExplain)
   ```
   Register in BOTH auth-enabled and auth-disabled sections.

**Bug 3: Fix replication client_addr inet scan**

Find the replication query in `internal/api/replication.go`. Add `::text` cast to
`client_addr` in the SQL SELECT:
```sql
-- Before:
client_addr, ...
-- After:
client_addr::text, ...
```

**Bug 4: Fix progress command_desc column**

Find the progress query in `internal/api/handler_progress.go`. Change `command_desc`
to the correct column name. Check PG 16 docs — likely just `command` or check with:
```sql
SELECT column_name FROM information_schema.columns 
WHERE table_name = 'pg_stat_progress_vacuum';
```

**Bug 5: Fix lock tree NULL datname**

Find the lock tree query in `internal/api/handler_locks.go`. Add COALESCE:
```sql
-- Before:
d.datname
-- After:  
COALESCE(d.datname, '') AS datname
```

**Bug 6: Verify replication_status fix**

Check `internal/collector/replication_status.go` — confirm `flushed_lsn` is used
instead of `received_lsn`. If there are still errors in the logs, fix them.

### FRONTEND AGENT

**Bug 2: Fix breadcrumb "Servers" link**

Search for the breadcrumb component:
```bash
grep -rn "Servers" web/src/ --include="*.tsx" | grep -i "bread\|link\|nav\|to="
```

Change the "Servers" link from `/servers` to `/fleet`.

### VERIFICATION

After all fixes:
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

ALL checks must pass. Zero errors.
