# MW_01b — Session Log

**Iteration:** MW_01b — Bugfixes
**Date:** 2026-03-12
**Chat:** Claude Code (single agent, no team decomposition)

---

## Bugs Fixed

| # | Bug | Commit | Fix Summary |
|---|-----|--------|-------------|
| 1 | Default port 8080 instead of 8989 | `3e3705f` fix(config): handle empty log level and improve CLI instance name | Set cfg.Server.Port = 8989 in empty-config path |
| 2 | build-release.sh fails on Windows (no `zip`) | `abc202a` fix(build): add PowerShell fallback for ZIP on Windows | OS detection + PowerShell Compress-Archive fallback |
| 3 | Cache Hit Ratio stat card "--" / chart "No data" | `a4f960a` fix(collector): align cache hit ratio metric key with frontend | Aligned metric key between collector and UI |
| 4 | Replication Lag Y-axis "819.2 undefined" | `5321375` fix(frontend): handle edge cases in formatBytes for chart axis labels | Fixed formatBytes to handle undefined/edge cases |
| 5 | Instance display name from DSN (cosmetic) | `3e3705f` (same as #1) | Improved CLI instance name display |

---

## Build Verification

```
cd web && npm run build && npm run typecheck && npm run lint   ✅
cd .. && go build ./cmd/pgpulse-server                        ✅
go test ./cmd/... ./internal/... -count=1                     ✅
golangci-lint run                                             ✅
```

All 4 commits clean. No regressions.
