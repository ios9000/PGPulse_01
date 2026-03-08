# M5_06 — Stabilization + Instance Management: Team Prompt

**Paste this into Claude Code to begin implementation.**

---

Build stabilization fixes and instance management for PGPulse.
Read `docs/iterations/M5_06_03042026_stabilization-instance-mgmt/design.md` for the full technical design.
Read `.claude/CLAUDE.md` for project context.

This iteration has two parts:
1. **Backend stabilization**: connection pool, NoOp evaluator, instance name field
2. **Instance management**: CRUD API, CSV bulk import, YAML seeding, orchestrator hot-reload, Administration UI

Create a team of 3 specialists:

---

## API & BACKEND AGENT

You own `internal/`, `cmd/`, and `migrations/`. Your task covers both stabilization and instance management backend.

### Step 1: Connection Pool Refactor

**Modify `internal/orchestrator/runner.go`:**
- Replace `*pgx.Conn` with `*pgxpool.Pool`
- Import `github.com/jackc/pgx/v5/pgxpool`
- `newInstanceRunner()` creates a pool: `pgxpool.New(ctx, dsn)` with config:
  - MinConns = 1
  - MaxConns = int32(cfg.MaxConns) (default 5)
  - ConnConfig.RuntimeParams["application_name"] = "pgpulse_collector"
- `stop()` calls `pool.Close()`

**Modify `internal/orchestrator/group.go`:**
- `runCycle()` receives `*pgxpool.Pool` instead of `*pgx.Conn`
- At start of each cycle: `conn, err := pool.Acquire(ctx)` + `defer conn.Release()`
- Pass `conn.Conn()` to collector's `Collect()` method (collectors expect `*pgx.Conn`)
- Remove any nil evaluator guards — evaluator will always be non-nil after Step 2

**Modify `internal/config/config.go`:**
- Add `Name string` field to InstanceConfig with koanf tag `"name"`
- Add `MaxConns int` field with koanf tag `"max_conns"` and default value 5
- In validation, set default: `if inst.MaxConns == 0 { inst.MaxConns = 5 }`

### Step 2: NoOp Evaluator

**Create `internal/alert/noop.go`:**
```go
package alert

import "context"

// NoOpEvaluator satisfies AlertEvaluator but discards all calls.
// Used when alerting.enabled = false.
type NoOpEvaluator struct{}

func (n *NoOpEvaluator) Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error {
    return nil
}
```

**Modify `cmd/pgpulse-server/main.go`:**
- When `cfg.Alerting.Enabled == false`, create `&alert.NoOpEvaluator{}` instead of nil
- Pass the evaluator to orchestrator — it's always non-nil now

### Step 3: Instance Store

**Create `internal/storage/instances.go`:**

Define the InstanceRecord struct and InstanceStore interface:
```go
type InstanceRecord struct {
    ID        string
    Name      string
    DSN       string
    Host      string
    Port      int
    Enabled   bool
    Source    string // "yaml" or "manual"
    MaxConns  int
    CreatedAt time.Time
    UpdatedAt time.Time
}

type InstanceStore interface {
    List(ctx context.Context) ([]InstanceRecord, error)
    Get(ctx context.Context, id string) (InstanceRecord, error)
    Create(ctx context.Context, r InstanceRecord) error
    Update(ctx context.Context, r InstanceRecord) error
    Delete(ctx context.Context, id string) error
    Seed(ctx context.Context, r InstanceRecord) error
}
```

Implement `PGInstanceStore` using the existing storage pool (pgxpool.Pool from storage.dsn):
- `Seed()`: INSERT ON CONFLICT (id) DO NOTHING — sets source='yaml'
- `Create()`: INSERT with source='manual', auto-generate UUID if ID empty
- `Update()`: UPDATE ... SET name=$2, dsn=$3, enabled=$4, max_conns=$5, updated_at=now() WHERE id=$1
- `Delete()`: DELETE FROM instances WHERE id=$1
- `List()`: SELECT * FROM instances ORDER BY name
- `Get()`: SELECT * FROM instances WHERE id=$1

### Step 4: Migration

**Create the next migration file** (check existing migrations directory for the correct sequence number):

```sql
-- Add instance management columns
-- Use IF NOT EXISTS / ADD COLUMN IF NOT EXISTS for idempotency

ALTER TABLE instances ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE instances ADD COLUMN IF NOT EXISTS dsn TEXT NOT NULL DEFAULT '';
ALTER TABLE instances ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE instances ADD COLUMN IF NOT EXISTS max_conns INTEGER NOT NULL DEFAULT 5;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE instances ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
```

Check the existing `instances` table schema first — some columns may already exist. Only add what's missing. If `instances` table doesn't exist at all, create it with all columns.

### Step 5: Instance CRUD API

**Create `internal/api/instances_crud.go`:**

New handler struct `InstanceHandler` with `InstanceStore` dependency.

Routes (register in router.go):
```go
r.Route("/api/v1/instances", func(r chi.Router) {
    r.Use(authMiddleware)            // all require auth
    r.Get("/", h.List)               // existing, enhanced
    r.Get("/{id}", h.Get)            // existing, enhanced
    r.Group(func(r chi.Router) {
        r.Use(adminOnlyMiddleware)   // mutations require admin
        r.Post("/", h.Create)
        r.Put("/{id}", h.Update)
        r.Delete("/{id}", h.Delete)
        r.Post("/bulk", h.BulkImport)
        r.Post("/{id}/test", h.TestConnection)
    })
})
```

**Create handler:**
- `POST /` — validate name+DSN required, extract host:port from DSN, call store.Create()
- `PUT /{id}` — partial update (only non-nil fields), call store.Update()
- `DELETE /{id}` — call store.Delete()
- `POST /bulk` — parse CSV (Content-Type: text/csv) or JSON (application/json), process each row independently, return array of results
- `POST /{id}/test` — get instance DSN from store, attempt pgx.Connect with 5s timeout, query `SELECT version()`, return success/error/latency

**DSN masking:** Never return raw DSN in responses. Mask password: `postgres://user:***@host:port/db`

**CSV parsing:** Use `encoding/csv` stdlib. Header row required: `id,name,dsn,enabled`. Handle quoted fields (DSN contains special chars).

### Step 6: YAML Seeding in main.go

**Modify `cmd/pgpulse-server/main.go`:**

After running migrations, before starting orchestrator:
```go
for _, inst := range cfg.Instances {
    host, port := parseHostPort(inst.DSN)
    rec := storage.InstanceRecord{
        ID: inst.ID, Name: inst.Name, DSN: inst.DSN,
        Host: host, Port: port, Enabled: inst.Enabled,
        MaxConns: inst.MaxConns, Source: "yaml",
    }
    if err := instanceStore.Seed(ctx, rec); err != nil {
        slog.Error("failed to seed instance", "id", inst.ID, "err", err)
    }
}
```

Write a `parseHostPort(dsn string) (string, int)` helper that extracts host and port from a PostgreSQL DSN string.

### Step 7: Orchestrator Hot-Reload

**Modify `internal/orchestrator/orchestrator.go`:**

- Add `instanceStore storage.InstanceStore` field
- Add `reloadInterval time.Duration` field (default 60s)
- In `Start()`, launch `go o.reloadLoop(ctx)` goroutine
- `reloadLoop`: ticker every reloadInterval, calls `reload(ctx)`
- `reload(ctx)`:
  1. List enabled instances from DB
  2. Compare with currently running runners
  3. Stop runners for removed/disabled instances
  4. Start runners for new instances
  5. Restart runners where DSN changed
  6. Log all changes

- Protect `o.runners` map with `sync.Mutex` (reload + shutdown access it)

**Modify constructor:** Orchestrator should accept InstanceStore and read initial instances from DB (not from config). Remove direct config.InstanceConfig dependency.

### Step 8: Modify existing GET /api/v1/instances

If there's an existing instances handler that reads from config, change it to read from InstanceStore instead. Add `name` and `source` fields to the JSON response.

---

## FRONTEND AGENT

You own all files in `web/src/`. Your task is the Administration page and sidebar fix.

### Step 1: Types

**Modify `web/src/types/models.ts`** — add:
```typescript
export interface ManagedInstance {
  id: string;
  name: string;
  dsn_masked: string;
  host: string;
  port: number;
  enabled: boolean;
  source: 'yaml' | 'manual';
  max_conns: number;
  created_at: string;
  updated_at: string;
}

export interface CreateInstanceRequest {
  id?: string;
  name: string;
  dsn: string;
  enabled: boolean;
  max_conns?: number;
}

export interface UpdateInstanceRequest {
  name?: string;
  dsn?: string;
  enabled?: boolean;
  max_conns?: number;
}

export interface TestConnectionResult {
  success: boolean;
  version?: string;
  error?: string;
  latency_ms?: number;
}

export interface BulkImportResult {
  row: number;
  id?: string;
  success: boolean;
  error?: string;
}
```

### Step 2: Hooks

**Create `web/src/hooks/useInstanceManagement.ts`:**
- `useManagedInstances()` — GET /api/v1/instances, refetchInterval 30s
- `useCreateInstance()` — POST /api/v1/instances mutation
- `useUpdateInstance()` — PUT /api/v1/instances/:id mutation
- `useDeleteInstance()` — DELETE /api/v1/instances/:id mutation
- `useTestConnection(id)` — POST /api/v1/instances/:id/test mutation
- `useBulkImport()` — POST /api/v1/instances/bulk mutation
- All mutations invalidate the instances query on success

### Step 3: Administration Page

**Replace `web/src/pages/AdministrationPage.tsx`:**
- Two tabs: "Instances" and "Users"
- Default to Instances tab
- Users tab: placeholder text "User management coming soon"
- Instances tab renders InstancesTab component
- Permission-gated: requires admin role

### Step 4: InstancesTab Component

**Create `web/src/components/admin/InstancesTab.tsx`:**
- Header with "Instances" title + "Add Instance" button + "Bulk Import" button
- Table columns: Name, Host:Port, Source (badge: yaml=blue, manual=green), Enabled (toggle), Status, Actions (edit, delete)
- Enable/disable toggle calls useUpdateInstance inline
- Edit button opens InstanceForm modal
- Delete button opens DeleteInstanceModal
- Source badge: "yaml" instances show subtle indicator
- Auto-refresh via TanStack Query refetchInterval

### Step 5: InstanceForm Modal

**Create `web/src/components/admin/InstanceForm.tsx`:**
- Props: `open`, `onClose`, `instance?` (undefined=create, defined=edit)
- Fields:
  - Name (text, required)
  - DSN (text, required, monospace font — it's a connection string)
  - Max Connections (number, default 5)
  - Enabled (checkbox, default true)
- "Test Connection" button:
  - For edit mode: calls POST /api/v1/instances/:id/test
  - For create mode: show note "Save first, then test connection"
  - Shows result: green checkmark + PG version + latency, or red X + error
- Submit: create calls POST, edit calls PUT
- On success: close modal, show brief success state, invalidate query
- On error: show API error inline

### Step 6: BulkImportModal

**Create `web/src/components/admin/BulkImportModal.tsx`:**
- Props: `open`, `onClose`
- Large textarea for pasting CSV
- "Upload .csv" button (file input, reads file content into textarea)
- Help text: "CSV format: id,name,dsn,enabled (header row required, id optional)"
- "Preview" button: parse CSV client-side, show table of rows
- "Import" button: POST /api/v1/instances/bulk with Content-Type: text/csv
- Results display: per-row success/error after import completes
- Client-side CSV parsing: split by newlines, split by commas (handle quoted fields)

### Step 7: DeleteInstanceModal

**Create `web/src/components/admin/DeleteInstanceModal.tsx`:**
- Props: `open`, `onClose`, `instance`
- Confirmation text: "Delete instance {name}?"
- If source='yaml': warning text "This instance will be re-added on next restart if still in config."
- Delete button calls useDeleteInstance mutation
- On success: close, invalidate

### Step 8: Sidebar Fix

**Modify `web/src/components/layout/Sidebar.tsx`:**
- Where instance name is displayed, use: `instance.name || instance.id || \`${instance.host}:${instance.port}\``
- Ensure the sidebar reads from the updated API response that now includes `name`

### Styling Rules

- Dark-mode-first: bg-gray-900, bg-gray-800 for cards, text-gray-100 for primary text
- Tabs: bottom border active indicator (blue-500), inactive text-gray-400
- Source badges: yaml=blue-500/blue-900 chip, manual=green-500/green-900 chip
- Status indicators: green dot = connected, red dot = error, gray dot = disabled
- Tables: same styling as other tables in the app
- Modals: same pattern as RuleFormModal (bg-gray-900 border border-gray-700 rounded-lg shadow-xl)
- Form inputs: bg-gray-800 border-gray-700 text-gray-100 rounded focus:ring-blue-500
- DSN input: font-mono text for readability
- Toggle switch: same pattern as AlertRules enable/disable toggle
- No new npm dependencies
- No new CSS files — Tailwind utility classes only

---

## QA & REVIEW AGENT

Your job is to verify code quality after each step.

### Backend Checks

1. **go build ./...** — must pass
2. **go vet ./...** — must pass
3. **go test ./...** — all existing tests must still pass
4. **golangci-lint run** — must pass
5. **New test coverage:**
   - instances_test.go: CRUD operations, Seed idempotency, List ordering
   - instances_crud_test.go: HTTP handlers with httptest
   - CSV parsing: valid CSV, missing headers, quoted fields, empty id
   - noop_test.go: NoOpEvaluator implements interface, returns nil
   - Verify pool lifecycle in runner (at minimum: constructor creates pool, stop closes pool)

### Frontend Checks

1. **npx tsc --noEmit** from web/ — zero errors
2. **npx eslint src/** from web/ — zero errors
3. **npx vite build** from web/ — success

### Integration Verification

- Verify migration SQL is valid (no syntax errors)
- Verify DSN masking never leaks passwords in API responses
- Verify all mutation endpoints require admin role middleware
- Verify CSV parser handles: quoted fields, empty id, missing enabled column
- Check that existing instance GET endpoints still work (backward compatible)
- Verify pgxpool import is added to go.mod (may need go mod tidy)

### Final Verification

After all steps complete:
1. `go build ./...` — pass
2. `go vet ./...` — pass  
3. `go test ./...` — all pass (existing + new)
4. `golangci-lint run` — pass
5. `npx tsc --noEmit` — pass
6. `npx eslint src/` — pass
7. `npx vite build` — pass

List all files created/modified with line counts.

---

## Coordination Notes

- API Agent starts immediately — Steps 1-2 (pool + noop) are independent
- Frontend Agent can start types/hooks (Steps 1-2) in parallel
- Frontend Agent waits for API Agent to finish Step 5 (CRUD endpoints) before building the UI that calls them
- QA Agent verifies after each step, not just at the end
- Team Lead: merge only when all QA checks pass
- Agents can run build/test/lint directly (Claude Code v2.1.63, bash works)
- Run `go mod tidy` after adding pgxpool if not already in go.mod
