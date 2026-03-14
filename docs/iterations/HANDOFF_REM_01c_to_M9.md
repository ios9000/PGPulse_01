# PGPulse — Iteration Handoff: REM_01c -> M9

**Date:** 2026-03-14
**From:** REM_01c (Remediation Metric Key Fix)
**To:** M9 (Reports & Export)
**Commit:** dbed45a

---

## DO NOT RE-DISCUSS

All items from previous handoffs remain in force:

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- EXPLAIN query body is intentionally NOT parameterized — auth gate is the protection
- DB is source of truth for instances; YAML seeds on first start, DB overrides after
- DBCollector is a parallel interface to Collector — do NOT merge them
- Version-adaptive SQL uses the Gate pattern in internal/version/
- Frontend uses lib/forecastUtils.ts for forecast chart helpers (moved from components/ to break circular import)
- `TimeSeriesChart.tsx` accepts `extraSeries`, `xAxisMax`, `nowMarkLine` for forecast overlays
- `Base.point()` in `internal/collector/base.go` prepends `"pg."` via `metricPrefix = "pg"`
- OSSQLCollector and ClusterCollector have their own `point()` methods — isolated from Base
- Test scope must be `./cmd/... ./internal/...` (not `./...`) to prevent scanning `web/node_modules/`
- `go:embed` bakes the React build into the Go binary
- **MemoryStore** implements `collector.MetricStore` — 2h default retention, configurable via `--history`
- **Live mode** auto-detected when `storage.dsn` is absent — ML/forecast/plan/settings disabled
- **Auth bypass** auto-enabled on localhost; `--no-auth` flag for remote bind
- **APIServer extensions** use setter pattern: `SetLiveMode()`, `SetAuthMode()`
- Do NOT reintroduce `pgpulse.*` prefix for PG metrics or `os.diskstat.*` for disk metrics

### Remediation Engine — IMPLEMENTED (REM_01a-c)

- 25 compiled-in rules (17 PG + 8 OS) in `internal/remediation/`
- Dual evaluation modes: alert-triggered (`EvaluateMetric`) and full-scan (`Diagnose`)
- `getOS(snap, suffix)` helper checks both `os.*` and `pg.os.*` prefixes — do NOT normalize at ingestion
- `isOSMetric(key, suffix)` matches alert-triggered metric key against both prefixes
- Replication slot rule Diagnose mode returns nil — per-slot labeled metrics don't map to flat snapshot
- Connection rules use `pg.connections.utilization_pct` directly (already a percentage, no division)
- `NullStore` for live mode, `PGStore` for persistent mode — both implement `remediation.Store`
- 5 API endpoints under `/api/v1/instances/{id}/recommendations`, `/api/v1/recommendations`, etc.
- Advisor page in frontend with Diagnose button

---

## What Exists Now

### Complete Feature Set (M0-M8, REM_01a-c)
- **33+ collectors** covering instance metrics, replication, progress, statements, locks, wait events, per-database analysis (17 DB sub-collectors), logical replication
- **ML engine**: STL decomposition baselines, Holt-Winters forecasting, anomaly detection with Z-score/IQR scoring, model persistence to DB
- **57 REST API endpoints** with 4-role RBAC (super_admin, roles_admin, dba, app_admin)
- **React web UI**: Fleet overview, server detail (15+ sections), database detail, alert management, instance management, user management, query plan viewer, settings diff/timeline, plan history, Advisor page
- **25 remediation rules** (17 PG + 8 OS) with alert-triggered and diagnose modes
- **Interactive DBA tools**: session cancel/terminate, on-demand EXPLAIN, cross-instance settings diff
- **23 alert rules** (14 PGAM thresholds, 2 replication, 3 forecast-based, 1 logical replication, 3 deferred)
- **OS agent** (separate binary, Linux only, procfs/sysfs)
- **Cluster providers** (Patroni REST API, ETCD v3)
- **13 SQL migrations**
- **Demo environment** on Ubuntu 24 / PG 16.13

### Key Packages
| Package | Purpose |
|---------|---------|
| internal/collector/ | Instance + DB collectors, registry |
| internal/api/ | REST handlers (57 endpoints) |
| internal/auth/ | JWT, RBAC, rate limiting |
| internal/alert/ | Evaluator, rules, forecast provider |
| internal/ml/ | STL baseline, detector, forecast, persistence |
| internal/remediation/ | Rule-based remediation engine (25 rules) |
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
go test ./cmd/... ./internal/...: all pass (17 packages)
golangci-lint run: 0 issues
npm run build: success
npm run typecheck: 0 errors
npm run lint: clean (1 pre-existing warning)
```

---

## What Was Just Completed

### REM_01a — Remediation Engine Backend (2026-03-13)
- Rule-based remediation engine: 25 rules (17 PG + 8 OS)
- Engine, PGStore, NullStore, AlertAdapter
- 5 API endpoints, dispatcher integration

### REM_01b — Remediation Frontend + Backend Gaps (2026-03-14)
- Advisor page with Diagnose button
- Alert enrichment (recommendations inline with alerts)
- Email template recommendations
- AlertRow expand/collapse UI
- Handler/store tests

### REM_01c — Metric Key Fix (2026-03-14, bugfix)
Fixed 13 broken remediation rules whose metric keys didn't match actual collector output:

| Old Key | New Key | Rules Affected |
|---------|---------|----------------|
| `pg.connections.active` + `max_connections` | `pg.connections.utilization_pct` | rem_conn_high, rem_conn_exhausted |
| `pg.transactions.commit_ratio` | `pg.transactions.commit_ratio_pct` | rem_commit_ratio_low |
| `pg.replication.replay_lag_bytes` | `pg.replication.lag.replay_bytes` | rem_repl_lag_bytes, rem_repl_lag_critical |
| `pg.replication.slot_inactive` | `pg.replication.slot.active` (inverted) | rem_repl_slot_inactive |
| `pg.transactions.oldest_active_sec` | `pg.long_transactions.oldest_seconds` | rem_long_txn_warn, rem_long_txn_crit |
| `pg.statements.fill_pct` | `pg.extensions.pgss_fill_pct` | rem_pgss_fill |
| `pg.db.bloat.ratio` | `pg.db.bloat.table_ratio` | rem_bloat_high, rem_bloat_extreme |
| `os.*` only | `os.*` or `pg.os.*` | all 8 OS rules |

Additional:
- Added `pg.server.wraparound_pct` metric to ServerInfoCollector
- Added `getOS()`/`isOSMetric()` helpers for dual OS prefix support
- All 25 rules have test cases

---

## Demo Environment

```
Ubuntu 24.04 VM: 185.159.111.139

PGPulse UI:     http://185.159.111.139:8989     (persistent mode)
Login:          admin / pgpulse_admin

PostgreSQL 16.13:
  Primary:      localhost:5432
  Replica:      localhost:5433
  Chaos:        localhost:5434

Monitor user:   pgpulse_monitor / pgpulse_monitor_demo
Storage DB:     pgpulse_storage on port 5432

OS Metrics:     Flowing on all 3 instances via pg_read_file('/proc/*')
```

---

## Known Issues

| Issue | Status | Notes |
|-------|--------|-------|
| ~14 PGAM queries not ported (Q22-Q35 etc.) | Deferred | Mostly VTB-internal or pre-PG14 |
| CGO_ENABLED=0 blocks -race on Windows | Known | Test without -race locally, CI has -race |
| Docker Desktop unavailable | Known | BIOS virtualization disabled — integration tests CI-only |
| Disk `read_bytes_per_sec` is delta-per-interval | Cosmetic | Name follows competitive standard |
| `c.command_desc` SQL bug in cluster progress | Open | Pre-existing, PG16 |
| `002_timescaledb.sql` migration skip warning | Open | Pre-existing |
| Slot rule Diagnose returns nil | By design | Per-slot labels don't map to flat snapshot |

---

## Next Task: M9 — Reports & Export

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
- `migrations/014_reports.sql` — reports table
- `web/src/pages/Reports.tsx` — reports management page
- `web/src/hooks/useReports.ts` — React Query hooks

---

## Build & Deploy

```bash
# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...

# Cross-compile
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/pgpulse-server-windows-amd64.exe ./cmd/pgpulse-server

# Deploy to demo VM
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'

# Never use: go test ./... (scans web/node_modules/)
```

---

## Workflow Reminder

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6

# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

---

## Project Knowledge Status

| Document | Status |
|----------|--------|
| PGPulse_Development_Strategy_v2.md | Current |
| PGAM_FEATURE_AUDIT.md | Current |
| Chat_Transition_Process.md | Current |
| Save_Point_System.md | Current |
| CODEBASE_DIGEST.md | Current (regenerated REM_01c, 2026-03-14) |
