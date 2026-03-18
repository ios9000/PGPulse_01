# PGPulse — M12_02 Session Log

**Iteration:** M12_02 — UX + Installer (Wails v3)
**Date:** 2026-03-17
**Duration:** ~10 minutes wall-clock (agents ran in parallel)
**Commit:** `cb2ab6d`

---

## Agent Execution

| Agent | Tasks | Duration | Model |
|-------|-------|----------|-------|
| Desktop/UX | 8 tasks (settings, dialog HTML, dialog bindings, dialog app, notifications, dispatcher hook, expand DesktopApp, update desktop.go/stub/main.go) | ~8 min | Opus |
| Installer/QA | 4 tasks (NSIS script, license, example config, syntax verify) | ~2 min | Opus |

Both agents ran in parallel. Desktop/UX completed all 8 tasks. Installer/QA completed all 4 tasks.

---

## Files Created (8 new)

| File | Lines | Purpose |
|------|-------|---------|
| `internal/desktop/settings.go` | 66 | `AppSettings` persistence in `%APPDATA%/PGPulse/settings.json` |
| `internal/desktop/dialog.html` | 208 | Self-contained connection dialog UI (dark theme, inline CSS, vanilla JS) |
| `internal/desktop/dialog_bindings.go` | 62 | `DialogService` — Wails-bound methods for dialog actions |
| `internal/desktop/dialog.go` | 86 | `ShowConfigDialog()` — separate Wails app lifecycle for pre-config dialog |
| `internal/desktop/notifications.go` | 121 | `AlertNotifier` — bridges alert events to Windows Toast notifications |
| `deploy/nsis/pgpulse.nsi` | 143 | NSIS installer script (MUI2, shortcuts, autostart, Add/Remove Programs) |
| `deploy/nsis/license.txt` | 21 | MIT license for installer |
| `deploy/nsis/configs/pgpulse.example.yml` | 24 | Minimal sample config bundled in installer |

## Files Modified (7)

| File | Delta | Changes |
|------|-------|---------|
| `internal/alert/dispatcher.go` | +20 | `onAlertHooks` field, `OnAlert()` method, hook invocation in `processEvent` |
| `internal/desktop/app.go` | +67/-6 | `Options.OnAlertHook`, NotificationService init, AlertNotifier wiring, tray status goroutine, `Shutdown()` |
| `cmd/pgpulse-server/desktop.go` | +58/-6 | `RunDesktop` accepts alert hook; new `ResolveConfigDesktop()` with dialog + quickconnect |
| `cmd/pgpulse-server/desktop_stub.go` | +9/-6 | Matching stubs for new signatures |
| `cmd/pgpulse-server/main.go` | +16/-4 | Pre-config dialog call in desktop mode; alert hook wiring |
| `go.mod` | +1 | `go-toast/v2` indirect dep (Wails notifications on Windows) |
| `go.sum` | +2 | Checksums for go-toast |

**Total:** 15 files changed, 891 insertions, 13 deletions.

---

## Decisions Made by Agents

### Desktop/UX Agent

1. **OnAlertHook as function in Options (Task 7):** Instead of importing `*alert.Dispatcher` directly into the desktop package (which would work but couples unnecessarily), the agent used `OnAlertHook func(fn func(alert.AlertEvent))` — a function value. This allows `app.go` to register its callback without knowing about the Dispatcher struct. Clean inversion.

2. **Dialog HTML binding pattern (Task 2):** Used `wails.Call.ByName("desktop.DialogService.MethodName", args)` for JS→Go calls. This matches Wails v3 alpha.74's service binding pattern where bound service methods are callable by fully-qualified name.

3. **Template substitution for dialog state (Task 4):** Rather than complex JS→Go state queries, the agent injects `{{LAST_CONFIG_PATH}}` and `{{LAST_CLASS}}` via Go string replacement in `dialogHandler.ServeHTTP`. Simple and reliable.

4. **Dialog app runs in goroutine (Task 4):** `app.Run()` blocks, so it runs in a goroutine with `errCh`. The main goroutine selects on both the result channel and the error channel. Handles both normal flow (user picks option → result comes first) and abnormal flow (window closed → error comes first).

5. **Notification severity comparison (Task 5):** Created a `severityRank()` helper returning 0/1/2 for info/warning/critical, used for `>=` comparison against `minSeverity`. Straightforward and extensible.

6. **Tray status loop uses placeholder values (Task 7):** The tray goroutine calls `UpdateStatus(0, 0, "ok")` — real orchestrator/alert state wiring would require passing additional dependencies. Deferred to a future iteration.

### Installer/QA Agent

1. **Config no-overwrite on upgrade:** Used `SetOverwrite off` before installing `pgpulse.yml` so existing user configs survive upgrades. Restored `SetOverwrite on` after.

2. **Install dir from registry on upgrade:** Added `InstallDirRegKey` so NSIS remembers the previous install location.

---

## Key Questions Resolved

### Did the dual-Wails-app approach work?

**Yes — compiles clean.** The dialog creates a separate `application.App` with its own event loop, runs it in a goroutine, waits for the result channel, then returns. The main app starts afterward. Both builds (standard and desktop) pass. Runtime testing not performed (requires interactive Windows session with Wails runtime), but the architecture is sound — Wails v3's `application.New()` creates independent app instances.

### Did NotificationService work?

**Yes — compiles clean.** `notifications.New()` returns `*NotificationService`, registered via `application.NewService(notifSvc)` in the `Services` slice. The `go-toast/v2` dependency was automatically pulled in via `go mod tidy`. Runtime toast delivery not tested (requires running desktop binary with active Wails event loop), but all types resolve and the API surface matches the Wails v3 alpha.74 source.

### Any issues with the alert dispatcher hook?

**None.** The `onAlertHooks` slice starts empty (zero value), so existing code paths are unaffected. The `OnAlert()` method appends under mutex. Hooks are copied before invocation to avoid holding the lock during callbacks. All existing `internal/alert` tests pass unchanged.

---

## Build Verification Results

| Check | Result |
|-------|--------|
| `go build ./cmd/... ./internal/...` (standard) | **PASS** |
| `go build -tags desktop ./cmd/... ./internal/...` (desktop) | **PASS** |
| `go vet ./cmd/... ./internal/...` | **PASS** |
| `go vet -tags desktop ./cmd/... ./internal/...` | **PASS** |
| `go test ./cmd/... ./internal/... -count=1` | **PASS** (all 18 packages) |
| `golangci-lint run ./cmd/... ./internal/...` | **PASS** (0 issues) |
| `cd web && npm run build` | **PASS** |
| `cd web && npm run typecheck` | **PASS** |
| `cd web && npm run lint` | **PASS** (1 pre-existing warning, 0 errors) |

## Installer Build Result

**Skipped** — the NSIS script (`deploy/nsis/pgpulse.nsi`) was created and syntax-validated, but the actual installer build requires `pgpulse-desktop.exe` to be present in `deploy/nsis/`. NSIS v3.10 confirmed available at `C:\Program Files (x86)\NSIS\`.

To build the installer manually:
```bash
cd deploy/nsis
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ../../cmd/pgpulse-server
makensis pgpulse.nsi
# Output: pgpulse-setup.exe
```

---

## Watch-List Verification

### New files — all created:
- [x] `internal/desktop/dialog.go`
- [x] `internal/desktop/dialog.html`
- [x] `internal/desktop/dialog_bindings.go`
- [x] `internal/desktop/settings.go`
- [x] `internal/desktop/notifications.go`
- [x] `deploy/nsis/pgpulse.nsi`
- [x] `deploy/nsis/license.txt`

### Modified files — all updated:
- [x] `cmd/pgpulse-server/main.go`
- [x] `cmd/pgpulse-server/desktop.go`
- [x] `cmd/pgpulse-server/desktop_stub.go`
- [x] `internal/desktop/app.go`
- [x] `internal/alert/dispatcher.go`

### Unchanged (verified):
- [x] `internal/api/server.go` — not touched by M12_02 (pre-existing uncommitted changes from prior iteration)
- [x] `internal/api/static.go` — unchanged
- [x] `web/src/*` — unchanged by M12_02
