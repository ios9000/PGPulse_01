# PGPulse — Iteration Handoff: M12 → M14

**Date:** 2026-03-17
**From:** M12 (Desktop App — Wails v3, complete: 2 sub-iterations)
**To:** M14 (RCA Engine)

---

## DO NOT RE-DISCUSS

All items from M11 handoff remain in force, plus:

### Desktop App — COMPLETE (M12_01 + M12_02)

**M12_01 — Core Desktop Shell:**
- Wails v3 alpha.74 integrated via `//go:build desktop` tag gating
- Native window (1440×900) with chi router as `AssetOptions.Handler`
- System tray: green/yellow/red severity icons, left-click toggle, right-click menu
- Close→hide (minimize to tray instead of quit)
- `--mode=desktop|server` flag; `desktop_stub.go` for non-desktop builds
- Standard `go build` produces Wails-free binary (verified via symbol check)
- Go bumped to 1.25 (Wails v3 requirement)
- Placeholder icons generated via `cmd/icongen/main.go`

**M12_02 — UX + Installer:**
- Connection dialog: separate Wails app lifecycle, shows file picker / DSN input / last-used config
- `%APPDATA%/PGPulse/settings.json` persists last config path
- Quick-connect mode: generates temp config → live mode (MemoryStore)
- Alert→notification bridge: `AlertNotifier` rate-limits (5 min/rule) and sends Windows Toast
- `OnAlert` hook on `AlertDispatcher` (additive, non-breaking, existing tests pass)
- Tray status goroutine (10s polling, placeholder values — real wiring deferred)
- NSIS installer script: `deploy/nsis/pgpulse.nsi` (MUI2, shortcuts, auto-start option, uninstaller)

### Key Architecture Decisions (M12)

| Decision | Value |
|----------|-------|
| D300 | Wails v3 alpha.74 |
| D301 | Thin wrapper — chi as AssetOptions.Handler, REST unchanged |
| D302 | Windows-only (macOS/Linux deferred) |
| D303 | Single codebase, build-tag gated (`//go:build desktop`) |
| Integration | `OnAlertHook func(fn func(alert.AlertEvent))` in Options — clean inversion, no direct dispatcher import |
| Dialog | Separate Wails app lifecycle → result channel → main app starts |
| Notifications | Wails NotificationService → go-toast/v2 on Windows |
| Settings | `%APPDATA%/PGPulse/settings.json` |

### Files Added in M12 (Total)

| Package | Files | Lines |
|---------|-------|-------|
| `internal/desktop/` | 13 (app, tray, icon, stub, dialog, dialog_bindings, dialog.html, settings, notifications, icons, tests) | ~750 |
| `cmd/pgpulse-server/` | 2 (desktop.go, desktop_stub.go) | ~120 |
| `cmd/icongen/` | 1 | 153 |
| `deploy/nsis/` | 3 (pgpulse.nsi, license.txt, example config) | ~190 |
| **Total** | **19 new files** | **~1,210 lines** |

### Modified in M12

| File | Change |
|------|--------|
| `cmd/pgpulse-server/main.go` | +31 lines: mode flag, pre-config dialog intercept, alert hook wiring |
| `internal/alert/dispatcher.go` | +20 lines: OnAlert hook mechanism |
| `go.mod` | Wails v3 alpha.74 + go-toast/v2, Go 1.25 |

---

## What Was Just Completed

### M12_01 — Core Desktop (1 session, ~20 min)
- 13 new files, 3 modified, ~565 lines

### M12_02 — UX + Installer (1 session, ~10 min)
- 8 new files, 7 modified, ~891 lines

### M12 Totals
- **19 new files**, **10 modified files** (some overlap)
- **~1,456 lines added**
- **2 sessions**, **~30 minutes** total agent execution

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
```

Note: Demo VM runs server mode (Linux headless). Desktop mode is Windows-only.

---

## Known Issues (Post M12)

| Issue | Severity | Notes |
|-------|----------|-------|
| Tray status goroutine uses placeholder values | Low | `UpdateStatus(0, 0, "ok")` — real orchestrator/alert wiring deferred |
| Dialog + notifications not runtime-tested | Medium | Compile-verified; requires interactive Windows session with Wails runtime |
| NSIS installer not built (script only) | Low | `makensis deploy/nsis/pgpulse.nsi` — requires pgpulse-desktop.exe in deploy/nsis/ |
| Icons are placeholder circles | Low | Replace with real PGPulse logo when available |
| `Options.WebFS` field unused in DesktopApp | Info | Chi router handles all static serving |
| `c.command_desc` SQL bug in cluster progress | Pre-existing | PG16 compatibility |
| Pre-existing uncommitted M11 web changes | Info | `server.go`, `App.tsx`, `Sidebar.tsx`, `index.css`, `models.ts` — from M11_02 sidebar nav additions |

---

## Codebase Scale (Post M12)

- **Go files:** ~235 (~38,000 lines)
- **TypeScript files:** ~135 (~12,750 lines)
- **Metric keys:** ~220
- **API endpoints:** ~65
- **Collectors:** 27
- **Frontend pages:** 13
- **React components:** ~55
- **Desktop package:** 13 files (~750 lines)

---

## Roadmap: Updated

### Completed

| Milestone | Scope | Status |
|-----------|-------|--------|
| M1–M8 | Core platform (collectors → ML) | ✅ Done |
| MN_01 | Metric naming standardization | ✅ Done |
| MW_01 | Windows executable + live mode | ✅ Done |
| M9 | Alert rules fix + navigation | ✅ Done |
| M10 | Advisor auto-population | ✅ Done |
| M11 | Competitive enrichment (PGSS, Query Insights, Workload Report) | ✅ Done |
| M_UX_01 | Alert detail panel, metric descriptions, UX polish | ✅ Done |
| **M12** | **Desktop App — Wails v3 (window, tray, dialog, notifications, NSIS)** | **✅ Done** |

### Queue (locked order)

1. **M14 — RCA Engine** ← NEXT (management request)
2. **M15 — Maintenance Op Forecasting** (management request)
3. **M13 — Prometheus Exporter**
4. M12_03 — Desktop: macOS/Linux builds (deferred)

### M14 — RCA Engine: Initial Notes

Management asked for this after the demo — the advisor feature showed that PGPulse can identify issues, and the natural next step is "why is this happening?" RCA correlates metric anomalies across layers (DB metrics + OS metrics + alert timeline) to identify probable root causes.

From competitive research:
- Datadog's APM↔DBM correlation is the gold standard for cross-layer RCA, but requires full SaaS ecosystem
- PGPulse's approach: correlate DB metric anomalies with OS metric anomalies on a shared timeline, without requiring application instrumentation
- ML detector already tracks anomalies per-metric; RCA needs to correlate ACROSS metrics

Key building blocks already in place:
- ML anomaly detection (M8) with per-metric baselines
- Alert engine with rule evaluation
- Background advisor with remediation rules
- OS metrics collection (SQL + agent modes)
- Metric store with time-range queries

---

## Build & Deploy

```bash
# Standard server build (unchanged)
go build ./cmd/pgpulse-server

# Desktop build (Windows, console)
go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server

# Desktop build (Windows, GUI subsystem — for installer)
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# NSIS installer (from deploy/nsis/, requires pgpulse-desktop.exe present)
makensis deploy/nsis/pgpulse.nsi

# Full verification
cd web && npm run build && npm run lint && npm run typecheck && cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/... -count=1 && golangci-lint run ./cmd/... ./internal/...

# Cross-compile server for Linux (demo VM)
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
| CODEBASE_DIGEST.md | ⚠️ Re-upload after M12_02 digest regeneration |
