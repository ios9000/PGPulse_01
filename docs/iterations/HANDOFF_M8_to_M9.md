# PGPulse — Iteration Handoff: M8 -> M9

---

## DO NOT RE-DISCUSS

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- EXPLAIN query body is intentionally NOT parameterized — auth gate is the protection
- DB is source of truth for instances; YAML seeds on first start, DB overrides after
- DBCollector is a parallel interface to Collector — do NOT merge them
- Version-adaptive SQL uses the Gate pattern in internal/version/
- Frontend uses lib/forecastUtils.ts for forecast chart helpers (moved from components/ to break circular import)
- `TimeSeriesChart.tsx` accepts `extraSeries`, `xAxisMax`, `nowMarkLine` for forecast overlays

---

## What Exists Now

### Complete Feature Set (M0-M8)
- **33+ collectors** covering instance metrics, replication, progress, statements, locks, wait events, per-database analysis (17 DB sub-collectors), logical replication
- **ML engine**: STL decomposition baselines, Holt-Winters forecasting, anomaly detection with Z-score/IQR scoring, model persistence to DB
- **38+ REST API endpoints** with 4-role RBAC (super_admin, roles_admin, dba, app_admin)
- **React web UI**: Fleet overview, server detail (15+ sections), database detail, alert management, instance management, user management, query plan viewer, settings diff/timeline, plan history
- **Interactive DBA tools**: session cancel/terminate, on-demand EXPLAIN, cross-instance settings diff
- **23 alert rules** (14 PGAM thresholds, 2 replication, 3 forecast-based, 1 logical replication, 3 deferred)
- **OS agent** (separate binary, Linux only, procfs/sysfs)
- **Cluster providers** (Patroni REST API, ETCD v3)
- **11 SQL migrations**
- **Demo environment** on Ubuntu 24 / PG 16.13 (deployed, tested, hotfixed)

### Key Packages
| Package | Purpose |
|---------|---------|
| internal/collector/ | Instance + DB collectors, registry |
| internal/api/ | REST handlers (38+ endpoints) |
| internal/auth/ | JWT, RBAC, rate limiting |
| internal/alert/ | Evaluator, rules, forecast provider |
| internal/ml/ | STL baseline, detector, forecast, persistence |
| internal/plans/ | Auto-capture, store, retention |
| internal/settings/ | Temporal snapshots, diff |
| internal/storage/ | PGStore, instances, migrations |
| internal/orchestrator/ | Runner, group, DB runner, hot-reload |
| internal/version/ | PG version detection + Gate pattern |
| internal/agent/ | Linux OS metrics (separate binary) |
| internal/cluster/ | Patroni + ETCD providers |
| web/src/ | React 18 + TypeScript + Tailwind + ECharts |

### Build Status
```
go build ./...: clean
go test ./cmd/... ./internal/...: all pass (15 packages)
golangci-lint run: 0 issues
tsc --noEmit: 0 errors
npm run build: success
```

---

## What Was Just Completed (M8)

M8 combined P1 features with ML Phase 1 across 10 sub-iterations (M8_01-M8_08 + two hotfix rounds M8_09-M8_10):

1. **M8_01**: Session kill API, EXPLAIN API, cross-instance settings diff API
2. **M8_02**: Auto-capture query plans, temporal settings snapshots, STL-based ML anomaly detection (gonum)
3. **M8_03**: DB instance lister (replaces static config), ML model persistence (JSONB), session kill routes wiring
4. **M8_04**: STL-based N-step-ahead forecasting with confidence bounds, forecast REST endpoint
5. **M8_05**: Forecast alert wiring (sustained crossing logic), forecast chart overlay on connections_active
6. **M8_06**: Session kill UI, settings diff UI, query plan viewer UI, forecast overlay on all charts, toast system
7. **M8_07**: Plan history UI, settings timeline UI, application_name enrichment, Administration.tsx lint fix
8. **M8_08**: Logical replication monitoring (PGAM Q41) — DB sub-collector, API endpoint, frontend section, alert rule
9. **M8_09**: Hotfix — TDZ crash in production bundle, CSP, bloat PG16 compat, WAL receiver column, sequences NULL, port config
10. **M8_10**: Hotfix — explain handler recreation, breadcrumb fix, replication/lock/progress scan errors

---

## Known Issues

| Issue | Status |
|-------|--------|
| ~14 PGAM queries not ported (Q22-Q35 etc.) | Deferred — mostly VTB-internal or pre-PG14 |
| CGO_ENABLED=0 blocks -race on Windows | Test without -race locally, CI has -race |
| Docker Desktop unavailable | BIOS virtualization disabled — integration tests CI-only |

---

## Next Task: M9 — Reports & Export

M9 adds reporting and data export capabilities to PGPulse.

### Scope (proposed)

1. **Scheduled Health Reports** — PDF or HTML reports generated on a cron schedule
   - Instance health summary (connections, cache, replication, alerts)
   - Top-N queries by execution time
   - Anomaly highlights from ML detector
   - Configurable schedule (daily, weekly, monthly)
   - Email delivery via existing SMTP notifier

2. **CSV/JSON Metric Export** — REST endpoints for data export
   - `GET /api/v1/instances/{id}/metrics/export?format=csv&start=...&end=...`
   - `GET /api/v1/instances/{id}/databases/{db}/export?format=json`
   - Streaming response for large datasets
   - Configurable column selection

3. **Dashboard Snapshot** — Point-in-time capture of dashboard state
   - API endpoint to trigger snapshot
   - Store as HTML blob or JSON
   - History of past snapshots with comparison

4. **Report Templates** — Customizable report content
   - Default template with all sections
   - User-configurable section selection
   - Template stored in DB or YAML

### Architecture Notes
- Reports should use existing `MetricStore.Query()` for data access
- PDF generation: consider `go-wkhtmltopdf` or pure-Go alternative
- Email delivery: reuse `internal/alert/notifier/email.go`
- Storage: new `reports` table for generated reports
- API: new routes under `/api/v1/reports/`

### Files to Create (estimated)
- `internal/reports/generator.go` — report generation engine
- `internal/reports/templates/` — HTML report templates
- `internal/reports/store.go` — report storage
- `internal/api/reports.go` — REST handlers
- `migrations/012_reports.sql` — reports table
- `web/src/pages/Reports.tsx` — reports management page
- `web/src/hooks/useReports.ts` — React Query hooks

---

## Workflow Reminder

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6

# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run

# Never use: go test ./... (scans web/node_modules/)
```
