# PGPulse — M5_04 Technical Design: Statements, Lock Tree, Progress Monitoring

**Iteration:** M5_04
**Date:** 2026-03-03
**Author:** Claude.ai (Brain) + Evlampiy
**Companion:** M5_04_requirements.md

---

## 1. Architecture Overview

All three features follow the same backend pattern established in M5_03:

```
Browser (React)
  │  TanStack Query (polling)
  ▼
REST API Handler
  │  JWT middleware (existing)
  ▼
ConnProvider.ConnFor(instanceID)
  │  borrows pgx.Conn from orchestrator
  │  SET statement_timeout = '5000'
  ▼
PostgreSQL Instance
  │  query → result
  ▼
Handler marshals JSON → response
  │  releases conn
  ▼
React component renders
```

No new architectural patterns introduced. Three new handlers, three new React sections, three new TanStack Query hooks.

---

## 2. Backend Design

### 2.1 Statements Handler

**File:** `internal/api/handler_statements.go`

#### SQL — Version-Gated

**PG ≥ 13:**
```sql
SELECT
    s.queryid,
    LEFT(s.query, 500) AS query_text,
    d.datname,
    r.rolname AS usename,
    s.calls,
    s.total_exec_time AS total_exec_time_ms,
    s.total_exec_time / NULLIF(s.calls, 0) AS mean_exec_time_ms,
    s.rows,
    s.blk_read_time AS blk_read_time_ms,
    s.blk_write_time AS blk_write_time_ms,
    (s.blk_read_time + s.blk_write_time) AS io_time_ms,
    (s.total_exec_time - s.blk_read_time - s.blk_write_time) AS cpu_time_ms,
    s.shared_blks_hit,
    s.shared_blks_read,
    CASE WHEN (s.shared_blks_hit + s.shared_blks_read) > 0
         THEN s.shared_blks_hit::float8 / (s.shared_blks_hit + s.shared_blks_read)
         ELSE 0 END AS hit_ratio,
    s.total_exec_time / NULLIF(sum(s.total_exec_time) OVER (), 0) * 100 AS pct_of_total_time
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
JOIN pg_roles r ON r.oid = s.userid
ORDER BY {sort_column} DESC
LIMIT $1
```

**PG ≤ 12:** Replace `total_exec_time` with `total_time` throughout. Same structure otherwise.

**Version gate key:** `statements.top_queries`

**Sort column mapping (server-side, NOT interpolated — use a Go switch):**

```go
func sortColumn(sort string, pgVersion int) string {
    switch sort {
    case "io_time":
        return "(s.blk_read_time + s.blk_write_time)"
    case "cpu_time":
        if pgVersion >= 130000 {
            return "(s.total_exec_time - s.blk_read_time - s.blk_write_time)"
        }
        return "(s.total_time - s.blk_read_time - s.blk_write_time)"
    case "calls":
        return "s.calls"
    case "rows":
        return "s.rows"
    default: // "total_time"
        if pgVersion >= 130000 {
            return "s.total_exec_time"
        }
        return "s.total_time"
    }
}
```

**IMPORTANT:** The sort column is selected via a Go switch on a whitelist of allowed values — NOT by interpolating user input into SQL. The `$1` parameter is used for LIMIT only.

#### pgss Config Query

```sql
SELECT
    (SELECT setting FROM pg_settings WHERE name = 'pg_stat_statements.max') AS max_setting,
    (SELECT setting FROM pg_settings WHERE name = 'pg_stat_statements.track') AS track_setting,
    (SELECT setting FROM pg_settings WHERE name = 'track_io_timing') AS io_timing,
    (SELECT count(*) FROM pg_stat_statements) AS current_count
```

**PG ≥ 14 addition:**
```sql
SELECT stats_reset,
       EXTRACT(EPOCH FROM (now() - stats_reset)) AS stats_reset_age_seconds
FROM pg_stat_statements_info
```

These can be combined into a single response or returned as a nested `config` object in the statements response.

#### Handler Signature

```go
// GET /api/v1/instances/:id/activity/statements
func (h *Handler) GetStatements(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "id")
    sort := r.URL.Query().Get("sort")       // validated against whitelist
    limitStr := r.URL.Query().Get("limit")  // parsed, clamped 1-100, default 25

    conn, err := h.connProvider.ConnFor(r.Context(), instanceID)
    // ... SET statement_timeout = '5000'
    // ... detect PG version (from orchestrator cache or quick query)
    // ... run statements query + config query
    // ... marshal response
    // ... release conn
}
```

#### Response Shape

```json
{
  "config": {
    "max": 5000,
    "track": "top",
    "io_timing": true,
    "current_count": 3847,
    "fill_pct": 76.94,
    "stats_reset": "2026-03-01T08:00:00Z",
    "stats_reset_age_seconds": 172800
  },
  "statements": [
    {
      "queryid": 1234567890,
      "query_text": "SELECT u.id, u.name FROM users u WHERE u.status = $1 ...",
      "dbname": "myapp",
      "username": "app_user",
      "calls": 458293,
      "total_exec_time_ms": 89234.56,
      "mean_exec_time_ms": 0.195,
      "rows": 916586,
      "blk_read_time_ms": 1234.56,
      "blk_write_time_ms": 0.0,
      "io_time_ms": 1234.56,
      "cpu_time_ms": 88000.0,
      "shared_blks_hit": 9283746,
      "shared_blks_read": 12345,
      "hit_ratio": 0.9987,
      "pct_of_total_time": 23.45
    }
  ]
}
```

#### Extension Detection

Before running the statements query, check if pg_stat_statements is installed:

```sql
SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')
```

If not installed, return:
```json
{ "error": "pg_stat_statements extension not available", "code": "EXTENSION_NOT_FOUND" }
```

With HTTP status 404.

---

### 2.2 Lock Tree Handler

**File:** `internal/api/handler_locks.go`

#### SQL

```sql
SELECT
    sa.pid,
    sa.usename,
    sa.datname,
    sa.state,
    sa.wait_event_type,
    sa.wait_event,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    LEFT(sa.query, 200) AS query,
    pg_blocking_pids(sa.pid) AS blocking_pids
FROM pg_stat_activity sa
WHERE sa.pid != pg_backend_pid()
  AND sa.state IS NOT NULL
```

No version gate needed — `pg_blocking_pids()` is available since PG 9.6 and PGPulse minimum is PG 14.

#### Go Tree-Building Algorithm

Reuse the BFS approach from `internal/collector/locks_tree.go`:

```go
type LockEntry struct {
    PID            int      `json:"pid"`
    Depth          int      `json:"depth"`
    Usename        string   `json:"usename"`
    Datname        string   `json:"datname"`
    State          string   `json:"state"`
    WaitEventType  *string  `json:"wait_event_type"`
    WaitEvent      *string  `json:"wait_event"`
    DurationSeconds float64 `json:"duration_seconds"`
    Query          string   `json:"query"`
    BlockedByCount int      `json:"blocked_by_count"`
    BlockingCount  int      `json:"blocking_count"`
    IsRoot         bool     `json:"is_root"`
    ParentPID      *int     `json:"parent_pid"`
}
```

**Algorithm:**

1. Query `pg_stat_activity` with `pg_blocking_pids()` for all active sessions.
2. Build a map: `pid → []blocking_pids` (who is blocking me).
3. Build reverse map: `pid → []blocked_pids` (who am I blocking).
4. Find root blockers: PIDs that appear in someone's `blocking_pids` but have empty `blocking_pids` themselves.
5. BFS from each root blocker, assigning depth levels.
6. Return flat array sorted by depth, then PID — the frontend renders this as an indented table.

**Only include PIDs involved in blocking chains.** Sessions with empty `blocking_pids` that don't appear as blockers for anyone else are excluded.

#### Response Shape

```json
{
  "summary": {
    "root_blockers": 2,
    "total_blocked": 7,
    "max_depth": 3
  },
  "locks": [
    {
      "pid": 12345,
      "depth": 0,
      "usename": "admin",
      "datname": "production",
      "state": "idle in transaction",
      "wait_event_type": null,
      "wait_event": null,
      "duration_seconds": 154.3,
      "query": "UPDATE accounts SET ...",
      "blocked_by_count": 0,
      "blocking_count": 4,
      "is_root": true,
      "parent_pid": null
    },
    {
      "pid": 12350,
      "depth": 1,
      "usename": "app_user",
      "datname": "production",
      "state": "active",
      "wait_event_type": "Lock",
      "wait_event": "transactionid",
      "duration_seconds": 89.1,
      "query": "DELETE FROM orders WHERE ...",
      "blocked_by_count": 1,
      "blocking_count": 2,
      "is_root": false,
      "parent_pid": 12345
    }
  ]
}
```

---

### 2.3 Progress Handler

**File:** `internal/api/handler_progress.go`

#### SQL — 6 Version-Gated Queries

**VACUUM progress (PG ≥ 9.6 — always available for PGPulse):**
```sql
SELECT
    v.pid,
    sa.datname,
    v.relid::regclass::text AS relname,
    v.phase,
    v.heap_blks_total,
    v.heap_blks_scanned,
    v.heap_blks_vacuumed,
    v.index_vacuum_count,
    v.max_dead_tuples,
    v.num_dead_tuples,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN v.heap_blks_total > 0
         THEN (v.heap_blks_scanned::float8 / v.heap_blks_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_vacuum v
JOIN pg_stat_activity sa ON sa.pid = v.pid
```

**CLUSTER/VACUUM FULL (PG ≥ 12):**
```sql
SELECT
    c.pid,
    sa.datname,
    c.relid::regclass::text AS relname,
    c.command_desc AS phase,
    c.heap_tuples_scanned,
    c.heap_tuples_written,
    c.heap_blks_total,
    c.heap_blks_scanned,
    c.index_rebuild_count,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN c.heap_blks_total > 0
         THEN (c.heap_blks_scanned::float8 / c.heap_blks_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_cluster c
JOIN pg_stat_activity sa ON sa.pid = c.pid
```

**CREATE INDEX (PG ≥ 12):**
```sql
SELECT
    ci.pid,
    sa.datname,
    ci.relid::regclass::text AS relname,
    ci.index_relid::regclass::text AS index_name,
    ci.command,
    ci.phase,
    ci.tuples_total,
    ci.tuples_done,
    ci.partitions_total,
    ci.partitions_done,
    ci.blocks_total,
    ci.blocks_done,
    ci.lockers_total,
    ci.lockers_done,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN ci.blocks_total > 0
         THEN (ci.blocks_done::float8 / ci.blocks_total) * 100
         WHEN ci.tuples_total > 0
         THEN (ci.tuples_done::float8 / ci.tuples_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_create_index ci
JOIN pg_stat_activity sa ON sa.pid = ci.pid
```

**ANALYZE (PG ≥ 13):**
```sql
SELECT
    a.pid,
    sa.datname,
    a.relid::regclass::text AS relname,
    a.phase,
    a.sample_blks_total,
    a.sample_blks_scanned,
    a.ext_stats_total,
    a.ext_stats_computed,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN a.sample_blks_total > 0
         THEN (a.sample_blks_scanned::float8 / a.sample_blks_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_analyze a
JOIN pg_stat_activity sa ON sa.pid = a.pid
```

**BASEBACKUP (PG ≥ 13):**
```sql
SELECT
    b.pid,
    sa.datname,
    b.phase,
    b.backup_total,
    b.backup_streamed,
    b.tablespaces_total,
    b.tablespaces_streamed,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN b.backup_total > 0
         THEN (b.backup_streamed::float8 / b.backup_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_basebackup b
JOIN pg_stat_activity sa ON sa.pid = b.pid
```

**COPY (PG ≥ 14):**
```sql
SELECT
    cp.pid,
    sa.datname,
    cp.relid::regclass::text AS relname,
    cp.command,
    cp.type AS copy_type,
    cp.bytes_total,
    cp.bytes_processed,
    cp.tuples_processed,
    cp.tuples_excluded,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN cp.bytes_total > 0
         THEN (cp.bytes_processed::float8 / cp.bytes_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_copy cp
JOIN pg_stat_activity sa ON sa.pid = cp.pid
```

#### Handler Logic

```go
func (h *Handler) GetProgress(w http.ResponseWriter, r *http.Request) {
    // 1. Borrow conn via ConnProvider
    // 2. Detect PG version (cached in orchestrator)
    // 3. Run applicable queries based on version:
    //    - vacuum:    always (PG ≥ 9.6)
    //    - cluster:   PG ≥ 12
    //    - index:     PG ≥ 12
    //    - analyze:   PG ≥ 13
    //    - basebackup: PG ≥ 13
    //    - copy:      PG ≥ 14
    // 4. Merge all results into a single array with operation_type field
    // 5. Return JSON, empty array if nothing running
}
```

#### Response Shape

```json
{
  "operations": [
    {
      "operation_type": "vacuum",
      "pid": 23456,
      "datname": "production",
      "relname": "public.large_table",
      "phase": "scanning heap",
      "progress_pct": 45.2,
      "duration_seconds": 312.5,
      "details": {
        "heap_blks_total": 1000000,
        "heap_blks_scanned": 452000,
        "heap_blks_vacuumed": 200000,
        "index_vacuum_count": 1,
        "max_dead_tuples": 50000,
        "num_dead_tuples": 12345
      }
    },
    {
      "operation_type": "create_index",
      "pid": 23460,
      "datname": "production",
      "relname": "public.orders",
      "phase": "building index: scanning table",
      "progress_pct": 78.9,
      "duration_seconds": 89.3,
      "details": {
        "index_name": "public.idx_orders_created",
        "command": "CREATE INDEX CONCURRENTLY",
        "tuples_total": 5000000,
        "tuples_done": 3945000,
        "blocks_total": 125000,
        "blocks_done": 98625
      }
    }
  ]
}
```

---

### 2.4 Router Registration

**File:** `internal/api/router.go` — add to existing route group:

```go
// Activity endpoints (existing)
r.Get("/instances/{id}/activity/wait-events", h.GetWaitEvents)
r.Get("/instances/{id}/activity/long-transactions", h.GetLongTransactions)

// Activity endpoints (new — M5_04)
r.Get("/instances/{id}/activity/statements", h.GetStatements)
r.Get("/instances/{id}/activity/locks", h.GetLockTree)
r.Get("/instances/{id}/activity/progress", h.GetProgress)
```

---

## 3. Frontend Design

### 3.1 TypeScript Types

**File:** `web/src/types/models.ts` — add:

```typescript
// Statements
export interface StatementsConfig {
  max: number;
  track: string;
  io_timing: boolean;
  current_count: number;
  fill_pct: number;
  stats_reset: string | null;         // ISO timestamp, null if PG < 14
  stats_reset_age_seconds: number | null;
}

export interface StatementEntry {
  queryid: number;
  query_text: string;
  dbname: string;
  username: string;
  calls: number;
  total_exec_time_ms: number;
  mean_exec_time_ms: number;
  rows: number;
  blk_read_time_ms: number;
  blk_write_time_ms: number;
  io_time_ms: number;
  cpu_time_ms: number;
  shared_blks_hit: number;
  shared_blks_read: number;
  hit_ratio: number;
  pct_of_total_time: number;
}

export interface StatementsResponse {
  config: StatementsConfig;
  statements: StatementEntry[];
}

export type StatementSortField = 'total_time' | 'io_time' | 'cpu_time' | 'calls' | 'rows';

// Lock Tree
export interface LockTreeSummary {
  root_blockers: number;
  total_blocked: number;
  max_depth: number;
}

export interface LockEntry {
  pid: number;
  depth: number;
  usename: string;
  datname: string;
  state: string;
  wait_event_type: string | null;
  wait_event: string | null;
  duration_seconds: number;
  query: string;
  blocked_by_count: number;
  blocking_count: number;
  is_root: boolean;
  parent_pid: number | null;
}

export interface LockTreeResponse {
  summary: LockTreeSummary;
  locks: LockEntry[];
}

// Progress
export interface ProgressOperation {
  operation_type: 'vacuum' | 'analyze' | 'create_index' | 'cluster' | 'basebackup' | 'copy';
  pid: number;
  datname: string;
  relname: string | null;
  phase: string;
  progress_pct: number | null;
  duration_seconds: number;
  details: Record<string, unknown>;
}

export interface ProgressResponse {
  operations: ProgressOperation[];
}
```

### 3.2 TanStack Query Hooks

**File:** `web/src/hooks/useStatements.ts`

```typescript
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../lib/apiClient';
import type { StatementsResponse, StatementSortField } from '../types/models';

export function useStatements(instanceId: string, sort: StatementSortField = 'total_time', limit = 25) {
  return useQuery<StatementsResponse>({
    queryKey: ['statements', instanceId, sort, limit],
    queryFn: () => apiFetch(
      `/api/v1/instances/${instanceId}/activity/statements?sort=${sort}&limit=${limit}`
    ),
    refetchInterval: 10_000,
  });
}
```

**File:** `web/src/hooks/useLockTree.ts`

```typescript
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../lib/apiClient';
import type { LockTreeResponse } from '../types/models';

export function useLockTree(instanceId: string) {
  return useQuery<LockTreeResponse>({
    queryKey: ['locks', instanceId],
    queryFn: () => apiFetch(`/api/v1/instances/${instanceId}/activity/locks`),
    refetchInterval: 10_000,
  });
}
```

**File:** `web/src/hooks/useProgress.ts`

```typescript
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../lib/apiClient';
import type { ProgressResponse } from '../types/models';

export function useProgress(instanceId: string) {
  return useQuery<ProgressResponse>({
    queryKey: ['progress', instanceId],
    queryFn: () => apiFetch(`/api/v1/instances/${instanceId}/activity/progress`),
    refetchInterval: 5_000,
  });
}
```

### 3.3 Component Tree

```
ServerDetailPage.tsx
  │
  ├── ProgressSection.tsx              ← CONDITIONAL: only renders when operations > 0
  │   └── ProgressCard.tsx             ← one per active operation
  │
  ├── HeaderCard.tsx                   (existing)
  ├── KeyMetricsRow.tsx                (existing)
  ├── ConnectionGauge + section        (existing)
  ├── CacheHit section                 (existing)
  ├── ReplicationSection.tsx           (existing)
  ├── WaitEventsSection.tsx            (existing)
  │
  ├── StatementsSection.tsx            ← NEW
  │   ├── StatementsConfigBar.tsx      ← pgss config badges
  │   └── StatementRow.tsx[]           ← sortable table rows, expandable
  │       └── (expanded) TimeSeriesChart.tsx × 2-3   ← historical ECharts
  │
  ├── LockTreeSection.tsx              ← NEW
  │   ├── (summary line)
  │   └── LockTreeRow.tsx[]            ← indented rows
  │
  └── LongTransactionsTable.tsx        (existing)
      InstanceAlerts.tsx               (existing)
```

### 3.4 Statements Section — Detailed Design

#### StatementsConfigBar.tsx

Compact horizontal flex row with pill badges:

```
[ Fill: 76.9% ● ] [ Max: 5000 ] [ Track: top ] [ IO Timing: ON ✓ ] [ Reset: 2d ago ]
```

- Fill % pill turns yellow at ≥80%, red at ≥95%
- IO Timing pill turns yellow if OFF
- Reset age pill turns yellow at ≥1 day
- Uses the `config` object from the statements response (no separate API call)

#### Statements Table

| # | Query | DB | User | Total Time | Mean | Calls | Rows | IO Time | CPU Time | Hit% |
|---|-------|----|------|-----------|------|-------|------|---------|----------|------|

- "#" column: row index (1-based)
- "Query" column: monospace, truncated to ~80 chars, ellipsis if longer
- Time columns: formatted with `formatDuration()` (e.g., "1.23s", "456ms", "12.3min")
- Calls/Rows: formatted with `formatNumber()` (e.g., "458K", "1.2M")
- Hit%: percentage with 1 decimal
- Column headers are clickable buttons that trigger re-sort via `sort` state → new API call
- Active sort column highlighted with arrow indicator (▼ for desc)

#### StatementRow.tsx — Expandable

**Collapsed state:** Single table row as described above. Cursor pointer. Subtle hover highlight.

**Expanded state:** Row expands to show a detail panel below:

```
┌─────────────────────────────────────────────────────────────────┐
│  Query Text (full, monospace, code block, max-height: 200px,   │
│  scrollable if longer)                                          │
│                                                                 │
│  SELECT u.id, u.name, u.email, u.created_at                    │
│  FROM users u                                                   │
│  WHERE u.status = $1 AND u.org_id = $2                         │
│  ORDER BY u.created_at DESC LIMIT $3                           │
├─────────────────────────────────────────────────────────────────┤
│  ┌─ Execution Time ────────┐  ┌─ Calls per Interval ──────────┐│
│  │   [EChart line graph]   │  │   [EChart bar graph]          ││
│  │   time → exec_time_ms   │  │   time → calls count          ││
│  └─────────────────────────┘  └────────────────────────────────┘│
│  ┌─ IO vs CPU Time ───────────────────────────────────────────┐ │
│  │   [EChart stacked area: io_time + cpu_time over time]      │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Historical data source:** `useMetricsHistory` hook with:
- metrics: `['pgpulse.statements.top.io_time_ms', 'pgpulse.statements.top.cpu_time_ms', 'pgpulse.statements.top.calls']`
- Label filter: `queryid={queryid}` (the queryid from the clicked row)
- Time range: from global `useTimeRangeStore`

**Note:** Historical data is only available for queries that were in the top-N at collection time. If a query just entered the top list, historical data may be sparse. The charts should handle empty/sparse data gracefully (show "No historical data available" if empty).

### 3.5 Lock Tree Section — Detailed Design

#### LockTreeSection.tsx

**Summary line (top):**
- When blocking exists: `"⚠ 2 root blockers affecting 7 processes (max depth: 3)"` with orange/red text
- When no blocking: `"✓ No blocking locks detected"` with green text

**Indented table:**

| PID | User | Database | State | Wait Event | Duration | Blocking | Query |
|-----|------|----------|-------|------------|----------|----------|-------|

#### LockTreeRow.tsx

Each row receives a `depth` value and renders accordingly:

```
depth=0: ● 12345  admin  production  idle in transaction  —       2m 34s  blocks 4  UPDATE accounts SET...
depth=1: ├── 12350  app_user  production  active  Lock/transactionid  1m 29s  blocks 2  DELETE FROM orders...
depth=2: │   ├── 12355  app_user  production  active  Lock/transactionid  45s  blocks 0  INSERT INTO audit...
depth=2: │   └── 12360  batch_user  production  active  Lock/tuple  32s  blocks 0  UPDATE inventory...
depth=1: └── 12370  app_user  production  active  Lock/transactionid  1m 10s  blocks 0  SELECT ... FOR UPDATE
```

**Implementation:**

- Left padding: `paddingLeft: depth * 24px` (Tailwind: dynamic style)
- Tree connectors: `├──` for non-last children, `└──` for last child, `│` for continuation
  - Computing "last child" requires knowing sibling relationships from parent_pid
  - Simpler alternative: just use left-padding without connector chars (still very readable)
  - Recommendation: start with left-padding only, add connectors as a polish item
- Root blocker rows (depth=0): left border `border-l-4 border-red-500` + subtle background `bg-red-500/5`
- Duration: color-coded: <1min normal, 1-5min yellow, >5min red
- "Blocking" column: badge with count, red if > 0
- Query: truncated, tooltip on hover for full text (max 200 chars from API anyway)

### 3.6 Progress Section — Detailed Design

#### ProgressSection.tsx

**Conditional rendering:** The component returns `null` when `operations.length === 0`. No wrapper div, no empty state, no "No active operations" — the section simply disappears.

When operations exist, renders a card with title "Active Operations" and the operation cards inside.

#### ProgressCard.tsx

Each operation renders as a compact card:

```
┌──────────────────────────────────────────────────────────────┐
│  [VACUUM]  production / public.large_table                   │
│  Phase: scanning heap               PID: 23456   2m 34s     │
│  ██████████████████░░░░░░░░░░░░░░░ 45.2%                    │
└──────────────────────────────────────────────────────────────┘
```

- Operation type badge: color-coded pill
  - VACUUM: `bg-blue-500`
  - ANALYZE: `bg-green-500`
  - CREATE INDEX: `bg-purple-500`
  - CLUSTER/VACUUM FULL: `bg-orange-500`
  - BASEBACKUP: `bg-cyan-500`
  - COPY: `bg-yellow-500`
- Progress bar: Tailwind `bg-{color}-500` fill inside `bg-slate-700` track
  - Animated striped pattern when progress_pct is not null (CSS `background-image: linear-gradient`)
  - Indeterminate style (pulsing) when progress_pct is null
- Phase: text label for current phase
- Duration: human-readable elapsed time, updates on each 5s refresh
- PID: smaller secondary text

---

## 4. Server Detail Page — Section Order

Updated section order for ServerDetailPage.tsx:

```typescript
return (
  <div>
    <HeaderCard />
    <ProgressSection />         {/* ← NEW: conditional, only when active ops */}
    <KeyMetricsRow />
    {/* connections section */}
    {/* cache hit section */}
    <ReplicationSection />
    <WaitEventsSection />
    <StatementsSection />        {/* ← NEW */}
    <LockTreeSection />          {/* ← NEW */}
    <LongTransactionsTable />
    <InstanceAlerts />
  </div>
);
```

**Rationale:** Progress goes at the top (high priority, time-sensitive). Statements and Lock Tree go between Wait Events and Long Transactions — this groups all "activity analysis" sections together.

---

## 5. ECharts Theme Integration

Historical charts in the expandable statement row use the existing ECharts dark theme from `web/src/lib/echartsTheme.ts`. The charts should be:

- Height: 180px each
- Two charts side-by-side (exec time + calls), one full-width below (IO vs CPU)
- Responsive: stack vertically on narrow viewports
- Use `EChartWrapper` component from `web/src/components/charts/EChartWrapper.tsx`
- Tooltip on hover showing exact values
- Grid lines subdued (dark mode appropriate)

---

## 6. Error Handling

All three handlers follow the same error pattern:

| Error | HTTP Status | Response |
|-------|-------------|----------|
| Instance not found | 404 | `{"error": "instance not found"}` |
| Extension not installed (statements only) | 404 | `{"error": "pg_stat_statements extension not available", "code": "EXTENSION_NOT_FOUND"}` |
| Connection failed | 502 | `{"error": "failed to connect to instance"}` |
| Query timeout | 504 | `{"error": "query timed out"}` |
| Internal error | 500 | `{"error": "internal server error"}` |

Frontend hooks handle these via TanStack Query's `error` state — display a brief error message in the section, don't crash the page.

---

## 7. Testing Notes

### Backend Tests

- `internal/api/handler_statements_test.go` — test sort validation (whitelist), limit clamping, extension not found response
- `internal/api/handler_locks_test.go` — test tree building from mock pg_stat_activity data, empty tree, single root, multi-root
- `internal/api/handler_progress_test.go` — test version gating (PG 14 gets 6 queries, PG 12 gets 3), empty results

### Frontend Tests

Deferred — no frontend test framework established yet in M5. Manual verification via browser.

### Build Verification

After implementation, verify:
- `go build ./cmd/... ./internal/...`
- `go vet ./...`
- `golangci-lint run`
- `go test ./...`
- `cd web && npx tsc --noEmit && npx eslint src/ && npx vite build`
