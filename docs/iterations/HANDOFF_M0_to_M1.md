# PGPulse — Iteration Handoff: M0 → M1_01

> **Purpose:** This document contains EVERYTHING the next Claude.ai chat needs
> to begin M1_01 without re-discovery. Upload this file when starting the new chat.
> **Created:** 2026-02-25 (end of M0 session)

---

## DO NOT RE-DISCUSS

These decisions are final. The new chat should not revisit them:

1. **Stack**: Go 1.25.7, pgx v5.8.0, chi v5, koanf, slog, testcontainers-go
2. **Architecture**: Single binary, version-adaptive SQL, Collector interface pattern
3. **Module ownership**: Collector Agent owns internal/collector/* and internal/version/*
4. **Agent Teams bash bug**: Claude Code cannot run bash on Windows. Agents create files only. Developer runs go build/test/commit manually.
5. **Go module path**: `github.com/ios9000/PGPulse_01`
6. **Project path**: `C:\Users\Archer\Projects\PGPulse_01`
7. **PG version support**: 14, 15, 16, 17 (18 optional)
8. **Monitoring user**: pg_monitor role, never superuser
9. **OS metrics**: via Go agent (M6), NEVER via COPY TO PROGRAM

---

## What Exists After M0

### Repository: github.com/ios9000/PGPulse_01

```
cmd/pgpulse-server/main.go       ← placeholder
cmd/pgpulse-agent/main.go        ← placeholder
internal/collector/collector.go   ← interfaces (MetricPoint, Collector, MetricStore, AlertEvaluator)
internal/version/version.go      ← PGVersion struct, Detect(), AtLeast()
internal/version/gate.go         ← VersionRange, Gate, SQLVariant, Select()
configs/pgpulse.example.yml      ← full sample config
Makefile, Dockerfile, docker-compose.yml, .golangci.yml, CI pipeline
```

### Key Interface: internal/collector/collector.go

```go
type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error)
    Interval() time.Duration
}
```

### Key Interface: internal/version/version.go

```go
type PGVersion struct {
    Major int    // e.g., 16
    Minor int    // e.g., 4
    Num   int    // e.g., 160004
    Full  string // e.g., "16.4 (Ubuntu 16.4-1.pgdg22.04+1)"
}

func Detect(ctx context.Context, conn *pgx.Conn) (PGVersion, error)
func (v PGVersion) AtLeast(major, minor int) bool
```

### Key Interface: internal/version/gate.go

```go
type Gate struct {
    Name     string
    Variants []SQLVariant
}

func (g Gate) Select(v PGVersion) (string, bool)
```

---

## Next Task: M1_01 — Instance Metrics Collector

### Goal
Port PGAM queries 1–19 into Go collectors using the interfaces from M0.

### PGAM Queries 1–19 (from PGAM_FEATURE_AUDIT.md)

**Query 1 — PG Version** (ALREADY DONE in M0: internal/version/version.go)
```sql
SELECT version();
```

**Query 2 — Server Start Time**
```sql
SELECT pg_postmaster_start_time();
```

**Query 3 — Uptime** (computed in PHP, will compute in Go)
```sql
SELECT date_trunc('second', current_timestamp - pg_postmaster_start_time());
```

**Query 4 — Hostname via COPY TO PROGRAM** (SKIP — OS agent M6)
```sql
COPY (SELECT '') TO PROGRAM 'hostname > /tmp/WatchDog_hostname.log';
SELECT pg_read_file('/tmp/WatchDog_hostname.log');
```

**Query 5 — Top via COPY TO PROGRAM** (SKIP — OS agent M6)
```sql
COPY (SELECT '') TO PROGRAM 'top -b -n 1 | head -5 > /tmp/WatchDog_top.log';
```

**Query 6 — df via COPY TO PROGRAM** (SKIP — OS agent M6)
```sql
COPY (SELECT '') TO PROGRAM 'df -h > /tmp/WatchDog_df.log';
```

**Query 7 — iostat via COPY TO PROGRAM** (SKIP — OS agent M6)
```sql
COPY (SELECT '') TO PROGRAM 'iostat > /tmp/WatchDog_iostat.log';
```

**Query 8 — meminfo via COPY TO PROGRAM** (SKIP — OS agent M6)
```sql
COPY (SELECT '') TO PROGRAM 'cat /proc/meminfo > /tmp/WatchDog_meminfo.log';
```

**Query 9 — Recovery State**
```sql
SELECT pg_is_in_recovery();
```

**Query 10 — Backup State** (VERSION GATE: pg_is_in_backup() removed in PG 15)
```sql
-- PG 14 only:
SELECT pg_is_in_backup();
-- PG 15+: skip or return false
```

**Query 11 — Connection Count**
```sql
SELECT count(*) FROM pg_stat_activity;
```
Note: PGAM bug — counts its own connection. Fix: WHERE pid != pg_backend_pid()

**Query 12 — Max Connections**
```sql
SHOW max_connections;
```

**Query 13 — Superuser Reserved Connections**
```sql
SHOW superuser_reserved_connections;
```

**Query 14 — Global Cache Hit Ratio**
```sql
SELECT
  sum(blks_hit) * 100.0 / NULLIF(sum(blks_hit) + sum(blks_read), 0) AS ratio
FROM pg_stat_database;
```
Note: PGAM uses division without null guard. Fix: add NULLIF.

**Query 15 — Committed/Rollback Ratio**
```sql
SELECT
  sum(xact_commit) * 100.0 / NULLIF(sum(xact_commit) + sum(xact_rollback), 0)
FROM pg_stat_database;
```

**Query 16 — Database Sizes**
```sql
SELECT datname, pg_database_size(datname) FROM pg_database
WHERE datistemplate = false;
```

**Query 17 — check track_io_timing**
```sql
SHOW track_io_timing;
```

**Query 18 — check pg_stat_statements loaded**
```sql
SELECT count(*) FROM pg_extension WHERE extname = 'pg_stat_statements';
```

**Query 19 — pg_stat_statements.max fill percentage**
```sql
SELECT count(*) * 100.0 / current_setting('pg_stat_statements.max')::int
FROM pg_stat_statements;
```

### Query Classification

| Query | Description | Action | Target |
|-------|-------------|--------|--------|
| Q1 | version() | DONE (M0) | version/version.go |
| Q2 | server start time | PORT | collector/server_info.go |
| Q3 | uptime | PORT (compute in Go) | collector/server_info.go |
| Q4-Q8 | OS via COPY TO PROGRAM | SKIP → M6 | — |
| Q9 | recovery state | PORT | collector/server_info.go |
| Q10 | backup state | PORT + VERSION GATE (PG14 only) | collector/server_info.go |
| Q11 | connection count | PORT + FIX (exclude self) | collector/connections.go |
| Q12 | max_connections | PORT | collector/connections.go |
| Q13 | superuser_reserved | PORT | collector/connections.go |
| Q14 | cache hit ratio | PORT + FIX (NULLIF) | collector/cache.go |
| Q15 | commit/rollback ratio | PORT | collector/transactions.go |
| Q16 | database sizes | PORT | collector/database_sizes.go |
| Q17 | track_io_timing | PORT | collector/settings.go |
| Q18 | pg_stat_statements ext | PORT | collector/extensions.go |
| Q19 | pgss fill % | PORT | collector/extensions.go |

**Real work: 12 queries → 7 collector files** (Q1 done, Q4-Q8 skipped)

### Version Gates Required

Only ONE real version gate in queries 1–19:
- **Q10: pg_is_in_backup()** — exists in PG 14, removed in PG 15+

### Collector Modules to Create

1. `internal/collector/server_info.go` — Q2, Q3, Q9, Q10
2. `internal/collector/connections.go` — Q11, Q12, Q13 (enhanced with state breakdown)
3. `internal/collector/cache.go` — Q14
4. `internal/collector/transactions.go` — Q15
5. `internal/collector/database_sizes.go` — Q16
6. `internal/collector/settings.go` — Q17 (extend to check key settings)
7. `internal/collector/extensions.go` — Q18, Q19
8. `internal/collector/registry.go` — RegisterCollector(), CollectAll()

Plus test files:
- `internal/collector/server_info_test.go`
- `internal/collector/connections_test.go`
- `internal/collector/cache_test.go`
- etc.

---

## Known Issues Affecting M1

1. **Claude Code bash broken on Windows** — agents create files, developer runs bash
2. **Go auto-upgraded to 1.25.7** — pgx v5.8.0 requires it, not a problem
3. **LF/CRLF warnings** — add .gitattributes before M1 commits
4. **Agent Teams work for file creation** — file editing, reading, glob, grep all functional

---

## Workflow for M1_01

```
1. This Claude.ai chat: finalize design.md + team-prompt.md
2. Developer: copy to docs/iterations/M1_01_.../
3. Developer: open Claude Code, paste team-prompt.md
4. Agents: create all .go files
5. Developer: go mod tidy → go build → go vet → fix cycle
6. Developer: go test (when testcontainers tests are written)
7. Developer: git commit + push
8. This Claude.ai chat: create session-log.md
```
