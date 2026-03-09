# M8_09 Hotfix — Production Crash + Collector Bugs

**Iteration:** M8_09 (hotfix)
**Date:** 2026-03-09
**Priority:** CRITICAL — demo environment completely broken

---

## Context

PGPulse deployed to Ubuntu 24 demo server with PG 16. The production Vite build
crashes on both Fleet Overview and Server Detail pages. Additionally, several
collector queries fail against PG 16.13.

## Bug 1: CRITICAL — `Cannot access 'h' before initialization`

**Symptom:** Both Fleet Overview and Server Detail crash with:
```
ReferenceError: Cannot access 'h' before initialization
    at index-B3xtER02.js:196:90726
    at Array.map (<anonymous>)
    at index-B3xtER02.js:196:90451
    at Object.Ba [as useMemo]
```

**Root cause:** Circular import in the production bundle. A component references
another component (or hook/utility) that hasn't been initialized yet due to
import ordering in the minified build. This works in Vite dev mode (ESM lazy
resolution) but breaks in the production bundle (eagerly evaluated).

**How to find it:** The crash is inside a `useMemo` with an `Array.map` call.
This is likely in Fleet Overview's instance card rendering or ServerDetail's
tab/section rendering. Search for `useMemo` calls that `.map()` over data and
reference imported components or functions inside the map body.

**Common patterns that cause this:**
1. Barrel exports (`index.ts` re-exporting from files that import each other)
2. A hook file importing a component that imports the same hook
3. A utility function importing from a file that imports from the utility

**How to fix:**
1. Run `npx madge --circular web/src/**/*.{ts,tsx}` to detect circular imports
   (install with `npm install -D madge`)
2. If madge isn't available, manually trace imports from the crashing component
3. Break the cycle by inlining the shared dependency, or moving it to a separate file
4. VERIFY: `npm run build` succeeds AND `node -e "import('./web/dist/assets/index-*.js')"` doesn't throw

**Verification:** After fixing, rebuild and test:
```bash
cd web && npm run build && npm run typecheck && npm run lint
# Then visually test by serving the dist:
npx serve dist -l 3000
# Open http://localhost:3000 and verify Fleet Overview + Server Detail load
```

## Bug 2: HIGH — CSP blocks Google Fonts

**Symptom:** Console warning:
```
Loading the stylesheet 'https://fonts.googleapis.com/css2?...' violates
Content Security Policy directive: "style-src 'self' 'unsafe-inline'"
```

**Fix:** In the security headers middleware (likely `internal/api/middleware.go` or
`internal/api/server.go`), add `fonts.googleapis.com` and `fonts.gstatic.com` to the CSP:

```go
// Before:
"style-src 'self' 'unsafe-inline'"

// After:
"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com"
"font-src 'self' https://fonts.gstatic.com"
```

Alternatively, if the fonts aren't essential, remove the Google Fonts `<link>` from
`index.html` and use system fonts via Tailwind's `font-sans` stack.

## Bug 3: MEDIUM — `stanullfrac` column missing (PG 16+)

**Log:** `bloat query: ERROR: column "stanullfrac" does not exist (SQLSTATE 42703)`

**Cause:** PostgreSQL 17 renamed `pg_statistic.stanullfrac` to `pg_statistic.stanullpercent`.
But PG 16.13 still has `stanullfrac`. This error might actually be referencing
`pg_stats.null_frac` vs a direct `pg_statistic` query. Check the bloat CTE in
`internal/collector/database.go` for the exact column name used and which catalog
view/table it queries.

**Fix:** Use the `pg_stats` view (which has `null_frac` across all versions) instead
of directly querying `pg_statistic`. Or add a version gate if `pg_statistic` is used
directly.

## Bug 4: MEDIUM — `received_lsn` column missing on replica

**Log:** `replication_status replica: ERROR: column "received_lsn" does not exist`

**Cause:** The WAL receiver query on the replica uses `received_lsn` but the correct
column name in `pg_stat_wal_receiver` is `flushed_lsn` or `received_lsn` depending
on PG version. In PG 16 the column exists but may be named differently, OR the query
is running against the wrong view.

**Fix:** Check `internal/collector/replication.go` for the WAL receiver query. Verify
column names against PG 16 documentation:
```sql
SELECT * FROM pg_stat_wal_receiver LIMIT 0;  -- check actual columns
```

## Bug 5: MEDIUM — sequences `pct_used` NULL scan

**Log:** `sequences scan: can't scan into dest[5] (col: pct_used): cannot scan NULL into *float64`

**Fix:** In `internal/collector/database.go`, the sequences sub-collector calculates
a `pct_used` that can be NULL (when `max_value` is NULL or sequence has no cap).
Change the Go struct field from `float64` to `*float64`, or wrap the SQL in `COALESCE`:
```sql
COALESCE(last_value::numeric / NULLIF(max_value, 0) * 100, 0) AS pct_used
```

## Bug 6: LOW — `server.port` config ignored

**Log:** Server starts on `:8080` regardless of `server.port: 8989` in config.

**Fix:** Check `cmd/pgpulse-server/main.go` or `internal/config/config.go` for where
the listen address is constructed. The `server.port` field may not be wired to the
HTTP server's `ListenAndServe` call.

## Bug 7: LOW — 404 on `/activity/statements`

**Console:** `404 on /api/v1/instances/*/activity/statements?sort=total_time&limit=25`

**Fix:** Either the route path or query params don't match between frontend and backend.
Check `internal/api/server.go` for the statements route registration and compare with
what `useStatements` hook sends.

---

## Team Prompt

Read CLAUDE.md for full project context.

This is a **hotfix iteration** — fix the 7 bugs listed in this document. Prioritize
Bug 1 (the crash) above everything else. Use a **2-specialist team** (Frontend Agent
handles bugs 1-2, Collector Agent handles bugs 3-6, both handle bug 7).

Create a team of 2 specialists:

### FRONTEND AGENT

**Bug 1 (CRITICAL): Fix circular import crash**
1. Install madge: `npm install -D madge`
2. Run: `npx madge --circular src/**/*.{ts,tsx}` from the `web/` directory
3. If madge can't resolve it, manually trace from the crash:
   - The crash is in a `useMemo` with `.map()` — likely in FleetOverview or ServerDetail
   - Search all files for `useMemo` containing `.map` that reference imported functions
   - Check if any hooks import components or vice versa
4. Break the circular dependency:
   - Move shared types/functions to a separate utility file
   - Inline small shared functions if they only have one consumer
   - Never import a component from inside a hook
5. **VERIFY**: `npm run build` succeeds, AND manually test by opening the built `dist/index.html`

**Bug 2: Fix CSP for Google Fonts**
- Either add Google Fonts domains to the CSP header in the Go backend middleware
- OR remove the Google Fonts `<link>` tag from `web/index.html` and rely on system fonts
- Recommend: keep the fonts, update the CSP — it's a one-line Go change

**Bug 7: Fix 404 on statements endpoint**
- Compare the URL in `useStatements` hook with the route in `server.go`
- Fix whichever side has the wrong path

### COLLECTOR AGENT

**Bug 3: Fix bloat query for PG 16**
- Find the bloat CTE in `internal/collector/database.go`
- Check which column it references (`stanullfrac` vs `null_frac`)
- Fix to use `pg_stats.null_frac` (works on all PG versions)

**Bug 4: Fix WAL receiver column name**
- Find the WAL receiver query in `internal/collector/replication.go`
- Run `SELECT * FROM pg_stat_wal_receiver LIMIT 0` on PG 16 to see actual column names
- Fix the column name

**Bug 5: Fix sequences NULL scan**
- Find the sequences query in `internal/collector/database.go`
- Add `COALESCE(..., 0)` around the `pct_used` calculation
- Or change the Go field to `*float64` and handle nil

**Bug 6: Fix server.port config**
- Find where the listen address is constructed in `main.go` or `config.go`
- Wire `cfg.Server.Port` into the `http.ListenAndServe` call

### VERIFICATION

After all fixes:
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

ALL checks must pass. Zero errors on lint and typecheck.
