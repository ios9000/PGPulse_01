# PGPulse — Iteration Handoff: M5_07 → M6_01

## DO NOT RE-DISCUSS

- **M5 is complete.** All six M5 iterations shipped: scaffold, server detail, charts, polish, alerts UI, stabilization + instance management, user management UI.
- **UserStore interface:** Now has `CountActiveByRole` and `Delete` — do not add duplicates.
- **User management routes:** Registered at `/api/v1/auth/users/` with PermUserManagement guard — do not re-register.
- **Admin password reset:** Separate endpoint from self-service (`/api/v1/auth/me/password`) — two different security models — locked.
- **Hard delete for users:** No soft delete in MVP — locked (D-M5_07).
- **YAML seeding pattern:** INSERT ON CONFLICT DO NOTHING, source='yaml' vs 'manual' — locked (D109).
- **Connection pool:** pgxpool.Pool throughout — done.
- **Frontend framework:** React + TypeScript + Tailwind CSS + TanStack Query + Apache ECharts — locked.
- **TimescaleDB:** Installed and working — metrics pipeline functional.

---

## What Exists Now

### Completed Milestones
- **M0:** Project setup
- **M1:** Core collectors (33/76 PGAM queries ported + 2 new)
- **M2:** Config, orchestrator, storage layer (TimescaleDB)
- **M3:** Auth (JWT + RBAC, admin/viewer roles)
- **M4:** Alert engine (evaluator, rules, notifiers)
- **M5_01–M5_07:** Full web UI MVP — fleet overview, server detail, charts, alerts UI, administration page (instances + users)

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
// internal/auth/store.go
type UserStore interface {
    GetByUsername(ctx context.Context, username string) (*User, error)
    GetByID(ctx context.Context, id int64) (*User, error)
    Create(ctx context.Context, username, passwordHash, role string) (*User, error)
    Count(ctx context.Context) (int64, error)
    CountActiveByRole(ctx context.Context, role string) (int64, error)  // added M5_07
    List(ctx context.Context) ([]*User, error)
    Update(ctx context.Context, id int64, fields UpdateFields) error
    UpdatePassword(ctx context.Context, id int64, passwordHash string) error
    UpdateLastLogin(ctx context.Context, id int64) error
    Delete(ctx context.Context, id int64) error  // added M5_07
}
```

```go
// internal/storage/instances.go
type InstanceStore interface {
    List(ctx context.Context) ([]InstanceRecord, error)
    Get(ctx context.Context, id string) (InstanceRecord, error)
    Create(ctx context.Context, r InstanceRecord) error
    Update(ctx context.Context, r InstanceRecord) error
    Delete(ctx context.Context, id string) error
    Seed(ctx context.Context, r InstanceRecord) error
}
```

### API Endpoints (complete as of M5_07)

```
Auth:         POST /api/v1/auth/login, /auth/refresh, GET /auth/me
              PUT  /api/v1/auth/me/password  (self-service, requires current password)
Users:        GET    /api/v1/auth/users                    (PermUserManagement)
              POST   /api/v1/auth/users                    (PermUserManagement)
              PUT    /api/v1/auth/users/{id}               (PermUserManagement)
              DELETE /api/v1/auth/users/{id}               (PermUserManagement)
              PUT    /api/v1/auth/users/{id}/password      (PermUserManagement)
Instances:    GET /instances, GET /instances/:id
              POST /instances, PUT /instances/:id, DELETE /instances/:id (admin)
              POST /instances/bulk, POST /instances/:id/test (admin)
Metrics:      GET /instances/:id/metrics
Alerts:       GET /alerts, GET /alerts/rules
              POST /alerts/rules, POST /alerts/test (admin)
Health:       GET /health
```

### Roles (RBAC)

| Role | Permissions |
|------|-------------|
| `super_admin` | user_management, instance_management, alert_management, view_all, self_management |
| `roles_admin` | user_management, view_all, self_management |
| `dba` | instance_management, alert_management, view_all, self_management |
| `app_admin` | alert_management, view_all, self_management |

### Backend File Structure (key files)

```
cmd/pgpulse-server/main.go
internal/config/config.go
internal/orchestrator/orchestrator.go, runner.go, group.go
internal/storage/pgstore.go, instances.go, migrations.go
internal/api/router.go (or server.go), instances.go, instances_crud.go, auth.go, alerts.go
internal/auth/jwt.go, middleware.go, password.go, ratelimit.go, rbac.go, store.go
internal/alert/evaluator.go, noop.go, rules.go, notifier/
internal/collector/  — 20+ collector files
internal/version/    — version detection + gate pattern
migrations/001–006   — schema up to instance management
```

### Frontend File Structure (key files)

```
web/src/
  pages/
    FleetOverview.tsx, ServerDetail.tsx, AlertsDashboard.tsx, AlertRules.tsx
    AdministrationPage.tsx  — tabbed: Instances (done) + Users (done as of M5_07)
    LoginPage.tsx
  components/
    admin/
      InstancesTab.tsx, InstanceForm.tsx, BulkImportModal.tsx, DeleteInstanceModal.tsx
      UsersTab.tsx, UserFormModal.tsx, DeleteUserModal.tsx, ResetPasswordModal.tsx  [NEW M5_07]
    alerts/  — AlertFilters, AlertRow, RuleRow, RuleFormModal, DeleteConfirmModal
    server/  — ConnectionsSection, CacheSection, ReplicationSection, etc.
    layout/Sidebar.tsx
  hooks/
    useInstanceManagement.ts, useAlerts.ts, useAlertRules.ts
    useInstances.ts, useMetrics.ts
    useUsers.ts  [updated M5_07 — added useDeleteUser, useResetUserPassword]
  types/models.ts
```

### Build Verification

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

---

## What Was Just Completed (M5_07)

User Management UI — the Users tab on the Administration page:

**Backend:**
- Added `CountActiveByRole` and `Delete` to UserStore interface and PGUserStore
- `handleDeleteUser` with self-deletion and last-active-super_admin guards
- `handleAdminResetPassword` (no current password required, min 8 chars)
- Both routes registered under PermUserManagement group
- Tests updated in auth_test.go (mock store expanded)

**Frontend:**
- UsersTab with role-colored badges, status, last login, action buttons
- UserFormModal (create/edit, role filtering by current user's role)
- DeleteUserModal (super_admin warning, last-super-admin disable)
- ResetPasswordModal (confirm field, client-side match validation)
- useDeleteUser, useResetUserPassword hooks added to useUsers.ts
- Administration.tsx wired up

Build: go build ✅ · go vet ✅ · golangci-lint 0 issues ✅ · npm run build ✅

---

## Known Issues

| Issue | Impact | Status |
|-------|--------|--------|
| Claude Code OOM in Agent Teams | Crash at 4.82GB RSS | Close other terminals before running |
| Go test scope | `./...` scans web/node_modules | Use `./cmd/... ./internal/...` |
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

## Roadmap

| Milestone | Name | Status |
|-----------|------|--------|
| M0 | Project Setup | ✅ Done |
| M1 | Core Collectors | ✅ Done |
| M2 | Storage & API | ✅ Done |
| M3 | Auth & Security | ✅ Done |
| M4 | Alerting | ✅ Done |
| M5 | Web UI (MVP) | ✅ Done (M5_01 through M5_07) |
| M6 | OS Agent | 🔲 Next |
| M7 | Per-Database Analysis | 🔲 |
| M8 | P1 Features | 🔲 |
| M9 | ML Phase 1 | 🔲 |

---

## Next Task: M6_01 — OS Agent

### Goal
Port the OS and cluster metrics from PGAM (queries Q4–Q8, Q22–Q35) to Go using
procfs/sysfs instead of COPY TO PROGRAM. This is the most significant architectural
departure from PGAM — PGAM ran shell commands through PostgreSQL itself; PGPulse
uses a separate Go binary with direct OS access.

### Scope for M6_01 (first M6 iteration)
- Decide deployment model: sidecar agent vs. embedded OS collector
- Port Q4–Q8 (hostname, OS dist, uptime, system time, RAM) to procfs
- Port Q22–Q25 (memory overcommit, meminfo, top summary, disk usage) to procfs/sysfs
- Port Q26–Q27 (iostat) to procfs/sysfs (or os/exec as fallback)
- Define the OSMetric collection interface and how it integrates with the orchestrator
- Add OS metrics section to the ServerDetail frontend page

### Open Design Questions for M6 (discuss before writing team-prompt)
1. **Deployment model:** Embed OS collector in the main server binary (for same-host monitoring)
   vs. ship a separate `pgpulse-agent` binary (for remote host monitoring)?
   PGAM ran everything through PG's COPY TO PROGRAM, which required superuser.
   PGPulse should NOT require superuser — so the OS metrics must come from a process
   that has OS access directly.
2. **Patroni/ETCD integration:** PGAM used shell commands to call patronictl/etcdctl.
   For M6, we should use Patroni's REST API and etcd's HTTP API instead.
   These binaries are optional — how do we handle absent/non-Patroni environments gracefully?
3. **Metric collection frequency:** OS metrics (CPU, memory, disk) change faster than
   PG metrics. What polling interval? (Suggest: 15s for CPU/memory, 60s for disk)

### PGAM Queries Covered by M6
| PGAM # | Metric | PGAM Method | PGPulse Approach |
|--------|--------|-------------|-----------------|
| Q4 | Hostname | COPY TO PROGRAM 'hostname -f' | os.Hostname() |
| Q5 | OS distribution | pg_read_file('/etc/lsb-release') | os.ReadFile('/etc/os-release') |
| Q6 | PG start time | pg_postmaster_start_time() | already in instance collector |
| Q7 | PG uptime | now() - pg_postmaster_start_time() | already in instance collector |
| Q8 | OS uptime | COPY TO PROGRAM 'uptime' | /proc/uptime |
| Q9 | OS system time | COPY TO PROGRAM 'date' | time.Now() |
| Q12 | Total RAM | COPY TO PROGRAM 'free -m' | /proc/meminfo |
| Q22 | Memory overcommit | COPY TO PROGRAM 'grep Comm /proc/meminfo' | /proc/meminfo |
| Q23 | Full meminfo + free | COPY TO PROGRAM | /proc/meminfo |
| Q24 | CPU/top summary | COPY TO PROGRAM 'top -b' | /proc/stat, /proc/loadavg |
| Q25 | Disk usage | COPY TO PROGRAM 'df -Tih' | syscall.Statfs or /proc/mounts |
| Q26–Q27 | I/O stats | COPY TO PROGRAM 'iostat' | /proc/diskstats |
| Q28–Q31 | Patroni status | COPY TO PROGRAM 'patronictl ...' | Patroni REST API |
| Q32–Q33 | ETCD status | COPY TO PROGRAM 'etcdctl ...' | etcd HTTP API |

---

## Environment

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.0 windows/amd64 | |
| Node.js | 22.14.0 | |
| Claude Code | 2.1.63+ | Bash works on Windows (EINVAL fixed) |
| golangci-lint | v2.10.1 | v2 config format |
| PostgreSQL | 16 | Local native install, TimescaleDB installed |

---

## Workflow Reminder

```bash
# Build + verify
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...

# Run locally
./pgpulse-server.exe --config configs/pgpulse.yml

# Commit pattern for M6
git add .
git commit -m "feat(collector): add OS metrics via procfs"
git push
```
