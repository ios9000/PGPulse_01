# MW_01 — Session Log

**Iteration:** MW_01 — Portable Windows Executable + Live Mode
**Date:** 2026-03-11 → 2026-03-12
**Chat:** Claude.ai (planning) + Claude Code (implementation)
**Duration:** Planning ~2h, Agent execution 14m 43s, Smoke testing + bugfixes ~30m

---

## Goals

1. ✅ Add in-memory MetricStore (MemoryStore) with configurable retention
2. ✅ Add CLI flag parsing (--target, --listen, --history, --no-auth, --config)
3. ✅ Auto-detect live mode when no storage DSN configured
4. ✅ Auth bypass on localhost + --no-auth flag
5. ✅ /api/v1/system/mode endpoint
6. ✅ Live Mode badge in frontend header
7. ✅ Gate ML/forecast/plan/settings UI on persistent mode
8. ✅ NullAlertHistoryStore for live mode
9. ✅ Cross-compile build script (Windows + Linux)
10. ✅ config.sample.yaml + README.txt
11. ✅ ZIP packaging for distribution

---

## Agent Activity

### Team: 3 Specialists (parallel)

| Specialist | Scope | Status |
|------------|-------|--------|
| A — Storage & Config | MemoryStore, CLI flags, config merge, live mode wiring | ✅ Complete |
| B — API & Auth | AuthMode middleware, /system/mode, NullAlertHistoryStore | ✅ Complete |
| C — Frontend & Build | useSystemMode hook, Live Mode badge, forecast gating, build script | ✅ Complete |

Total agent time: 14m 43s (all green on first run)

### Pre-Flight Corrections Applied

6 corrections identified via pre-flight grep, delivered as MW_01_corrections.md:
1. Auth system uses Claims (not User) in context
2. RequireAuth takes 2 params (jwtSvc + errorWriter)
3. APIServer uses api.New() factory — added setter pattern instead
4. AlertHistoryStore has 5 methods (not 2)
5. NullAlertHistoryStore wired as non-nil to prevent panics
6. Frontend targets confirmed (TopBar.tsx, TimeSeriesChart.tsx)

---

## Files Created (7)

| File | Lines | Purpose |
|------|-------|---------|
| internal/storage/memory.go | ~200 | MemoryStore — in-memory MetricStore with expiry goroutine |
| internal/storage/memory_test.go | ~250 | 9 unit tests (write, query, filter, expiry, concurrent) |
| internal/alert/nullstore.go | ~35 | NullAlertHistoryStore — no-op for live mode (5 methods) |
| web/src/hooks/useSystemMode.tsx | ~40 | React context + hook for live/persistent mode |
| scripts/build-release.sh | ~80 | Cross-compile script (Windows + Linux + ZIP) |
| config.sample.yaml | ~40 | Annotated sample config |
| README.txt | ~60 | Quick-start guide, CLI flags reference |

## Files Modified (9+)

| File | Changes |
|------|---------|
| cmd/pgpulse-server/main.go | CLI flags, config merge, MemoryStore wiring, synthesizeCLIInstance(), DSN synthesis, live mode detection |
| internal/auth/middleware.go | AuthMode enum, NewAuthMiddleware (AuthDisabled injects implicit Claims) |
| internal/auth/middleware_test.go | 2 new tests: disabled mode + enabled mode |
| internal/api/server.go | liveMode/memoryRetention/authMode fields, SetLiveMode()/SetAuthMode() setters, handleSystemMode endpoint, auth-disabled route group |
| internal/api/auth.go | handleLogin guard for auth-disabled mode |
| internal/api/*_test.go (3 files) | Updated api.New() calls for new constructor signature |
| web/src/App.tsx | Wrapped with SystemModeProvider |
| web/src/components/layout/TopBar.tsx | Live Mode badge (animated blue pill with tooltip) |
| web/src/pages/ServerDetail.tsx | Plan/Settings tabs hidden in live mode |
| web/src/stores/authStore.ts | initialize() probes /auth/me for auth-disabled mode |

---

## Smoke Test Results

### Build Verification — All Pass
- go build ./cmd/pgpulse-server ✅
- go test ./cmd/... ./internal/... ✅ (47+ test suites)
- golangci-lint run ✅ (0 issues)
- npm run build ✅
- npm run typecheck ✅
- npm run lint ✅ (1 benign fast-refresh warning)
- GOOS=windows cross-compile ✅
- GOOS=linux cross-compile ✅

### Live Mode Smoke Test
- pgpulse-server.exe --target=... --config=config.sample.yaml
- Live mode detected ✅ (log: "starting in live mode storage=memory retention=2h0m0s")
- No login screen ✅ (auth auto-disabled on localhost)
- /api/v1/system/mode returns {"mode":"live"} ✅
- Live Mode badge visible in TopBar ✅
- Dashboard renders with cli-target instance ✅
- Charts show loading spinners (waiting for data collection) ✅

### Bugs Found During Smoke Test (3)
1. **MaxConns=0** — synthesizeCLIInstance didn't set MaxConns → pgxpool refused. **Fixed by Claude Code.**
2. **Port 8080 not 8989** — empty config.Config{} defaults port to 0, falls back to 8080 somewhere. **Known, deferred to MW_01b.**
3. **Auth endpoints missing** — /auth/refresh returned 404 in auth-disabled mode, frontend redirected to login. **Fixed by Claude Code.**

### Distribution
- pgpulse-dev-windows-amd64.zip (6.7 MB) — built and ready
- pgpulse-dev-linux-amd64.zip (6.6 MB) — built and ready
- ZIP sent to colleague for DBA testing

---

## Commits

1. Planning docs: `docs: MW_01 planning — portable Windows executable + live mode`
2. Implementation: `feat(MW_01): portable mode — MemoryStore, CLI flags, auth bypass, live mode UI`
3. Bugfixes: MaxConns default + auth-disabled routes (Claude Code commit)

---

## Decisions Made

| ID | Decision | Rationale |
|----|----------|-----------|
| D-MW-1 | In-memory ring buffer (MemoryStore) | Zero dependencies, perfect for diagnostic sessions |
| D-MW-2 | 2h default retention | ~65K points, ~60-70 MB RAM, configurable via --history |
| D-MW-3 | CLI flags > YAML > defaults | DBA-friendly quick start |
| D-MW-4 | --target for single instance, YAML for multi | Clean separation of portable vs persistent |
| D-MW-5 | Auth auto-skip on localhost | Zero friction for local diagnostic use |
| D-MW-6 | No storage DSN → MemoryStore auto | Live mode is the default, persistent is opt-in |
| D-MW-7 | ZIP distribution (exe + config + readme) | Simplest possible packaging |
| D-MW-8 | Setter pattern for APIServer extensions | Consistent with existing SetPlanStore/SetMLDetector |

---

## Known Issues Remaining

| Issue | Status | Notes |
|-------|--------|-------|
| Port defaults to 8080 not 8989 | **Open** | Fix in MW_01b — set cfg.Server.Port=8989 in empty config path |
| build-release.sh uses `zip` (not on Git Bash) | **Workaround** | Use PowerShell Compress-Archive on Windows |
| Instance name shows raw DSN host (e.g., "1localhost") | **Minor** | extractHostPort parses OK, display cosmetic only |
| Cache Hit Ratio metric name mismatch | Open | Pre-existing from earlier iterations |
| `c.command_desc` SQL bug in cluster progress | Open | Pre-existing, PG16 column name |
| `002_timescaledb.sql` migration skip warning | Open | Pre-existing |
