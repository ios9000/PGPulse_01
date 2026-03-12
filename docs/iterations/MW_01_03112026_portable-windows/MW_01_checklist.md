# MW_01 — Developer Checklist

**Iteration:** MW_01
**Date:** 2026-03-11

---

## Pre-Flight

### 1. Copy docs to iteration folder

```bash
cd C:\Users\Archer\Projects\PGPulse_01

mkdir -p docs/iterations/MW_01_03112026_portable-windows
cp /path/to/MW_01_requirements.md docs/iterations/MW_01_03112026_portable-windows/MW_01_requirements.md
cp /path/to/MW_01_design.md docs/iterations/MW_01_03112026_portable-windows/MW_01_design.md
cp /path/to/MW_01_team-prompt.md docs/iterations/MW_01_03112026_portable-windows/MW_01_team-prompt.md
cp /path/to/MW_01_checklist.md docs/iterations/MW_01_03112026_portable-windows/MW_01_checklist.md
```

### 2. Update CLAUDE.md current iteration

Edit `.claude/CLAUDE.md` — update the "Current Iteration" section to:

```
## Current Iteration: MW_01 — Portable Windows Executable + Live Mode

Docs: docs/iterations/MW_01_03112026_portable-windows/
Team: 3 specialists (Storage & Config, API & Auth, Frontend & Build)
```

### 3. Commit planning docs

```bash
git add docs/iterations/MW_01_03112026_portable-windows/
git add .claude/CLAUDE.md
git commit -m "docs: MW_01 planning — portable Windows executable + live mode"
git push
```

---

## Pre-Flight Issue Check

Before spawning agents, verify these potential issues:

### Issue 1: MetricStore interface location

The design assumes `MetricStore` is in `internal/collector/collector.go`. Verify:

```bash
grep -rn "MetricStore" internal/collector/collector.go
grep -rn "MetricStore" internal/storage/
```

If `MetricStore` is in `internal/storage/` instead, the MemoryStore import path is fine (same package). If it's in `internal/collector/`, the MemoryStore in `internal/storage/` will need to import it — verify no circular dependency.

### Issue 2: APIServer constructor signature

The design adds `liveMode bool` and `memoryRetention time.Duration` to APIServer. Check the current constructor:

```bash
grep -n "func New.*APIServer\|func NewAPIServer" internal/api/server.go
```

Specialists A and B both touch `main.go` and `server.go`. Since they're in parallel worktrees, Specialist A passes the new params, and Specialist B adds the fields. The merge must be done carefully.

### Issue 3: AlertHistoryStore interface

Verify the exact interface before Specialist B writes the null implementation:

```bash
grep -A 20 "AlertHistoryStore" internal/alert/store.go
```

### Issue 4: Auth middleware imports

Verify `UserContextKey`, `User` struct, and `RoleAdmin` are accessible from `internal/auth/`:

```bash
grep -rn "UserContextKey\|type User struct\|RoleAdmin" internal/auth/
```

### Issue 5: Frontend header component name

```bash
ls web/src/components/
grep -rl "header\|Header\|layout\|Layout\|Navbar\|navbar" web/src/components/ --include="*.tsx"
```

Record the actual component name for Specialist C.

---

## Agent Spawn

### Specialist A — Storage & Config

```
Read docs/iterations/MW_01_03112026_portable-windows/MW_01_team-prompt.md, section "Specialist A — Storage & Config". Execute all tasks in order. Start with Step 0 (read existing code first).
```

### Specialist B — API & Auth

```
Read docs/iterations/MW_01_03112026_portable-windows/MW_01_team-prompt.md, section "Specialist B — API & Auth". Execute all tasks in order. Start with Step 0 (read existing code first).
```

### Specialist C — Frontend & Build

```
Read docs/iterations/MW_01_03112026_portable-windows/MW_01_team-prompt.md, section "Specialist C — Frontend & Build". Execute all tasks in order. Start with Step 0 (read existing code first).
```

---

## Watch List — Expected Files

After all agents complete, verify these files exist:

### New Files
- [ ] `internal/storage/memory.go`
- [ ] `internal/storage/memory_test.go`
- [ ] `internal/alert/nullstore.go`
- [ ] `web/src/hooks/useSystemMode.ts`
- [ ] `scripts/build-release.sh`
- [ ] `config.sample.yaml`
- [ ] `README.txt`

### Modified Files
- [ ] `cmd/pgpulse-server/main.go` — CLI flags, config merge, live mode wiring
- [ ] `internal/auth/middleware.go` — AuthMode, NewAuthMiddleware
- [ ] `internal/auth/middleware_test.go` — new test cases
- [ ] `internal/api/server.go` — liveMode fields, /system/mode endpoint
- [ ] `web/src/App.tsx` — SystemModeProvider wrapper
- [ ] Header component — Live Mode badge
- [ ] Forecast/ML components — conditional rendering
- [ ] Login/auth component — redirect when auth disabled

---

## Build Verification

### Step 1: Frontend

```bash
cd web
npm run build
npm run typecheck
npm run lint
cd ..
```

### Step 2: Backend build

```bash
go build ./cmd/pgpulse-server
```

### Step 3: Backend tests

```bash
go test ./cmd/... ./internal/... -count=1 -race
```

### Step 4: Lint

```bash
golangci-lint run ./cmd/... ./internal/...
```

### Step 5: Cross-compile check

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o build/pgpulse-server.exe ./cmd/pgpulse-server
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/pgpulse-server ./cmd/pgpulse-server
```

### Step 6: Smoke test (local)

```bash
# Live mode — no config, single target
./build/pgpulse-server --target=postgres://pgpulse_monitor:pgpulse_monitor_demo@185.159.111.139:5432/postgres

# Open http://localhost:8989
# Verify: no login screen, Live Mode badge visible, charts populating
# Verify: ML/forecast controls NOT visible
# Ctrl+C to stop
```

---

## Commit Clean Build

```bash
git add -A
git status  # Review changes

git commit -m "feat(MW_01): portable mode — MemoryStore, CLI flags, auth bypass, live mode UI

- Add in-memory MetricStore with configurable retention (default 2h)
- Add CLI flags: --target, --listen, --history, --no-auth, --config
- Auto-detect live mode when no storage DSN configured
- Auto-skip auth on localhost, --no-auth flag for remote
- Add /api/v1/system/mode endpoint
- Add Live Mode badge in frontend header
- Gate ML/forecast UI on persistent mode
- Add NullAlertHistoryStore for live mode
- Add cross-compile build script and ZIP packaging
- Add config.sample.yaml and README.txt"

git push
```

---

## Deploy to Demo VM (Optional)

```bash
# Build and deploy
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/pgpulse-server ./cmd/pgpulse-server
scp build/pgpulse-server ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/

# On VM:
sudo systemctl stop pgpulse
sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server /opt/pgpulse/bin/pgpulse-server
sudo chmod +x /opt/pgpulse/bin/pgpulse-server
sudo systemctl start pgpulse
```

---

## Wrap-Up

### 1. Create session-log

Create `docs/iterations/MW_01_03112026_portable-windows/MW_01_session-log.md` with:
- Goals and outcomes
- Agent activity summary
- Files created/modified
- Build verification results
- Issues encountered and resolutions
- Commits made

### 2. Create handoff document

Create `docs/HANDOFF_MW_01_to_next.md` — self-contained for next chat.

### 3. Update roadmap

Edit `docs/roadmap.md`:
- Mark MW_01 as complete
- Update next steps (competitive research, metric naming, M6)

### 4. Update CHANGELOG

Edit `docs/CHANGELOG.md`:
- Add MW_01 entry with feature list

### 5. Generate CODEBASE_DIGEST

Instruct Claude Code to regenerate `docs/CODEBASE_DIGEST.md` per `.claude/rules/codebase-digest.md`.

### 6. Upload to Project Knowledge

Upload updated `docs/CODEBASE_DIGEST.md` to Claude.ai Project Knowledge.

### 7. Final commit

```bash
git add docs/
git commit -m "docs: MW_01 session-log, handoff, roadmap update, codebase digest"
git push
```
