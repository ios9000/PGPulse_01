# PGPulse M12_01 — Core Desktop (Wails v3) — Design Document

**Date:** 2026-03-17
**Iteration:** M12_01
**Parent:** M12_requirements.md
**Scope:** Wails scaffold, build tags, chi→Wails integration, native window, system tray, `--mode` flag, Windows build verification

---

## 1. Architecture: Build-Tag Gating

The entire Wails integration is isolated behind `//go:build desktop`. The plain server binary remains identical — zero new dependencies, zero CGO, zero behavioral changes.

### 1.1 New Files

| File | Build Tag | Purpose |
|------|-----------|---------|
| `cmd/pgpulse-server/desktop.go` | `//go:build desktop` | Registers `--mode` flag, provides `RunDesktop()` function |
| `cmd/pgpulse-server/desktop_stub.go` | `//go:build !desktop` | No-op stubs: `RunDesktop()` returns error, `RegisterDesktopFlags()` is empty |
| `internal/desktop/app.go` | `//go:build desktop` | `DesktopApp` struct: Wails application, window, chi integration |
| `internal/desktop/tray.go` | `//go:build desktop` | System tray: icon, menu, left-click toggle, right-click context |
| `internal/desktop/icon.go` | `//go:build desktop` | Embedded icon assets (go:embed for PNG/ICO) |
| `assets/icons/pgpulse-tray.png` | — | 64×64 tray icon (PNG, green/healthy default) |
| `assets/icons/pgpulse-tray-warning.png` | — | 64×64 tray icon (yellow/warning) |
| `assets/icons/pgpulse-tray-critical.png` | — | 64×64 tray icon (red/critical) |
| `assets/icons/pgpulse.ico` | — | Multi-size ICO for window title bar (16/32/48/256px) |

### 1.2 Modified Files

| File | Change |
|------|--------|
| `cmd/pgpulse-server/main.go` | Add `--mode` flag handling: if mode=desktop, call `RunDesktop()`; otherwise existing flow. ~15 lines changed. |
| `go.mod` | Add `github.com/wailsapp/wails/v3` dependency (build-tag gated, only pulled when building with `-tags desktop`) |

### 1.3 Build Matrix

```
go build ./cmd/pgpulse-server
  → desktop_stub.go compiled (RegisterDesktopFlags = no-op, RunDesktop = error)
  → No Wails dependency in binary
  → Behavior: identical to today

go build -tags desktop ./cmd/pgpulse-server
  → desktop.go compiled (real --mode flag, real RunDesktop)
  → internal/desktop/* compiled (Wails app, tray, icons)
  → Binary includes Wails + WebView2 loader
  → Behavior: --mode=server (default) or --mode=desktop
```

---

## 2. main.go Integration Point

Current `main.go` (687 lines) follows this flow:

```
main()
  → config.Load()
  → setup logging
  → open storage pool (or nil for live mode)
  → run migrations
  → create auth services
  → create orchestrator
  → create APIServer → apiServer.Routes() returns chi.Router
  → http.ListenAndServe(chiRouter)
  → graceful shutdown
```

**The change:** Insert a mode check after `apiServer.Routes()` returns the chi router, but **before** `http.ListenAndServe()`:

```go
// In main.go, after building chiRouter:
chiRouter := apiServer.Routes()

mode := getMode() // "server" or "desktop"
if mode == "desktop" {
    // desktop.go provides this — returns error on !desktop build
    if err := RunDesktop(chiRouter, webDistFS, cfg, orchestrator, alertDispatcher); err != nil {
        slog.Error("desktop mode failed", "error", err)
        os.Exit(1)
    }
    return // desktop mode handles its own lifecycle
}

// Existing server mode — unchanged
srv := &http.Server{Addr: cfg.Server.Listen, Handler: chiRouter}
// ... existing graceful shutdown ...
```

### 2.1 desktop.go (build tag: desktop)

```go
//go:build desktop

package main

import (
    "github.com/go-chi/chi/v5"
    "io/fs"

    "github.com/ios9000/PGPulse_01/internal/alert"
    "github.com/ios9000/PGPulse_01/internal/config"
    "github.com/ios9000/PGPulse_01/internal/desktop"
    "github.com/ios9000/PGPulse_01/internal/orchestrator"
)

var desktopMode string

func RegisterDesktopFlags() {
    // Called from main() before flag.Parse()
    flag.StringVar(&desktopMode, "mode", "server", "Run mode: server or desktop")
}

func getMode() string {
    return desktopMode
}

func RunDesktop(
    router chi.Router,
    assets fs.FS,
    cfg *config.Config,
    orch *orchestrator.Orchestrator,
    dispatcher *alert.AlertDispatcher,
) error {
    app := desktop.NewDesktopApp(desktop.Options{
        Router:     router,
        Assets:     assets,
        Config:     cfg,
        Orchestrator: orch,
        AlertDispatcher: dispatcher,
    })
    return app.Run() // blocks until quit
}
```

### 2.2 desktop_stub.go (build tag: !desktop)

```go
//go:build !desktop

package main

import (
    "fmt"
    "github.com/go-chi/chi/v5"
    "io/fs"

    "github.com/ios9000/PGPulse_01/internal/alert"
    "github.com/ios9000/PGPulse_01/internal/config"
    "github.com/ios9000/PGPulse_01/internal/orchestrator"
)

var desktopMode string = "server" // always server in non-desktop build

func RegisterDesktopFlags() {} // no-op

func getMode() string { return "server" }

func RunDesktop(
    _ chi.Router, _ fs.FS, _ *config.Config,
    _ *orchestrator.Orchestrator, _ *alert.AlertDispatcher,
) error {
    return fmt.Errorf("desktop mode not available: binary built without -tags desktop")
}
```

---

## 3. internal/desktop/app.go — Core Wails Integration

### 3.1 DesktopApp struct

```go
//go:build desktop

package desktop

import (
    "context"
    "io/fs"
    "log/slog"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/wailsapp/wails/v3/pkg/application"

    "github.com/ios9000/PGPulse_01/internal/alert"
    "github.com/ios9000/PGPulse_01/internal/config"
    "github.com/ios9000/PGPulse_01/internal/orchestrator"
)

type Options struct {
    Router          chi.Router
    Assets          fs.FS
    Config          *config.Config
    Orchestrator    *orchestrator.Orchestrator
    AlertDispatcher *alert.AlertDispatcher
}

type DesktopApp struct {
    app    *application.App
    window *application.WebviewWindow
    tray   *SystemTray
    opts   Options
}
```

### 3.2 NewDesktopApp — Wails initialization

```go
func NewDesktopApp(opts Options) *DesktopApp {
    da := &DesktopApp{opts: opts}

    da.app = application.New(application.Options{
        Name:        "PGPulse",
        Description: "PostgreSQL Monitoring Platform",
        // Services will be registered in M12_02 (notifications)
    })

    // Create main window with chi router as asset handler
    da.window = da.app.Window.New(application.WebviewWindowOptions{
        Title:  da.windowTitle(),
        Width:  1440,
        Height: 900,
        URL:    "/",  // serves index.html from assets FS
        Assets: application.AssetOptions{
            FS:      opts.Assets,     // go:embed web/dist
            Handler: opts.Router,     // chi router handles /api/v1/* requests
        },
        // Windows-specific: use standard chrome
        MinWidth:  1024,
        MinHeight: 700,
    })

    // Close minimizes to tray instead of quitting
    da.window.On(events.WindowEventClosing, func(ctx *application.WindowEventContext) {
        da.window.Hide()
        ctx.Cancel() // prevent actual close
    })

    // System tray
    da.tray = NewSystemTray(da.app, da.window)

    return da
}

func (da *DesktopApp) Run() error {
    slog.Info("starting PGPulse desktop mode")
    return da.app.Run()
}

func (da *DesktopApp) windowTitle() string {
    if len(da.opts.Config.Instances) == 1 {
        return "PGPulse — " + da.opts.Config.Instances[0].Name
    }
    return "PGPulse"
}
```

### 3.3 Key Integration Detail: Chi as Asset Handler

Wails v3's `AssetOptions` has two fields that work together:
- `FS` — serves static files (index.html, JS bundles, CSS, images)
- `Handler` — receives requests that the FS cannot serve (API calls, POST/PUT/DELETE)

**Request flow in desktop mode:**

```
Browser (WebView2)
  → GET /                    → FS serves index.html
  → GET /assets/index-abc.js → FS serves JS bundle
  → GET /api/v1/instances    → FS returns 404 → Handler (chi) serves JSON
  → POST /api/v1/auth/login  → Handler (chi) directly (non-GET)
```

This is identical to how the embedded static handler works in server mode — `static.go` serves from `go:embed`, chi handles `/api/v1/*`. The only difference: Wails uses in-memory IPC instead of HTTP sockets.

---

## 4. internal/desktop/tray.go — System Tray

```go
//go:build desktop

package desktop

import (
    "github.com/wailsapp/wails/v3/pkg/application"
)

type SystemTray struct {
    systray *application.SystemTray
    window  *application.WebviewWindow
    menu    *application.Menu
}

func NewSystemTray(app *application.App, window *application.WebviewWindow) *SystemTray {
    st := &SystemTray{window: window}

    st.systray = app.SystemTray.New()
    st.systray.SetIcon(iconTrayDefault) // from icon.go
    st.systray.SetLabel("PGPulse")

    // Build context menu
    st.menu = app.NewMenu()
    st.menu.Add("Show PGPulse").OnClick(func(ctx *application.Context) {
        window.Show()
        window.Focus()
    })
    st.menu.AddSeparator()
    st.menu.Add("Status: Starting...").SetEnabled(false)
    st.menu.AddSeparator()
    st.menu.Add("Quit").OnClick(func(ctx *application.Context) {
        app.Quit()
    })

    st.systray.SetMenu(st.menu)

    // Left-click toggles window visibility
    st.systray.OnClick(func() {
        if window.IsVisible() {
            window.Hide()
        } else {
            window.Show()
            window.Focus()
        }
    })

    return st
}

// UpdateStatus updates the tray tooltip and status menu item.
// Called periodically by orchestrator status callback.
func (st *SystemTray) UpdateStatus(instanceCount int, alertCount int, maxSeverity string) {
    // Update icon based on severity
    switch maxSeverity {
    case "critical":
        st.systray.SetIcon(iconTrayCritical)
    case "warning":
        st.systray.SetIcon(iconTrayWarning)
    default:
        st.systray.SetIcon(iconTrayDefault)
    }

    // Update tooltip
    st.systray.SetTooltip(
        fmt.Sprintf("PGPulse — %d instances, %d active alerts", instanceCount, alertCount),
    )

    // Update menu status item (index 2 = status line)
    // Note: Wails v3 menu update approach — rebuild or use SetLabel
    // Will be refined during implementation based on actual v3 API behavior
}
```

---

## 5. internal/desktop/icon.go — Embedded Assets

```go
//go:build desktop

package desktop

import _ "embed"

//go:embed assets/icons/pgpulse-tray.png
var iconTrayDefault []byte

//go:embed assets/icons/pgpulse-tray-warning.png
var iconTrayWarning []byte

//go:embed assets/icons/pgpulse-tray-critical.png
var iconTrayCritical []byte

//go:embed assets/icons/pgpulse.ico
var iconWindow []byte
```

**Icon creation:** The agent team will generate placeholder icons using Go's `image` package during build. These are functional 64×64 colored squares (green/yellow/red) with "PP" text. Real branded icons can be swapped later without code changes.

---

## 6. go.mod Dependency Strategy

Wails v3 is added as a dependency, but due to build tags, it is **not compiled** into the standard server binary:

```
require (
    github.com/wailsapp/wails/v3 v3.0.0-alpha.70
    // ... existing deps ...
)
```

**Important:** Even though `go.mod` lists Wails, `go build ./cmd/pgpulse-server` (without `-tags desktop`) will NOT import it because all `//go:build desktop` files are excluded. The binary remains Wails-free.

**Verification test:** The QA agent must confirm:
1. `go build ./cmd/pgpulse-server` succeeds without CGO
2. The resulting binary does NOT contain Wails symbols (`go tool nm` check)
3. `go build -tags desktop ./cmd/pgpulse-server` succeeds
4. The desktop binary does contain Wails symbols

---

## 7. Windows-Specific Considerations

### 7.1 No CGO Required

Wails v3 on Windows uses a pure Go WebView2 loader (`go-webview2`). The desktop build works with `CGO_ENABLED=0`:

```bash
# Windows desktop build (from Git Bash / MINGW64)
go build -tags desktop -ldflags="-s -w -H windowsgui" ./cmd/pgpulse-server
```

The `-H windowsgui` linker flag suppresses the console window on launch (standard for Windows GUI apps).

### 7.2 Console vs GUI Subsystem

- `--mode=server`: needs console output → built WITHOUT `-H windowsgui`
- `--mode=desktop`: should suppress console → built WITH `-H windowsgui`

**Problem:** A single binary can only be one subsystem. **Solution:** Two build variants:

```bash
# Server variant (console subsystem — existing)
go build -ldflags="-s -w" -o pgpulse-server.exe ./cmd/pgpulse-server

# Desktop variant (GUI subsystem — suppresses console)
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server
```

This means the "single binary" is technically two build outputs from the same source, differing only in build tags and linker flags. The `--mode` flag still works in both, but `pgpulse-desktop.exe` defaults to `--mode=desktop`.

**Alternative approach the agent may consider:** Use `AllocConsole()` Windows API to attach a console when `--mode=server` is used in the GUI subsystem binary. This is more complex but truly single-binary. Leave this to agent judgment — either approach is acceptable.

---

## 8. Agent Team

### Team: 2 agents

**Agent 1 — Desktop/Backend:**
- Install Wails v3 dependency
- Create `cmd/pgpulse-server/desktop.go` and `desktop_stub.go`
- Create `internal/desktop/app.go` — Wails app, window, chi integration
- Create `internal/desktop/tray.go` — system tray with icon, menu, toggle
- Create `internal/desktop/icon.go` — embedded icon assets
- Generate placeholder icon PNGs (colored squares with "PP" text)
- Modify `cmd/pgpulse-server/main.go` — add mode check after chi router creation
- Update `go.mod` with Wails v3 dependency
- Verify desktop window loads the full PGPulse UI

**Agent 2 — QA/Build:**
- Build verification: `go build ./cmd/pgpulse-server` (no tags) — must succeed, no Wails
- Build verification: `go build -tags desktop ./cmd/pgpulse-server` — must succeed
- Symbol check: standard binary must NOT contain Wails symbols
- Run `go test ./cmd/... ./internal/... -count=1`
- Run `golangci-lint run ./cmd/... ./internal/...`
- Test `--mode=server` behavior unchanged
- Test `--mode=desktop` on non-desktop binary prints error
- Write test stubs for `internal/desktop/` (build-tag gated)

---

## 9. DO NOT RE-DISCUSS

All items from M11 handoff remain in force. Additionally:

| Decision | Status |
|----------|--------|
| D300: Wails v3 | **LOCKED** |
| D301: Thin wrapper (chi as Handler) | **LOCKED** — do NOT rewrite any API calls as Wails bindings |
| D302: Windows-only | **LOCKED** — do NOT add macOS/Linux desktop code |
| D303: Single binary + build tags | **LOCKED** — `//go:build desktop` on ALL Wails code |
| Frontend changes | **ZERO** — the React app works identically through Wails asset handler |
| Wails binding generation | **NOT NEEDED** — no `wails3 generate bindings` command |
| `go:embed` strategy | Reuse existing `web/dist` embed from `internal/api/static.go` — pass the same `embed.FS` to Wails |

---

## 10. Watch-List (Expected Files)

After M12_01 completes, these files should exist or be modified:

**New:**
- [ ] `cmd/pgpulse-server/desktop.go`
- [ ] `cmd/pgpulse-server/desktop_stub.go`
- [ ] `internal/desktop/app.go`
- [ ] `internal/desktop/tray.go`
- [ ] `internal/desktop/icon.go`
- [ ] `assets/icons/pgpulse-tray.png`
- [ ] `assets/icons/pgpulse-tray-warning.png`
- [ ] `assets/icons/pgpulse-tray-critical.png`
- [ ] `assets/icons/pgpulse.ico`

**Modified:**
- [ ] `cmd/pgpulse-server/main.go` (~15 lines: mode flag + desktop branch)
- [ ] `go.mod` (Wails v3 dependency)
- [ ] `go.sum` (updated)

**Unchanged (verify):**
- [ ] `internal/api/server.go` — zero changes
- [ ] `internal/api/static.go` — zero changes
- [ ] All `web/src/**` files — zero changes
