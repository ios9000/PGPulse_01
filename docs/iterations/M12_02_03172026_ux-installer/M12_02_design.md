# PGPulse M12_02 — UX + Installer — Design Document

**Date:** 2026-03-17
**Iteration:** M12_02
**Parent:** M12_requirements.md
**Depends on:** M12_01 (complete — Wails shell, tray, build tags)
**Scope:** Connection dialog, OS alert notifications, NSIS installer, tray status wiring

---

## 1. Connection Dialog — Architecture

### 1.1 The Problem

Today's `main()` flow:

```
main() → parseFlags() → config.Load(configPath) → storage → auth → orchestrator → apiServer.Routes() → RunDesktop(router, assets)
```

The connection dialog must appear **before** `config.Load()` when:
- `--config` flag is not provided AND
- no default config exists (`pgpulse.yml`, `configs/pgpulse.yml`) AND
- `--mode=desktop`

In server mode, missing config is a fatal error (existing behavior, unchanged).

### 1.2 Solution: Pre-Config Desktop Intercept

Add a new function `ResolveConfigDesktop()` that runs between flag parsing and `config.Load()`:

```
main()
  → parseFlags()
  → RegisterDesktopFlags()   // M12_01: init()
  → IF mode == "desktop" AND configPath == "" AND no default config found:
      configPath = ShowConfigDialog()   // NEW: native dialog
      IF configPath == "" AND quickConnectDSN != "":
          generate temp config from DSN → configPath = tempConfig
      IF configPath == "":
          exit (user cancelled)
  → config.Load(configPath)  // existing, unchanged
  → ... rest of startup ...
  → RunDesktop(router, assets, alertDispatcher)  // expanded signature
```

### 1.3 ShowConfigDialog — Implementation

This is a **pre-window dialog**. It runs before the main Wails application starts. Two implementation approaches:

**Approach A — Wails dialog APIs (preferred):**

Wails v3 provides `application.OpenFileDialog` and `application.MessageDialog`. However, these typically require a running Wails application context.

**Approach B — Minimal pre-launch Wails app:**

Create a lightweight Wails app with a single small dialog window (HTML-based) that presents the three options. On selection, the app quits, returns the result, and main() continues with the full app startup.

**Approach C — Windows-native dialog (win32 API):**

Use `golang.org/x/sys/windows` to call `GetOpenFileName` directly. No Wails dependency for the dialog. Simple but platform-specific (which is fine — M12 is Windows-only).

**Recommendation:** Approach B. A small Wails dialog window is consistent with the rest of the desktop experience and allows a proper UI with the three options. It's more work than C but cleaner.

### 1.4 Dialog Window Spec

A small (600×400) Wails window with an embedded HTML page:

```
┌──────────────────────────────────────┐
│        PGPulse — Welcome             │
│                                      │
│  How would you like to start?        │
│                                      │
│  ┌────────────────────────────────┐  │
│  │ 📁 Open config file (.yml)    │  │
│  │    Browse for pgpulse.yml     │  │
│  └────────────────────────────────┘  │
│                                      │
│  ┌────────────────────────────────┐  │
│  │ ⚡ Quick connect (DSN)        │  │
│  │    postgresql://user:pass@... │  │
│  │    [____________________]     │  │
│  │              [Connect]        │  │
│  └────────────────────────────────┘  │
│                                      │
│  Last used: C:\pgpulse.yml  [Use]   │
│                                      │
│             [Cancel]                 │
└──────────────────────────────────────┘
```

**Implementation:** The dialog is a separate Wails app instance with a single window. The HTML is embedded via `go:embed`. Communication uses Wails bindings (this is the ONE place we use bindings — just for the dialog, not for the main app).

The dialog returns a `DialogResult`:

```go
type DialogResult struct {
    Mode       string // "config", "quickconnect", "cancel"
    ConfigPath string // path to .yml file (Mode=="config")
    DSN        string // connection string (Mode=="quickconnect")
}
```

### 1.5 Quick Connect Mode

When the user enters a DSN in the dialog:
1. Generate a minimal in-memory config with one instance
2. Set `storage.dsn = ""` (no persistent storage — live mode)
3. The existing live-mode path (`MemoryStore`, `NullStore` for alerts, etc.) handles this

This reuses the MW_01 live-mode infrastructure entirely.

### 1.6 Remember Last Config Path

Store the last-used config path in a small JSON file at:
- `%APPDATA%\PGPulse\settings.json` (Windows)

```json
{
    "last_config_path": "C:\\opt\\pgpulse\\configs\\pgpulse.yml"
}
```

Read on startup. If the file exists and the config path is valid, show "Last used: {path} [Use]" in the dialog.

### 1.7 New/Modified Files for Dialog

| File | Tag | Purpose |
|------|-----|---------|
| `internal/desktop/dialog.go` | `desktop` | `ShowConfigDialog()` → `DialogResult`, dialog app + window |
| `internal/desktop/dialog.html` | — | Embedded HTML for dialog UI |
| `internal/desktop/dialog_bindings.go` | `desktop` | Go methods bound to dialog (file picker, DSN submit, last-path) |
| `internal/desktop/settings.go` | `desktop` | `LoadLastConfig()`, `SaveLastConfig()` — reads/writes %APPDATA%/PGPulse/settings.json |
| `cmd/pgpulse-server/main.go` | — | Add pre-config dialog intercept (~20 lines) |
| `cmd/pgpulse-server/desktop.go` | `desktop` | Expand `RunDesktop` signature to accept `*alert.AlertDispatcher` |

---

## 2. OS Alert Notifications

### 2.1 Wails v3 NotificationService

Wails v3 provides `pkg/services/notifications` with:

```go
notificationService := notifications.New()

// Register as Wails service
app := application.New(application.Options{
    Services: []application.Service{
        application.NewService(notificationService),
    },
})

// Send notification
notificationService.SendNotification(notifications.NotificationOptions{
    ID:    "alert-123",
    Title: "PGPulse Alert: CRITICAL",
    Body:  "primary-db: Connection utilization at 95%",
})
```

On Windows, this uses Toast notifications via `go-toast`.

### 2.2 Alert→Notification Bridge

Create `internal/desktop/notifications.go` that:
1. Subscribes to alert events from the alert dispatcher
2. Converts `AlertEvent` → `NotificationOptions`
3. Applies rate limiting (1 per rule per 5 minutes)
4. Respects severity filter (configurable via config or default: WARNING + CRITICAL)

```go
type AlertNotifier struct {
    service    *notifications.NotificationService
    window     *application.WebviewWindow
    lastFired  map[string]time.Time  // ruleID → last notification time
    cooldown   time.Duration         // 5 minutes
    minSeverity string               // "warning" or "critical"
    mu         sync.Mutex
}

func NewAlertNotifier(svc *notifications.NotificationService, window *application.WebviewWindow) *AlertNotifier

// HandleAlert is called by the alert dispatcher when an alert fires.
// It checks rate limits and sends an OS notification if appropriate.
func (n *AlertNotifier) HandleAlert(event alert.AlertEvent)
```

### 2.3 Wiring to Alert Dispatcher

The alert dispatcher (`internal/alert/dispatcher.go`) has a channel-based event flow. M12_02 needs to add a callback hook or additional subscriber.

**Preferred approach:** Add an optional `OnAlert func(AlertEvent)` callback to the dispatcher (or to `DesktopApp`). The desktop app registers this callback during startup. When an alert fires, the dispatcher calls it in addition to normal notification channels.

**Alternative:** Export the dispatcher's event channel and have the `AlertNotifier` read from it. This is simpler but couples the packages more tightly.

The callback approach is preferred. Add to `internal/alert/dispatcher.go`:

```go
// In AlertDispatcher struct:
onAlertHooks []func(AlertEvent)

// New method:
func (d *AlertDispatcher) OnAlert(fn func(AlertEvent)) {
    d.onAlertHooks = append(d.onAlertHooks, fn)
}

// In dispatch loop, after normal processing:
for _, hook := range d.onAlertHooks {
    hook(event)
}
```

This is a small, non-breaking change to an existing package.

### 2.4 Click-to-Navigate

When user clicks a notification toast, the app should:
1. Show the PGPulse window (if hidden)
2. Focus it
3. Navigate to the alert detail page

Wails v3 `NotificationService` supports `OnNotificationResponse` callback. On click:

```go
notificationService.OnNotificationResponse(func(result notifications.NotificationResult) {
    window.Show()
    window.Focus()
    // Navigate to alert page — use window.ExecJS or URL navigation
    window.SetURL("/alerts?highlight=" + result.Response.ID)
})
```

### 2.5 Notification Fallback

If `NotificationService` fails to initialize (alpha bug, missing permissions, etc.):
- Log a warning
- Disable OS notifications
- Tray icon status changes still work (M12_01 already implemented `UpdateStatus`)
- In-app alerts page still works (existing, unchanged)

---

## 3. NSIS Installer

### 3.1 File Structure

```
deploy/nsis/
├── pgpulse.nsi         # Main NSIS script
├── pgpulse-header.bmp  # 150x57 installer header image (optional)
└── license.txt         # License text shown during install
```

### 3.2 NSIS Script Spec

```nsi
!define APPNAME "PGPulse"
!define VERSION "1.0.0"
!define PUBLISHER "PGPulse"
!define HELPURL "https://github.com/ios9000/PGPulse_01"
!define INSTALLSIZE 30000  ; KB estimate

Name "${APPNAME}"
OutFile "PGPulse-${VERSION}-Setup.exe"
InstallDir "$PROGRAMFILES\${APPNAME}"
RequestExecutionLevel admin

; Pages
!insertmacro MUI2 pages (Welcome, License, Directory, Install, Finish)

Section "Install"
    SetOutPath "$INSTDIR"

    ; Main binary (desktop build with GUI subsystem)
    File "..\..\pgpulse-desktop.exe"
    Rename "$INSTDIR\pgpulse-desktop.exe" "$INSTDIR\pgpulse.exe"

    ; Sample config
    File "..\..\configs\pgpulse.example.yml"
    Rename "$INSTDIR\pgpulse.example.yml" "$INSTDIR\pgpulse.yml"

    ; Create shortcuts
    CreateDirectory "$SMPROGRAMS\${APPNAME}"
    CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\pgpulse.exe" "--mode=desktop"
    CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\pgpulse.exe" "--mode=desktop"

    ; Uninstaller
    WriteUninstaller "$INSTDIR\Uninstall.exe"

    ; Registry for Add/Remove Programs
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "DisplayName" "${APPNAME}"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "UninstallString" "$\"$INSTDIR\Uninstall.exe$\""
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "Publisher" "${PUBLISHER}"
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "EstimatedSize" ${INSTALLSIZE}
SectionEnd

; Optional: auto-start
Section /o "Start with Windows" SEC_AUTOSTART
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APPNAME}" "$INSTDIR\pgpulse.exe --mode=desktop"
SectionEnd

Section "Uninstall"
    Delete "$INSTDIR\pgpulse.exe"
    Delete "$INSTDIR\pgpulse.yml"
    Delete "$INSTDIR\Uninstall.exe"
    RMDir "$INSTDIR"
    Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
    RMDir "$SMPROGRAMS\${APPNAME}"
    Delete "$DESKTOP\${APPNAME}.lnk"
    DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Run\${APPNAME}"
SectionEnd
```

### 3.3 Build Workflow

```bash
# 1. Build desktop binary with GUI subsystem
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# 2. Build NSIS installer
makensis deploy/nsis/pgpulse.nsi
# Output: deploy/nsis/PGPulse-1.0.0-Setup.exe
```

---

## 4. Tray Status Wiring

M12_01 created `SystemTray.UpdateStatus()` but it's not wired to anything. M12_02 connects it:

In `internal/desktop/app.go`, after the orchestrator starts, register a periodic status check:

```go
// In DesktopApp, after orchestrator is running:
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    for range ticker.C {
        instanceCount := len(opts.Config.Instances)
        alertCount, maxSeverity := getAlertSummary(opts.AlertDispatcher)
        da.tray.UpdateStatus(instanceCount, alertCount, maxSeverity)
    }
}()
```

This requires expanding `Options` to include the alert dispatcher (or a status provider interface).

---

## 5. Expanded RunDesktop Signature

M12_01's signature: `RunDesktop(router chi.Router, assets fs.FS) error`

M12_02 expands to:

```go
func RunDesktop(router chi.Router, assets fs.FS, alertDispatcher *alert.AlertDispatcher, cfg *config.Config) error
```

The `alertDispatcher` enables:
- Alert→notification bridge
- Tray status polling

The `cfg` enables:
- Notification severity filter from config
- Instance count for tray status

Both `desktop.go` and `desktop_stub.go` must update signatures. The stub ignores the new params.

---

## 6. File Summary

### New Files

| File | Tag | Lines Est. | Purpose |
|------|-----|-----------|---------|
| `internal/desktop/dialog.go` | `desktop` | ~120 | Dialog app, window, result handling |
| `internal/desktop/dialog.html` | — | ~80 | Embedded HTML for connection dialog UI |
| `internal/desktop/dialog_bindings.go` | `desktop` | ~60 | File picker + DSN submit bindings |
| `internal/desktop/settings.go` | `desktop` | ~50 | %APPDATA%/PGPulse/settings.json read/write |
| `internal/desktop/notifications.go` | `desktop` | ~90 | AlertNotifier: rate-limited alert→toast bridge |
| `deploy/nsis/pgpulse.nsi` | — | ~80 | NSIS installer script |
| `deploy/nsis/license.txt` | — | ~20 | MIT license text for installer |

### Modified Files

| File | Change |
|------|--------|
| `cmd/pgpulse-server/main.go` | Pre-config dialog intercept (~20 lines) |
| `cmd/pgpulse-server/desktop.go` | Expanded `RunDesktop` signature, `ResolveConfigDesktop()` |
| `cmd/pgpulse-server/desktop_stub.go` | Expanded `RunDesktop` stub signature, `ResolveConfigDesktop()` stub |
| `internal/desktop/app.go` | Expanded `Options` struct (AlertDispatcher, Config), tray status goroutine, notification init |
| `internal/desktop/tray.go` | Minor: ensure `UpdateStatus` is goroutine-safe |
| `internal/alert/dispatcher.go` | Add `OnAlert` hook mechanism (~10 lines) |

### Estimated Totals

- **New files:** 7
- **Modified files:** 6
- **New lines:** ~500–600
- **Modified lines:** ~60

---

## 7. Agent Team

### Agent 1 — Desktop/UX

**Owns:** `internal/desktop/dialog.go`, `dialog.html`, `dialog_bindings.go`, `settings.go`, `notifications.go`, modifications to `app.go`, `tray.go`, `desktop.go`, `desktop_stub.go`, `main.go`

**Tasks (in order):**

1. Create `internal/desktop/settings.go` — `LoadLastConfig()`/`SaveLastConfig()` for %APPDATA%/PGPulse/settings.json
2. Create `internal/desktop/dialog.html` — embedded HTML for connection dialog
3. Create `internal/desktop/dialog_bindings.go` — Go methods bound to dialog (OpenFilePicker, SubmitDSN, UseLastConfig)
4. Create `internal/desktop/dialog.go` — `ShowConfigDialog()` → `DialogResult`, creates temporary Wails app for dialog
5. Create `internal/desktop/notifications.go` — `AlertNotifier` with rate limiting, severity filter, toast sending
6. Modify `internal/desktop/app.go` — expand `Options`, add `AlertDispatcher`, `Config`, init `NotificationService`, start tray status goroutine
7. Modify `cmd/pgpulse-server/desktop.go` — expand `RunDesktop` signature, add `ResolveConfigDesktop()`
8. Modify `cmd/pgpulse-server/desktop_stub.go` — match new signatures
9. Modify `cmd/pgpulse-server/main.go` — insert pre-config dialog intercept
10. Modify `internal/alert/dispatcher.go` — add `OnAlert` hook

### Agent 2 — Installer/QA

**Owns:** `deploy/nsis/*`, test files, build verification

**Tasks (in order):**

1. Create `deploy/nsis/pgpulse.nsi` — full NSIS installer script
2. Create `deploy/nsis/license.txt` — MIT license
3. Build desktop binary: `go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server`
4. Build installer: `makensis deploy/nsis/pgpulse.nsi` (requires NSIS in PATH)
5. Run full test suite: `go test ./cmd/... ./internal/... -count=1`
6. Run lint: `golangci-lint run ./cmd/... ./internal/...`
7. Verify frontend unchanged: `cd web && npm run build && npm run typecheck && npm run lint`
8. Standard build verification: `go build ./cmd/pgpulse-server` (no tags, no Wails)
9. Desktop build verification: `go build -tags desktop ./cmd/pgpulse-server`

---

## 8. DO NOT RE-DISCUSS

All M12_01 decisions remain locked (see handoff). Additionally:

| Decision | Status |
|----------|--------|
| Dialog approach | Approach B: small Wails dialog window with embedded HTML |
| Notification bridge | Callback hook on AlertDispatcher (`OnAlert`) |
| NSIS installer | Standard NSIS with MUI2, optional auto-start section |
| Settings persistence | %APPDATA%/PGPulse/settings.json — last config path only |
| Tray status wiring | 10-second polling goroutine in DesktopApp |
| Wails bindings | Used ONLY in dialog window — main app remains REST-only (D301) |

---

## 9. Risk Register (M12_02 Specific)

| Risk | Impact | Mitigation |
|------|--------|------------|
| NotificationService not working in alpha.74 | No OS toast notifications | Fallback to tray icon color only; log warning |
| Pre-config dialog requires separate Wails app lifecycle | Complexity, potential crash | Keep dialog app minimal; clean shutdown before main app starts |
| NSIS not installed on dev machine | Can't build installer | Agent documents manual NSIS install steps; installer build is optional |
| Alert dispatcher modification breaks existing tests | Test failures | `OnAlert` hook is additive — nil hooks slice is no-op; existing tests unaffected |
| Dialog HTML styling inconsistent with main app | Visual polish issue | Use Tailwind CDN in dialog.html for consistency; dialog is small/simple |

---

## 10. Watch-List

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
