# PGPulse — Restore Context

**Last updated:** 2026-03-10
**Current milestone:** M8 complete, M9 next

This is an emergency recovery document. For full context, see:
- `docs/save-points/LATEST.md` — comprehensive project snapshot
- `docs/iterations/HANDOFF_M8_to_M9.md` — transition to next milestone
- `.claude/CLAUDE.md` — project rules, interfaces, module ownership

## Quick Status

| Item | Value |
|------|-------|
| Go module | github.com/ios9000/PGPulse_01 |
| Go version | 1.24.0 |
| Last commit | e773c01 |
| All tests | pass (15 packages) |
| golangci-lint | 0 issues |
| TypeScript | 0 errors |
| Deployed | Ubuntu 24 / PG 16.13 (185.159.111.139) |
| Milestones done | M0-M8 (10 sub-iterations + 2 hotfixes) |
| Next | M9 — Reports & Export |
| PGAM queries ported | ~70/76 |
| REST API endpoints | 38+ |
| Alert rules | 23 |

## How to Continue

1. Read `docs/save-points/LATEST.md`
2. Read `docs/iterations/HANDOFF_M8_to_M9.md`
3. Read `.claude/CLAUDE.md`
4. Start M9 work

## Build Commands

```bash
# Never use go test ./... — it scans web/node_modules/
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```
