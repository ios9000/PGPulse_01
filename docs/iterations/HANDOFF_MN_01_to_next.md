# PGPulse — Iteration Handoff: MN_01 → Next

**Date:** 2026-03-13
**From:** MN_01 (Metric Naming Standardization)
**To:** ML/DL Remediation (rule-based approach first)
**Commit:** 2f96bed

---

## DO NOT RE-DISCUSS

All items from MW_01b handoff remain in force, plus:

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- Forecast polling in the frontend is 5 minutes
- `forecastUtils.ts` in `web/src/lib/` is canonical for `buildForecastSeries` and `getNowMarkLine`
- YAML seeds the database on startup; database becomes source of truth after first run
- `go:embed` bakes the React build into the Go binary — PGPulse is its own web server
- PGPulse listens on port 8989
- Test scope must be `./cmd/... ./internal/...` (not `./...`) to prevent scanning `web/node_modules/`
- OSSQLCollector reuses agent parsers from `internal/agent/` — no code duplication
- Per-instance `os_metrics_method` config: "sql" (default), "agent", "disabled"
- `docs/CODEBASE_DIGEST.md` is auto-generated at end of each iteration — always re-upload to Project Knowledge
- **MemoryStore** implements `collector.MetricStore` — 2h default retention, configurable via `--history`
- **Live mode** auto-detected when `storage.dsn` is absent — ML/forecast/plan/settings disabled
- **Auth bypass** auto-enabled on localhost; `--no-auth` flag for remote bind
- **APIServer extensions** use setter pattern: `SetLiveMode()`, `SetAuthMode()` — do NOT add more params to `api.New()`
- **NullAlertHistoryStore** implements all 5 methods of `AlertHistoryStore` — used in live mode
- **`/api/v1/system/mode`** returns `{"mode":"live","retention":"2h0m0s"}` or `{"mode":"persistent"}` — no auth required

### Metric Naming — IMPLEMENTED (MN_01)

- **Four top-level prefixes:**
  - `pg.*` — PostgreSQL metrics (connections, cache, replication, bgwriter, etc.)
  - `os.*` — OS metrics (cpu, memory, disk, load) — both SQL and agent paths
  - `cluster.*` — HA infrastructure (Patroni, etcd) — UNCHANGED per D200
  - `pgpulse.*` — reserved for internal/meta (none exist currently)
- **`Base.point()` in `internal/collector/base.go`** prepends `"pg."` to all metric names via `metricPrefix = "pg"`
- **OSSQLCollector** has its own standalone `point()` method — emits `os.*` directly, unaffected by `Base.point()`
- **ClusterCollector** has its own `point()` closure — emits `cluster.*` directly, unaffected by `Base.point()`
- **`os.diskstat.*` eliminated** — all disk I/O metrics now under `os.disk.*`
- **Disk values converted from KB to bytes** (×1024) when renaming `read_kb`/`write_kb` → `read_bytes_per_sec`/`write_bytes_per_sec`. Values are delta-per-interval, not true per-second rates — naming is aspirational per competitive standard.
- **Frontend disk chart labels** updated: Y-axis `"B/s"`, series `"Read B/s"` / `"Write B/s"`
- **Config ML keys** use bare metric names (`"connections.active"`) without prefix — no config changes needed
- **`models.ts` `diskstats` field** is a TypeScript struct field (matching Go JSON tag), NOT a metric key — left unchanged
- **Migration:** `012_metric_naming_standardization.sql` renames existing data in `metrics`, `alert_rules`, `ml_baseline_snapshots` tables
- **Do NOT reintroduce `pgpulse.*` prefix** for PG metrics or `os.diskstat.*` for disk metrics

### Competitive Research — KEY CONCLUSIONS (Do Not Revisit)

- **pgwatch v5:** Best reference for SQL-first collector extensibility
- **PMM v3:** Best reference for QAN investigation UX
- **pganalyze:** Best reference for query analysis chain depth and least-privilege security. Deterministic planner-aware analysis (not ML).
- **Datadog DBM:** Best reference for APM↔DB correlation and dot-notation naming
- **pg_profile:** AWR-style snapshot-and-diff reports. PGPulse should build equivalent "workload snapshot reports"
- **PGPulse's unique moat:** ML forecasting, single-binary simplicity, SQL-first agentless OS collection, three-mode OS config, forecast-threshold alerting

---

## What Was Just Completed

### MN_01 — Metric Naming Standardization (1 session, 1 commit: 2f96bed)

**43 files modified** (8 production Go, 24 test Go, 5 frontend, 6 design docs)

| Change | Scope |
|--------|-------|
| `metricPrefix = "pgpulse"` → `"pg"` in `base.go` | ~120 PG metric keys renamed |
| `os.diskstat.*` → `os.disk.*` in os_sql.go, os.go | 7 disk metric keys renamed |
| Disk values ×1024 (KB → bytes) | os_sql.go |
| Frontend metric key references | 5 .tsx files updated |
| Alert builtin rule metric keys | `rules.go` — 20 rules updated |
| API database handler metric keys | `databases.go` — ~30 references |
| Orchestrator telemetry keys | `db_runner.go` — `pg.agent.db.*` |
| Test assertions | 24 test files, 187 replacements |
| Data migration | `012_metric_naming_standardization.sql` — NEW |

**All builds pass:** Go build, go test (14 packages), golangci-lint, npm build, typecheck, lint.

**Grep audit:** Zero remaining `"pgpulse."` in production or test Go, zero in frontend, zero `os.diskstat` anywhere.

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

**NOTE:** After deploying MN_01, run the migration on the demo DB:
```bash
ssh ml4dbs@185.159.111.139 'psql -U pgpulse_monitor -d pgpulse_storage -f /opt/pgpulse/migrations/012_metric_naming_standardization.sql'
```

### Portable Mode

```
Windows:  pgpulse-server.exe --target=postgres://user:pass@host:5432/postgres
Linux:    pgpulse-server --target=postgres://user:pass@host:5432/postgres

Opens:    http://localhost:8989
Auth:     Auto-skipped on localhost
Storage:  In-memory, 2h retention (configurable via --history)
```

---

## Known Issues (Post MN_01)

| Issue | Status | Notes |
|-------|--------|-------|
| Disk `read_bytes_per_sec` is delta-per-interval, not true per-second | Cosmetic | Name follows competitive standard; value is usable for charting |
| `c.command_desc` SQL bug in cluster progress | Open | Pre-existing, PG16 |
| `002_timescaledb.sql` migration skip warning | Open | Pre-existing |
| Demo VM not yet updated with MN_01 build | Pending deploy | Migration script ready |

---

## Key Interfaces (Current)

```go
// internal/collector/base.go — metricPrefix changed in MN_01
const metricPrefix = "pg"

func (b *Base) point(name string) string {
    return metricPrefix + "." + name
}
// All PG collectors (connections, cache, replication, etc.) go through this.
// OSSQLCollector and ClusterCollector have their own point() — isolated.
```

```go
// internal/collector/collector.go — unchanged
type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

// internal/storage/memory.go
type MemoryStore struct { /* implements MetricStore */ }
func NewMemoryStore(retention time.Duration) *MemoryStore

// internal/auth/middleware.go
type AuthMode int
const ( AuthEnabled AuthMode = iota; AuthDisabled )

// internal/alert/forecast.go
type ForecastProvider interface {
    ForecastForAlert(ctx context.Context, instanceID, metricKey string, horizon int) ([]ForecastPoint, error)
}

// internal/api/server.go
func (s *APIServer) SetLiveMode(live bool, retention time.Duration)
func (s *APIServer) SetAuthMode(mode auth.AuthMode)
```

---

## Next Task: ML/DL Remediation

### Context (from Competitive Research)

pganalyze's deterministic advisor model (planner-aware, rule-based) outperforms
pure ML for actionable diagnostics. PGPulse's current ML stack (STL anomaly
detection + forecasting) is strong for "something is wrong" but weak for
"here's what to do about it."

### Goal

Add a rule-based remediation engine that maps detected anomalies and threshold
breaches to actionable recommendations. Rule-based approach first — ML-driven
root cause analysis is a later phase.

### Scope (to be refined in planning)

- Define a remediation rule schema (condition → recommendation)
- Seed with rules for common scenarios:
  - High connection utilization → "Increase max_connections or add pgbouncer"
  - Cache hit ratio drop → "Check shared_buffers sizing, look for seq scans"
  - Replication lag spike → "Check replica load, network, wal_sender limits"
  - Lock chain depth → "Investigate blocking queries, consider lock_timeout"
  - Forecast threshold crossing → "Proactive: metric projected to breach in N hours"
- Surface recommendations in the UI (alert detail view or dedicated panel)
- Integrate with existing alert events (recommendations attach to fired alerts)

### Agent Team Configuration (suggested)
- **API & Security Agent:** Rule schema, rule store, API endpoints
- **Frontend Agent:** Recommendation display in alert detail / server detail
- **QA Agent:** Rule evaluation tests, integration tests

---

## Roadmap: Updated Priorities

### Queue (locked order)

1. ~~Metric naming standardization~~ ✅ **MN_01 DONE**
2. **ML/DL remediation** (rule-based approach first) ← NEXT
3. **Prometheus exporter** (NEW-3) — metric naming finalized, ready to build
4. **Workload snapshot reports** (NEW-5) — pg_profile paradigm
5. **pg_settings history + configuration drift** (NEW-6)

### Milestone Status

| # | Milestone | Status |
|---|-----------|--------|
| ~~MW_01~~ | Windows executable + live mode | ✅ Done |
| ~~MW_01b~~ | Bugfixes (5 bugs) | ✅ Done |
| ~~MN_01~~ | Metric naming standardization | ✅ Done |
| NEW-2 | ML/DL remediation | 🔲 Next |
| NEW-3 | Prometheus exporter | 🔲 |
| NEW-4 | Desktop App (Wails) | Parked |
| NEW-5 | Workload snapshot reports | 🔲 |
| NEW-6 | pg_settings history + drift | 🔲 |

### Deferred UI Items (from M8 series)
- Session kill UI
- Settings diff UI
- Query plan viewer UI
- Forecast overlay on remaining charts

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

# Run MN_01 migration on demo DB (one-time)
ssh ml4dbs@185.159.111.139 'psql -U pgpulse_monitor -d pgpulse_storage -f /opt/pgpulse/migrations/012_metric_naming_standardization.sql'
```

---

## Project Knowledge Status

| Document | Status |
|----------|--------|
| PGPulse_Development_Strategy_v2.md | ✅ Current |
| PGAM_FEATURE_AUDIT.md | ✅ Current |
| Chat_Transition_Process.md | ✅ Current |
| Save_Point_System.md | ✅ Current |
| PGPulse_Competitive_Research_Synthesis.md | ✅ Current |
| CODEBASE_DIGEST.md | ⚠️ Re-upload after MN_01 (Section 3 metric keys completely changed) |
