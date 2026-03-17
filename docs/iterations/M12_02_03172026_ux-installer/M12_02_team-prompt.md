# PGPulse M12_02 — Team Prompt

**Iteration:** M12_02 — UX + Installer
**Date:** 2026-03-17
**Agent team size:** 2 (Desktop/UX + Installer/QA)
**Team lead model:** Opus

---

## Context

Read these files before starting:
- `docs/iterations/M12_02_03172026_ux-installer/design.md` — full architecture
- `docs/iterations/M12_01_03172026_core-desktop/requirements.md` — requirements (covers both M12_01 + M12_02)
- `docs/iterations/M12_01_03172026_core-desktop/HANDOFF_M12_01_to_M12_02.md` — what M12_01 built
- `CLAUDE.md` — project conventions
- `docs/CODEBASE_DIGEST.md` — current file inventory

PGPulse M12_01 already created the Wails v3 desktop shell: native window, system tray, build-tag gating, `--mode` flag. M12_02 adds the connection dialog (first-launch experience), OS alert notifications (Windows Toast), NSIS installer, and wires the tray icon to live alert status.

---

## DO NOT RE-DISCUSS

All M12_01 decisions remain locked:

| Decision | Locked Value |
|----------|-------------|
| D300 | Wails v3 alpha.74 |
| D301 | Thin wrapper — chi as Handler. Wails bindings used ONLY for connection dialog, NOT for main app. |
| D302 | Windows-only |
| D303 | Build-tag gating: `//go:build desktop` / `!desktop` |
| Go version | 1.25 (required by Wails alpha.74) |
| Icons | `internal/desktop/icons/` |
| Flag registration | `init()` in desktop.go |
| Wails alpha.74 API | Use `app.Window.NewWithOptions()`, `app.SystemTray.New()`, `window.OnWindowEvent()` — NOT the design doc v2 patterns |
| Frontend | ZERO changes to `web/src/` |
| Build scope | `./cmd/... ./internal/...` — never `./...` |
| Git branch | `master` |

**M12_02 additions:**

| Decision | Locked Value |
|----------|-------------|
| Connection dialog | Small Wails dialog window with embedded HTML — separate app lifecycle, runs before main app |
| Notification bridge | `OnAlert` callback hook on `AlertDispatcher` — additive, non-breaking |
| Settings persistence | `%APPDATA%\PGPulse\settings.json` for last config path |
| Tray status wiring | 10-second polling goroutine in DesktopApp |
| NSIS installer | Standard NSIS with MUI2, optional auto-start |

---

## Team Structure

### Agent 1 — Desktop/UX

**Owns:** `internal/desktop/dialog.go`, `dialog.html`, `dialog_bindings.go`, `settings.go`, `notifications.go`, modifications to `app.go`, `tray.go`, `desktop.go`, `desktop_stub.go`, `main.go`, `internal/alert/dispatcher.go`

**Does NOT touch:** `internal/api/*`, `internal/collector/*`, `internal/auth/*`, `internal/storage/*`, `web/src/*`, `deploy/*`.

**Tasks (in order):**

#### Task 1 — Settings persistence

Create `internal/desktop/settings.go` with `//go:build desktop`:

```go
type AppSettings struct {
    LastConfigPath string `json:"last_config_path"`
}

func LoadSettings() (*AppSettings, error)   // reads %APPDATA%/PGPulse/settings.json
func SaveSettings(s *AppSettings) error     // writes %APPDATA%/PGPulse/settings.json
func settingsPath() string                  // returns %APPDATA%/PGPulse/settings.json
```

Use `os.UserConfigDir()` to get the base path. Create `PGPulse/` dir if it doesn't exist.

#### Task 2 — Connection dialog HTML

Create `internal/desktop/dialog.html` — a self-contained HTML page for the connection dialog:

- Title: "PGPulse — Welcome"
- Three sections: "Open config file", "Quick connect (DSN input)", "Last used config (if available)"
- Style with inline CSS (no Tailwind CDN — keep it self-contained and fast)
- Use `window.wails` or `@wailsio/runtime` calls to invoke Go-bound methods:
  - `OpenFilePicker()` → returns selected path
  - `SubmitDSN(dsn string)` → validates and returns
  - `UseLastConfig(path string)` → returns path
  - `Cancel()` → closes dialog
- Keep it simple: no React, no build step. Plain HTML + vanilla JS.
- Size: ~600×400 viewport

#### Task 3 — Dialog bindings

Create `internal/desktop/dialog_bindings.go` with `//go:build desktop`:

```go
type DialogService struct {
    result chan DialogResult
    app    *application.App
}

type DialogResult struct {
    Mode       string // "config", "quickconnect", "cancel"
    ConfigPath string
    DSN        string
}

func (s *DialogService) OpenFilePicker() string {
    // Use Wails file dialog to pick .yml file
    // Return selected path
}

func (s *DialogService) SubmitDSN(dsn string) {
    // Validate DSN format, send result
    s.result <- DialogResult{Mode: "quickconnect", DSN: dsn}
    s.app.Quit()
}

func (s *DialogService) UseLastConfig(path string) {
    s.result <- DialogResult{Mode: "config", ConfigPath: path}
    s.app.Quit()
}

func (s *DialogService) Cancel() {
    s.result <- DialogResult{Mode: "cancel"}
    s.app.Quit()
}
```

**IMPORTANT:** These bindings are for the dialog window ONLY. The main PGPulse app does NOT use Wails bindings — it uses chi REST (D301).

#### Task 4 — Dialog app

Create `internal/desktop/dialog.go` with `//go:build desktop`:

```go
func ShowConfigDialog(lastConfigPath string) (DialogResult, error)
```

This function:
1. Creates a new `application.App` (separate from main app)
2. Binds `DialogService` methods
3. Creates a small window (600×400) loading `dialog.html` from embedded FS
4. Runs the app (blocks)
5. Returns the `DialogResult` from the channel
6. The dialog app fully shuts down before returning

If `lastConfigPath` is empty or the file doesn't exist, hide the "Last used" section in the dialog.

**CRITICAL:** This must be a completely separate Wails application lifecycle from the main PGPulse desktop app. It starts, shows the dialog, gets the answer, quits. Then main() continues with config loading and starts the main desktop app.

**If the dual-Wails-app approach causes issues** (e.g., alpha.74 doesn't support creating two apps sequentially), fall back to: (a) a plain Windows file picker dialog via `win32` syscalls, or (b) CLI prompts with a subsequent window launch. Document the issue and fallback in session-log.

#### Task 5 — Alert notification bridge

Create `internal/desktop/notifications.go` with `//go:build desktop`:

```go
type AlertNotifier struct {
    service     *notifications.NotificationService
    window      *application.WebviewWindow
    lastFired   map[string]time.Time
    cooldown    time.Duration
    minSeverity string
    mu          sync.Mutex
}

func NewAlertNotifier(svc *notifications.NotificationService, window *application.WebviewWindow) *AlertNotifier
func (n *AlertNotifier) HandleAlert(event alert.AlertEvent)
func (n *AlertNotifier) shouldNotify(event alert.AlertEvent) bool  // rate limit + severity filter
```

Rate limiting: `lastFired` maps `ruleID` to last toast time. Skip if within 5 minutes.
Severity filter: default to WARNING + CRITICAL (skip INFO).

Register notification response handler:
```go
svc.OnNotificationResponse(func(result notifications.NotificationResult) {
    window.Show()
    window.Focus()
    // Navigate to alerts page
})
```

**If NotificationService fails to initialize** (alpha bugs, permissions), log warning and set `AlertNotifier` to nil. All call sites must nil-check. This is the fallback — tray icon still shows severity colors.

#### Task 6 — Add OnAlert hook to dispatcher

Modify `internal/alert/dispatcher.go`:

Add a field and method:
```go
// Add to AlertDispatcher struct:
onAlertHooks []func(AlertEvent)

// New exported method:
func (d *AlertDispatcher) OnAlert(fn func(AlertEvent)) {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.onAlertHooks = append(d.onAlertHooks, fn)
}
```

In the dispatch loop (wherever events are processed), after existing notification logic:
```go
for _, hook := range d.onAlertHooks {
    go hook(event)  // non-blocking
}
```

This is a small, additive change. Existing tests should not be affected (empty hooks slice = no-op).

#### Task 7 — Expand DesktopApp

Modify `internal/desktop/app.go`:

- Expand `Options` struct to add `AlertDispatcher *alert.AlertDispatcher` and `Config *config.Config`
- In `NewDesktopApp`: initialize `NotificationService`, create `AlertNotifier`, register `OnAlert` hook with dispatcher
- Start tray status goroutine (10-second ticker, polls alert dispatcher for active count + max severity)
- Add `Shutdown()` method for cleanup

#### Task 8 — Update desktop.go / desktop_stub.go / main.go

Modify `cmd/pgpulse-server/desktop.go`:
- Expand `RunDesktop` to accept `*alert.AlertDispatcher` and `*config.Config`
- Add `ResolveConfigDesktop(configPath string) (string, error)` — calls `ShowConfigDialog` if needed, generates temp config for quick-connect mode

Modify `cmd/pgpulse-server/desktop_stub.go`:
- Match new `RunDesktop` signature (return error)
- Add `ResolveConfigDesktop(configPath string) (string, error)` stub that returns configPath unchanged

Modify `cmd/pgpulse-server/main.go`:
- Before `config.Load()`: if desktop mode and no config specified, call `ResolveConfigDesktop()`
- After orchestrator + alert dispatcher created: pass them to `RunDesktop()`

---

### Agent 2 — Installer/QA

**Owns:** `deploy/nsis/*`, test files, build verification, installer testing.

**Does NOT touch:** `internal/desktop/*`, `internal/alert/*`, `cmd/pgpulse-server/*`.

**Tasks (in order):**

#### Task 1 — NSIS installer script

Create `deploy/nsis/pgpulse.nsi`:
- Install to `$PROGRAMFILES\PGPulse`
- Include `pgpulse-desktop.exe` (renamed to `pgpulse.exe` during install)
- Include sample `configs/pgpulse.example.yml` (installed as `pgpulse.yml`)
- Desktop shortcut + Start Menu shortcut (both pass `--mode=desktop`)
- Optional section: "Start with Windows" (writes Run registry key)
- Uninstaller: removes files, shortcuts, registry keys
- Add/Remove Programs registry entries
- Use MUI2 for modern look (Welcome, Directory, Install, Finish pages)

Create `deploy/nsis/license.txt`:
- MIT license text (copy from LICENSE in repo root, or write standard MIT)

#### Task 2 — Build desktop binary for installer

```bash
cd web && npm run build && cd ..
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server
```

#### Task 3 — Build NSIS installer

```bash
makensis deploy/nsis/pgpulse.nsi
```

If NSIS is not installed, document the installation steps and mark installer build as manual.

**NSIS install on Windows:**
1. Download from https://nsis.sourceforge.io/Download
2. Install, ensure `makensis.exe` is in PATH
3. Re-run: `makensis deploy/nsis/pgpulse.nsi`

#### Task 4 — Full build verification

```bash
# Standard build
go build ./cmd/... ./internal/...

# Desktop build
go build -tags desktop ./cmd/... ./internal/...

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...

# Frontend
cd web && npm run build && npm run typecheck && npm run lint && cd ..
```

#### Task 5 — Test new dispatcher hook

Verify `internal/alert/dispatcher_test.go` still passes. If there are existing tests that mock the dispatcher, ensure `onAlertHooks` doesn't break them.

#### Task 6 — Commit

```bash
git add -A
git commit -m "feat(desktop): M12_02 — connection dialog, OS notifications, NSIS installer

- Add connection dialog for first-launch config selection (file picker / DSN / last-used)
- Add OS notification bridge: alert events → Windows Toast via Wails NotificationService
- Add NSIS installer: shortcuts, auto-start option, Add/Remove Programs
- Wire tray icon status to live alert severity (10s polling)
- Add OnAlert hook to AlertDispatcher for desktop notification bridge
- Persist last config path in %APPDATA%/PGPulse/settings.json"
```

---

## Coordination Notes

- Desktop/UX agent does tasks 1-5 first (settings, dialog, notifications). QA agent can work on NSIS independently.
- Task 6 (dispatcher.go change) should be done early — it's small and unblocks notification wiring.
- Task 7 (expand DesktopApp) depends on tasks 5 and 6.
- Task 8 (main.go changes) is last for Desktop/UX agent.
- If dual-Wails-app lifecycle doesn't work, the dialog fallback is documented in task 4. Don't spend more than 15 minutes debugging — use the fallback.
- If `NotificationService` fails to init, log it and disable. Don't block the iteration.
- NSIS build is optional — if `makensis` is not available, create the script anyway and document manual build steps.

---

## Build Commands Reference

```bash
# Frontend
cd web && npm run build && cd ..

# Standard server (unchanged)
go build ./cmd/pgpulse-server

# Desktop (console)
go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server

# Desktop (GUI subsystem — for installer)
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# NSIS installer
makensis deploy/nsis/pgpulse.nsi

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...
```

---

## Watch-List

**New files:**
- [ ] `internal/desktop/dialog.go`
- [ ] `internal/desktop/dialog.html`
- [ ] `internal/desktop/dialog_bindings.go`
- [ ] `internal/desktop/settings.go`
- [ ] `internal/desktop/notifications.go`
- [ ] `deploy/nsis/pgpulse.nsi`
- [ ] `deploy/nsis/license.txt`

**Modified files:**
- [ ] `cmd/pgpulse-server/main.go`
- [ ] `cmd/pgpulse-server/desktop.go`
- [ ] `cmd/pgpulse-server/desktop_stub.go`
- [ ] `internal/desktop/app.go`
- [ ] `internal/desktop/tray.go`
- [ ] `internal/alert/dispatcher.go`

**Unchanged (verify):**
- [ ] `internal/api/server.go`
- [ ] `internal/api/static.go`
- [ ] `web/src/*`
