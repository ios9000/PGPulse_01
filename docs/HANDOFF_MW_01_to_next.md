# PGPulse — Iteration Handoff: MW_01 → Next

**Date:** 2026-03-12
**From:** MW_01 (Portable Windows Executable + Live Mode)
**To:** MW_01b bugfixes → Competitive Research → Metric Naming → M6 OS Agent

---

## DO NOT RE-DISCUSS

All items from M8_11 handoff remain in force, plus:

- `internal/alert` must NEVER import `internal/ml` — use `alert.ForecastProvider` interface
- `internal/mlerrors` is the canonical home for `ErrNotBootstrapped` and `ErrNoBaseline`
- Sustained crossing (N consecutive) is the only supported mode for forecast alerts
- Forecast polling in the frontend is 5 minutes
- `forecastUtils.ts` in `web/src/lib/` is canonical for `buildForecastSeries` and `getNowMarkLine`
- YAML seeds the database on startup; database becomes source of truth after first run
- `go:embed` bakes the React build into the Go binary — PGPulse is its own web server
- PGPulse listens on port 8989 on the demo VM (but see Known Issues — port defaults to 8080 in live mode)
- Test scope must be `./cmd/... ./internal/...` (not `./...`) to prevent scanning `web/node_modules/`
- OSSQLCollector reuses agent parsers from `internal/agent/` — no code duplication
- Per-instance `os_metrics_method` config: "sql" (default), "agent", "disabled"
- Metric naming standardization is parked for competitive research — do NOT rename keys until then
- `docs/CODEBASE_DIGEST.md` is auto-generated at end of each iteration — always re-upload to Project Knowledge
- **MemoryStore** implements `collector.MetricStore` — 2h default retention, configurable via `--history`
- **Live mode** auto-detected when `storage.dsn` is absent — ML/forecast/plan/settings disabled
- **Auth bypass** auto-enabled on localhost; `--no-auth` flag for remote bind
- **APIServer extensions** use setter pattern: `SetLiveMode()`, `SetAuthMode()` — do NOT add more params to `api.New()`
- **NullAlertHistoryStore** implements all 5 methods of `AlertHistoryStore` — used in live mode
- **Auth disabled** injects implicit `*Claims{Username:"admin", Role:"admin"}` via `claimsContextKey`
- **`/api/v1/system/mode`** returns `{"mode":"live","retention":"2h0m0s"}` or `{"mode":"persistent"}` — no auth required

---

## What Was Just Completed

### MW_01 — Portable Windows Executable + Live Mode

**Core: MemoryStore** — `internal/storage/memory.go` (~200 lines)
- In-memory `MetricStore` with `sync.RWMutex`, map-based storage, 30s expiry goroutine
- Binary search for timestamp cutoff, automatic eviction of old data
- 9 unit tests covering write/query/filter/expiry/concurrent access

**Core: CLI Flags** — `cmd/pgpulse-server/main.go`
- `--target=<DSN>` for single-instance quick start
- `--target-host/port/user/password/dbname` decomposed form
- `--listen=:PORT`, `--history=DURATION`, `--no-auth`, `--config=PATH`
- `synthesizeCLIInstance()` builds ephemeral instance from CLI flags
- Config merge: CLI flags > YAML > defaults
- No config file + `--target` → works with zero-config defaults

**Core: Live Mode Detection**
- No `storage.dsn` → MemoryStore, ML/plan/settings disabled
- `NullAlertHistoryStore` prevents nil panics on alert writes

**Core: Auth Bypass**
- `auth.AuthMode` enum: `AuthEnabled`, `AuthDisabled`
- `auth.NewAuthMiddleware()` — injects implicit admin Claims when disabled
- Auto-disabled on localhost; `--no-auth` for explicit opt-out
- Auth-disabled route group handles `/auth/me`, `/auth/login`, `/auth/refresh`

**Frontend**
- `useSystemMode.tsx` — React context + provider, fetches `/api/v1/system/mode`
- `TopBar.tsx` — animated "Live Mode" blue pill badge with tooltip
- `ServerDetail.tsx` — Plan/Settings tabs hidden in live mode
- `TimeSeriesChart.tsx` — forecast controls gated on persistent mode
- `authStore.ts` — probes `/auth/me` for auth-disabled detection

**Build & Packaging**
- `scripts/build-release.sh` — cross-compile for linux/amd64 + windows/amd64
- `config.sample.yaml` — annotated sample config
- `README.txt` — quick-start guide, CLI flags reference, upgrade path
- ZIPs: `pgpulse-dev-windows-amd64.zip` (6.7 MB), `pgpulse-dev-linux-amd64.zip` (6.6 MB)

---

## Demo Environment (Live)

```
Ubuntu 24.04 VM: 185.159.111.139

PGPulse UI:     http://185.159.111.139:8989     (persistent mode, existing deployment)
Login:          admin / pgpulse_admin

PostgreSQL 16.13:
  Primary:      localhost:5432
  Replica:      localhost:5433
  Chaos:        localhost:5434

Monitor user:   pgpulse_monitor / pgpulse_monitor_demo
Storage DB:     pgpulse_storage on port 5432

OS Metrics:     Flowing on all 3 instances via pg_read_file('/proc/*')
```

### Portable Mode (NEW)

```
Windows:  pgpulse-server.exe --target=postgres://user:pass@host:5432/postgres
Linux:    pgpulse-server --target=postgres://user:pass@host:5432/postgres

Opens:    http://localhost:8080  (NOTE: port defaults to 8080, not 8989 — see Known Issues)
Auth:     Auto-skipped on localhost
Storage:  In-memory, 2h retention (configurable via --history)
```

---

## Known Issues

| Issue | Status | Notes |
|-------|--------|-------|
| **Port defaults to 8080 in live mode** | **Fix in MW_01b** | Empty `config.Config{}` doesn't set Port=8989. Quick fix: set default after `cfg = config.Config{}` |
| **build-release.sh uses `zip`** | **Workaround** | `zip` not on Git Bash. Use `powershell Compress-Archive` on Windows. Fix: add OS detection + PowerShell fallback |
| **Instance name shows raw host from DSN** | Minor | `extractHostPort()` parses correctly, display only |
| `os.diskstat.*` vs `os.disk.*` naming | Parked | Fix in metric naming standardization |
| Cache Hit Ratio metric name mismatch | Open | Pre-existing |
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

// internal/storage/memory.go — NEW
type MemoryStore struct { /* implements MetricStore */ }
func NewMemoryStore(retention time.Duration) *MemoryStore

// internal/auth/middleware.go — NEW
type AuthMode int
const ( AuthEnabled AuthMode = iota; AuthDisabled )
func NewAuthMiddleware(jwtSvc *JWTService, mode AuthMode, errorWriter ...) func(http.Handler) http.Handler

// internal/alert/nullstore.go — NEW
type NullAlertHistoryStore struct{}
func NewNullAlertHistoryStore() *NullAlertHistoryStore
// Implements: Record, Resolve, ListUnresolved, Query, Cleanup

// internal/api/server.go — EXTENDED
func (s *APIServer) SetLiveMode(live bool, retention time.Duration)
func (s *APIServer) SetAuthMode(mode auth.AuthMode)
// GET /api/v1/system/mode — no auth required
```

---

## CLI Flags Reference

```
--target=<DSN>          PostgreSQL connection string (single instance quick start)
--target-host=<host>    PostgreSQL host (alternative to --target)
--target-port=<port>    PostgreSQL port (default: 5432)
--target-user=<user>    PostgreSQL user (default: pgpulse_monitor)
--target-password=<pw>  PostgreSQL password
--target-dbname=<db>    PostgreSQL database (default: postgres)
--listen=<addr>         HTTP listen address:port (default: :8080 — BUG, should be :8989)
--history=<duration>    In-memory retention (default: 2h)
--no-auth               Disable authentication
--config=<path>         Config file path (default: config.yaml)
```

---

## Roadmap: Updated Priorities

### Immediate: MW_01b Bugfixes (tiny, no planning docs needed)
1. Fix default port: 8080 → 8989
2. Fix build-release.sh: add PowerShell fallback for Windows
3. Optional: improve instance display name from DSN

### Queue

1. **Competitive research** — PMM, Datadog, Zabbix, pgCenter, pg_profile, pganalyze
   - Split into focused pieces (metric naming, agent architecture, feature gap)
   - Reference for naming: SQL Server perf counters pattern (Object → Counter → Preferred Value)

2. **Metric naming standardization** — define standard based on research, apply in dedicated iteration

3. **M6 OS Agent** — full agent binary, informed by competitive research

4. **Deferred UI work** — session kill UI, settings diff UI, query plan viewer UI, forecast overlay on remaining charts

### New Milestones (from M8_11 handoff, priority confirmed)

| # | Milestone | Priority | Dependencies |
|---|-----------|----------|-------------|
| ~~NEW-1~~ | ~~Windows executable~~ | ✅ **MW_01 DONE** | — |
| NEW-2 | ML/DL remediation | After competitive research | M8 ML, competitive research |
| NEW-3 | Prometheus exporter | Needs research | Metric naming finalized |

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

# ZIP (Windows — no zip command, use PowerShell)
powershell -Command "Compress-Archive -Path 'dist/pgpulse-server-windows-amd64.exe','config.sample.yaml','README.txt' -DestinationPath 'dist/pgpulse-dev-windows-amd64.zip' -Force"

# Deploy to demo VM (persistent mode)
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```
