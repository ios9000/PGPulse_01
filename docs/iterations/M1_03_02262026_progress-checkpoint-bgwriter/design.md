# M1_03 — Design: Progress Monitoring + Checkpoint/BGWriter

**Iteration:** M1_03
**Date:** 2026-02-26
**Depends on:** M1_02b committed (InstanceContext in Collector interface)

---

## 1. Progress Collectors — File Layout

Three files, six independent collector structs. Each struct has its own
Name(), Collect(), Interval(), constructor. They share no state. Grouped
by operational similarity.

| File | Structs | Queries |
|------|---------|---------|
| `progress_vacuum.go` | VacuumProgressCollector | Q42 |
| `progress_maintenance.go` | ClusterProgressCollector, AnalyzeProgressCollector | Q43, Q45 |
| `progress_operations.go` | CreateIndexProgressCollector, BasebackupProgressCollector, CopyProgressCollector | Q44, Q46, Q47 |

All progress collectors share these conventions:
- Embed `Base`, interval 10s
- No version gates (all views exist on PG 14+)
- No InstanceContext role check (progress views work on both primary and replica)
- Empty result sets → return empty slice, not error
- `completion_pct` = done / total * 100 (0 when total = 0)
- Table names: `COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text)`

---

## 2. progress_vacuum.go

### Struct

```go
type VacuumProgressCollector struct {
    Base
}

func NewVacuumProgressCollector(instanceID string, v version.PGVersion) *VacuumProgressCollector {
    return &VacuumProgressCollector{
        Base: newBase(instanceID, v, 10*time.Second),
    }
}

func (c *VacuumProgressCollector) Name() string { return "progress_vacuum" }
```

### SQL

```go
const progressVacuumSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.phase,
    p.heap_blks_total,
    p.heap_blks_scanned,
    p.heap_blks_vacuumed,
    p.index_vacuum_count,
    p.max_dead_tuples,
    p.num_dead_tuples
FROM pg_stat_progress_vacuum p
JOIN pg_stat_activity a ON a.pid = p.pid`
```

### Collect

```go
func (c *VacuumProgressCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
    qCtx, cancel := queryContext(ctx)
    defer cancel()

    rows, err := conn.Query(qCtx, progressVacuumSQL)
    if err != nil {
        return nil, fmt.Errorf("progress_vacuum: %w", err)
    }
    defer rows.Close()

    var points []MetricPoint
    for rows.Next() {
        var (
            pid, datname, tableName, phase             string
            blksTotal, blksScanned, blksVacuumed       float64
            indexVacuumCount, maxDeadTuples, numDeadTuples float64
        )
        if err := rows.Scan(
            &pid, &datname, &tableName, &phase,
            &blksTotal, &blksScanned, &blksVacuumed,
            &indexVacuumCount, &maxDeadTuples, &numDeadTuples,
        ); err != nil {
            return nil, fmt.Errorf("progress_vacuum scan: %w", err)
        }

        labels := map[string]string{
            "pid": pid, "datname": datname,
            "table_name": tableName, "phase": phase,
        }

        pct := completionPct(blksScanned, blksTotal)

        points = append(points,
            c.point("progress.vacuum.heap_blks_total", blksTotal, labels),
            c.point("progress.vacuum.heap_blks_scanned", blksScanned, labels),
            c.point("progress.vacuum.heap_blks_vacuumed", blksVacuumed, labels),
            c.point("progress.vacuum.index_vacuum_count", indexVacuumCount, labels),
            c.point("progress.vacuum.max_dead_tuples", maxDeadTuples, labels),
            c.point("progress.vacuum.num_dead_tuples", numDeadTuples, labels),
            c.point("progress.vacuum.completion_pct", pct, labels),
        )
    }
    return points, rows.Err()
}
```

### Shared Helper (in progress_vacuum.go or a common location)

```go
// completionPct computes percentage safely. Returns 0 when total is 0.
func completionPct(done, total float64) float64 {
    if total <= 0 {
        return 0
    }
    return (done / total) * 100
}
```

This helper is used by all six progress collectors. Place it in
`progress_vacuum.go` (first file alphabetically) or in `base.go`.
Since it's a package-level function, all files in the package can use it.

---

## 3. progress_maintenance.go

Two independent collectors in one file.

### ClusterProgressCollector

```go
type ClusterProgressCollector struct {
    Base
}

func NewClusterProgressCollector(instanceID string, v version.PGVersion) *ClusterProgressCollector {
    return &ClusterProgressCollector{Base: newBase(instanceID, v, 10*time.Second)}
}

func (c *ClusterProgressCollector) Name() string { return "progress_cluster" }
```

**SQL:**
```go
const progressClusterSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.command, p.phase,
    p.heap_tuples_scanned, p.heap_tuples_written,
    p.heap_blks_total, p.heap_blks_scanned,
    p.index_rebuild_count
FROM pg_stat_progress_cluster p
JOIN pg_stat_activity a ON a.pid = p.pid`
```

**Collect:** Labels: pid, datname, table_name, command, phase.
Metrics: heap_tuples_scanned, heap_tuples_written, heap_blks_total,
heap_blks_scanned, index_rebuild_count, completion_pct.
completion_pct = heap_blks_scanned / heap_blks_total.

### AnalyzeProgressCollector

```go
type AnalyzeProgressCollector struct {
    Base
}

func NewAnalyzeProgressCollector(instanceID string, v version.PGVersion) *AnalyzeProgressCollector {
    return &AnalyzeProgressCollector{Base: newBase(instanceID, v, 10*time.Second)}
}

func (c *AnalyzeProgressCollector) Name() string { return "progress_analyze" }
```

**SQL:**
```go
const progressAnalyzeSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.phase,
    p.sample_blks_total, p.sample_blks_scanned,
    p.ext_stats_total, p.ext_stats_computed,
    p.child_tables_total, p.child_tables_done,
    COALESCE(p.current_child_table_relid::regclass::text, '') AS current_child
FROM pg_stat_progress_analyze p
JOIN pg_stat_activity a ON a.pid = p.pid`
```

**Collect:** Labels: pid, datname, table_name, phase (current_child as label too).
Metrics: sample_blks_total, sample_blks_scanned, ext_stats_total, ext_stats_computed,
child_tables_total, child_tables_done, completion_pct.
completion_pct = sample_blks_scanned / sample_blks_total.

---

## 4. progress_operations.go

Three independent collectors in one file.

### CreateIndexProgressCollector

```go
type CreateIndexProgressCollector struct {
    Base
}

func NewCreateIndexProgressCollector(instanceID string, v version.PGVersion) *CreateIndexProgressCollector {
    return &CreateIndexProgressCollector{Base: newBase(instanceID, v, 10*time.Second)}
}

func (c *CreateIndexProgressCollector) Name() string { return "progress_create_index" }
```

**SQL:**
```go
const progressCreateIndexSQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    COALESCE(p.index_relid::regclass::text, '') AS index_name,
    p.command, p.phase,
    p.lockers_total, p.lockers_done,
    p.blocks_total, p.blocks_done,
    p.tuples_total, p.tuples_done,
    p.partitions_total, p.partitions_done
FROM pg_stat_progress_create_index p
JOIN pg_stat_activity a ON a.pid = p.pid`
```

**Collect:** Labels: pid, datname, table_name, index_name, command, phase.
Metrics: blocks_total, blocks_done, tuples_total, tuples_done, lockers_total,
lockers_done, partitions_total, partitions_done, completion_pct.
completion_pct = blocks_done / blocks_total.

### BasebackupProgressCollector

```go
type BasebackupProgressCollector struct {
    Base
}

func NewBasebackupProgressCollector(instanceID string, v version.PGVersion) *BasebackupProgressCollector {
    return &BasebackupProgressCollector{Base: newBase(instanceID, v, 10*time.Second)}
}

func (c *BasebackupProgressCollector) Name() string { return "progress_basebackup" }
```

**SQL:**
```go
const progressBasebackupSQL = `
SELECT
    a.pid::text,
    COALESCE(a.usename, '') AS usename,
    COALESCE(a.application_name, '') AS app_name,
    COALESCE(a.client_addr::text, '') AS client_addr,
    p.phase,
    p.backup_total, p.backup_streamed,
    p.tablespaces_total, p.tablespaces_streamed
FROM pg_stat_progress_basebackup p
JOIN pg_stat_activity a ON a.pid = p.pid`
```

**Collect:** Labels: pid, usename, app_name, client_addr, phase.
Metrics: backup_total, backup_streamed, tablespaces_total, tablespaces_streamed, completion_pct.
completion_pct = backup_streamed / backup_total.

### CopyProgressCollector

```go
type CopyProgressCollector struct {
    Base
}

func NewCopyProgressCollector(instanceID string, v version.PGVersion) *CopyProgressCollector {
    return &CopyProgressCollector{Base: newBase(instanceID, v, 10*time.Second)}
}

func (c *CopyProgressCollector) Name() string { return "progress_copy" }
```

**SQL:**
```go
const progressCopySQL = `
SELECT
    a.pid::text,
    COALESCE(a.datname, '') AS datname,
    COALESCE(p.relid::regclass::text, 'oid:' || p.relid::text) AS table_name,
    p.command, p.type,
    p.bytes_processed, p.bytes_total,
    p.tuples_processed, p.tuples_excluded
FROM pg_stat_progress_copy p
JOIN pg_stat_activity a ON a.pid = p.pid`
```

**Collect:** Labels: pid, datname, table_name, command, type.
Note: `phase` is not available in pg_stat_progress_copy — use command + type instead.
Metrics: bytes_processed, bytes_total, tuples_processed, tuples_excluded, completion_pct.
completion_pct = bytes_processed / bytes_total.

---

## 5. CheckpointCollector (checkpoint.go)

Unchanged from previous design. See the earlier design document for full details.

### Struct (stateful)

```go
type checkpointSnapshot struct {
    checkpointsTimed       float64
    checkpointsReq         float64
    writeTimeMs            float64
    syncTimeMs             float64
    buffersWritten         float64
    buffersClean           float64
    maxwrittenClean        float64
    buffersAlloc           float64
    buffersBackend         float64 // -1 if unavailable (PG ≥ 17)
    buffersBackendFsync    float64 // -1 if unavailable (PG ≥ 17)
    restartpointsTimed     float64 // -1 if unavailable (PG ≤ 16)
    restartpointsDone      float64
    restartpointsRequested float64
}

type CheckpointCollector struct {
    Base
    sqlGate  version.Gate
    mu       sync.Mutex
    prev     *checkpointSnapshot
    prevTime time.Time
}
```

### Version-Gated SQL (2 variants, 13 columns each)

PG 14–16: Single query on `pg_stat_bgwriter`, `-1` sentinels for restartpoint columns.
PG 17+: `CROSS JOIN` of `pg_stat_checkpointer` and `pg_stat_bgwriter`, `-1` sentinels for backend columns.

### Key Methods

- `Collect()` — queries PG, calls `computeMetrics()`, updates state under mutex
- `computeMetrics(curr, prev, prevTime, now)` — pure logic, testable without PG
- `absolutePoints(snap)` — emits counters, conditionally emits version-specific metrics
- `ratePoints(curr, elapsedSec)` — computes deltas from `c.prev`
- `isStatsReset(curr)` — detects counter decrease

---

## 6. Test Design

### progress_vacuum_test.go

| Test | Assertion |
|------|-----------|
| `TestVacuumProgress_NameAndInterval` | Name="progress_vacuum", Interval=10s |
| `TestCompletionPct` | 0 when total=0, 50 when half, 100 when complete |
| `TestVacuumProgress_Integration` | `//go:build integration` stub |

### progress_maintenance_test.go

| Test | Assertion |
|------|-----------|
| `TestClusterProgress_NameAndInterval` | Name="progress_cluster", Interval=10s |
| `TestAnalyzeProgress_NameAndInterval` | Name="progress_analyze", Interval=10s |
| `TestClusterProgress_Integration` | stub |
| `TestAnalyzeProgress_Integration` | stub |

### progress_operations_test.go

| Test | Assertion |
|------|-----------|
| `TestCreateIndexProgress_NameAndInterval` | Name="progress_create_index", Interval=10s |
| `TestBasebackupProgress_NameAndInterval` | Name="progress_basebackup", Interval=10s |
| `TestCopyProgress_NameAndInterval` | Name="progress_copy", Interval=10s |
| Integration stubs for all three | stubs |

### checkpoint_test.go

| Test | Assertion |
|------|-----------|
| `TestCheckpoint_NameAndInterval` | Name="checkpoint", Interval=60s |
| `TestCheckpoint_GateSelectPG14` | PG 14–16 variant |
| `TestCheckpoint_GateSelectPG17` | PG 17+ variant |
| `TestCheckpoint_FirstCycleNoRates` | computeMetrics(curr, nil, ...) → no _per_second metrics |
| `TestCheckpoint_SecondCycleEmitsRates` | computeMetrics(curr, prev, ...) → has _per_second |
| `TestCheckpoint_StatsResetSkipsRates` | curr < prev → isStatsReset returns true |
| `TestCheckpoint_PG16NoRestartpoints` | restartpoints_timed=-1 → no restartpoint metrics |
| `TestCheckpoint_PG17HasRestartpoints` | restartpoints_timed≥0 → restartpoint metrics present |
| `TestCheckpoint_ZeroElapsedSafe` | No panic when elapsed=0 |
| `TestCheckpoint_Integration` | stub |

Rate tests use `computeMetrics()` directly — no PG connection needed.

---

## 7. File Summary

| File | Structs | Lines (est.) |
|------|---------|-------------|
| `progress_vacuum.go` | VacuumProgressCollector + completionPct() | ~80 |
| `progress_maintenance.go` | ClusterProgressCollector, AnalyzeProgressCollector | ~150 |
| `progress_operations.go` | CreateIndexProgressCollector, BasebackupProgressCollector, CopyProgressCollector | ~200 |
| `checkpoint.go` | CheckpointCollector + snapshot + rate logic | ~250 |
| `progress_vacuum_test.go` | | ~40 |
| `progress_maintenance_test.go` | | ~50 |
| `progress_operations_test.go` | | ~60 |
| `checkpoint_test.go` | | ~150 |

**Total: 8 new files, ~980 lines estimated**
