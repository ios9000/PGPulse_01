# PGPulse M12_02 — Developer Checklist

**Iteration:** M12_02 — UX + Installer
**Date:** 2026-03-17

---

## Step 1 — Create iteration folder and copy docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M12_02_03172026_ux-installer
```

Copy downloaded files:

```bash
cp /path/to/downloaded/M12_02_design.md docs/iterations/M12_02_03172026_ux-installer/design.md
cp /path/to/downloaded/M12_02_team-prompt.md docs/iterations/M12_02_03172026_ux-installer/team-prompt.md
cp /path/to/downloaded/M12_02_checklist.md docs/iterations/M12_02_03172026_ux-installer/checklist.md
```

Also copy the handoff from M12_01 into M12_02's folder for reference:

```bash
cp docs/iterations/M12_01_03172026_core-desktop/HANDOFF_M12_01_to_M12_02.md docs/iterations/M12_02_03172026_ux-installer/
```

---

## Step 2 — Update CLAUDE.md current iteration

Edit `CLAUDE.md` and update:

```
Current iteration: M12_02 — UX + Installer (connection dialog, OS notifications, NSIS)
```

---

## Step 3 — Update Project Knowledge (if needed)

If CODEBASE_DIGEST.md was regenerated after M12_01, re-upload it now. Otherwise skip — re-upload after M12_02 completes.

---

## Step 4 — Commit docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/M12_02_03172026_ux-installer/
git add CLAUDE.md
git commit -m "docs(M12_02): design, team-prompt, checklist — connection dialog, notifications, NSIS"
```

---

## Step 5 — Pre-flight: Verify NSIS availability

```bash
makensis /VERSION
```

If not found, install NSIS:
1. Download from https://nsis.sourceforge.io/Download
2. Install to default location
3. Add `C:\Program Files (x86)\NSIS\Bin` to PATH
4. Restart Git Bash, verify: `makensis /VERSION`

If NSIS can't be installed now, the installer build step is optional — the `.nsi` script will still be created.

---

## Step 6 — Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste or point to:
```
Read docs/iterations/M12_02_03172026_ux-installer/team-prompt.md and execute.
```

---

## Step 7 — Watch-list of expected files

Monitor agent progress:

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
- [ ] `internal/api/server.go` — no changes
- [ ] `internal/api/static.go` — no changes
- [ ] `web/src/` — zero file changes

---

## Step 8 — Build verification

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Frontend
cd web && npm run build && npm run typecheck && npm run lint && cd ..

# Standard server build (no desktop, no Wails)
go build ./cmd/pgpulse-server

# Desktop build (console)
go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server

# Desktop build (GUI subsystem — for installer)
go build -tags desktop -ldflags="-s -w -H windowsgui" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...

# Symbol verification — standard binary clean
go tool nm pgpulse-server.exe 2>/dev/null | grep -ci wails
# Expected: 0

# NSIS installer (if makensis available)
makensis deploy/nsis/pgpulse.nsi
# Output: deploy/nsis/PGPulse-1.0.0-Setup.exe (or similar)
```

### Optional: Manual testing on Windows

```bash
# Test desktop mode with dialog (no config)
./pgpulse-desktop.exe --mode=desktop
# Expected: connection dialog appears

# Test desktop mode with config
./pgpulse-desktop.exe --mode=desktop --config=configs/pgpulse.example.yml
# Expected: main window opens directly (no dialog)

# Test server mode unchanged
./pgpulse-server.exe --config=configs/pgpulse.example.yml
# Expected: headless HTTP server, no GUI

# Test installer (if built)
# Run PGPulse-1.0.0-Setup.exe → install → verify shortcuts → run from Start Menu → uninstall
```

---

## Step 9 — Commit clean build

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add -A
git status

git commit -m "feat(desktop): M12_02 — connection dialog, OS notifications, NSIS installer

- Add connection dialog for first-launch config selection (file picker / DSN / last-used)
- Add OS notification bridge: alert events → Windows Toast via Wails NotificationService
- Add NSIS installer: shortcuts, auto-start option, Add/Remove Programs
- Wire tray icon status to live alert severity (10s polling)
- Add OnAlert hook to AlertDispatcher for desktop notification bridge
- Persist last config path in %APPDATA%/PGPulse/settings.json"
```

---

## Step 10 — Wrap-up

### 10a — Regenerate codebase digest

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Prompt:
```
Regenerate docs/CODEBASE_DIGEST.md per codebase-digest.md rules.
```

### 10b — Session log

Produce `docs/iterations/M12_02_03172026_ux-installer/M12_02_session-log.md` with:
- Agent execution time
- Files created/modified
- Decisions made by agents (deviations from design)
- Issues encountered (especially: did dual-Wails-app work? Did NotificationService work?)
- Build verification results
- Installer build result (success/skipped)

### 10c — Handoff document

Produce `docs/iterations/M12_02_03172026_ux-installer/HANDOFF_M12_to_M14.md` with:
- M12 complete summary (M12_01 + M12_02)
- Codebase state post-M12
- What M14 (RCA Engine) builds on
- Known issues
- Updated roadmap

### 10d — Commit wrap-up docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/M12_02_03172026_ux-installer/M12_02_session-log.md
git add docs/iterations/M12_02_03172026_ux-installer/HANDOFF_M12_to_M14.md
git add docs/CODEBASE_DIGEST.md
git commit -m "docs(M12_02): session-log, handoff to M14, codebase digest update"
```

### 10e — Upload CODEBASE_DIGEST.md to Project Knowledge

Re-upload `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge.

### 10f — Push

```bash
git push origin master
```

---

## Summary of all 10 steps

| # | Step | Status |
|---|------|--------|
| 1 | Copy docs to iteration folder | ☐ |
| 2 | Update CLAUDE.md current iteration | ☐ |
| 3 | Update Project Knowledge (if needed) | ☐ |
| 4 | Commit docs | ☐ |
| 5 | Pre-flight: verify NSIS availability | ☐ |
| 6 | Spawn agents | ☐ |
| 7 | Watch-list check | ☐ |
| 8 | Build verification | ☐ |
| 9 | Commit clean build | ☐ |
| 10 | Wrap-up: session-log + handoff + digest + push | ☐ |
