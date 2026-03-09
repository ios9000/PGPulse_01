# M8_08 Requirements — Logical Replication Monitoring

**Iteration:** M8_08
**Date:** 2026-03-09
**Scope:** Backend collector + API endpoint + frontend section

---

## Goal

Port PGAM query Q41 (logical replication sync status) to PGPulse, using the M7 DBRunner
framework for per-database connections. This was deferred since M1_02b because it requires
connecting to each database individually — a constraint that no longer applies since M7
introduced the `DBRunner` with per-database pool management.

---

## Background

### PGAM Query (Q41)
```sql
-- Connects to each database, not just 'postgres'
SELECT
    s.subname       AS subscription_name,
    r.srrelid::regclass AS table_name,
    r.srsubstate    AS sync_state,
    r.srsublsn      AS sync_lsn
FROM pg_subscription_rel r
JOIN pg_subscription s ON s.oid = r.srsubid
WHERE r.srsubstate <> 'r'   -- exclude fully synced ('r' = ready)
```

States in `srsubstate`:
- `i` — initializing (initial table copy in progress)
- `d` — data copy (bulk copy phase)
- `s` — synchronized (caught up, waiting for final switch)
- `f` — finalized (sync complete, not yet marked ready)
- `r` — ready (fully synced — excluded from query)

### Why It Was Deferred

The original collector pattern used a single `pgx.Conn` to the `postgres` database per instance.
`pg_subscription_rel` is per-database — subscriptions exist in the database where they were
created, not in `postgres`. M7's `DBRunner` solves this by maintaining a dynamic pool map
with one `pgxpool.Pool` per discovered database.

### What Already Exists

PGPulse already monitors physical replication (M1_02b):
- `collector/replication.go` — physical replica lag, WAL receiver status
- Replication slots (physical + logical slot names, active status, WAL retention)
- `GET /api/v1/instances/{id}/replication` — serves physical replication data

What's missing is the **logical subscription sync table status** — knowing whether
subscribed tables are still catching up or fully synchronized.

---

## Requirements

### Backend: Logical Replication Sub-Collector

1. New DB sub-collector registered with DBRunner (same pattern as the 16 sub-collectors in M7)
2. Queries `pg_subscription_rel JOIN pg_subscription WHERE srsubstate <> 'r'` per database
3. Also query `pg_stat_subscription` for subscription-level stats:
   ```sql
   SELECT
       s.subname,
       ss.pid,
       ss.received_lsn,
       ss.latest_end_lsn,
       ss.latest_end_time,
       ss.last_msg_send_time,
       ss.last_msg_receipt_time
   FROM pg_stat_subscription ss
   JOIN pg_subscription s ON s.oid = ss.subid
   ```
4. Version gate: `pg_subscription` exists in PG 10+ — we support PG 14+ so no gate needed
   for basic subscription queries. However, `pg_stat_subscription` columns differ:
   - PG 15+: adds `apply_error_count`, `sync_error_count`
   - Check and include if available
5. Return as MetricPoints AND as structured data for the API
6. Produce both:
   - Numeric metrics: `logical_replication_pending_sync_tables` (count of non-ready tables per DB)
   - Structured snapshot for API consumption (full table list with states)

### Backend: API Endpoint

1. `GET /api/v1/instances/{id}/logical-replication` — returns logical subscription status
2. Response shape:
   ```json
   {
     "subscriptions": [
       {
         "database": "mydb",
         "subscription_name": "my_sub",
         "tables_pending": [
           {
             "table_name": "public.orders",
             "sync_state": "d",
             "sync_state_label": "Data Copy",
             "sync_lsn": "0/1A2B3C4D"
           }
         ],
         "stats": {
           "pid": 12345,
           "received_lsn": "0/1A2B3C5E",
           "latest_end_lsn": "0/1A2B3C5E",
           "latest_end_time": "2026-03-09T12:00:00Z",
           "apply_error_count": 0,
           "sync_error_count": 0
         }
       }
     ],
     "total_pending_tables": 3
   }
   ```
3. When no subscriptions exist or all tables are synced → return empty subscriptions array
   and `total_pending_tables: 0`

### Frontend: Logical Replication Section

1. New section in ServerDetail: "Logical Replication" (in the replication area,
   after existing physical replication section)
2. When no subscriptions: show "No logical subscriptions configured" info card
3. When subscriptions exist:
   - Summary card: total pending tables count, subscription count
   - Per-subscription expandable card:
     - Subscription name, database, worker PID, received LSN
     - Table of pending tables: table name, sync state (with colour-coded badge),
       sync LSN
     - State badges: `i` = blue "Initializing", `d` = amber "Copying",
       `s` = green "Synchronized", `f` = teal "Finalized"
   - Error counts (PG 15+): if apply_error_count > 0 or sync_error_count > 0,
     show red warning badge
4. Auto-refresh: 30s polling (matches replication section cadence)
5. Alert integration: `logical_replication_pending_sync_tables > 0` for longer than
   threshold → existing evaluator handles this if a rule is seeded

### Alert Rule (Optional)

Seed one new alert rule (disabled by default):
- Metric: `logical_replication_pending_sync_tables`
- Operator: `>`
- Threshold: `0`
- Severity: `warning`
- Cooldown: 10 minutes
- Description: "Logical replication tables not fully synchronized"

---

## Out of Scope

- Logical replication slot lag (already monitored in replication.go via slot queries)
- Subscription CREATE/ALTER/DROP management from UI
- Publisher-side monitoring (pg_publication, pg_publication_tables)
- Conflict resolution or error recovery actions
