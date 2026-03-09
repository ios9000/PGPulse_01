# Session: 2026-03-09 — M8_07 Deferred UI + Small Fixes

## Goal

Ship frontend for two M8_02 backend features (plan capture history, temporal settings
snapshots), fix the application_name enrichment gap in session kill, and eliminate the
last lint error in the codebase.

## Agent Team Configuration

- **Team Lead:** Opus 4.6
- **Specialists:** Frontend Agent, QA Agent (2-specialist team)
- **Duration:** ~9 minutes

## Pre-Flight Result

Route verification confirmed all M8_02 handler routes were already registered in
`server.go`. No backend route changes needed.

## Files Created

| File | Purpose | Agent |
|------|---------|-------|
| `web/src/hooks/usePlanHistory.ts` | Hooks for plan list, detail, regressions, manual capture | Frontend |
| `web/src/components/PlanHistory.tsx` | All Plans / Regressions tabs, expandable rows with PlanNode trees, trigger badges | Frontend |
| `web/src/hooks/useSettingsTimeline.ts` | Hooks for snapshots, diff, pending restart, manual snapshot | Frontend |
| `web/src/components/SettingsTimeline.tsx` | Snapshot list, dual-dropdown compare, diff with colour-coded sections, permission-gated "Take Snapshot" | Frontend |

## Files Modified

| File | Change |
|------|--------|
| `internal/api/activity.go` | Added `ApplicationName` field to `LongTransaction` struct, `COALESCE(application_name, '')` in SQL, updated `Scan()` |
| `internal/plans/capture.go` | Added JSON tags for proper API serialization |
| `web/src/pages/Administration.tsx` | Moved `useState` above early return — fixes conditional hook violation |
| `web/src/pages/ServerDetail.tsx` | Added "Plan History" and "Settings Timeline" tabs |
| `web/src/components/server/LongTransactionsTable.tsx` | Passes `application_name` to `SessionActions` |
| `web/src/types/models.ts` | Added `application_name` to `LongTransaction` interface |

## Build Verification Results

| Check | Status |
|-------|--------|
| go build | PASS |
| go vet | PASS |
| go test | PASS (all packages) |
| golangci-lint | PASS (0 issues) |
| npm run lint | PASS (**0 errors** — first time in project history) |
| npm run typecheck | PASS (0 errors) |
| npm run build | PASS |

## Key Observations

1. **Routes already registered** — M8_02 Team Lead had wired them. The design doc's caution was warranted but unnecessary.
2. **JSON tags missing on capture.go** — Agent caught that `internal/plans/capture.go` lacked JSON serialization tags. Would have caused wrong field names in API responses. Good catch.
3. **0 lint errors achieved** — The Administration.tsx conditional hook fix eliminates the project's only remaining lint error.
4. **application_name uses COALESCE** — Handles NULL application_name from idle connections gracefully.
