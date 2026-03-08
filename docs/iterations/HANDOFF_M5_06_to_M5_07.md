# PGPulse — Iteration Handoff: M5_06 → M5_07/M5_08

## DO NOT RE-DISCUSS

- **Connection pool:** pgxpool.Pool replaces single pgx.Conn — done, merged, tested
- **NoOpEvaluator:** Proper interface implementation when alerting disabled — done
- **Instance name field:** Added to config, API, and sidebar — done
- **YAML seeding pattern:** INSERT ON CONFLICT DO NOTHING, source='yaml' vs 'manual' — locked (D109)
- **Orchestrator hot-reload:** Polls DB every 60s for instance changes — done
- **Instance CRUD API:** POST/PUT/DELETE + bulk CSV import + test connection — done
- **Administration page layout:** Two tabs (Instances + Users), Instances tab complete — done
- **Frontend framework:** React + TypeScript + Tailwind CSS + TanStack Query + Apache ECharts — locked
- **TimescaleDB:** Installed and working — metrics pipeline functional

---

## What Exists Now

### Completed Milestones
- **M0:** Project setup
- **M1:** Core collectors (33/76 PGAM queries ported + 2 new)
- **M2:** Config, orchestrator, storage layer (TimescaleDB)
- **M3:** Auth (JWT + RBAC, admin/viewer roles)
- **M4:** Alert engine (evaluator, rules, notifiers)
- **M5_01:** Frontend scaffold (React/TS/Tailwind, routing, auth flow, fleet overview)
- **M5_02:** Server detail page (11 monitoring sections)
- **M5_03:** Real-time charts (ECharts, time range selector)
- **M5_04:** Dashboard polish (responsive layout, dark theme)
- **M5_05:** Alert management UI (alerts page, rules page, filters, modals)
- **M5_06:** Stabilization + instance management (pool, CRUD, CSV, admin page)

### Key Interfaces

```go
// internal/collector/collector.go
type InstanceContext struct {
    IsRecovery bool
}

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
    Interval() time.Duration
}

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

type AlertEvaluator interface {
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}
```

```go
// internal/storage/instances.go (NEW in M5_06)
type InstanceStore interface {
    List(ctx context.Context) ([]InstanceRecord, error)
    Get(ctx context.Context, id string) (InstanceRecord, error)
    Create(ctx context.Context, r InstanceRecord) error
    Update(ctx context.Context, r InstanceRecord) error
    Delete(ctx context.Context, id string) error
    Seed(ctx context.Context, r InstanceRecord) error // INSERT ON CONFLICT DO NOTHING
}
```

### Backend File Structure (key files)

```
cmd/pgpulse-server/main.go          — wires everything: config → migrations → seed → orchestrator → HTTP
internal/config/config.go            — YAML config with InstanceConfig{ID, Name, DSN, Enabled, MaxConns}
internal/orchestrator/
  orchestrator.go                    — manages instance runners, reloadLoop (60s)
  runner.go                          — per-instance pgxpool.Pool + interval groups
  group.go                           — pool.Acquire/Release per cycle, collectors run
internal/storage/
  pgstore.go                         — MetricStore (TimescaleDB metrics table)
  instances.go                       — InstanceStore (instances CRUD)
  migrations.go                      — embedded SQL migrations
internal/api/
  router.go                          — chi routes, middleware stack
  instances.go                       — GET /instances (list, get)
  instances_crud.go                  — POST/PUT/DELETE /instances, bulk import, test connection
  auth.go                            — login, refresh, me
  alerts.go                          — alert CRUD
internal/auth/                       — JWT, bcrypt, RBAC middleware
internal/alert/
  evaluator.go                       — threshold evaluation, state machine
  noop.go                            — NoOpEvaluator for disabled alerting
  rules.go                           — 19 built-in alert rules
  notifier/                          — email, telegram, slack, webhook
internal/collector/                  — 20+ collector files (33 PGAM queries + 2 new)
internal/version/                    — PG version detection + gate pattern
migrations/
  001_initial_schema.sql through 006_instance_management.sql
```

### Frontend File Structure (key files)

```
web/src/
  pages/
    FleetOverview.tsx                — instance cards with status
    ServerDetail.tsx                 — 11 monitoring sections
    AlertsDashboard.tsx              — active alerts with filters
    AlertRules.tsx                   — rule management with modals
    AdministrationPage.tsx           — tabbed: Instances (done) + Users (placeholder)
    LoginPage.tsx                    — JWT auth flow
  components/
    admin/
      InstancesTab.tsx               — instance list, add/edit/delete/bulk
      InstanceForm.tsx               — add/edit modal with test connection
      BulkImportModal.tsx            — CSV paste/upload with preview
      DeleteInstanceModal.tsx        — confirmation with yaml warning
    alerts/
      AlertFilters.tsx, AlertRow.tsx, RuleRow.tsx, RuleFormModal.tsx, DeleteConfirmModal.tsx
    server/
      ConnectionsSection.tsx, CacheSection.tsx, ReplicationSection.tsx, ...
    layout/
      Sidebar.tsx                    — nav + server list (name || id || host:port fallback)
  hooks/
    useInstanceManagement.ts         — CRUD + bulk + test hooks
    useAlerts.ts, useAlertRules.ts   — alert hooks
    useInstances.ts, useMetrics.ts   — monitoring hooks
  types/models.ts                    — all TypeScript interfaces
```

### API Endpoints (complete)

```
Auth:       POST /auth/login, /auth/refresh, GET /auth/me
Instances:  GET /instances, GET /instances/:id
            POST /instances, PUT /instances/:id, DELETE /instances/:id (admin)
            POST /instances/bulk, POST /instances/:id/test (admin)
Metrics:    GET /instances/:id/metrics
Alerts:     GET /alerts, GET /alerts/rules
            POST /alerts/rules, POST /alerts/test (admin)
Health:     GET /health
```

### Build Verification

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

---

## What Was Just Completed (M5_06)

### Stabilization (3 demo issues fixed)
1. **pgxpool.Pool** replaces single pgx.Conn — eliminates "conn busy" errors
2. **NoOpEvaluator** — proper interface when alerting disabled, no nil panic
3. **Instance name** — added to config struct, API response, sidebar display

### Instance Management (full CRUD)
4. **InstanceStore** — DB-backed CRUD with PG implementation
5. **Migration 006** — adds name, source, max_conns, timestamps to instances table
6. **YAML seeding** — INSERT ON CONFLICT DO NOTHING on startup, source='yaml'
7. **CRUD API** — 5 new endpoints (create, update, delete, bulk, test connection)
8. **Orchestrator hot-reload** — polls DB every 60s, starts/stops instance runners
9. **Administration page** — Instances tab with full management UI, CSV bulk import
10. **Sidebar fix** — displays name || id || host:port

3 commits: 22c864f, b34165a, 4d94901

---

## Known Issues

| Issue | Impact | Status |
|-------|--------|--------|
| TimescaleDB required for metrics | Charts empty without extension | Resolved — user installed TimescaleDB |
| Claude Code OOM in Agent Teams | Crash at 4.82GB RSS | Workaround: close other terminals before running |
| Go test scope | `./...` scans web/node_modules | Use `./cmd/... ./internal/...` instead |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests CI-only |

---

## PGAM Query Porting Status: 33/76

| Source | Status |
|--------|--------|
| analiz2.php Q1–Q19 (instance) | ✅ Done (M1_01) |
| analiz2.php Q20–Q21, Q37–Q38, Q40 (replication) | ✅ Done (M1_02b) |
| analiz2.php Q42–Q47 (progress) | ✅ Done (M1_03) |
| analiz2.php Q48–Q51 (statements) | ✅ Done (M1_04) |
| analiz2.php Q53–Q57 (locks) | ✅ Done (M1_05) |
| analiz2.php Q58 (extensions) | ✅ Covered by Q18 |
| analiz2.php Q4–Q8, Q22–Q35 (OS/cluster) | 🔲 M6 (OS Agent) |
| analiz2.php Q36, Q39 (PG < 10) | ⏭️ Skipped (below PG 14 min) |
| analiz2.php Q41 (logical replication) | ⏭️ Deferred (per-DB connections) |
| analiz2.php Q52 (normalized report) | ⏭️ Covered by Q50+Q51 at metric level |
| analiz_db.php Q1–Q18 (per-DB) | 🔲 M7 (Per-Database Analysis) |
| New: checkpoint.go, io_stats.go | ✅ Done (M1_03, M1_03b) |

---

## Roadmap — What's Next

### Immediate (M5 completion)

**M5_07/M5_08 — User Management UI**
- Second tab on Administration page
- User list, create/edit/delete users
- Role assignment (admin/viewer)
- Password reset
- Backend: user CRUD endpoints already partially exist (auth module)
- Scope: frontend-heavy, small backend additions

### Post-MVP

| Milestone | Name | Description |
|-----------|------|-------------|
| M6 | OS Agent | procfs metrics, Patroni/ETCD status (19 deferred PGAM queries) |
| M7 | Per-Database Analysis | analiz_db.php Q1–Q18 (bloat, vacuum, indexes, TOAST) |
| M8 | P1 Features | Query plan capture, session kill, CMDB integration, pg_settings diff |
| M9 | ML Phase 1 | Anomaly detection, workload forecasting |
| M10 | Reports & Export | PDF/CSV reports, scheduled digests |
| M11 | Polish & Release | Helm chart, systemd, docs, performance |

---

## Environment

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | |
| Node.js | 22.14.0 | |
| Claude Code | 2.1.63+ | Bash works on Windows (EINVAL fixed) |
| golangci-lint | v2.10.1 | v2 config format |
| PostgreSQL | 16 | Local native install, TimescaleDB installed |
| Git | 2.52.0 | |

---

## Workflow Reminder

```bash
# Build + verify
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...

# Run locally
./pgpulse-server.exe --config configs/pgpulse.yml

# Cross-compile for Linux
cd web && npm run build && cd ..
GOOS=linux GOARCH=amd64 go build -o pgpulse-server ./cmd/pgpulse-server/
```
