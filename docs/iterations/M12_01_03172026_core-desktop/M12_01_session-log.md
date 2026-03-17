# M12_01 Session Log — Core Desktop (Wails v3)

**Date:** 2026-03-17
**Iteration:** M12_01
**Commit:** bcea155e313e88c7171c94e932aee5881f1e0bd7
**Team:** 2 agents (Desktop/Backend + QA/Build), Opus team lead

---

## Agent Execution Time

| Agent | Duration | Tasks |
|-------|----------|-------|
| Desktop/Backend | ~15 min | Icons, Wails dep, internal/desktop/ package, desktop.go/stub, main.go mod |
| QA/Build | ~4.5 min | Standard + desktop builds, symbol checks, tests, lint, test stubs, mode flag, frontend |
| **Total wall-clock** | **~20 min** | (agents ran sequentially) |

---

## Files Created

| File | Lines | Build Tag | Purpose |
|------|-------|-----------|---------|
| `cmd/icongen/main.go` | 153 | — | One-shot icon generator (green/yellow/red circles + ICO) |
| `cmd/pgpulse-server/desktop.go` | 34 | `desktop` | `--mode` flag via `init()`, `GetDesktopMode()`, `RunDesktop()` |
| `cmd/pgpulse-server/desktop_stub.go` | 17 | `!desktop` | No-op stubs: `GetDesktopMode()` → "server", `RunDesktop()` → error |
| `internal/desktop/app.go` | 70 | `desktop` | `DesktopApp` struct, Wails v3 app + window (1440x900), close→hide |
| `internal/desktop/tray.go` | 68 | `desktop` | `SystemTray`, context menu, `UpdateStatus()` icon swap |
| `internal/desktop/icon.go` | 17 | `desktop` | `//go:embed` for 4 icon assets |
| `internal/desktop/stub.go` | 3 | `!desktop` | Empty package for non-desktop builds |
| `internal/desktop/app_test.go` | 14 | `desktop` | Compile-check stub (GUI requires runtime) |
| `internal/desktop/tray_test.go` | 17 | `desktop` | Compile-check stub (GUI requires runtime) |
| `internal/desktop/icons/pgpulse-tray.png` | — | — | 64x64 green circle |
| `internal/desktop/icons/pgpulse-tray-warning.png` | — | — | 64x64 yellow circle |
| `internal/desktop/icons/pgpulse-tray-critical.png` | — | — | 64x64 red circle |
| `internal/desktop/icons/pgpulse.ico` | — | — | 32x32 PNG-in-ICO |

## Files Modified

| File | Change | Lines |
|------|--------|-------|
| `cmd/pgpulse-server/main.go` | Added `io/fs` + `web` imports; desktop mode check in `startServer()` | +15 / -1 |
| `go.mod` | Added `wailsapp/wails/v3 v3.0.0-alpha.74`; Go bumped 1.24→1.25 | +25 / -3 |
| `go.sum` | Wails transitive dependencies | +124 / -6 |

---

## Decisions Made by Agents (Deviations from Design)

| Decision | Design Doc Said | Agent Did | Rationale |
|----------|----------------|-----------|-----------|
| Icon location | `assets/icons/` | `internal/desktop/icons/` | Go `//go:embed` cannot use `..` paths to escape package directory; placing icons under the package allows direct embed |
| Flag registration | Explicit `RegisterDesktopFlags()` call before `flag.Parse()` | `init()` function in `desktop.go` | Runs automatically before `flag.Parse()` — zero changes to main.go's flag block |
| `RunDesktop` signature | `RunDesktop(router, assets, cfg)` with `*config.Config` | `RunDesktop(router, assets)` without config | `DesktopApp.Options` doesn't use config; removed unused parameter to satisfy linter |
| `AssetOptions.FS` | Pass `fs.FS` to `AssetOptions.FS` | Only `AssetOptions.Handler` set | Chi router already serves static files via SPA fallback handler; FS field unnecessary |
| Wails v3 API | `app.NewWebviewWindow()` | `app.Window.NewWithOptions()` | Alpha.74 uses `WindowManager` pattern — adapted to actual API |
| System tray API | Direct constructor | `app.SystemTray.New()` | Alpha.74 uses `SystemTrayManager` — adapted to actual API |
| Go version | 1.24.0 | 1.25.0 in go.mod | Wails v3 alpha.74 requires Go 1.25; Go 1.25 is installed on dev machine |

---

## Issues Encountered

| Issue | Severity | Resolution |
|-------|----------|------------|
| Wails v3 API drift from design doc | Low | Agent adapted to actual alpha.74 API (WindowManager, SystemTrayManager patterns) |
| `errcheck` lint failures in `cmd/icongen/main.go` | Low | QA agent fixed 4 unchecked return values (`f.Close()`, `binary.Write()`) |
| Go embed path constraint | Low | Moved icons from `assets/icons/` to `internal/desktop/icons/` |
| No blocking issues | — | — |

---

## Build Verification Results

| Check | Result |
|-------|--------|
| `go build ./cmd/... ./internal/...` (standard) | PASS |
| `go build -tags desktop ./cmd/... ./internal/...` | PASS |
| Symbol check: standard binary has NO Wails symbols | PASS (0 matches) |
| Symbol check: desktop binary HAS Wails symbols | PASS (many matches) |
| `go test ./cmd/... ./internal/... -count=1` | PASS (all packages) |
| `golangci-lint run ./cmd/... ./internal/...` | PASS (after errcheck fix) |
| `cd web && npm run build && npm run typecheck && npm run lint` | PASS |
| Mode flag: standard binary rejects `--mode=desktop` | PASS |
| Mode flag: desktop binary `--mode=server` → no GUI | PASS |
| Unchanged: `internal/api/server.go`, `internal/api/static.go`, `web/src/*` | PASS |

---

## Commits

| Hash | Message |
|------|---------|
| `5b7078f` | docs(M12_01): requirements, design, team-prompt, checklist — Wails v3 desktop shell |
| `bcea155` | feat(desktop): M12_01 — Wails v3 desktop shell with system tray |
