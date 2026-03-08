# M5_06 — Stabilization + Instance Management: Technical Design

**Iteration:** M5_06
**Date:** 2026-03-04
**Depends on:** M5_01–M5_05 (complete), M4 (alert engine, complete)

---

## 1. Connection Pool Refactor

### Current Problem

`internal/orchestrator/runner.go` creates a single `pgx.Conn` per instance. Three interval groups (high/medium/low) run as concurrent goroutines, each calling `conn.QueryRow()` or `conn.Query()` on the shared connection. pgx connections are not safe for concurrent use — this produces `conn busy` errors.

### Solution: pgxpool.Pool

Replace `*pgx.Conn` with `*pgxpool.Pool` in `instanceRunner`. Each interval group acquires a connection from the pool for its cycle.

```go
// internal/orchestrator/runner.go — changed

import "github.com/jackc/pgx/v5/pgxpool"

type instanceRunner struct {
    instanceID string
    pool       *pgxpool.Pool        // was: conn *pgx.Conn
    groups     []*intervalGroup
    cancel     context.CancelFunc
}

func newInstanceRunner(ctx context.Context, cfg config.InstanceConfig) (*instanceRunner, error) {
    poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
    if err != nil {
        return nil, fmt.Errorf("parse DSN: %w", err)
    }
    poolCfg.MinConns = 1
    poolCfg.MaxConns = int32(cfg.MaxConns) // default 5, from YAML
    poolCfg.ConnConfig.RuntimeParams["application_name"] = "pgpulse_collector"
    
    pool, err := pgxpool.New(ctx, poolCfg.ConnString())
    if err != nil {
        return nil, fmt.Errorf("create pool: %w", err)
    }
    // ... rest unchanged
}
```

```go
// internal/orchestrator/group.go — changed

func (g *intervalGroup) runCycle(ctx context.Context, pool *pgxpool.Pool) {
    conn, err := pool.Acquire(ctx)
    if err != nil {
        slog.Error("failed to acquire connection", "instance", g.instanceID, "err", err)
        return
    }
    defer conn.Release()
    
    // Detect role (was using shared conn, now uses acquired conn)
    var isRecovery bool
    err = conn.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)
    if err != nil {
        slog.Error("failed to detect recovery state", "err", err)
        return
    }
    
    ic := collector.InstanceContext{IsRecovery: isRecovery}
    
    for _, c := range g.collectors {
        points, err := c.Collect(ctx, conn.Conn(), ic)
        // ... existing logic
    }
}
```

### Config Addition

```yaml
instances:
  - id: "prod-primary"
    name: "Production Primary"
    dsn: "postgres://pgpulse_monitor:pass@host:5432/postgres"
    enabled: true
    max_conns: 5    # NEW — pool size, default 5
```

```go
// internal/config/config.go — add field
type InstanceConfig struct {
    ID       string `koanf:"id"`
    Name     string `koanf:"name"`      // NEW
    DSN      string `koanf:"dsn"`
    Enabled  bool   `koanf:"enabled"`
    MaxConns int    `koanf:"max_conns"` // NEW, default 5
}
```

---

## 2. NoOp Evaluator

### Current Problem

`main.go` passes `nil` evaluator when `alerting.enabled = false`. Interval group calls `evaluator.Evaluate()` → panic.

### Solution

```go
// internal/alert/noop.go — NEW
package alert

import "context"

type NoOpEvaluator struct{}

func (n *NoOpEvaluator) Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error {
    return nil
}
```

```go
// cmd/pgpulse-server/main.go — changed
var eval collector.AlertEvaluator
if cfg.Alerting.Enabled {
    eval = realEvaluator // existing code
} else {
    eval = &alert.NoOpEvaluator{}
}
```

Remove the nil guard from `group.go`.

---

## 3. Database Schema Changes

### Migration: 00X_instance_management.sql

```sql
-- Add columns to existing instances table
ALTER TABLE instances ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE instances ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual' 
    CHECK (source IN ('yaml', 'manual'));
ALTER TABLE instances ADD COLUMN IF NOT EXISTS max_conns INTEGER NOT NULL DEFAULT 5;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS dsn TEXT NOT NULL DEFAULT '';
ALTER TABLE instances ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE instances ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- If instances table doesn't exist yet (fresh install), create it
CREATE TABLE IF NOT EXISTS instances (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    dsn         TEXT NOT NULL,
    host        TEXT NOT NULL DEFAULT '',
    port        INTEGER NOT NULL DEFAULT 5432,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    source      TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('yaml', 'manual')),
    max_conns   INTEGER NOT NULL DEFAULT 5,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Note: Check what the existing migration already created for `instances`. The new migration should be additive (ALTER ADD IF NOT EXISTS) to handle both fresh installs and upgrades.

---

## 4. YAML Seeding

### Startup Flow

```
main.go startup:
  1. Load YAML config
  2. Run DB migrations
  3. Seed instances from YAML → DB
  4. Create orchestrator (reads instances from DB)
  5. Start HTTP server
```

```go
// internal/storage/instances.go — NEW

type InstanceRecord struct {
    ID        string    `db:"id"`
    Name      string    `db:"name"`
    DSN       string    `db:"dsn"`
    Host      string    `db:"host"`
    Port      int       `db:"port"`
    Enabled   bool      `db:"enabled"`
    Source    string    `db:"source"`
    MaxConns  int       `db:"max_conns"`
    CreatedAt time.Time `db:"created_at"`
    UpdatedAt time.Time `db:"updated_at"`
}

type InstanceStore interface {
    List(ctx context.Context) ([]InstanceRecord, error)
    Get(ctx context.Context, id string) (InstanceRecord, error)
    Create(ctx context.Context, r InstanceRecord) error
    Update(ctx context.Context, r InstanceRecord) error
    Delete(ctx context.Context, id string) error
    Seed(ctx context.Context, r InstanceRecord) error // INSERT ON CONFLICT DO NOTHING
}
```

```go
// Seed implementation
func (s *PGInstanceStore) Seed(ctx context.Context, r InstanceRecord) error {
    _, err := s.pool.Exec(ctx, `
        INSERT INTO instances (id, name, dsn, host, port, enabled, source, max_conns)
        VALUES ($1, $2, $3, $4, $5, $6, 'yaml', $7)
        ON CONFLICT (id) DO NOTHING
    `, r.ID, r.Name, r.DSN, r.Host, r.Port, r.Enabled, r.MaxConns)
    return err
}
```

### Seeding in main.go

```go
// After migrations, before orchestrator start
for _, inst := range cfg.Instances {
    host, port := parseHostPort(inst.DSN)
    rec := storage.InstanceRecord{
        ID:       inst.ID,
        Name:     inst.Name,
        DSN:      inst.DSN,
        Host:     host,
        Port:     port,
        Enabled:  inst.Enabled,
        MaxConns: inst.MaxConns,
    }
    if err := instanceStore.Seed(ctx, rec); err != nil {
        slog.Error("failed to seed instance", "id", inst.ID, "err", err)
    } else {
        slog.Info("seeded instance from config", "id", inst.ID)
    }
}
```

---

## 5. Instance CRUD API

### Endpoints

```
GET    /api/v1/instances          — list all (existing, add name+source)
GET    /api/v1/instances/:id      — get one (existing, add name+source)
POST   /api/v1/instances          — create instance
PUT    /api/v1/instances/:id      — update instance
DELETE /api/v1/instances/:id      — delete instance
POST   /api/v1/instances/bulk     — bulk import (CSV or JSON)
POST   /api/v1/instances/:id/test — test connection to instance
```

### Request/Response Types

```go
// POST /api/v1/instances
type CreateInstanceRequest struct {
    ID       string `json:"id,omitempty"`      // optional, auto-UUID if empty
    Name     string `json:"name"`              // required
    DSN      string `json:"dsn"`               // required
    Enabled  bool   `json:"enabled"`           // default true
    MaxConns int    `json:"max_conns,omitempty"` // default 5
}

// PUT /api/v1/instances/:id
type UpdateInstanceRequest struct {
    Name     *string `json:"name,omitempty"`
    DSN      *string `json:"dsn,omitempty"`
    Enabled  *bool   `json:"enabled,omitempty"`
    MaxConns *int    `json:"max_conns,omitempty"`
}

// Response for all instance endpoints
type InstanceResponse struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    DSN       string `json:"dsn_masked"` // mask password in response
    Host      string `json:"host"`
    Port      int    `json:"port"`
    Enabled   bool   `json:"enabled"`
    Source    string `json:"source"` // "yaml" or "manual"
    MaxConns  int    `json:"max_conns"`
    CreatedAt string `json:"created_at"`
    UpdatedAt string `json:"updated_at"`
}

// POST /api/v1/instances/:id/test
type TestConnectionResponse struct {
    Success   bool   `json:"success"`
    Version   string `json:"version,omitempty"`    // e.g. "PostgreSQL 16.3"
    Error     string `json:"error,omitempty"`
    LatencyMs int64  `json:"latency_ms,omitempty"` // connection + query time
}
```

### DSN Masking

Never return raw DSN with password in API responses:
```go
func maskDSN(dsn string) string {
    u, err := url.Parse(dsn)
    if err != nil {
        return "***"
    }
    if u.User != nil {
        u.User = url.UserPassword(u.User.Username(), "***")
    }
    return u.String()
}
```

### Test Connection

```go
func testConnection(ctx context.Context, dsn string) TestConnectionResponse {
    start := time.Now()
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    conn, err := pgx.Connect(ctx, dsn)
    if err != nil {
        return TestConnectionResponse{Success: false, Error: err.Error()}
    }
    defer conn.Close(ctx)
    
    var version string
    err = conn.QueryRow(ctx, "SELECT version()").Scan(&version)
    if err != nil {
        return TestConnectionResponse{Success: false, Error: err.Error()}
    }
    
    return TestConnectionResponse{
        Success:   true,
        Version:   version,
        LatencyMs: time.Since(start).Milliseconds(),
    }
}
```

---

## 6. CSV Bulk Import

### CSV Format

```csv
id,name,dsn,enabled
prod-db1,Production Primary,postgres://pgpulse_monitor:pass@db1:5432/postgres,true
prod-db2,Production Replica,postgres://pgpulse_monitor:pass@db2:5432/postgres,true
,Staging DB,postgres://pgpulse_monitor:pass@staging:5432/postgres,true
```

- Header row required
- `id` column optional (auto-generated UUID if empty)
- `enabled` column optional (defaults to true)
- Quoted fields supported (DSN may contain special chars)

### Implementation

```go
// internal/api/instances_bulk.go

func (h *InstanceHandler) handleBulkImport(w http.ResponseWriter, r *http.Request) {
    contentType := r.Header.Get("Content-Type")
    
    var requests []CreateInstanceRequest
    
    switch {
    case strings.HasPrefix(contentType, "text/csv"):
        parsed, err := parseCSV(r.Body)
        // ...
    case strings.HasPrefix(contentType, "application/json"):
        json.NewDecoder(r.Body).Decode(&requests)
    default:
        http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
        return
    }
    
    type BulkResult struct {
        Row     int    `json:"row"`
        ID      string `json:"id,omitempty"`
        Success bool   `json:"success"`
        Error   string `json:"error,omitempty"`
    }
    
    var results []BulkResult
    for i, req := range requests {
        err := h.store.Create(ctx, toRecord(req))
        if err != nil {
            results = append(results, BulkResult{Row: i+1, Success: false, Error: err.Error()})
        } else {
            results = append(results, BulkResult{Row: i+1, ID: req.ID, Success: true})
        }
    }
    
    json.NewEncoder(w).Encode(results)
}
```

---

## 7. Orchestrator Hot-Reload

### Design

Add a `reloadLoop` goroutine to the orchestrator that polls DB every 60s:

```go
// internal/orchestrator/orchestrator.go — additions

func (o *Orchestrator) reloadLoop(ctx context.Context) {
    ticker := time.NewTicker(o.reloadInterval) // default 60s
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            o.reload(ctx)
        }
    }
}

func (o *Orchestrator) reload(ctx context.Context) {
    dbInstances, err := o.instanceStore.List(ctx)
    if err != nil {
        slog.Error("failed to list instances for reload", "err", err)
        return
    }
    
    dbMap := make(map[string]storage.InstanceRecord)
    for _, inst := range dbInstances {
        if inst.Enabled {
            dbMap[inst.ID] = inst
        }
    }
    
    // Stop removed/disabled instances
    for id, runner := range o.runners {
        if _, exists := dbMap[id]; !exists {
            slog.Info("stopping monitoring (instance removed/disabled)", "id", id)
            runner.stop()
            delete(o.runners, id)
        }
    }
    
    // Start new instances
    for id, inst := range dbMap {
        if _, exists := o.runners[id]; !exists {
            slog.Info("starting monitoring (new instance)", "id", id)
            runner, err := newInstanceRunner(ctx, toConfig(inst))
            if err != nil {
                slog.Error("failed to start instance runner", "id", id, "err", err)
                continue
            }
            o.runners[id] = runner
            go runner.run(ctx)
        }
    }
    
    // Detect DSN changes (stop old, start new)
    for id, inst := range dbMap {
        if runner, exists := o.runners[id]; exists {
            if runner.dsn != inst.DSN {
                slog.Info("restarting monitoring (DSN changed)", "id", id)
                runner.stop()
                newRunner, err := newInstanceRunner(ctx, toConfig(inst))
                if err != nil {
                    slog.Error("failed to restart instance runner", "id", id, "err", err)
                    delete(o.runners, id)
                    continue
                }
                o.runners[id] = newRunner
                go newRunner.run(ctx)
            }
        }
    }
}
```

### Concurrency Safety

`o.runners` map is accessed only from the reload goroutine (single-writer). Read access from API (for status) uses `sync.RWMutex`.

---

## 8. Frontend: Administration Page

### Component Structure

```
web/src/pages/AdministrationPage.tsx     — tab container (Instances / Users)
web/src/components/admin/InstancesTab.tsx — instance list + actions
web/src/components/admin/InstanceForm.tsx — add/edit form modal
web/src/components/admin/BulkImportModal.tsx — CSV import modal
web/src/components/admin/DeleteInstanceModal.tsx — confirmation dialog
web/src/hooks/useInstanceManagement.ts    — CRUD + bulk hooks
```

### InstancesTab Layout

```
┌────────────────────────────────────────────────────────┐
│ Instances                        [+ Add] [Bulk Import] │
├────────────────────────────────────────────────────────┤
│ Name          │ Host:Port    │ Source │ Status │ Actions│
│───────────────┼──────────────┼────────┼────────┼────────│
│ Prod Primary  │ db1:5432     │ yaml   │ ● OK   │ ✎ 🗑  │
│ Prod Replica  │ db2:5432     │ yaml   │ ● OK   │ ✎ 🗑  │
│ Staging       │ stg:5432     │ manual │ ● err  │ ✎ 🗑  │
│ [toggle]      │              │        │        │        │
└────────────────────────────────────────────────────────┘
```

### InstanceForm Modal

```
┌─────────────────────────────────────────┐
│ Add Instance                         ✕  │
├─────────────────────────────────────────┤
│ Name:  [________________________]       │
│ DSN:   [________________________]       │
│ Max Connections: [5___]                 │
│ Enabled: [✓]                            │
│                                         │
│ [Test Connection]                       │
│ ✓ Connected: PostgreSQL 16.3 (42ms)     │
│                                         │
│            [Cancel]  [Save]             │
└─────────────────────────────────────────┘
```

### BulkImportModal

```
┌─────────────────────────────────────────┐
│ Bulk Import Instances                ✕  │
├─────────────────────────────────────────┤
│ Paste CSV or upload a .csv file:        │
│ ┌─────────────────────────────────────┐ │
│ │ id,name,dsn,enabled                 │ │
│ │ db1,Primary,postgres://...          │ │
│ │ db2,Replica,postgres://...          │ │
│ └─────────────────────────────────────┘ │
│ [Upload .csv file]                      │
│                                         │
│ Preview (3 rows parsed):                │
│ ┌─────┬──────────┬─────────┬─────────┐ │
│ │ Row │ Name     │ DSN     │ Status  │ │
│ │ 1   │ Primary  │ db1:... │ pending │ │
│ │ 2   │ Replica  │ db2:... │ pending │ │
│ └─────┴──────────┴─────────┴─────────┘ │
│                                         │
│            [Cancel]  [Import]           │
│                                         │
│ Results:                                │
│  ✓ Row 1: db1 created                  │
│  ✗ Row 2: duplicate id 'db2'           │
└─────────────────────────────────────────┘
```

### Sidebar Fix

```tsx
// web/src/components/layout/Sidebar.tsx — change instance display
const displayName = instance.name || instance.id || `${instance.host}:${instance.port}`;
```

---

## 9. File Changes Summary

### New Backend Files
| File | Purpose |
|------|---------|
| `internal/alert/noop.go` | NoOpEvaluator for disabled alerting |
| `internal/storage/instances.go` | InstanceStore interface + PG implementation |
| `internal/api/instances_crud.go` | CRUD + bulk import handlers |
| `migrations/00X_instance_management.sql` | Add columns to instances table |

### Modified Backend Files
| File | Change |
|------|--------|
| `internal/config/config.go` | Add Name, MaxConns fields to InstanceConfig |
| `internal/orchestrator/runner.go` | pgx.Conn → pgxpool.Pool |
| `internal/orchestrator/group.go` | Use pool.Acquire/Release; remove nil evaluator guard |
| `internal/orchestrator/orchestrator.go` | Add reloadLoop, reload from DB, read from InstanceStore |
| `internal/api/router.go` | Register new instance CRUD routes |
| `internal/api/instances.go` | Add name, source to existing GET responses |
| `cmd/pgpulse-server/main.go` | Create InstanceStore, seed YAML, wire NoOpEvaluator, pass store to orchestrator |

### New Frontend Files
| File | Purpose |
|------|---------|
| `web/src/hooks/useInstanceManagement.ts` | CRUD + bulk + test connection hooks |
| `web/src/components/admin/InstancesTab.tsx` | Instance list with actions |
| `web/src/components/admin/InstanceForm.tsx` | Add/edit form modal |
| `web/src/components/admin/BulkImportModal.tsx` | CSV import modal |
| `web/src/components/admin/DeleteInstanceModal.tsx` | Confirmation dialog |

### Modified Frontend Files
| File | Change |
|------|--------|
| `web/src/pages/AdministrationPage.tsx` | Replace placeholder with tabbed layout |
| `web/src/components/layout/Sidebar.tsx` | Display instance.name with fallback |
| `web/src/types/models.ts` | Add instance management types |

---

## 10. Testing Strategy

### Backend Tests

| Test | Type | What |
|------|------|------|
| `internal/storage/instances_test.go` | Unit | CRUD operations, seed idempotency |
| `internal/api/instances_crud_test.go` | Unit | HTTP handler tests with httptest |
| `internal/api/instances_bulk_test.go` | Unit | CSV parsing, JSON parsing, partial failure |
| `internal/orchestrator/reload_test.go` | Unit | Hot-reload logic: add, remove, DSN change |
| `internal/alert/noop_test.go` | Unit | NoOpEvaluator satisfies interface |

### Frontend Checks

1. `npx tsc --noEmit` — zero errors
2. `npx eslint src/` — zero errors
3. `npx vite build` — success
4. `go build ./...` — success
5. `go test ./...` — all pass
6. `golangci-lint run` — clean
