# PGPulse — Iteration Handoff: MW_01b → Mordin: Old Blood

**Date:** 2026-03-13
**From:** MW_01b (Bugfixes) + Competitive Research Session
**To:** Metric Naming Standardization → Prometheus Exporter → Workload Snapshot Reports
**Chat name:** Mordin: Old Blood

---

## DO NOT RE-DISCUSS

All items from MW_01 handoff remain in force, plus:

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- Forecast polling in the frontend is 5 minutes
- `forecastUtils.ts` in `web/src/lib/` is canonical for `buildForecastSeries` and `getNowMarkLine`
- YAML seeds the database on startup; database becomes source of truth after first run
- `go:embed` bakes the React build into the Go binary — PGPulse is its own web server
- PGPulse listens on port 8989 (fixed in MW_01b — was 8080 in live mode)
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

### Metric Naming — DECIDED (Competitive Research Outcome)

- **Internal naming convention: dot-notation** — validated by Datadog's approach (`postgresql.connections`, `system.cpu.user`). PGPulse uses `pgpulse.{category}.{metric}` (e.g., `pgpulse.cache.hit_ratio`, `pgpulse.connections.active`, `os.cpu.user_percent`)
- **Prometheus export: mapping layer** — when Prometheus exporter is built, internal names are rewritten to Prometheus convention (`pgpulse_pg_connections{state="active"}`)
- **Do NOT change internal storage to Prometheus-style underscores** — dot-notation is more readable in PGPulse's built-in UI
- **Naming rules (§3.3 of competitive research):**
  1. Top-level prefix = layer: `pg.` or `pgpulse.` for PG metrics, `os.` for OS metrics, `pgpulse.` for internal/meta
  2. Second level = category: `connections`, `database`, `transactions`, `replication`, `bgwriter`, `statements`, `locks`, `tables`, `indexes`, `vacuum`, `wal`; OS: `cpu`, `memory`, `disk`, `network`, `load`
  3. Third level = specific metric: descriptive, snake_case within level
  4. Units in name when ambiguous: `_bytes`, `_percent`, `_per_sec`, `_seconds`, `_count`
  5. Labels/tags for dimensions: `{instance, database, table, state}`
  6. Counter vs. gauge: document explicitly per metric

### Competitive Research — KEY CONCLUSIONS (Do Not Revisit)

- **pgwatch v5:** Best reference for SQL-first collector extensibility. Flat snake_case naming (metric = table). No alerting/ML/UI.
- **PMM v3:** Best reference for QAN investigation UX. Heavy deployment. SUPERUSER recommended. No ML.
- **pganalyze:** Best reference for query analysis chain depth and least-privilege security. SaaS-only. Deterministic planner-aware analysis (not ML). Not SQL-extensible.
- **Datadog DBM:** Best reference for APM↔DB correlation and dot-notation naming. Per-host SaaS cost. No Index/VACUUM advisor.
- **pg_profile:** AWR-style snapshot-and-diff reports. Good diagnostic tool, NOT a monitoring platform. PGPulse should build equivalent "workload snapshot reports" feature.
- **pgpro_pwr:** Genuinely strong (per-plan statistics, plan instability detection), but locked to Postgres Pro. Cannot take anything directly since we lack `pgpro_stats`. Take the paradigm: snapshot-and-diff.
- **PGPulse's unique moat:** ML forecasting, single-binary simplicity, SQL-first agentless OS collection, three-mode OS config, forecast-threshold alerting. No competitor has any of these.
- **Full document:** `PGPulse_Competitive_Research_Synthesis.md` in Project Knowledge

---

## What Was Just Completed

### MW_01b — Bugfixes (5 bugs, 4 commits)

| Bug | Fix | Commit |
|-----|-----|--------|
| Bug 1: Port crash (empty log level) | `parseLogLevel("")` → Info without error | 3e3705f |
| Bug 5: Instance display name | Shows host:port/dbname from DSN | 3e3705f |
| Bug 3: Cache Hit Ratio no data | Collector key `cache.hit_ratio_pct` → `cache.hit_ratio` | a4f960a |
| Bug 4: Replication Lag "undefined" units | `formatBytes` handles NaN, negative, sub-1 values | 5321375 |
| Bug 2: build-release.sh Windows | PowerShell `Compress-Archive` fallback when `zip` unavailable | abc202a |

All builds pass: Go build, go vet, go test, golangci-lint, npm build, typecheck, lint.

### Competitive Research Session (same chat)

Produced `PGPulse_Competitive_Research_Synthesis.md`:
- 6 competitors × 6 dimensions (privilege model, deployment complexity, query analysis depth, stateful history, managed-service friendliness, extensibility model)
- Metric naming recommendation with proposed standard
- Feature gap analysis with roadmap implications
- PGPulse positioning statement

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

### Portable Mode

```
Windows:  pgpulse-server.exe --target=postgres://user:pass@host:5432/postgres
Linux:    pgpulse-server --target=postgres://user:pass@host:5432/postgres

Opens:    http://localhost:8989  (port fixed in MW_01b)
Auth:     Auto-skipped on localhost
Storage:  In-memory, 2h retention (configurable via --history)
```

---

## Known Issues (Post MW_01b)

| Issue | Status | Notes |
|-------|--------|-------|
| `os.diskstat.*` vs `os.disk.*` naming | **Fix in metric naming standardization** | First item for Mordin |
| Metric key prefix inconsistency (`pgpulse.` vs bare) | **Fix in metric naming standardization** | Some metrics have `pgpulse.` prefix, some don't |
| `c.command_desc` SQL bug in cluster progress | Open | Pre-existing, PG16 |
| `002_timescaledb.sql` migration skip warning | Open | Pre-existing |

---

## Key Interfaces (Current)

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
func NewAuthMiddleware(jwtSvc *JWTService, mode AuthMode, errorWriter ...) func(http.Handler) http.Handler

// internal/alert/nullstore.go
type NullAlertHistoryStore struct{}
func NewNullAlertHistoryStore() *NullAlertHistoryStore

// internal/api/server.go
func (s *APIServer) SetLiveMode(live bool, retention time.Duration)
func (s *APIServer) SetAuthMode(mode auth.AuthMode)
// GET /api/v1/system/mode — no auth required
```

---

## Next Task: Metric Naming Standardization

### Goal
Systematically rename all ~157 metric keys across the entire codebase to conform to the naming standard decided in the competitive research session (§3.3 of the synthesis document).

### Scope
1. **Audit all metric keys** — from CODEBASE_DIGEST Section 3 (Metric Keys). Generate a complete mapping table: `current_key → new_key`
2. **Apply renames in collectors** — every `MetricPoint.Metric` string in `internal/collector/*.go`
3. **Apply renames in frontend** — every metric key reference in `web/src/**/*.ts{x}`
4. **Apply renames in API** — any hardcoded metric keys in `internal/api/*.go`
5. **Apply renames in alert rules** — default alert rule metric references in `internal/alert/seed.go` and `rules.go`
6. **Apply renames in ML** — metric references in `internal/ml/*.go`
7. **Update CODEBASE_DIGEST** — regenerate Section 3
8. **Migration concern:** If TimescaleDB has historical data with old keys, consider a migration SQL script or a backward-compatible alias layer

### Known Renames Needed (from research + known issues)
| Current Key | Proposed Key | Reason |
|------------|-------------|--------|
| `pgpulse.cache.hit_ratio` | Review: keep or move to `pg.cache.hit_ratio` | Prefix consistency |
| `os.diskstat.read_kb` | `os.disk.read_bytes_per_sec` | Standard units + hierarchy |
| `os.diskstat.write_kb` | `os.disk.write_bytes_per_sec` | Standard units + hierarchy |
| Various `pgpulse.*` vs bare keys | Standardize prefix | Some metrics have `pgpulse.` prefix, some don't |

### Agent Team Configuration
- **Collector Agent:** Renames in `internal/collector/*.go`, `internal/agent/*.go`
- **API & Security Agent:** Renames in `internal/api/*.go`, `internal/alert/*.go`, `internal/ml/*.go`, migration script
- **Frontend Agent:** Renames in `web/src/**/*.ts{x}`
- **QA Agent:** Verify all renames are consistent, build passes, charts render correctly

### Critical Rule
**Every rename must be applied atomically across all layers (collector → storage → API → frontend → alerts → ML).** A partial rename breaks the dashboard.

### Build Verification
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

---

## Roadmap: Updated Priorities

### Queue (locked order)

1. **Metric naming standardization** ← NEXT (Mordin: Old Blood starts here)
2. **ML/DL remediation** (rule-based approach first) — informed by pganalyze's deterministic advisor model
3. **Prometheus exporter** (NEW-3) — requires naming standardization first
4. **Workload snapshot reports** (NEW, research-driven) — pg_profile paradigm, static HTML post-mortem reports
5. **pg_settings history + configuration drift** — informed by pg_profile comparison reports

### New Milestones (updated)

| # | Milestone | Priority | Dependencies |
|---|-----------|----------|-------------|
| ~~NEW-1~~ | ~~Windows executable~~ | ✅ **MW_01 DONE** | — |
| NEW-2 | ML/DL remediation | After metric naming | M8 ML, competitive research |
| NEW-3 | Prometheus exporter | After metric naming | Metric naming finalized |
| NEW-4 | Desktop App (Wails) | Parked | MW_01 backend, Wails v2 |
| NEW-5 | Workload snapshot reports | After Prometheus | pg_profile paradigm |
| NEW-6 | pg_settings history + drift | After snapshot reports | Config collector |

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

# ZIP (Windows — PowerShell fallback added in MW_01b)
powershell -Command "Compress-Archive -Path 'dist/pgpulse-server-windows-amd64.exe','config.sample.yaml','README.txt' -DestinationPath 'dist/pgpulse-dev-windows-amd64.zip' -Force"

# Deploy to demo VM (persistent mode)
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

---

## Project Knowledge Status

The following documents should be in Project Knowledge for the new chat:

| Document | Status |
|----------|--------|
| PGPulse_Development_Strategy_v2.md | ✅ Already there |
| PGAM_FEATURE_AUDIT.md | ✅ Already there |
| Chat_Transition_Process.md | ✅ Already there |
| Save_Point_System.md | ✅ Already there |
| CODEBASE_DIGEST.md | ⚠️ Re-upload after MW_01b (metric key changed: `cache.hit_ratio_pct` → `cache.hit_ratio`) |
| PGPulse_Competitive_Research_Synthesis.md | ⚠️ NEW — upload now |
