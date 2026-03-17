# PGPulse M12_01 — Developer Checklist

**Iteration:** M12_01 — Core Desktop (Wails v3)
**Date:** 2026-03-17

---

## Step 1 — Create iteration folder and copy docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/M12_01_03172026_core-desktop
```

Copy the following files into `docs/iterations/M12_01_03172026_core-desktop/`:
- `M12_requirements.md` → `requirements.md`
- `M12_01_design.md` → `design.md`
- `M12_01_team-prompt.md` → `team-prompt.md`
- `M12_01_checklist.md` → `checklist.md`

```bash
cp /path/to/downloaded/M12_requirements.md docs/iterations/M12_01_03172026_core-desktop/requirements.md
cp /path/to/downloaded/M12_01_design.md docs/iterations/M12_01_03172026_core-desktop/design.md
cp /path/to/downloaded/M12_01_team-prompt.md docs/iterations/M12_01_03172026_core-desktop/team-prompt.md
cp /path/to/downloaded/M12_01_checklist.md docs/iterations/M12_01_03172026_core-desktop/checklist.md
```

---

## Step 2 — Update CLAUDE.md current iteration

Edit `CLAUDE.md` and update the current iteration line:

```
Current iteration: M12_01 — Core Desktop (Wails v3)
```

---

## Step 3 — Update Project Knowledge (if needed)

CODEBASE_DIGEST.md is current as of M11_02. No re-upload needed before M12_01 starts. Re-upload **after** M12_01 completes (new files in `internal/desktop/`, modified `main.go`).

---

## Step 4 — Commit docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/M12_01_03172026_core-desktop/
git add CLAUDE.md
git commit -m "docs(M12_01): requirements, design, team-prompt, checklist — Wails v3 desktop shell"
```

---

## Step 5 — Spawn agents

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Paste the team-prompt contents or point to:
```
Read docs/iterations/M12_01_03172026_core-desktop/team-prompt.md and execute.
```

---

## Step 6 — Watch-list of expected files

Monitor agent progress. These files should appear:

**New files:**
- [ ] `cmd/pgpulse-server/desktop.go`
- [ ] `cmd/pgpulse-server/desktop_stub.go`
- [ ] `internal/desktop/app.go`
- [ ] `internal/desktop/tray.go`
- [ ] `internal/desktop/icon.go`
- [ ] `internal/desktop/app_test.go`
- [ ] `internal/desktop/tray_test.go`
- [ ] `assets/icons/pgpulse-tray.png`
- [ ] `assets/icons/pgpulse-tray-warning.png`
- [ ] `assets/icons/pgpulse-tray-critical.png`
- [ ] `assets/icons/pgpulse.ico`

**Modified files:**
- [ ] `cmd/pgpulse-server/main.go` (~15-25 lines: mode flag + desktop branch)
- [ ] `go.mod` (Wails v3 dependency added)
- [ ] `go.sum` (updated)

**Unchanged (verify):**
- [ ] `internal/api/server.go` — no changes
- [ ] `internal/api/static.go` — no changes
- [ ] `web/src/` — zero file changes

---

## Step 7 — Build verification

Run these commands after agents complete. All must pass.

```bash
cd C:\Users\Archer\Projects\PGPulse_01

# Frontend build
cd web && npm run build && npm run typecheck && npm run lint && cd ..

# Standard server build (no desktop, no CGO)
go build ./cmd/pgpulse-server

# Desktop build
go build -tags desktop -ldflags="-s -w" -o pgpulse-desktop.exe ./cmd/pgpulse-server

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...

# Symbol verification — standard binary must NOT contain Wails
go tool nm pgpulse-server.exe 2>/dev/null | grep -ci wails
# Expected: 0

# Symbol verification — desktop binary MUST contain Wails
go tool nm pgpulse-desktop.exe 2>/dev/null | grep -ci wails
# Expected: > 0
```

### Optional: Test desktop mode on Windows

```bash
# Test desktop binary opens native window
./pgpulse-desktop.exe --mode=desktop --config=configs/pgpulse.example.yml

# Test standard binary rejects desktop mode
./pgpulse-server.exe --mode=desktop
# Expected: error "desktop mode not available: binary built without -tags desktop"
```

---

## Step 8 — Commit clean build

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add -A
git status

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

## Step 9 — Wrap-up

### 9a — Regenerate codebase digest

```bash
cd C:\Users\Archer\Projects\PGPulse_01
claude --model claude-opus-4-6
```

Prompt:
```
Regenerate docs/CODEBASE_DIGEST.md per codebase-digest.md rules.
```

### 9b — Session log

Produce `docs/iterations/M12_01_03172026_core-desktop/M12_01_session-log.md` with:
- Agent execution time
- Files created/modified
- Decisions made by agents (if any deviations from design)
- Issues encountered
- Build verification results

### 9c — Handoff document

Produce `docs/iterations/M12_01_03172026_core-desktop/HANDOFF_M12_01_to_M12_02.md` with:
- What was completed
- Codebase state
- What M12_02 needs to build on (connection dialog, notifications, NSIS installer)
- Known issues

### 9d — Commit wrap-up docs

```bash
cd C:\Users\Archer\Projects\PGPulse_01

git add docs/iterations/M12_01_03172026_core-desktop/M12_01_session-log.md
git add docs/iterations/M12_01_03172026_core-desktop/HANDOFF_M12_01_to_M12_02.md
git add docs/CODEBASE_DIGEST.md
git commit -m "docs(M12_01): session-log, handoff, codebase digest update"
```

### 9e — Upload CODEBASE_DIGEST.md to Project Knowledge

Re-upload `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge (replaces old version).

### 9f — Push

```bash
git push origin master
```

---

## Summary of all 9 steps

| # | Step | Status |
|---|------|--------|
| 1 | Copy docs to iteration folder | ☐ |
| 2 | Update CLAUDE.md current iteration | ☐ |
| 3 | Update Project Knowledge (skip — current) | ☐ |
| 4 | Commit docs | ☐ |
| 5 | Spawn agents | ☐ |
| 6 | Watch-list check | ☐ |
| 7 | Build verification | ☐ |
| 8 | Commit clean build | ☐ |
| 9 | Wrap-up: session-log + handoff + digest + push | ☐ |
