# Session: 2026-03-11 — M8_11 OS Metrics via PostgreSQL

## Goal

Collect OS metrics (memory, CPU, load, disk I/O, uptime) through PostgreSQL's `pg_read_file('/proc/*')` using the existing monitoring connection — no agent binary needed. Also: establish the Codebase Digest system, update strategy doc to v2.4.

## Agent Team Configuration

- Team Lead: Opus 4.6
- Specialists: Collector Agent, QA & Review Agent
- Duration: ~10 minutes
- Build result: all clean (go build, go test, golangci-lint, npm build/lint/typecheck)

## PGAM Queries Ported

| PGAM # | Description | PGPulse Method | Target |
|--------|-------------|---------------|--------|
| Q4 | Hostname | `pg_read_file('/etc/hostname')` | server_info.go |
| Q5 | OS distribution | `pg_read_file('/etc/os-release')` | server_info.go |
| Q8 | OS uptime + load | `pg_read_file('/proc/uptime')` + `/proc/loadavg` | os_sql.go |
| Q12 | Total RAM | `pg_read_file('/proc/meminfo')` | os_sql.go |
| Q22 | Memory overcommit | `pg_read_file('/proc/meminfo')` | os_sql.go |
| Q23 | Full meminfo | `pg_read_file('/proc/meminfo')` | os_sql.go |
| Q26 | I/O stats | `pg_read_file('/proc/diskstats')` | os_sql.go |
| Q27 | I/O stats interval | Same — delta between 60s cycles | os_sql.go |

**Running total: ~77/76 PGAM queries ported** (exceeded original 76 — includes new metrics beyond PGAM's scope)

## Agent Activity Summary

### Collector Agent
- Created: `internal/collector/os_sql.go` (~230 lines)
- Modified: `internal/collector/server_info.go` (+hostname, +OS release)
- Modified: `internal/agent/osmetrics.go` (exported parsers for reuse)
- Modified: `internal/agent/osmetrics_linux.go` (updated references)
- Modified: `internal/config/config.go` (+OSMetricsConfig)
- Modified: `internal/orchestrator/runner.go` (+resolveOSMethod, +conditional registration)
- Modified: `internal/orchestrator/orchestrator.go` (passes globalOSMethod)
- Modified: `configs/pgpulse.example.yml` (+os_metrics section)

### QA & Review Agent
- Created: `internal/collector/os_sql_test.go` (~240 lines)
- Modified: `internal/agent/osmetrics_test.go` (updated exported names)
- All tests passing ✅
- golangci-lint: 0 issues ✅

## Architecture Decisions (Made by Team Lead)

| # | Decision | Rationale |
|---|----------|-----------|
| D-M8_11-8 | Reuse agent parsers (ParseMeminfo, ParseCPURaw, ParseDiskStats, CPUDelta, DiskStatsDelta) from internal/agent instead of duplicating | Zero code duplication; identical metric keys guaranteed; parsers already tested |
| D-M8_11-9 | Export previously-unexported agent types (CPURaw, DiskStatRaw) | Required for cross-package reuse; minimal API surface change |

**Design deviation from plan:** The design doc specified implementing parsers in os_sql.go. The agents correctly identified that `internal/agent/` already had battle-tested parsers and chose to reuse them by exporting the relevant types and functions. This is better than the design — less code, guaranteed key compatibility, single source of truth for parsing logic.

## Metrics Emitted by OSSQLCollector

### Memory (from /proc/meminfo)
- `os.memory.total_kb`, `os.memory.free_kb`, `os.memory.available_kb`
- `os.memory.buffers_kb`, `os.memory.cached_kb`

### Uptime (from /proc/uptime)
- `os.uptime.seconds`

### Load (from /proc/loadavg)
- `os.load.1m`, `os.load.5m`, `os.load.15m`

### CPU (from /proc/stat, stateful delta)
- `os.cpu.user_pct`, `os.cpu.system_pct`, `os.cpu.idle_pct`, `os.cpu.iowait_pct`

### Disk I/O (from /proc/diskstats, stateful delta, per device label)
- `os.disk.read_bytes_per_sec`, `os.disk.write_bytes_per_sec`
- `os.disk.read_iops`, `os.disk.write_iops`
- `os.disk.read_await_ms`, `os.disk.write_await_ms`
- `os.disk.io_util_pct`

### Server Info (from /etc/hostname, /etc/os-release — in server_info.go)
- `pgpulse.server.hostname` (label: hostname)
- `pgpulse.server.os` (label: os)

## Process Improvements Shipped

1. **Codebase Digest system** — `.claude/rules/codebase-digest.md` added, first digest to be generated this session
2. **Strategy doc v2.4** — 22 stale items fixed: bash bug marked resolved, 4-layer persistence, RBAC updated, pgxpool, OS SQL method, versions updated
3. **Step 0 pattern** — team-prompt instructed agents to inventory existing code before writing anything; agents used this to discover and reuse parsers

## Not Done / Next

- [ ] Generate first CODEBASE_DIGEST.md (Step 9 in checklist)
- [ ] Deploy to demo VM + grant pg_read_server_files
- [ ] Competitive research session: PMM, Datadog, Zabbix, pgCenter, pg_profile, pganalyze → feeds M6/M7
- [ ] M6: OS Agent (full agent binary — now informed by competitive research)

# M8_11 Session-Log Addendum — Deployment Finding

**Date:** 2026-03-11 (same session, post-deploy)

## PG 16 Permission Gotcha: pg_read_file EXECUTE Revoked from PUBLIC

### Symptom
OSSQLCollector logs `permission denied for function pg_read_file (SQLSTATE 42501)` on every `/proc` file, despite `pgpulse_monitor` being a member of `pg_read_server_files` with `inherit_option = t`.

### Root Cause
Ubuntu 24.04's PostgreSQL 16 package ships with EXECUTE privilege on `pg_read_file` **revoked from PUBLIC**:

```sql
SELECT proname, proacl FROM pg_proc WHERE proname = 'pg_read_file';
-- proacl = {postgres=X/postgres}  ← only superuser can call it
```

The `pg_read_server_files` predefined role grants the **file access security check** (which paths are allowed), but the **function call permission** is separate. When EXECUTE is revoked from PUBLIC, even members of `pg_read_server_files` can't call the function.

### Fix — Required Grants (per monitored instance)

```sql
-- Role membership (for file path access check)
GRANT pg_read_server_files TO pgpulse_monitor;

-- Function call permission (needed when EXECUTE revoked from PUBLIC)
GRANT EXECUTE ON FUNCTION pg_read_file(text) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint) TO pgpulse_monitor;
GRANT EXECUTE ON FUNCTION pg_read_file(text, bigint, bigint, boolean) TO pgpulse_monitor;
```

### Diagnostic

```sql
-- Check if EXECUTE is restricted
SELECT proname, proacl FROM pg_proc WHERE proname = 'pg_read_file';
-- NULL proacl = default (PUBLIC has EXECUTE) → role membership alone is sufficient
-- {postgres=X/postgres} = restricted → EXECUTE grants needed
```

### Scope
- **Confirmed on:** Ubuntu 24.04, PostgreSQL 16.13
- **Likely affects:** Any hardened PG 16+ install where DBA ran `REVOKE EXECUTE ON FUNCTION pg_read_file FROM PUBLIC`
- **Does NOT affect:** Default vanilla PG installs where proacl is NULL

### Action Items
1. Update deployment documentation to include EXECUTE grants alongside role membership
2. Consider adding a startup check in the collector: attempt `SELECT pg_read_file('/proc/uptime')` and log a specific diagnostic message if it fails with 42501, suggesting the EXECUTE grant
3. Add to known issues in next handoff document
