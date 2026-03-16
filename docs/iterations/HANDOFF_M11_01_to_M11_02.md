# PGPulse — Iteration Handoff: M11_01 → M11_02

**Date:** 2026-03-16
**From:** M11_01 (PGSS Snapshots + Diff Engine + Query Insights API + Bug Fixes)
**To:** M11_02 (Query Insights UI + Workload Report Page + HTML Export)

---

## DO NOT RE-DISCUSS

All items from M10_01 handoff remain in force, plus:

### PGSS Snapshot System — IMPLEMENTED (M11_01)

- **`internal/statements/`** — New package with 12 files (~1,960 lines)
- **SnapshotCapturer** — periodic capture of all pg_stat_statements rows, version-gated SQL (PG ≤12 vs 13+), configurable interval (default 30m)
- **PGSnapshotStore** — PostgreSQL-backed store with CopyFrom bulk insert for entries
- **NullSnapshotStore** — no-op for live mode
- **ComputeDiff** — pure Go diff engine: per-query deltas, derived fields (avg_exec_time, io_pct, cpu_time, shared_hit_ratio), stats_reset detection, new/evicted query categorization
- **BuildQueryInsight** — inter-snapshot delta computation for per-query time-series
- **GenerateReport** — structured workload report with sections (top by exec_time, calls, rows, IO reads, avg_time, new, evicted)
- **Migration 015** — `pgss_snapshots` + `pgss_snapshot_entries` tables
- **Config section:** `statement_snapshots.enabled`, `interval` (30m), `retention_days` (30), `capture_on_startup`, `top_n` (50)
- **Guard:** `cfg.StatementSnapshots.Enabled && persistentStore != nil`
- **DB/user name resolution** at capture time — stored as `database_name`, `user_name` in entries table

### 7 New API Endpoints (M11_01)

| Method | Path | Purpose |
|--------|------|---------|
| GET | /instances/{id}/snapshots | List snapshots (paginated, time-range filter) |
| GET | /instances/{id}/snapshots/{snapId} | Snapshot detail with paginated entries |
| GET | /instances/{id}/snapshots/diff | Diff between two snapshots (by ID or time range) |
| GET | /instances/{id}/snapshots/latest-diff | Diff between last two snapshots |
| GET | /instances/{id}/query-insights/{queryid} | Per-query time-series across snapshots |
| GET | /instances/{id}/workload-report | Structured workload report data |
| POST | /instances/{id}/snapshots/capture | Manual snapshot trigger (instance_management) |

### Bug Fixes (M11_01)

- Debug log removed from main.go
- `wastedibytes`: NullInt64 → NullFloat64 scan in database.go
- `pg.server.multixact_pct`: added to ServerInfoCollector
- `srsubstate`: `::text` cast in 2 logical replication SQL queries

### Key Types (M11_01)

```go
// internal/statements/types.go
type Snapshot struct {
    ID, InstanceID, CapturedAt, PGVersion, StatsReset,
    TotalStatements, TotalCalls, TotalExecTime
}

type SnapshotEntry struct {
    SnapshotID, QueryID, UserID, DbID, Query, DatabaseName, UserName,
    Calls, TotalExecTime, TotalPlanTime*, Rows,
    SharedBlksHit/Read/Dirtied/Written, LocalBlksHit/Read,
    TempBlksRead/Written, BlkReadTime, BlkWriteTime,
    WALRecords*, WALFpi*, WALBytes*,
    MeanExecTime*, MinExecTime*, MaxExecTime*, StddevExecTime*
    // * = pointer types, PG 13+ only
}

type DiffResult struct {
    FromSnapshot, ToSnapshot, StatsResetWarning, Duration,
    TotalCallsDelta, TotalExecTimeDelta,
    Entries []DiffEntry, NewQueries, EvictedQueries, TotalEntries
}

type DiffEntry struct {
    QueryID, UserID, DbID, Query, DatabaseName, UserName,
    CallsDelta, ExecTimeDelta, PlanTimeDelta*, RowsDelta,
    SharedBlksReadDelta, SharedBlksHitDelta, TempBlks*Delta, BlkTime*Delta,
    WALBytesDelta*,
    // Derived: AvgExecTimePerCall, IOTimePct, CPUTimeDelta, SharedHitRatio
}

type QueryInsight struct {
    QueryID, Query, DatabaseName, UserName, FirstSeen,
    Points []QueryInsightPoint{CapturedAt, CallsDelta, ExecTimeDelta, RowsDelta, AvgExecTime, SharedHitRatio}
}

type WorkloadReport struct {
    InstanceID, FromTime, ToTime, Duration, StatsResetWarning,
    Summary, TopByExecTime, TopByCalls, TopByRows, TopByIOReads, TopByAvgTime,
    NewQueries, EvictedQueries
}

// internal/statements/store.go
type SnapshotStore interface {
    WriteSnapshot(ctx, snap, entries) (int64, error)
    GetSnapshot(ctx, id) (*Snapshot, error)
    GetSnapshotEntries(ctx, snapshotID, limit, offset) ([]SnapshotEntry, int, error)
    ListSnapshots(ctx, instanceID, opts) ([]Snapshot, int, error)
    GetLatestSnapshots(ctx, instanceID, n) ([]Snapshot, error)
    GetEntriesForQuery(ctx, instanceID, queryID, from, to) ([]SnapshotEntry, []Snapshot, error)
    CleanOld(ctx, olderThan) error
}
```

---

## What Was Just Completed

### M11_01 — PGSS Snapshots + Diff Engine + API + Bug Fixes (1 session)
- 16 new files, 6 modified files (~3,044 lines added)
- New `internal/statements/` package (12 files, ~1,960 lines)
- Migration 015 (2 tables + indexes)
- 7 new API endpoints (total now ~54)
- 4 bug fixes (debug log, wastedibytes, multixact_pct, srsubstate)
- Config section: `statement_snapshots.*`
- All tests pass, lint clean

---

## Demo Environment

```
Ubuntu 24.04 VM: 185.159.111.139

PGPulse UI:     http://185.159.111.139:8989     (persistent mode)
Login:          admin / pgpulse_admin
Config:         /opt/pgpulse/configs/pgpulse.yml

PostgreSQL 16.13:
  Primary:      localhost:5432
  Replica:      localhost:5433
  Chaos:        localhost:5434

Monitor user:   pgpulse_monitor / pgpulse_monitor_demo
Storage DB:     pgpulse_storage on port 5432

Chaos scripts:  /opt/pgpulse/chaos/*.sh (PGPASSWORD embedded)
```

**Post-deploy config addition needed:**
```yaml
statement_snapshots:
  enabled: true
  interval: 30m
  retention_days: 30
  capture_on_startup: true  # get first snapshot immediately on deploy
  top_n: 50
```

---

## Known Issues (Post M11_01)

| Issue | Severity | Notes |
|-------|----------|-------|
| `c.command_desc` SQL bug in cluster progress | Pre-existing | PG16 compatibility |
| `pg_stat_statements` not in shared_preload_libraries (some instances) | Expected | WARN logged, graceful degradation |
| `pg_largeobject` permission denied | Expected | Monitor user lacks access; sub-collector skips |
| Need 2+ snapshots before diff/report endpoints return data | Expected | First useful diff after 2× capture interval (60m default) |

*All 4 bugs from M10_01 handoff are now resolved.*

---

## M11_02 Scope — Frontend: Query Insights + Workload Report + HTML Export

### Pages to Build

1. **Query Insights Page** (`/servers/:serverId/query-insights`)
   - Top queries table (from latest-diff endpoint) with sortable columns: exec_time_delta, calls_delta, rows_delta, avg_exec_time, io_pct
   - Click a query → per-query detail panel with ECharts time-series (calls/interval, exec_time/interval, avg_exec_time over time)
   - Snapshot selector: dropdown to pick diff range (from/to snapshots or time range)
   - Stats reset warning banner when detected
   - Query text display with syntax highlighting and copy button

2. **Workload Report Page** (`/servers/:serverId/workload-report`)
   - Snapshot range selector (from/to or time range)
   - Summary card: total calls, total exec_time, unique queries, new/evicted counts, time range
   - Sections matching the WorkloadReport type: Top by Exec Time, Top by Calls, Top by Rows, Top by I/O Reads, Top by Avg Time, New Queries, Evicted Queries
   - Each section: collapsible, shows top-N with expandable query text
   - Print-friendly layout (CSS @media print)

3. **HTML Export Endpoint** (`GET /instances/{id}/workload-report/html`)
   - Server-rendered HTML using Go `html/template`
   - Standalone file: inline CSS, no external dependencies
   - Same sections as React page
   - Content-Disposition: attachment for download
   - Optional: `?inline=true` for browser rendering

### Sidebar Navigation

- Add "Query Insights" under the instance navigation group
- Add "Workload Report" as a sub-item or separate entry
- Snapshot count badge on Query Insights nav item (optional)

### API Hooks (React Query)

All API endpoints already exist from M11_01. Frontend needs:
- `useSnapshots(instanceId, opts)` → GET /snapshots
- `useSnapshotDiff(instanceId, from, to)` → GET /snapshots/diff
- `useLatestDiff(instanceId)` → GET /snapshots/latest-diff
- `useQueryInsights(instanceId, queryId, from, to)` → GET /query-insights/{queryid}
- `useWorkloadReport(instanceId, from, to)` → GET /workload-report
- `useManualCapture(instanceId)` → POST /snapshots/capture (mutation)

### Agent Team (M11_02)

2 agents recommended:
- **Agent 1 — Frontend Specialist:** Query Insights page, Workload Report page, React Query hooks, sidebar nav
- **Agent 2 — Full-Stack:** HTML export endpoint (Go template), print CSS, integration testing

---

## Roadmap: Updated Priorities

### Queue (locked order)

1. ~~Alert & Advisor Polish~~ ✅ **M9 DONE**
2. ~~Advisor Auto-Population~~ ✅ **M10 DONE**
3. ~~PGSS Snapshots + Diff Engine + API~~ ✅ **M11_01 DONE**
4. **Query Insights UI + Workload Report + HTML Export** (M11_02) ← NEXT
5. **Desktop App (Wails)** (M12)
6. **Prometheus Exporter** (M13)

### Milestone Status

| Milestone | Scope | Status |
|-----------|-------|--------|
| ~~MW_01~~ | Windows executable + live mode | ✅ Done |
| ~~MW_01b~~ | Bugfixes (5 bugs) | ✅ Done |
| ~~MN_01~~ | Metric naming standardization | ✅ Done |
| ~~REM_01~~ | Rule-based remediation (3 sub-iterations) | ✅ Done |
| ~~M9~~ | Alert & Advisor Polish | ✅ Done |
| ~~M10~~ | Advisor Auto-Population | ✅ Done |
| ~~M11_01~~ | PGSS Snapshots + Diff Engine + API | ✅ Done |
| M11_02 | Query Insights UI + Workload Report + HTML Export | 🔲 Next |
| M12 | Desktop App (Wails packaging) | 🔲 |
| M13 | Prometheus Exporter | 🔲 |

---

## Build & Deploy

```bash
# Build verification
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...

# Cross-compile (MINGW64 — use export, not set)
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED

# Deploy to demo VM
scp dist/pgpulse-server-linux-amd64 ml4dbs@185.159.111.139:/home/ml4dbs/pgpulse-demo/
ssh ml4dbs@185.159.111.139 'sudo systemctl stop pgpulse && sudo cp /home/ml4dbs/pgpulse-demo/pgpulse-server-linux-amd64 /opt/pgpulse/bin/pgpulse-server && sudo chmod +x /opt/pgpulse/bin/pgpulse-server && sudo systemctl start pgpulse'

# Add statement_snapshots config to demo VM
ssh ml4dbs@185.159.111.139 'sudo vi /opt/pgpulse/configs/pgpulse.yml'
# Add:
#   statement_snapshots:
#     enabled: true
#     interval: 30m
#     retention_days: 30
#     capture_on_startup: true
#     top_n: 50
# Then restart:
ssh ml4dbs@185.159.111.139 'sudo systemctl restart pgpulse'
```

---

## Project Knowledge Status

| Document | Status |
|----------|--------|
| PGPulse_Development_Strategy_v2.md | ✅ Current |
| PGAM_FEATURE_AUDIT.md | ✅ Current |
| Chat_Transition_Process.md | ✅ Current |
| Save_Point_System.md | ✅ Current |
| PGPulse_Competitive_Research_Synthesis.md | ✅ Current |
| CODEBASE_DIGEST.md | ⚠️ Re-upload after M11_01 digest regeneration |
