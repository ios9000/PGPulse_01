# PGPulse — Iteration Handoff: M12_01 → M12_02

**Date:** 2026-03-17
**From:** M12_01 (Core Desktop — complete)
**To:** M12_02 (UX + Installer)

---

## DO NOT RE-DISCUSS

| Decision | Status |
|----------|--------|
| D300: Wails v3 (alpha) | Locked — `v3.0.0-alpha.74` pinned in go.mod |
| D301: Thin wrapper — chi as AssetOptions.Handler | Locked — implemented, working |
| D302: Windows-only | Locked |
| D303: Build-tag gating (`//go:build desktop` / `!desktop`) | Locked — verified: standard binary has zero Wails symbols |
| D305: Two sub-iterations (M12_01 core → M12_02 UX+installer) | Locked |
| Go 1.25: required by Wails v3 alpha.74 | Locked — go.mod updated, Go 1.25 installed |
| Icons in `internal/desktop/icons/` (not `assets/icons/`) | Decided in M12_01 — Go embed cannot use `..` paths |
| Flag registration via `init()` in desktop.go | Decided in M12_01 — cleaner than explicit call in main.go |
| `RunDesktop(router, assets)` — no config param | Decided in M12_01 — config not needed by DesktopApp |
| `AssetOptions.Handler` only (no `.FS`) | Decided in M12_01 — chi router already serves static files |
| Zero frontend changes | Verified — `web/src/` untouched |

---

## What Was Just Completed

### M12_01 — Core Desktop (1 session, ~20 min agent time)

**16 files changed** (13 new, 3 modified), **565 lines** added.

Built a Wails v3 native Windows desktop shell for PGPulse:

1. **Build-tag gated desktop mode** — `go build -tags desktop` produces desktop binary; plain `go build` is unchanged
2. **Native window** — 1440x900, min 1024x700, title "PGPulse", loads at URL `/`
3. **System tray** — green/yellow/red icon based on severity, left-click toggles window, right-click context menu (Show / Status / Quit)
4. **Close→hide** — window close minimizes to tray instead of quitting
5. **`--mode` flag** — `--mode=desktop` activates Wails; `--mode=server` (default) runs headless HTTP server
6. **Placeholder icons** — generated via `cmd/icongen/main.go` (64x64 colored circles + 32x32 ICO)

### Key Integration Pattern

```go
// In startServer(), after apiServer.Routes():
chiRouter := apiServer.Routes()

if GetDesktopMode() == "desktop" {
    distFS, _ := fs.Sub(web.DistFS, "dist")
    if err := RunDesktop(chiRouter, distFS); err != nil {
        logger.Error("desktop mode failed", "error", err)
        os.Exit(1)
    }
    return
}

// ... existing HTTP server code unchanged ...
```

The chi router (API + SPA fallback) passes directly to Wails `AssetOptions.Handler`. The React frontend makes the same REST API calls — routed through Wails' in-memory IPC instead of HTTP.

---

## Codebase State

### New Package: `internal/desktop/`

| File | Lines | Purpose |
|------|-------|---------|
| `app.go` | 70 | `DesktopApp` struct, Wails v3 app + window creation, close→hide |
| `tray.go` | 68 | `SystemTray`, context menu, `UpdateStatus()` icon swap |
| `icon.go` | 17 | `//go:embed` for 4 icon assets |
| `stub.go` | 3 | Empty package declaration for `!desktop` builds |
| `app_test.go` | 14 | Compile-check stub (GUI requires runtime) |
| `tray_test.go` | 17 | Compile-check stub (GUI requires runtime) |
| `icons/` | 4 files | PNG tray icons (green/yellow/red) + ICO window icon |

### New Files in `cmd/pgpulse-server/`

| File | Lines | Purpose |
|------|-------|---------|
| `desktop.go` | 34 | `//go:build desktop` — `init()` registers `--mode` flag, `GetDesktopMode()`, `RunDesktop()` |
| `desktop_stub.go` | 17 | `//go:build !desktop` — no-op stubs |

### Modified Files

| File | Change |
|------|--------|
| `cmd/pgpulse-server/main.go` | +15 lines: `io/fs` + `web` imports, desktop mode check in `startServer()` |
| `go.mod` | `wailsapp/wails/v3 v3.0.0-alpha.74`, Go 1.24→1.25 |

### Wails v3 Alpha.74 API (Actual, Not Design Doc)

The design doc showed older API patterns. Alpha.74 uses:

| Design Doc | Actual Alpha.74 |
|------------|-----------------|
| `app.NewWebviewWindow(opts)` | `app.Window.NewWithOptions(opts)` |
| Direct system tray constructor | `app.SystemTray.New()` |
| `window.On(event, handler)` | `window.OnWindowEvent(events.Common.WindowClosing, handler)` with `event.Cancel()` |
| `application.AssetOptions{Handler, FS}` | Only `Handler` needed (chi router serves everything) |

### Key Interfaces for M12_02

```go
// internal/desktop/app.go
type Options struct {
    Router http.Handler  // chi router (API + static)
    WebFS  fs.FS         // embedded frontend (currently unused — Router handles it)
}

type DesktopApp struct {
    app    *application.App
    window *application.WebviewWindow
    tray   *SystemTray
}

func NewDesktopApp(opts Options) (*DesktopApp, error)
func (d *DesktopApp) Run() error
```

```go
// internal/desktop/tray.go
type SystemTray struct {
    tray       *application.SystemTray
    window     *application.WebviewWindow
    statusItem *application.MenuItem
}

func NewSystemTray(app *application.App, window *application.WebviewWindow) *SystemTray
func (s *SystemTray) UpdateStatus(instanceCount, alertCount int, maxSeverity string)
```

### Build Commands

```bash
# Standard server (unchanged)
go build ./cmd/pgpulse-server

# Desktop (Windows)
go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server

# Desktop with GUI subsystem (no console window)
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...
```

### Build Verification (All Pass)

| Check | Result |
|-------|--------|
| Standard build (no tags) | PASS — no Wails symbols |
| Desktop build (-tags desktop) | PASS — Wails symbols present |
| Test suite | PASS |
| Lint | PASS |
| Frontend build/typecheck/lint | PASS |

---

## What M12_02 Needs to Build On

### Scope: Connection Dialog + OS Notifications + NSIS Installer

From `M12_requirements.md` sections 4.3, 4.4, 4.5:

### 1. Connection Dialog (`internal/desktop/dialog.go`)

| Req | Description |
|-----|-------------|
| C-01 | On first launch (no config file), show native dialog before main window |
| C-02 | Dialog offers: "Open config file" (native file picker, .yml), "Use existing config" (if default path exists), "Quick connect" (DSN input field) |
| C-03 | Quick connect mode: DSN → in-memory ring buffer (live mode), no persistent storage |
| C-04 | Config file mode: parse pgpulse.yml, start full persistent mode |
| C-05 | Remember last-used config path for subsequent launches |

**Integration point:** Currently `RunDesktop()` is called from `startServer()` which already has a loaded config. The dialog needs to run BEFORE config loading when no config exists. This likely means moving the desktop intercept earlier in `main()` — possibly before `config.Load()` when in desktop mode. Consider using Wails dialog APIs or a dedicated pre-launch window.

### 2. OS Notifications (`internal/desktop/notifications.go`)

| Req | Description |
|-----|-------------|
| N-01 | Use Wails v3 `NotificationService` for Windows Toast notifications |
| N-02 | Bridge: alert engine events → OS toast when alert fires |
| N-03 | Toast shows: severity icon, instance name, rule name, metric value |
| N-04 | Click toast → bring window to foreground, navigate to alert detail |
| N-05 | Configurable: enable/disable per severity (INFO, WARNING, CRITICAL) |
| N-06 | Rate limiting: max 1 per rule per 5 minutes |

**Integration point:** The alert dispatcher (`internal/alert/dispatcher.go`) fires events. M12_02 needs a bridge that subscribes to alert events and forwards them to Wails notifications. The `DesktopApp` struct needs access to the alert dispatcher — may need to expand `Options` to accept it, or use a callback/channel pattern.

**Wails v3 note:** Check if `NotificationService` exists in alpha.74. The risk register flags potential bugs (#4449). If unavailable, fall back to in-app notifications only.

### 3. NSIS Installer

| Req | Description |
|-----|-------------|
| I-01 | NSIS installer (.exe setup) |
| I-02 | Install to `C:\Program Files\PGPulse\` |
| I-03 | Desktop shortcut + Start Menu entry |
| I-04 | Optional: add to Windows startup |
| I-05 | Uninstaller in Add/Remove Programs |
| I-06 | Include sample `pgpulse.yml` |
| I-07 | Size target: < 25 MB |

**Prerequisites:** NSIS 3.x must be installed on dev machine and in PATH. Create `deploy/nsis/pgpulse.nsi` script.

### Suggested Agent Team (2 agents)

- **Desktop/UX agent:** dialog.go, notifications.go, expand DesktopApp options, modify main.go for pre-config dialog
- **Installer/QA agent:** NSIS script, build scripts, installer testing, full verification suite

---

## Known Issues

| Issue | Severity | Notes |
|-------|----------|-------|
| Wails v3 is alpha | Medium | Pinned to alpha.74; API may change in future versions |
| Test stubs are compile-only | Low | Real GUI testing requires manual verification on Windows |
| `UpdateStatus()` not yet wired to alert engine | Info | M12_02 should wire this via the notification bridge |
| Icons are placeholder circles | Low | Replace with real PGPulse logo when available |
| `Options.WebFS` field unused | Info | Chi router handles all static serving; FS kept for potential future use |
| Pre-existing M11 uncommitted changes in working tree | Info | `internal/api/server.go`, `web/src/` have M11_02 changes not yet committed |

---

## Commits (M12_01)

| Hash | Message |
|------|---------|
| `5b7078f` | docs(M12_01): requirements, design, team-prompt, checklist — Wails v3 desktop shell |
| `bcea155` | feat(desktop): M12_01 — Wails v3 desktop shell with system tray |
