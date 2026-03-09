# Session: 2026-03-09 — M8_08 Logical Replication Monitoring

## Goal

Port PGAM query Q41 (logical replication sync status) to PGPulse using the M7 DBRunner
framework for per-database connections. Add API endpoint, frontend section, and alert rule.

## Agent Team Configuration

- **Team Lead:** Opus 4.6
- **Specialists:** Collector Agent, Frontend Agent, QA Agent (3-specialist team)
- **Duration:** ~10 minutes

## PGAM Query Ported

| Query # | Description | Target |
|---------|-------------|--------|
| Q41 | Logical replication sync tables (`pg_subscription_rel JOIN pg_subscription WHERE srsubstate <> 'r'`) | `internal/collector/database.go` (sub-collector) + `internal/api/logical_replication.go` (API) |

## Files Created

| File | Purpose | Agent |
|------|---------|-------|
| `internal/api/logical_replication.go` | API handler + response structs + per-DB connection logic | Collector |
| `web/src/hooks/useLogicalReplication.ts` | React Query hook with 30s refetch | Frontend |
| `web/src/components/server/LogicalReplicationSection.tsx` | UI section with 4 states (no subs, all synced, pending, error) | Frontend |

## Files Modified

| File | Change | Agent |
|------|--------|-------|
| `internal/collector/database.go` | Added `collectLogicalReplication` sub-collector (17th DB sub-collector) | Collector |
| `internal/api/server.go` | Registered route in both auth-enabled and auth-disabled sections | Collector |
| `internal/alert/rules.go` | Added `logical_repl_pending_sync` rule (disabled by default) | Collector |
| `internal/alert/rules_test.go` | Updated expected builtin rule count 21 → 22 | QA |
| `web/src/types/models.ts` | Added 4 new interfaces (LogicalReplicationResponse, SubscriptionStatus, PendingTable, SubscriptionStats) | Frontend |
| `web/src/pages/ServerDetail.tsx` | Added LogicalReplicationSection after ReplicationSection | Frontend |

## Build Verification Results

| Check | Status |
|-------|--------|
| go build | PASS |
| go vet | PASS |
| go test | PASS (after test count fix) |
| golangci-lint | PASS (0 issues) |
| npm run lint | PASS (0 errors) |
| npm run typecheck | PASS (0 errors) |
| npm run build | PASS |

## QA Fixes Applied

- `rules_test.go`: `TestBuiltinRulesCount` expected 21 → updated to 22 after adding the new alert rule

## Code Review Checks (All Passed)

- Parameterized SQL throughout
- Temporary connections closed in defer blocks
- PG 15+ version gate for `apply_error_count` / `sync_error_count`
- Graceful error handling when `pg_subscription` doesn't exist
- No `any` types in TypeScript
- Route in viewer permission group (read-only)
- Alert rule seeded with `Enabled: false`
- Sync state label mapping covers all states (i, d, s, f)

## Architecture Decisions

| Decision | Rationale |
|----------|-----------|
| Per-DB connection via SubstituteDatabase in API handler | Reuses existing M8_01 helper; no orchestrator changes needed |
| Sub-collector in database.go (not new file) | Consistent with M7's 16 existing DB sub-collectors in the same file |
| Alert rule disabled by default | Pending sync tables are normal during initial subscription setup; users enable and tune |
| 4 UI states (no subs / all synced / pending / error) | Covers all scenarios without misleading users |

## Query Porting Progress Update

- Q41 now ported → total ~70/76 PGAM queries ported
- Remaining ~6 are mostly VTB-internal (wait-event descriptions from internal DB) or pre-PG14 code paths
