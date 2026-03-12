# Session: 2026-03-11 — M8_11b OS Metrics Frontend UI

## Goal

Make the OS Metrics section data-driven — render whatever `os.*` data is available from the database, regardless of collection method (SQL or agent). Replace the `agent_url` gate with metric-existence check. Add 4 stat cards and 4 time-series charts.

## Agent Team Configuration

- Team Lead: Opus 4.6
- Specialists: Frontend Agent (single agent — frontend-only change)
- Duration: ~5 minutes (light iteration — team-prompt only, no requirements/design docs)
- Build result: all clean (npm build/lint/typecheck, go build)

## What Changed

### Frontend Files Modified

| File | Change |
|------|--------|
| OS Metrics section component | Removed `agent_url` config gate; replaced with data-driven rendering |
| OS Metrics section component | Added 4 stat cards: Memory (used/total GB), CPU (usr+sys%), Load (1m/5m/15m), Disk I/O Util |
| OS Metrics section component | Added 4 time-series charts: Memory Usage (stacked area), CPU Usage (stacked area), Load Average (3 lines), Disk I/O (read/write KB/s) |
| OS Metrics section component | Added subtle info badge for agent-only extras when agent not configured |

### Hotfix Applied Mid-Session

**Disk I/O chart showed "No data available"** — the team-prompt specified metric keys (`os.disk.read_bytes_per_sec`) that didn't match what the OSSQLCollector actually emits (`os.diskstat.read_kb`). Fixed by updating the chart to use actual keys:

| Chart Expected | Actual Key |
|---|---|
| `os.disk.read_bytes_per_sec` | `os.diskstat.read_kb` |
| `os.disk.write_bytes_per_sec` | `os.diskstat.write_kb` |
| `os.disk.io_util_pct` | `os.diskstat.util_pct` |

**Root cause:** Design doc specified idealized metric keys; the Claude Code agent implementing M8_11 used different names matching the agent's `ParseDiskStats` output. The Codebase Digest (once re-uploaded) will prevent this class of mismatch in future iterations.

## Deployment Notes

### PG 16 Permission Gotcha (discovered during M8_11 deploy)

Ubuntu 24.04's PostgreSQL 16 package ships with EXECUTE on `pg_read_file` revoked from PUBLIC (`proacl = {postgres=X/postgres}`). The `pg_read_server_files` role grants file path access checks but NOT function call permission.

**Required grants per monitored instance:**
```sql
GRANT pg_read_server_files TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint, boolean) TO pgpulse_monitor;
```

**Diagnostic:**
```sql
SELECT proname, proacl FROM pg_proc WHERE proname = 'pg_read_file';
-- NULL proacl = default (PUBLIC has EXECUTE) → role membership alone is sufficient
-- {postgres=X/postgres} = restricted → EXECUTE grants needed
```

Confirmed on: Ubuntu 24.04, PostgreSQL 16.13. Likely affects any hardened PG 16+ install.

## Architecture Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D-M8_11b-1 | Data-driven OS section (check for os.* metrics, not agent_url) | SQL collector produces identical metric keys; frontend shouldn't care about data source |
| D-M8_11b-2 | 4 stat cards + 4 charts (full scope) | Server is barely utilized (0.5% CPU, 5% memory) — all 4 charts prove the data pipeline works end-to-end |
| D-M8_11b-3 | Metric naming standardization parked for competitive research | Current keys work; renaming touches every layer (collector, storage, API, frontend, alerts, ML). Better to define standard after studying PMM/Datadog/pganalyze naming conventions |

## Verified on Demo VM

```
Memory:       3.3 GB / 62.9 GB (5.3%)
CPU:          0.5% (usr 0.2% + sys 0.2%)
Load Average: 0.00 (5m: 0.02, 15m: 0.07)
Disk I/O:     Read/Write KB/s chart populated, util 0.0%
```

All 4 stat cards rendering. All 4 time-series charts populated with live data. Tooltips functional. Dark theme consistent.

## Not Done / Queued

- [ ] Metric naming standardization → competitive research session (PMM, Datadog, Zabbix, pgCenter, pg_profile, pganalyze)
- [ ] M6 OS Agent → after competitive research
- [ ] Deferred UI work: session kill UI, settings diff UI, query plan viewer UI, forecast overlay on remaining charts
- [ ] Startup diagnostic: detect SQLSTATE 42501 on pg_read_file and log specific grant suggestion
- [ ] Memory chart: show Cached/Buffers breakdown (currently Available + Used only visible)
