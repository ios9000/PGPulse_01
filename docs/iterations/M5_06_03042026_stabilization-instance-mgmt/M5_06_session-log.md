# Session: 2026-03-04 to 2026-03-08 — M5_06 Stabilization + Instance Management

## Goal

Fix three demo-blocking issues (connection contention, missing instance names, nil evaluator panic) and build full instance management — CRUD API, CSV bulk import, YAML seeding with DB as source of truth, orchestrator hot-reload, Administration page UI.

## Context: Demo and Feedback

This session spanned the M5_05 demo deployment, a successful live demo to DBAs, and the M5_06 implementation.

### Demo Deployment (M5_05)

- Built and deployed PGPulse locally against PostgreSQL 16
- Resolved auth configuration (admin user seeding, bcrypt hashing on first startup)
- Resolved CORS issue by using embedded frontend (same-origin, no Vite dev server)
- Created PostgreSQL monitoring user: `pgpulse_monitor` with `pg_monitor` role
- Produced Linux cross-compilation package (`GOOS=linux GOARCH=amd64`)
- Produced deployment guide document for team

### Demo Results

- **Audience:** DBA team
- **Outcome:** Success — DBAs showed genuine curiosity
- **Feedback captured:** Need instance management through UI (not just YAML), including bulk import from CMDB or CSV
- **Issues observed during demo:**
  1. "conn busy" flood — three interval groups fighting over single pgx.Conn
  2. Sidebar showed green dot but no instance name — API missing `name` field
  3. Nil pointer panic when alerting disabled — evaluator was nil

### TimescaleDB Resolution

- Charts showed "No data available" because TimescaleDB extension was missing
- Migration creating metrics hypertable failed silently
- User installed TimescaleDB extension on PostgreSQL 16 instance
- Metrics pipeline now functional

## Agent Team Configuration

- **Team Lead:** Opus 4.6 (Claude Code Agent Teams)
- **Specialists:** 3 agents (API/Backend, Frontend, QA)
- **Duration:** 2 minutes 49 seconds
- **Note:** Initial OOM crash (4.82GB RSS on 8.59GB machine) — resolved by closing all other CLI terminals

## M5_06 Deliverables

### Commits (3 on master)

| Commit | Scope | What |
|--------|-------|------|
| 22c864f | orchestrator | Connection pool refactor (pgxpool.Pool), NoOp evaluator, config Name/MaxConns |
| b34165a | api/storage | Instance store, migration 006, CRUD API (5 endpoints), YAML seeding, hot-reload |
| 4d94901 | web | Admin page with tabs, InstancesTab, InstanceForm, BulkImportModal, DeleteInstanceModal, hooks, sidebar fix |

### New Backend Files

| File | Purpose |
|------|---------|
| internal/alert/noop.go | NoOpEvaluator for disabled alerting |
| internal/storage/instances.go | InstanceStore interface + PG implementation |
| internal/api/instances_crud.go | CRUD + bulk import + test connection handlers |
| migrations/006_instance_management.sql | Add name, source, max_conns columns |

### New Frontend Files

| File | Purpose |
|------|---------|
| web/src/hooks/useInstanceManagement.ts | CRUD + bulk + test hooks |
| web/src/components/admin/InstancesTab.tsx | Instance list with actions |
| web/src/components/admin/InstanceForm.tsx | Add/edit form modal |
| web/src/components/admin/BulkImportModal.tsx | CSV import with preview |
| web/src/components/admin/DeleteInstanceModal.tsx | Confirmation dialog |

### Modified Files (15+)

- internal/config/config.go — Name, MaxConns fields
- internal/orchestrator/runner.go — pgx.Conn → pgxpool.Pool
- internal/orchestrator/group.go — pool.Acquire/Release per cycle
- internal/orchestrator/orchestrator.go — reloadLoop (60s poll), read from DB
- internal/api/router.go — new CRUD routes
- internal/api/instances.go — name, source in GET responses
- cmd/pgpulse-server/main.go — InstanceStore, YAML seeding, NoOpEvaluator
- web/src/pages/AdministrationPage.tsx — real tabbed layout
- web/src/components/layout/Sidebar.tsx — name fallback
- web/src/types/models.ts — instance management types

## Architecture Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| D108 | pgxpool.Pool replaces single pgx.Conn | Three interval groups need concurrent connections |
| D109 | YAML seeds via INSERT ON CONFLICT DO NOTHING | YAML provides initial fleet; DB is source of truth after |
| D110 | `source` column: 'yaml' or 'manual' | Lets UI distinguish config-seeded vs manually-added instances |
| D111 | Orchestrator hot-reload every 60s | UI changes take effect without restart |
| D112 | CSV bulk import: id,name,dsn,enabled | Minimal schema matching YAML structure |
| D113 | NoOpEvaluator instead of nil guard | Proper interface satisfaction, no runtime panic |

## Verification

- go build: clean
- go vet: clean
- tsc --noEmit: clean
- vite build: clean
- All three demo issues fixed

## Not Done / Next Iteration

- [ ] User Management UI (M5_07/M5_08) — second tab on Administration page
- [ ] CMDB REST import integration — deferred to M8, hook interface designed
- [ ] Per-database analysis collectors (M7) — analiz_db.php Q1-Q18
- [ ] OS Agent (M6) — PGAM Q4-Q8, Q22-Q35 via procfs
