# PGPulse — Iteration Handoff: M11 → M12

**Date:** 2026-03-16
**From:** M11 (Competitive Enrichment — complete)
**To:** M12 (Desktop App — Wails)

---

## DO NOT RE-DISCUSS

All items from M11_01 handoff remain in force, plus:

### PGSS Snapshot System — COMPLETE (M11_01 + M11_02)

**Backend (M11_01):**
- `internal/statements/` — 12 files (~1,960 lines): types, store, pgstore (CopyFrom bulk), nullstore, diff engine, insights, report, capture
- SnapshotCapturer: periodic capture (default 30m), version-gated SQL (PG ≤12 vs 13+), capture_on_startup, manual trigger
- ComputeDiff: per-query deltas, derived fields (avg_exec_time, io_pct, cpu_time, shared_hit_ratio), stats_reset detection, new/evicted categorization
- Migration 015: `pgss_snapshots` + `pgss_snapshot_entries` tables
- Config: `statement_snapshots.enabled/interval/retention_days/capture_on_startup/top_n`
- Guard: `cfg.StatementSnapshots.Enabled && persistentStore != nil`

**Frontend (M11_02):**
- Query Insights page (`/servers/:serverId/query-insights`) — sortable diff table, per-query drill-down with 4 mini ECharts, snapshot selector, Capture Now button, stats reset warning
- Workload Report page (`/servers/:serverId/workload-report`) — summary card, 7 collapsible sections (5 top-by + new + evicted), Export HTML button
- 10 new components in `web/src/components/snapshots/`
- 6 React Query hooks in `web/src/hooks/useSnapshots.ts`
- Sidebar links: BarChart3 "Query Insights", FileText "Workload Report"

**HTML Export (M11_02):**
- `GET /instances/{id}/workload-report/html` — standalone HTML with inline CSS
- `internal/api/handler_report_html.go` + `internal/api/templates/workload_report.html`
- Content-Disposition: attachment (download) unless `?inline=true`

### Bug Fixes (M11_01)
- Debug log removed from main.go
- `wastedibytes`: NullInt64 → NullFloat64 scan
- `pg.server.multixact_pct`: added to ServerInfoCollector
- `srsubstate`: `::text` cast in logical replication SQL

### 8 New API Endpoints (M11 total)

| Method | Path | Added In |
|--------|------|----------|
| GET | /instances/{id}/snapshots | M11_01 |
| GET | /instances/{id}/snapshots/{snapId} | M11_01 |
| GET | /instances/{id}/snapshots/diff | M11_01 |
| GET | /instances/{id}/snapshots/latest-diff | M11_01 |
| GET | /instances/{id}/query-insights/{queryid} | M11_01 |
| GET | /instances/{id}/workload-report | M11_01 |
| POST | /instances/{id}/snapshots/capture | M11_01 |
| GET | /instances/{id}/workload-report/html | M11_02 |

---

## What Was Just Completed

### M11_01 — Backend (1 session, 5m 36s)
- 16 new files, 6 modified (~3,044 lines)
- `internal/statements/` package, migration 015, 7 API endpoints, 4 bug fixes

### M11_02 — Frontend + HTML Export (1 session, 1m 33s)
- 12 new files, 3 modified (~1,500 lines)
- 2 new pages, 10 new components, 1 HTML export endpoint, sidebar nav

### M11 Totals
- **28 new files**, **9 modified files**
- **~4,544 lines added**
- **8 new API endpoints** (total now ~55)
- **2 new pages**, **10 new React components**
- **2 sessions**, **7 minutes 9 seconds** total execution time

---

## Demo Environment

```
Ubuntu 24.04 VM: 185.159.111.139

PGPulse UI:     http://185.159.111.139:8989     (persistent mode)
Login:          admin / pgpulse_admin
Config:         /opt/pgpulse/configs/pgpulse.yml

PostgreSQL 16.13:
  Primary:      localhost:5432
  Replica:      localhost:5433
  Chaos:        localhost:5434

Monitor user:   pgpulse_monitor / pgpulse_monitor_demo
Storage DB:     pgpulse_storage on port 5432

Chaos scripts:  /opt/pgpulse/chaos/*.sh (PGPASSWORD embedded)
```

---

## Known Issues (Post M11)

| Issue | Severity | Notes |
|-------|----------|-------|
| `c.command_desc` SQL bug in cluster progress | Pre-existing | PG16 compatibility |
| `pg_stat_statements` not in shared_preload_libraries (replica) | Expected | WARN logged, snapshot captures 2/3 instances |
| `pg_largeobject` permission denied | Expected | Monitor user lacks access; sub-collector skips |
| Query Insights empty until 2+ snapshots exist | Expected | First useful diff after ~60 minutes |

---

## Codebase Scale (Post M11)

- **Go files:** ~220 (~36,400 lines)
- **TypeScript files:** ~135 (~12,750 lines)
- **Metric keys:** ~169
- **API endpoints:** ~55
- **Collectors:** 25
- **Frontend pages:** 13
- **React components:** ~55

---

## Roadmap: Updated Priorities

### Queue (locked order)

1. ~~Alert & Advisor Polish~~ ✅ **M9 DONE**
2. ~~Advisor Auto-Population~~ ✅ **M10 DONE**
3. ~~Competitive Enrichment~~ ✅ **M11 DONE** (2 sub-iterations)
4. **Desktop App (Wails)** (M12) ← NEXT
5. **Prometheus Exporter** (M13)

### M12 — Desktop App: Scoping Notes

From handoff history and strategy doc:
- **Wails-based native window** — wraps the existing React frontend in a native OS window (like pgAdmin/DBeaver)
- **Distinct from Windows executable** (MW_01) which is a server binary + browser
- **Key features:** System tray icon, auto-start with connection dialog, native file dialogs for config, OS notifications for alerts
- **Build targets:** Windows (.exe + installer), macOS (.app), Linux (.AppImage or .deb)
- **The frontend is already embedded** via `go:embed` — Wails can reuse the same embed
- **Research needed:** Wails v2 vs v3, embedding existing chi router vs Wails' built-in asset server

### Milestone Status

| Milestone | Scope | Status |
|-----------|-------|--------|
| ~~MW_01~~ | Windows executable + live mode | ✅ Done |
| ~~MW_01b~~ | Bugfixes (5 bugs) | ✅ Done |
| ~~MN_01~~ | Metric naming standardization | ✅ Done |
| ~~REM_01~~ | Rule-based remediation (3 sub-iterations) | ✅ Done |
| ~~M9~~ | Alert & Advisor Polish | ✅ Done |
| ~~M10~~ | Advisor Auto-Population | ✅ Done |
| ~~M11~~ | Competitive Enrichment (2 sub-iterations) | ✅ Done |
| M12 | Desktop App (Wails packaging) | 🔲 Next |
| M13 | Prometheus Exporter | 🔲 |

---

## Build & Deploy

```bash
# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...

# Cross-compile (MINGW64 — use export, not set)
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

# Deploy to demo VM
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
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
| CODEBASE_DIGEST.md | ⚠️ Re-upload after M11_02 digest regeneration |
