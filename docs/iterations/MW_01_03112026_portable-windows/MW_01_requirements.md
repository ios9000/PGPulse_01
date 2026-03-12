# MW_01 — Portable Windows Executable + Live Mode

**Iteration:** MW_01
**Date:** 2026-03-11
**Milestone:** MW (Windows / Portable Mode)
**Type:** Combined (backend + frontend + packaging)

---

## 1. Goal

Ship a self-contained, zero-dependency PGPulse executable that a DBA can download as a ZIP, unzip on a Windows workstation, point at a PostgreSQL instance via a single CLI flag, and immediately see live monitoring dashboards in a browser — with no external database, no installation, and no login screen.

**Product story:** PGPulse starts as a portable diagnostic scanner ("where does it hurt?") and transforms into a full monitoring platform when the DBA adds persistent storage.

---

## 2. User Scenario

```
1. DBA downloads pgpulse-windows-amd64.zip (or pgpulse-linux-amd64.zip)
2. Unzips to any folder
3. Opens terminal, runs:
      pgpulse.exe --target=postgres://pgpulse_monitor:pass@10.0.0.5:5432/postgres
4. Opens http://localhost:8989 — no login required
5. Sees dashboards filling up in real time (charts accumulate for up to 2 hours)
6. Diagnoses the issue, closes PGPulse — data evaporates, no cleanup
7. Later, wants persistent monitoring → adds storage DSN to config.yaml
      → PGPulse upgrades to full TimescaleDB mode with ML/forecasting
```

---

## 3. Requirements

### R1: In-Memory Storage (MemoryStore)

- New `MetricStore` implementation using an in-memory ring buffer
- Holds up to 2 hours of metric data by default (configurable via `--history` flag)
- Implements all methods of the existing `MetricStore` interface: `Write()`, `Query()`, `Close()`
- Thread-safe for concurrent reads and writes from multiple collectors
- Automatic expiry of data older than the configured retention window
- Memory footprint: reasonable for typical workloads (~30 metric keys × 3 instances × 10s intervals × 2h ≈ 60-70 MB)

### R2: CLI Flag Parsing

- `--target=<DSN>` — single PostgreSQL connection string, synthesizes an ephemeral instance config
- `--target-host`, `--target-port`, `--target-user`, `--target-password`, `--target-dbname` — decomposed form for when DSN is awkward
- `--listen=<addr>` — web UI bind address (default `:8989`)
- `--history=<duration>` — in-memory buffer duration (default `2h`)
- `--no-auth` — disable authentication regardless of bind address
- `--config=<path>` — path to config YAML (default `config.yaml`, optional — PGPulse runs without it)
- Precedence: CLI flags > YAML config > built-in defaults
- `--target` and `--target-*` flags are mutually exclusive with YAML instance definitions — if CLI target is provided, YAML instances are ignored
- If neither `--target` nor YAML instances are configured, PGPulse exits with a clear error message

### R3: Storage Auto-Detection

- If no `storage.dsn` is configured (neither in YAML nor via any flag), PGPulse automatically creates a MemoryStore instead of connecting to TimescaleDB
- No error, no warning — this is the expected portable mode behavior
- Startup log clearly indicates: `mode=live storage=memory retention=2h`
- If `storage.dsn` IS configured, existing TimescaleDB behavior is unchanged

### R4: Auth Bypass

- When PGPulse binds to `127.0.0.1` or `localhost` only → authentication is automatically skipped
- When `--no-auth` flag is set → authentication is skipped regardless of bind address
- When auth is skipped, all API requests are treated as an implicit admin user
- When PGPulse binds to `0.0.0.0` or any non-localhost address WITHOUT `--no-auth` → normal JWT auth is required (existing behavior)
- Startup log clearly indicates: `auth=disabled reason=localhost` or `auth=disabled reason=no-auth-flag` or `auth=enabled`

### R5: System Mode API

- `GET /api/v1/system/mode` returns `{"mode": "live", "retention": "2h"}` or `{"mode": "persistent"}`
- No authentication required for this endpoint (it's informational)
- Frontend uses this to adjust UI behavior

### R6: Frontend Adjustments

- "Live Mode" pill/badge in the main header when `mode=live` (subtle, not alarming — e.g., a small blue pill with a pulse dot)
- ML/forecast controls and forecast chart overlays hidden when `mode=live`
- Charts handle shallow history gracefully (no errors when < 1 hour of data exists)
- Tooltip or info popover on the Live Mode badge: "Metrics are stored in memory for {retention}. Add a storage database for persistent monitoring."

### R7: Build & Packaging

- Cross-compile script producing:
  - `pgpulse-linux-amd64` (current production target)
  - `pgpulse-windows-amd64.exe` (new)
- ZIP packaging for each platform containing:
  - The binary
  - `config.sample.yaml` — annotated example covering portable and persistent modes
  - `README.txt` — quick-start guide, CLI flags reference, upgrade path to persistent mode
- Build uses `CGO_ENABLED=0` and `-ldflags="-s -w"` (already the case)

---

## 4. Out of Scope

- OS metrics on Windows targets (graceful degradation already works via OSSQLCollector per-file fallback)
- macOS / ARM builds (trivial to add later)
- Installer / MSI / setup wizard
- Metric key renaming (waiting for competitive research)
- Startup diagnostic for pg_read_file permission errors
- Changes to existing collectors, ML engine, or alert rules
- Any new dashboard widgets or metric charts
- SQLite or other embedded database options

---

## 5. Success Criteria

1. `pgpulse.exe --target=postgres://pgpulse_monitor:pgpulse_monitor_demo@185.159.111.139:5432/postgres` launches and serves the UI on `localhost:8989` without requiring a storage database
2. Charts populate with real-time data within 30 seconds of launch
3. No login screen when accessing via localhost
4. ML/forecast UI elements are hidden
5. A "Live Mode" indicator is visible in the header
6. After 2+ hours, oldest data points are automatically evicted (no OOM)
7. Same binary, same config.yaml with `storage.dsn` added → full persistent mode with all features
8. `go build`, `go test`, `npm run build`, `npm run typecheck`, `golangci-lint run` all pass
9. ZIP packages produced for linux-amd64 and windows-amd64

---

## 6. Dependencies on Existing Code

- `MetricStore` interface in `internal/storage/` — MemoryStore implements this
- `internal/config/config.go` — extended with CLI flag support
- `cmd/pgpulse-server/main.go` — CLI flag parsing + startup logic changes
- `internal/api/` — auth middleware modifications, new `/system/mode` endpoint
- `web/src/` — frontend header component, ML/forecast visibility logic
