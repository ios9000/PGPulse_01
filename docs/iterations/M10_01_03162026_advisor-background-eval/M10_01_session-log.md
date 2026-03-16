# Session: 2026-03-16 — M10_01 Advisor Auto-Population

## Goal

Add background remediation evaluation so the Advisor page auto-populates without manual Diagnose clicks. Add "Create Alert Rule" promotion button, status filters, sidebar badge, and auto-refresh.

## Agent Configuration

- **Planning:** Claude.ai (Opus 4.6) — architecture, decisions, deliverables
- **Implementation:** Claude Code (Opus 4.6) — 2 parallel agents (Backend + Frontend)
- **Duration:** ~18 minutes implementation
- **Scope:** 3 new files, ~15 modified files

## Decisions

| ID | Decision |
|---|---|
| D1 | Configurable interval via YAML, default 5min |
| D2 | Results go to Advisor page with "promote to alert" option |
| D3 | Live mode: Diagnose-on-demand only, no background eval |
| D4 | Manual "Create Alert Rule" button per recommendation |
| D5 | Keep history with timestamps + retention cleanup |
| D6 | New config section: `remediation.enabled`, `remediation.background_interval`, `remediation.retention_days` |

## What Was Done

### Task A: Background Evaluation Worker

- Created `internal/remediation/background.go` — `BackgroundEvaluator` with ticker-based goroutine
- Runs `Diagnose()` for each active instance on configurable interval
- Resolves stale recommendations, cleans old per retention
- Logs evaluation cycle summary (instances, recommendations, duration)
- Created `internal/remediation/background_test.go`

### Task B: Migration 014

- Created `migrations/014_remediation_status.sql` — adds `status`, `evaluated_at`, `resolved_at` columns + indexes

### Task C: Config

- Added `RemediationConfig` struct to `internal/config/config.go`
- Fields: `enabled` (bool), `background_interval` (duration), `retention_days` (int)
- Defaults in `load.go`: enabled=false, interval=5m, retention=30d

### Task D: Wiring in main.go

- BackgroundEvaluator created and started when `cfg.Remediation.Enabled && persistentStore != nil`
- Uses `ml.NewDBInstanceLister(pgPool)` for instance listing
- Debug config log added during troubleshooting (can remove later)

### Task E: PGStore Updates

- Added `ResolveStale()` to `RecommendationStore` interface + PGStore + NullStore
- `Write()` updated with upsert logic (update evaluated_at if exists, reactivate if resolved)
- Status filter added to `ListAll()` and `ListByInstance()` via `ListOptions.Status`

### Task F: API Response Enhancement

- Status query parameter on recommendation list endpoints (`?status=active`)
- Response includes `status`, `evaluated_at`, `resolved_at`, `metric_key`, `metric_value`

### Task G1: Advisor Auto-Refresh

- 30s refetch interval on recommendations query
- "Last evaluated: X minutes ago" display with relative time formatting

### Task G2: "Create Alert Rule" Button

- Button on AdvisorRow, permission-gated to dba+ role
- Opens RuleFormModal pre-filled with metric key, threshold, severity mapping
- Uses existing `POST /api/v1/alerts/rules` endpoint

### Task G3: Sidebar Badge

- Red pill badge on Advisor nav item showing active recommendation count
- Hidden when count is 0

### Task G4: TypeScript Model Updates

- Added `status`, `evaluated_at`, `resolved_at` fields to Recommendation type

### Task G5: Advisor Filters Enhancement

- Status filter: Active / Resolved / Acknowledged / All
- Default: Active

## Files Changed

| File | Change |
|------|--------|
| `internal/remediation/background.go` | **NEW** — BackgroundEvaluator worker |
| `internal/remediation/background_test.go` | **NEW** — Worker tests |
| `migrations/014_remediation_status.sql` | **NEW** — Status + evaluation columns |
| `internal/remediation/store.go` | ResolveStale method on interface |
| `internal/remediation/pgstore.go` | ResolveStale + upsert + status filter |
| `internal/remediation/nullstore.go` | ResolveStale no-op |
| `internal/config/config.go` | RemediationConfig struct |
| `internal/config/load.go` | Defaults |
| `internal/api/remediation.go` | Status filter query param |
| `cmd/pgpulse-server/main.go` | BackgroundEvaluator wiring + debug log |
| `web/src/types/models.ts` | Recommendation type update |
| `web/src/pages/Advisor.tsx` | Auto-refresh, last evaluated |
| `web/src/components/advisor/AdvisorRow.tsx` | "Create Alert Rule" button |
| `web/src/components/advisor/AdvisorFilters.tsx` | Status filter |
| `web/src/components/layout/Sidebar.tsx` | Badge count |
| `web/src/hooks/useRecommendations.ts` | refetchInterval, status param |

## Build Status

- `go build` — clean
- `go test` — passing
- `golangci-lint` — 0 issues
- `npm run build` — clean
- `npm run typecheck` — clean
- `npm run lint` — clean

## Deployment Notes

- Config path on demo VM: `/opt/pgpulse/configs/pgpulse.yml`
- Added `remediation:` section with `enabled: true`, `background_interval: 5m`, `retention_days: 30`
- First deploy had no remediation logs — turned out old binary was still running (config updated but binary not replaced). Redeployed with correct binary.
- Background evaluator confirmed started: `"remediation background evaluator started" interval=5m0s retention_days=30`
- Advisor page auto-populated after first 5-minute cycle

## Verified on Demo

- [x] Background evaluator starts with correct interval
- [x] Advisor page auto-populates (no manual Diagnose)
- [x] "Last evaluated: 3 minutes ago" displays correctly
- [x] Sidebar badge shows active count (red "1")
- [x] Status filter defaults to Active
- [x] Acknowledge button works
- [x] "Enable track_io_timing for I/O analysis" recommendation auto-detected on production-replica
