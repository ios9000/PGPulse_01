# PGPulse — Restore Context

**Last updated:** 2026-03-17
**Current milestone:** M12_01 complete, M12_02 next

This is an emergency recovery document. For full context, see:
- `docs/save-points/LATEST.md` — comprehensive project snapshot
- `docs/iterations/M12_01_03172026_core-desktop/HANDOFF_M12_01_to_M12_02.md` — transition to next sub-iteration
- `.claude/CLAUDE.md` — project rules, interfaces, module ownership

## Quick Status

| Item | Value |
|------|-------|
| Go module | github.com/ios9000/PGPulse_01 |
| Go version | 1.25.0 |
| Last commit | bcea155 |
| All tests | pass |
| golangci-lint | 0 issues |
| TypeScript | 0 errors |
| Deployed | Ubuntu 24 / PG 16.13 (185.159.111.139) |
| Milestones done | M0-M8, REM_01, M9_01, M_UX_01, M10_01, M11_01, M11_02, M12_01 |
| Next | M12_02 — UX + Installer (connection dialog, OS notifications, NSIS) |
| PGAM queries ported | ~70/76 |
| REST API endpoints | 55 |
| Alert rules | 23 |
| Collectors | 25 |
| Desktop | Wails v3 alpha.74, build-tag gated |

## How to Continue

1. Read `docs/save-points/LATEST.md`
2. Read `docs/iterations/M12_01_03172026_core-desktop/HANDOFF_M12_01_to_M12_02.md`
3. Read `.claude/CLAUDE.md`
4. Start M12_02 work

## Build Commands

```bash
# Never use go test ./... — it scans web/node_modules/
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run

# Desktop build (Windows)
go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server
```
