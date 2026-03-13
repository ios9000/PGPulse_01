# MW_01b — Bugfixes (No Planning Docs Needed)

**Date:** 2026-03-12
**Type:** Bugfix-only, single agent, no design doc required
**Predecessor:** MW_01 (Portable Windows Executable + Live Mode)

---

## Context

Read these files first:
- `CLAUDE.md` — project state index
- `docs/CODEBASE_DIGEST.md` — Section 3 (Metric Keys), Section 5 (Frontend Component Map)
- `docs/iterations/HANDOFF_MW_01_to_next.md` — Known Issues table

## Bugs to Fix (5 items)

### Bug 1: Default port is 8080 instead of 8989 in live mode
**Symptom:** When running `pgpulse-server --target=DSN` with no config file, the server listens on :8080 instead of :8989.
**Root cause:** In `cmd/pgpulse-server/main.go`, when no config file is found, an empty `config.Config{}` is created. The Port field defaults to 0, and the fallback logic uses 8080 instead of 8989.
**Fix:** After creating the empty `cfg = config.Config{}`, set `cfg.Server.Port = 8989` (or wherever the port default is applied). Check how the config struct initializes and ensure the default matches the documented port.
**Verify:** `pgpulse-server --target=postgres://user:pass@localhost:5432/postgres` → should print "Listening on :8989"

### Bug 2: build-release.sh fails on Windows (no `zip` command)
**Symptom:** `scripts/build-release.sh` uses the `zip` CLI which doesn't exist in Git Bash on Windows.
**Fix:** Add OS detection. If running on Windows/MSYS/Git Bash, use PowerShell `Compress-Archive` as fallback:
```bash
if command -v zip &>/dev/null; then
    zip -j "$outfile" "${files[@]}"
else
    powershell -Command "Compress-Archive -Path '${files_csv}' -DestinationPath '$outfile' -Force"
fi
```
**Verify:** Run `scripts/build-release.sh` on Windows Git Bash — should produce both ZIP files without error.

### Bug 3: Cache Hit Ratio — stat card shows "--", chart shows "No data available"
**Symptom:** On the ServerDetail page, the Cache Hit Ratio stat card displays "--" and the time-series chart shows "No data available". See screenshot — all other metrics (Connections, Active Transactions) work fine.
**Root cause:** Metric key mismatch between what the collector writes and what the frontend queries. The collector writes `pgpulse.cache.hit_ratio_pct` (see `internal/collector/cache.go`). The frontend may be querying a different key.
**Investigation steps:**
1. Check `internal/collector/cache.go` — what exact metric key string is used in the MetricPoint?
2. Check `web/src/` — grep for "cache" in all .tsx/.ts files. Find which metric key the stat card component and the time-series chart component are requesting from the API.
3. Check the API handler — what metric key does `/api/v1/instances/{id}/metrics` expect?
4. Align all three: collector write key = API query key = frontend request key.
**Also check:** The stat card (KeyMetricsRow.tsx or similar) likely fetches latest metrics — is it using the same key as the chart?
**Verify:** After fix, Cache Hit Ratio stat card shows a percentage (e.g., "99.8%") and the chart shows a time-series line.

### Bug 4: Replication Lag chart Y-axis shows "819.2 undefined"
**Symptom:** The Replication Lag chart on ServerDetail renders values on the Y-axis as "819.2 undefined" instead of "819.2 B" or "819 bytes".
**Root cause:** The chart component's unit formatting function receives an undefined unit string for the replication lag metric. The metric key is `pgpulse.replication.lag.replay_bytes` (or similar `*_bytes` key).
**Investigation steps:**
1. Find the chart config for Replication Lag in `web/src/` — look at `TimeSeriesChart.tsx` or the ServerDetail page component.
2. Check how the unit is determined for each chart. Is it hardcoded per metric, or derived from the metric key suffix?
3. Find where "bytes" should be resolved but isn't — likely a missing entry in a unit mapping, or a metric key that doesn't match the expected pattern.
**Fix:** Ensure the Replication Lag chart has a proper unit formatter that converts bytes to human-readable format (B, KB, MB, GB).
**Verify:** Y-axis shows "819.2 B" or "819 B" or appropriate scaled unit.

### Bug 5 (Optional): Instance display name from DSN
**Symptom:** When using `--target=DSN`, the instance name in the sidebar shows the raw host:port from the DSN parsing, which is functional but ugly.
**Fix:** If the instance name looks like a raw host:port (no user-assigned name), show a friendlier display: `hostname:port` without the protocol prefix, or use the dbname if available.
**Priority:** Low — cosmetic only.

---

## Approach

This is a single-agent task. No team decomposition needed — one agent can fix all 5 bugs sequentially.

**Recommended order:** Bug 1 (port) → Bug 3 (cache hit) → Bug 4 (repl lag unit) → Bug 2 (build script) → Bug 5 (optional cosmetic)

## Build Verification

After all fixes:
```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
```

## Commit Convention

One commit per bug fix:
```
fix(config): default port to 8989 when no config file present
fix(build): add PowerShell fallback for ZIP on Windows
fix(frontend): align cache hit ratio metric key between collector and UI
fix(frontend): resolve replication lag chart unit formatting
fix(frontend): improve instance display name from DSN
```

## Deploy & Verify on Demo VM

After all fixes pass locally:
```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'
```

Then check http://185.159.111.139:8989 — Cache Hit Ratio card and chart should show data, Replication Lag Y-axis should show proper units.
