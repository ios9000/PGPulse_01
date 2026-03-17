# PGPulse M12_01 — Team Prompt

**Iteration:** M12_01 — Core Desktop (Wails v3)
**Date:** 2026-03-17
**Agent team size:** 2 (Desktop/Backend + QA/Build)
**Team lead model:** Opus

---

## Context

Read these files before starting:
- `docs/iterations/M12_01_03172026_core-desktop/design.md` — full architecture, code samples, integration points
- `docs/iterations/M12_01_03172026_core-desktop/requirements.md` — feature requirements and decisions
- `CLAUDE.md` — project conventions, build commands, module ownership
- `docs/CODEBASE_DIGEST.md` — current file inventory, interfaces, API endpoints

PGPulse is a PostgreSQL monitoring platform. We are wrapping the existing web UI in a native Windows desktop window using Wails v3. The existing chi router and React frontend remain **100% unchanged**. Wails serves as a thin native shell.

---

## DO NOT RE-DISCUSS

These decisions are final. Do not revisit, question, or propose alternatives.

| Decision | Locked Value |
|----------|-------------|
| D300 | Wails **v3** (alpha) — `github.com/wailsapp/wails/v3` |
| D301 | **Thin wrapper** — chi router as `AssetOptions.Handler`. Do NOT create Wails Go→JS bindings. Do NOT run `wails3 generate bindings`. |
| D302 | **Windows-only** — no macOS/Linux desktop code |
| D303 | **Build-tag gating** — ALL Wails code behind `//go:build desktop`. ALL stub code behind `//go:build !desktop`. |
| Frontend | **ZERO changes** to any file under `web/src/`. The React app works identically through Wails' asset handler. |
| embed.FS | Reuse the existing `web/dist` embed from `internal/api/static.go`. Pass the same `embed.FS` to Wails `AssetOptions.FS`. |
| Binding generation | **Not used**. No `wails3 generate bindings`. No `@wailsio/runtime` imports in frontend. |
| Build scope | Always `./cmd/... ./internal/...` — never `./...` (avoids scanning web/node_modules/) |
| Git branch | `master` (not main) |

---

## Team Structure

### Agent 1 — Desktop/Backend

**Owns:** `cmd/pgpulse-server/desktop.go`, `cmd/pgpulse-server/desktop_stub.go`, `internal/desktop/*`, `assets/icons/*`, modifications to `cmd/pgpulse-server/main.go` and `go.mod`

**Does NOT touch:** `internal/api/*`, `internal/collector/*`, `internal/alert/*`, `internal/auth/*`, `internal/storage/*`, `web/src/*`, or any other existing package.

**Tasks (in order):**

1. **Generate placeholder icon assets.** Create `assets/icons/` directory. Use Go's `image` package to generate:
   - `pgpulse-tray.png` — 64×64, green (#22c55e) circle on transparent background with "PP" text
   - `pgpulse-tray-warning.png` — 64×64, yellow (#eab308) circle with "PP" text
   - `pgpulse-tray-critical.png` — 64×64, red (#ef4444) circle with "PP" text
   - `pgpulse.ico` — Generate a valid ICO file with 32×32 icon (use PNG-in-ICO format)
   - Write a one-shot `cmd/icongen/main.go` tool to create these, run it, then commit the PNGs. The tool itself can be deleted or kept — your call.

2. **Add Wails v3 dependency.** Run:
   ```bash
   go get github.com/wailsapp/wails/v3@latest
   go mod tidy
   ```
   Verify `go.mod` includes the Wails v3 module.

3. **Create `internal/desktop/icon.go`** with `//go:build desktop` tag. Embed all four icon files using `//go:embed`. Export as package-level `[]byte` vars: `IconTrayDefault`, `IconTrayWarning`, `IconTrayCritical`, `IconWindow`.

4. **Create `internal/desktop/app.go`** with `//go:build desktop` tag. Implement:
   - `Options` struct accepting: `chi.Router`, `fs.FS` (web assets), `*config.Config`
   - `DesktopApp` struct with `*application.App`, `*application.WebviewWindow`, `*SystemTray`
   - `NewDesktopApp(opts Options) *DesktopApp` — creates Wails application, creates main window with chi router as `AssetOptions.Handler` and assets FS as `AssetOptions.FS`, sets window title to "PGPulse", size 1440×900, min size 1024×700
   - `Run() error` — calls `da.app.Run()`, blocks until quit
   - Window close event handler: intercept close → hide window (minimize to tray) instead of quitting. Use `window.On()` with the appropriate Wails v3 window closing event.
   - Set `URL: "/"` on the window so it loads `index.html` from the embedded assets on startup.

5. **Create `internal/desktop/tray.go`** with `//go:build desktop` tag. Implement:
   - `SystemTray` struct
   - `NewSystemTray(app *application.App, window *application.WebviewWindow) *SystemTray`
   - System tray icon using `IconTrayDefault`
   - Left-click handler: toggle window show/hide
   - Right-click context menu with items: "Show PGPulse" (shows + focuses window), separator, "Status: Monitoring..." (disabled info item), separator, "Quit" (calls `app.Quit()`)
   - `UpdateStatus(instanceCount int, alertCount int, maxSeverity string)` method that swaps the tray icon between green/yellow/red based on `maxSeverity` and updates the tooltip text.

6. **Create `cmd/pgpulse-server/desktop.go`** with `//go:build desktop` tag. Implement:
   - Package-level `var desktopMode string`
   - `RegisterDesktopFlags()` — registers `--mode` string flag with default "server"
   - `GetDesktopMode() string` — returns current mode value
   - `RunDesktop(router chi.Router, assets fs.FS, cfg *config.Config) error` — creates `DesktopApp` and calls `Run()`

7. **Create `cmd/pgpulse-server/desktop_stub.go`** with `//go:build !desktop` tag. Implement:
   - `RegisterDesktopFlags()` — no-op
   - `GetDesktopMode() string` — always returns `"server"`
   - `RunDesktop(router chi.Router, assets fs.FS, cfg *config.Config) error` — returns `fmt.Errorf("desktop mode not available: binary built without -tags desktop")`

8. **Modify `cmd/pgpulse-server/main.go`** — minimal changes:
   - Near the top of `main()`, after flag definitions: call `RegisterDesktopFlags()`
   - After `chiRouter := apiServer.Routes()` (or wherever the chi router is fully built) and before `http.ListenAndServe`: insert mode check:
     ```go
     if GetDesktopMode() == "desktop" {
         if err := RunDesktop(chiRouter, webDistFS, cfg); err != nil {
             slog.Error("desktop mode failed", "error", err)
             os.Exit(1)
         }
         return
     }
     ```
   - Find where the `embed.FS` for `web/dist` is declared (look in `internal/api/static.go` for `//go:embed all:dist`) — the same FS variable must be accessible to pass to `RunDesktop`. If it's unexported, either export it or declare a second embed in main.go. **Preferred:** Export the existing one from `internal/api/` package or pass it through the `APIServer` struct.
   - **CRITICAL:** Keep changes to main.go minimal. Do not refactor existing code. Add the mode branch, nothing more.

9. **Build and test desktop mode:**
   ```bash
   cd web && npm run build && cd ..
   go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server
   ```
   If on Windows, test `pgpulse-server.exe --mode=desktop --config=configs/pgpulse.example.yml` to verify the window opens and loads the UI.

---

### Agent 2 — QA/Build

**Owns:** Test files, build verification, lint compliance.

**Does NOT touch:** Production code (except test files).

**Tasks (in order):**

1. **Verify standard build is unchanged:**
   ```bash
   go build ./cmd/... ./internal/...
   ```
   Must succeed. Must NOT require CGO. Must NOT import Wails.

2. **Verify desktop build succeeds:**
   ```bash
   go build -tags desktop ./cmd/... ./internal/...
   ```
   Must succeed.

3. **Symbol check — standard binary must not contain Wails:**
   ```bash
   go build -o /tmp/pgpulse-server-standard ./cmd/pgpulse-server
   go tool nm /tmp/pgpulse-server-standard | grep -i wails
   ```
   Expected: zero matches.

4. **Symbol check — desktop binary must contain Wails:**
   ```bash
   go build -tags desktop -o /tmp/pgpulse-server-desktop ./cmd/pgpulse-server
   go tool nm /tmp/pgpulse-server-desktop | grep -i wails
   ```
   Expected: many matches.

5. **Test suite:**
   ```bash
   go test ./cmd/... ./internal/... -count=1
   ```
   All existing tests must pass. New desktop package tests may be stubs (build-tag gated).

6. **Lint:**
   ```bash
   golangci-lint run ./cmd/... ./internal/...
   ```
   Must pass with zero issues.

7. **Create test stubs** for `internal/desktop/`:
   - `internal/desktop/app_test.go` — `//go:build desktop` — `TestNewDesktopApp` verifying Options are stored correctly
   - `internal/desktop/tray_test.go` — `//go:build desktop` — `TestUpdateStatus` verifying icon selection logic (may need to mock Wails types — if mocking is impractical, document as manual-test-only)

8. **Verify mode flag behavior:**
   - Build without desktop tag → run with `--mode=desktop` → expect error message about missing build tag
   - Build with desktop tag → run with `--mode=server` → expect normal server startup (may fail due to missing config, that's fine — verify it does NOT open a Wails window)

9. **Frontend build verification:**
   ```bash
   cd web && npm run build && npm run typecheck && npm run lint && cd ..
   ```
   Must pass — confirms zero frontend changes.

10. **Commit clean build:**
    ```bash
    git add -A
    git commit -m "feat(desktop): M12_01 — Wails v3 desktop shell with system tray

    - Add Wails v3 dependency (build-tag gated: //go:build desktop)
    - Create internal/desktop/ package: app, tray, icon management
    - Add --mode=desktop|server flag to pgpulse-server
    - Chi router serves as Wails AssetOptions.Handler (zero frontend changes)
    - System tray with show/hide toggle, status menu, severity-colored icons
    - Standard build (no tags) remains unchanged — no Wails symbols in binary
    - Placeholder icons: green/yellow/red tray icons + window ICO"
    ```

---

## Build Commands Reference

```bash
# Frontend build (must run first — produces web/dist/)
cd web && npm run build && cd ..

# Standard server build (unchanged)
go build ./cmd/pgpulse-server

# Desktop build (Windows)
go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server

# Desktop build with GUI subsystem (no console window)
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...

# Cross-compile server for Linux (existing, unchanged)
export GOOS=linux && export GOARCH=amd64 && export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED
```

---

## Coordination Notes

- Desktop/Backend agent works first on icons + dependency + internal/desktop/ package. QA agent can start build verification as soon as go.mod is updated.
- Desktop/Backend agent modifies main.go last (after internal/desktop/ compiles cleanly).
- QA agent runs full verification suite after main.go changes are complete.
- **If Wails v3 API differs from design doc examples** (alpha API may have changed), adapt to actual API. The design doc shows the intended integration pattern — the exact struct/method names may vary. Check Wails v3 source and examples.
- **If embed.FS access is problematic** (e.g., the existing embed is in a `static.go` init block that can't be easily exported), declare a parallel `//go:embed` in `main.go` or `desktop.go`. Duplicating the embed directive is acceptable — Go deduplicates the data in the binary.

---

## Watch-List (Expected Files After Completion)

**New files:**
- `cmd/pgpulse-server/desktop.go`
- `cmd/pgpulse-server/desktop_stub.go`
- `internal/desktop/app.go`
- `internal/desktop/tray.go`
- `internal/desktop/icon.go`
- `internal/desktop/app_test.go`
- `internal/desktop/tray_test.go`
- `assets/icons/pgpulse-tray.png`
- `assets/icons/pgpulse-tray-warning.png`
- `assets/icons/pgpulse-tray-critical.png`
- `assets/icons/pgpulse.ico`

**Modified files:**
- `cmd/pgpulse-server/main.go` (~15-25 lines added)
- `go.mod` (Wails v3 dependency)
- `go.sum`

**Unchanged (verify explicitly):**
- `internal/api/server.go`
- `internal/api/static.go`
- ALL files under `web/src/`
