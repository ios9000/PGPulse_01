# PGPulse — Restore Context

**Last updated:** 2026-03-27
**Current milestone:** M14_04 complete, M15 next

This is an emergency recovery document. For full context, see:
- `docs/save-points/LATEST.md` — comprehensive project snapshot
- `.claude/CLAUDE.md` — project rules, interfaces, module ownership

## Quick Status

| Item | Value |
|------|-------|
| Go module | github.com/ios9000/PGPulse_01 |
| Go version | 1.25.0 |
| All tests | pass |
| golangci-lint | 0 issues |
| TypeScript | 0 errors |
| Deployed | Ubuntu 24 / PG 16.13 (185.159.111.139) |
| Milestones done | M0-M8, REM_01, M9_01, M_UX_01, M10_01, M11_01, M11_02, M12_01, M14_01-M14_04 |
| Next | M12_02 — UX + Installer (connection dialog, OS notifications, NSIS) |
| PGAM queries ported | ~70/76 |
| REST API endpoints | 82 |
| Alert rules | 23 |
| Remediation rules | 25 |
| Seed playbooks | 10 |
| Collectors | 25 |
| RCA causal chains | 20 |
| Desktop | Wails v3 alpha.74, build-tag gated |

## What Was Just Completed (M14)

M14 — RCA + Guided Remediation (4 sub-iterations):
- **M14_01**: RCA correlation engine — 20 causal chains, 9-step analysis algorithm, dual anomaly source
- **M14_02**: RCA UI — incidents list, detail with timeline, causal graph visualization
- **M14_03**: Expansion — threshold hardening, WhileEffective semantics, statement diff, RCA→Adviser bridge
- **M14_04**: Guided Remediation Playbooks — 4-tier execution safety, 10 seed playbooks, wizard UI, 19 API endpoints

## How to Continue

1. Read `docs/save-points/LATEST.md`
2. Read `.claude/CLAUDE.md`
3. Start next work (M12_02, M9, or M10)

## Build Commands

```bash
# Never use go test ./... — it scans web/node_modules/
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run

# Desktop build (Windows)
go build -tags desktop -ldflags="-s -w" ./cmd/pgpulse-server
```
