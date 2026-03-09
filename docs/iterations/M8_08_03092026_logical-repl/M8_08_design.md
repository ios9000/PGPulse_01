# M8_08 Design — Logical Replication Monitoring

**Iteration:** M8_08
**Date:** 2026-03-09
**Scope:** Backend sub-collector + API endpoint + frontend section

---

## Architecture

Logical replication monitoring uses the DBRunner pattern from M7. The sub-collector
runs per database (not per instance), because `pg_subscription_rel` is database-local.

```
Orchestrator → DBRunner → LogicalReplCollector.Collect(ctx, conn, dbName)
                                    ↓
                           pg_subscription_rel
                           pg_stat_subscription
                                    ↓
                           MetricPoints + structured data
                                    ↓
                              Storage (metrics)
                              API (structured)
```

The structured data (subscription details with pending tables) is served by a **new
API handler** that queries the data directly from the monitored instance — same pattern
as the replication endpoint from M5_03 which uses `Orchestrator.ConnFor()` for live queries.

---

## 1. Backend: Sub-Collector

### New File: Additions to `internal/collector/database.go`

Add a new DB sub-collector function following the existing pattern (16 sub-collectors
are already registered there). The new function:

```go
func collectLogicalReplication(ctx context.Context, conn *pgxpool.Conn, dbName string) ([]MetricPoint, error) {
    // Query 1: pending sync tables
    rows, err := conn.Query(ctx, `
        SELECT
            s.subname,
            r.srrelid::regclass::text AS table_name,
            r.srsubstate,
            r.srsublsn::text
        FROM pg_subscription_rel r
        JOIN pg_subscription s ON s.oid = r.srsubid
        WHERE r.srsubstate <> 'r'
    `)
    // ...

    // Return metric: count of non-ready tables
    return []MetricPoint{{
        Metric: "logical_replication_pending_sync_tables",
        Value:  float64(pendingCount),
        Labels: map[string]string{"database": dbName},
    }}, nil
}
```

Register in the DB sub-collector list alongside the existing 16.

**Error handling:** If `pg_subscription` doesn't exist (e.g., PG compiled without
logical replication support), the query will fail. Catch the error and return 0
pending tables — don't fail the entire DB collection cycle.

---

## 2. Backend: API Handler

### New File: `internal/api/logical_replication.go`

```go
func (s *Server) handleLogicalReplication(w http.ResponseWriter, r *http.Request) {
    instanceID := chi.URLParam(r, "id")

    // Use ConnFor pattern (same as replication endpoint)
    // But need per-database connections — iterate over databases

    // Option A: Query the collector's stored metrics + supplement with live query
    // Option B: Live query each database via DBRunner's pool map

    // Recommend Option B for freshness, but it requires access to DBRunner's pools.
    // If DBRunner pools aren't exposed to the API layer, fall back to Option A:
    // Use Orchestrator.ConnFor() for each database discovered via pg_database.
}
```

**Design decision needed by agent:** How to get per-database connections in the API handler.
Three options:

1. **Expose DBRunner pool map to API** — add a method like `Orchestrator.DBConnFor(instanceID, dbName)`.
   Clean but requires orchestrator changes.
2. **Open fresh connections per database** — use the instance's DSN with database name substituted
   (SubstituteDatabase helper from M8_01 already exists). Simple, no orchestrator changes.
3. **Query from storage** — serve the latest collected metrics. Stale by up to one collection
   interval (~60s) but zero live queries. Simplest.

Recommend **Option 2** for the structured API response (uses existing `SubstituteDatabase`),
plus **Option 3** for the numeric metric (already stored by the sub-collector).

### Route Registration

```go
// In server.go, viewer group:
r.Get("/instances/{id}/logical-replication", s.handleLogicalReplication)
```

### Response Shape

```go
type LogicalReplicationResponse struct {
    Subscriptions       []SubscriptionStatus `json:"subscriptions"`
    TotalPendingTables  int                  `json:"total_pending_tables"`
}

type SubscriptionStatus struct {
    Database         string            `json:"database"`
    SubscriptionName string            `json:"subscription_name"`
    TablesPending    []PendingTable    `json:"tables_pending"`
    Stats            *SubscriptionStats `json:"stats,omitempty"`
}

type PendingTable struct {
    TableName      string `json:"table_name"`
    SyncState      string `json:"sync_state"`
    SyncStateLabel string `json:"sync_state_label"`
    SyncLSN        string `json:"sync_lsn"`
}

type SubscriptionStats struct {
    PID               int    `json:"pid"`
    ReceivedLSN       string `json:"received_lsn"`
    LatestEndLSN      string `json:"latest_end_lsn"`
    LatestEndTime     string `json:"latest_end_time"`
    ApplyErrorCount   *int   `json:"apply_error_count,omitempty"`   // PG 15+
    SyncErrorCount    *int   `json:"sync_error_count,omitempty"`    // PG 15+
}
```

### Version Gate

`pg_stat_subscription.apply_error_count` and `sync_error_count` are PG 15+ only.
Use the existing version detection to conditionally include these columns:

```go
if pgVersion.AtLeast(15, 0) {
    // include error count columns
} else {
    // omit from SELECT, set to nil in response
}
```

---

## 3. Frontend

### New Files

**`web/src/hooks/useLogicalReplication.ts`**

```tsx
interface LogicalReplicationData {
  subscriptions: SubscriptionStatus[];
  total_pending_tables: number;
}

function useLogicalReplication(instanceId: string): {
  data: LogicalReplicationData | null;
  isLoading: boolean;
  error: Error | null;
}
// GET /api/v1/instances/{id}/logical-replication
// 30s refetch
```

**`web/src/components/LogicalReplicationSection.tsx`**

- No subscriptions → "No logical subscriptions configured" info card
- Has subscriptions:
  - Summary card: "{N} subscriptions, {M} tables pending sync"
  - Per-subscription card (expandable):
    - Header: subscription name, database badge, worker PID
    - Stats row: received LSN, latest end time
    - Error counts (PG 15+): if > 0, red badge with count
    - Table of pending tables:
      - Columns: Table Name, Sync State, Sync LSN
      - State badges with colours:
        - `i` → blue "Initializing"
        - `d` → amber "Copying"
        - `s` → green "Synchronized"
        - `f` → teal "Finalized"
  - All synced (empty tables_pending) → green checkmark "All tables synchronized"

### Integration Point

Add "Logical Replication" section in ServerDetail, placed after the existing physical
replication section. Show/hide based on whether data is available (if the instance has
no subscriptions, show the info card rather than hiding the section entirely — this lets
users know the feature exists).

---

## 4. Alert Rule (Optional)

Add to `internal/alert/rules.go` seed list:

```go
{
    Name:        "logical_repl_pending_sync",
    Metric:      "logical_replication_pending_sync_tables",
    Operator:    ">",
    Threshold:   0,
    Severity:    "warning",
    CooldownMin: 10,
    Enabled:     false,  // disabled by default
    Source:      "builtin",
    Description: "Logical replication tables not fully synchronized",
}
```

---

## File Summary

| Action | File |
|--------|------|
| MODIFY | `internal/collector/database.go` — add logical replication sub-collector |
| CREATE | `internal/api/logical_replication.go` — handler + response structs |
| MODIFY | `internal/api/server.go` — register route |
| MODIFY | `internal/alert/rules.go` — seed new alert rule (disabled) |
| CREATE | `web/src/hooks/useLogicalReplication.ts` |
| CREATE | `web/src/components/LogicalReplicationSection.tsx` |
| MODIFY | `web/src/pages/ServerDetail.tsx` — add section |

---

## Build Verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```
