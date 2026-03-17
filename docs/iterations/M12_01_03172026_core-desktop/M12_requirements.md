# PGPulse M12 — Desktop App (Wails) — Requirements

**Date:** 2026-03-17
**Iteration:** M12 (2 sub-iterations: M12_01, M12_02)
**Depends on:** M11 (complete), MW_01 (Windows executable — reference)

---

## 1. Goal

Package PGPulse as a native Windows desktop application using Wails v3, providing a native window, system tray presence, connection dialog, and OS-level alert notifications — while preserving the existing headless server mode as the default.

The desktop mode is activated via `--mode=desktop` flag (build-tag gated with `//go:build desktop`) so the standard server binary remains unchanged at `CGO_ENABLED=0`.

---

## 2. Locked Decisions

| ID | Decision | Choice | Rationale |
|----|----------|--------|-----------|
| D300 | Wails version | **v3 (alpha)** | System tray, multi-window, `AssetOptions.Handler` maps directly to chi router; alpha risk acceptable for internal enterprise tool |
| D301 | Integration depth | **Thin wrapper** | Chi router plugs into Wails `AssetOptions.Handler`; all 55 REST endpoints + React frontend work unchanged; no binding rewrites |
| D302 | Platform priority | **Windows-only** | DBA colleague is Windows; macOS/Linux desktop deferred to M12_03 |
| D303 | Binary strategy | **Single binary + build tag** | `go build -tags desktop` → desktop-enabled; plain `go build` → headless server (unchanged). `--mode=desktop\|server` flag selects runtime behavior |
| D304 | Feature scope | **All 4**: native window, tray, connection dialog, OS notifications, NSIS installer | Ship a complete desktop experience |
| D305 | Sub-iterations | **2**: M12_01 (core desktop) → M12_02 (UX + installer) | |

---

## 3. Architecture Overview

### 3.1 Build-Tag Isolation

```
//go:build desktop    → includes Wails, tray, notifications, --mode flag
//go:build !desktop   → stubs (no-ops), zero dependencies on Wails
```

**Plain server build (existing, unchanged):**
```bash
CGO_ENABLED=0 go build ./cmd/pgpulse-server
```

**Desktop build (Windows — no CGO needed, WebView2 uses pure Go loader):**
```bash
go build -tags desktop ./cmd/pgpulse-server
```

### 3.2 Runtime Modes

| Flag | Behavior |
|------|----------|
| `--mode=server` (default) | Existing headless HTTP server on `--listen` address. No GUI. |
| `--mode=desktop` | Wails native window. Chi router serves via `AssetOptions.Handler` (no network port). System tray. OS notifications. |

If `--mode=desktop` is requested but binary was built without `desktop` tag, print error and exit.

### 3.3 Chi Router Integration

Wails v3's `AssetOptions` accepts an `http.Handler`. PGPulse's existing chi router (`internal/api.Router`) is passed directly:

```go
// Pseudo-code
app := application.New(application.Options{
    Name: "PGPulse",
    Services: []application.Service{
        application.NewService(notificationService),
    },
})

window := app.Window.New(application.WebviewWindowOptions{
    Title:  "PGPulse",
    Width:  1440,
    Height: 900,
    Assets: application.AssetOptions{
        Handler: chiRouter,   // existing chi router, zero changes
        FS:      webDistFS,   // existing go:embed FS for static assets
    },
})
```

**Key property:** The React frontend communicates with the Go backend via the same REST API calls. In server mode, these go over HTTP. In desktop mode, they go through Wails' in-memory IPC via the asset handler — **zero frontend changes required.**

### 3.4 File Structure

```
cmd/pgpulse-server/
├── main.go              // existing — unchanged
├── desktop.go           // //go:build desktop — Wails init, --mode registration
├── desktop_stub.go      // //go:build !desktop — no-op stubs

internal/desktop/        // NEW package, all files //go:build desktop
├── app.go               // Wails application setup, window creation, chi integration
├── tray.go              // System tray icon, menu (Show/Hide, Status, Quit)
├── dialog.go            // Connection dialog — first-launch instance selector
├── notifications.go     // Bridge: alert engine events → Wails NotificationService
├── icon.go              // Embedded icon assets (PNG for tray, ICO for window)
```

---

## 4. Feature Requirements

### 4.1 Native Window (M12_01)

| Req | Description |
|-----|-------------|
| W-01 | Wails v3 native window with WebView2, title "PGPulse — {instance_name}" |
| W-02 | Window size 1440×900, resizable, remembers position/size (Wails built-in) |
| W-03 | Application icon (PGPulse logo) in title bar and taskbar |
| W-04 | Standard window chrome (minimize, maximize, close) |
| W-05 | Close button minimizes to tray instead of exiting (configurable) |
| W-06 | Ctrl+Shift+D opens WebView2 DevTools (debug builds only) |

### 4.2 System Tray (M12_01)

| Req | Description |
|-----|-------------|
| T-01 | System tray icon (PGPulse logo, 64×64 PNG) |
| T-02 | Left-click toggles window show/hide |
| T-03 | Right-click context menu: "Show PGPulse", separator, "Status: Monitoring N instances", separator, "Quit" |
| T-04 | Tray icon color indicates health: green (all OK), yellow (warnings), red (critical alerts active) |
| T-05 | Tooltip on hover: "PGPulse — N instances, M active alerts" |

### 4.3 Connection Dialog (M12_02)

| Req | Description |
|-----|-------------|
| C-01 | On first launch (no config file), show native dialog before main window |
| C-02 | Dialog offers: "Open config file" (native file picker, .yml), "Use existing config" (if default path exists), "Quick connect" (DSN input field) |
| C-03 | Quick connect mode: DSN → in-memory ring buffer (like MW_01 live mode), no persistent storage required |
| C-04 | Config file mode: parse pgpulse.yml, start full persistent mode |
| C-05 | Remember last-used config path for subsequent launches |

### 4.4 OS Notifications for Alerts (M12_02)

| Req | Description |
|-----|-------------|
| N-01 | Use Wails v3 `NotificationService` for Windows Toast notifications |
| N-02 | Bridge between PGPulse alert engine and notification service: when alert fires → OS toast |
| N-03 | Toast shows: alert severity icon, instance name, alert rule name, metric value |
| N-04 | Click on toast brings PGPulse window to foreground, navigates to alert detail |
| N-05 | Configurable: enable/disable OS notifications per severity (INFO, WARNING, CRITICAL) |
| N-06 | Rate limiting: max 1 notification per alert rule per 5 minutes (prevent toast spam) |

### 4.5 NSIS Installer (M12_02)

| Req | Description |
|-----|-------------|
| I-01 | NSIS installer for Windows (.exe setup) |
| I-02 | Install to `C:\Program Files\PGPulse\` |
| I-03 | Desktop shortcut + Start Menu entry |
| I-04 | Optional: add to Windows startup (auto-launch on login) |
| I-05 | Uninstaller in Add/Remove Programs |
| I-06 | Include sample `pgpulse.yml` in install directory |
| I-07 | Installer size target: < 25 MB (current binary is ~30 MB, UPX can compress) |

---

## 5. Non-Requirements (Explicitly Out of Scope)

| Item | Reason |
|------|--------|
| macOS / Linux desktop builds | Deferred to M12_03 — need CGO + platform-specific testing |
| Wails Go→JS bindings | D301 decided thin wrapper — chi router as Handler |
| Auto-updater | Future iteration — requires update server infrastructure |
| Multiple windows | Not needed — single main window + tray sufficient |
| Frontend code changes | Zero — REST API works identically through Wails asset handler |
| Deep Wails binding generation | Not using bindings — no `wails3 generate bindings` needed |

---

## 6. Dependencies & Tooling

| Tool | Version | Purpose |
|------|---------|---------|
| Wails v3 | alpha.70+ | Desktop framework |
| go-task | latest | Wails v3 build system |
| NSIS | 3.x | Windows installer creation |
| WebView2 Runtime | (pre-installed on Win 10/11) | Rendering engine |

**Windows-specific:** No CGO required. Wails v3 uses a pure Go WebView2 loader on Windows.

**Install on dev machine:**
```bash
# Wails CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# go-task
go install github.com/go-task/task/v3/cmd/task@latest

# NSIS (download from https://nsis.sourceforge.io/ — add to PATH)
```

---

## 7. Sub-Iteration Breakdown

### M12_01 — Core Desktop (Target: 1 session)

**Scope:** Wails scaffold, build tags, chi→Wails integration, native window, system tray, `--mode` flag, Windows build verification.

**New files:** ~8 (desktop.go, desktop_stub.go, internal/desktop/*.go, icon assets)
**Modified files:** ~3 (main.go flag registration, go.mod for Wails dep, Makefile/build scripts)
**Estimated lines:** ~600–800

**Agent team:** 2 agents (Backend/Desktop + QA)
- Backend agent: Wails scaffold, build tags, chi integration, window, tray
- QA agent: build verification (both `go build` and `go build -tags desktop`), test stubs, lint

**Acceptance criteria:**
1. `go build ./cmd/pgpulse-server` → existing server binary, no Wails dependency, unchanged behavior
2. `go build -tags desktop ./cmd/pgpulse-server` → desktop binary with Wails
3. `pgpulse-server --mode=desktop --config=pgpulse.yml` → native window opens, full UI loads, all pages work
4. System tray icon visible, left-click toggles window, right-click shows menu
5. Close button minimizes to tray
6. `golangci-lint run ./cmd/... ./internal/...` passes
7. `go test ./cmd/... ./internal/... -count=1` passes

### M12_02 — UX + Installer (Target: 1 session)

**Scope:** Connection dialog, OS alert notifications, NSIS installer, UX polish.

**New files:** ~4 (dialog.go, notifications.go, NSIS config, notification bridge)
**Modified files:** ~3 (alert engine hook, desktop app.go, tray.go for status colors)
**Estimated lines:** ~500–700

**Agent team:** 2 agents (Desktop/UX + Installer/QA)
- Desktop agent: connection dialog, notification bridge, tray status colors
- Installer agent: NSIS config, build scripts, installer testing

**Acceptance criteria:**
1. First launch without config → connection dialog appears
2. File picker selects .yml → app starts in persistent mode
3. Quick connect with DSN → app starts in live mode
4. Alert fires → Windows Toast notification appears
5. Click notification → window foregrounds to alert detail
6. NSIS installer installs to Program Files, creates shortcuts
7. Uninstaller removes cleanly

---

## 8. Risk Register

| Risk | Impact | Mitigation |
|------|--------|------------|
| Wails v3 alpha instability | Build breaks on update | Pin exact alpha version in go.mod |
| NSIS path separator bug on Windows (#4667) | Installer build fails | Known workaround: normalize paths in Taskfile |
| WebView2 not installed (rare on Win 10/11) | App won't launch | Installer bundles WebView2 bootstrapper or checks + prompts |
| Wails NotificationService bugs (#4449) | Toast notifications fail | Fallback: in-app notification only, skip OS toast |
| Build tag leaks Wails into server binary | Binary size bloat, unwanted dep | CI gate: verify `go build` (no tags) produces binary without Wails symbols |

---

## 9. Updated Roadmap

| Milestone | Scope | Status |
|-----------|-------|--------|
| ~~M11~~ | Competitive Enrichment | ✅ Done |
| **M12** | **Desktop App (Wails) — Windows** | **🔲 Next** |
| M12_03 | Desktop App — macOS/Linux | 🔲 |
| M14 | RCA Engine (management request) | 🔲 |
| M15 | Maintenance Op Forecasting (management request) | 🔲 |
| M13 | Prometheus Exporter | 🔲 |
