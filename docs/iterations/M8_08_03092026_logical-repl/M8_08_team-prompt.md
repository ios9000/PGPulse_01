# M8_08 Team Prompt — Logical Replication Monitoring

Read CLAUDE.md for full project context.
Read `docs/iterations/M8_08/M8_08_design.md` for detailed specs.

This iteration adds a new backend sub-collector, an API endpoint, and a frontend section.
Use a **3-specialist team** (Collector Agent + Frontend Agent + QA Agent).

---

Create a team of 3 specialists:

## COLLECTOR AGENT

### Task 1: Logical Replication DB Sub-Collector

Open `internal/collector/database.go` and study the existing 16 DB sub-collector functions
to understand the pattern. Then add a new function:

```go
func collectLogicalReplication(ctx context.Context, conn *pgxpool.Conn, dbName string) ([]MetricPoint, error)
```

This function should:
1. Query `pg_subscription_rel JOIN pg_subscription WHERE srsubstate <> 'r'`
   - SELECT: `s.subname`, `r.srrelid::regclass::text`, `r.srsubstate`, `r.srsublsn::text`
2. Count non-ready tables
3. Return MetricPoint with metric `logical_replication_pending_sync_tables`, value = count,
   label `database` = dbName
4. If the query fails (e.g., pg_subscription doesn't exist), return 0 pending tables and
   log the error — do NOT fail the entire collection cycle
5. Register this function in the DB sub-collector list (same place as the other 16)

### Task 2: API Handler

Create `internal/api/logical_replication.go`:

The handler for `GET /instances/{id}/logical-replication` needs per-database connections.
Use the `SubstituteDatabase()` helper (from M8_01, `internal/api/` or similar) to connect
to each database discovered via `pg_database`.

Steps in the handler:
1. Get instance config/DSN by instance ID
2. Query `SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate`
   using the instance's main connection (Orchestrator.ConnFor or similar)
3. For each database:
   a. Substitute the database name into the DSN
   b. Open a temporary connection (pgx.Connect with 5s timeout)
   c. Query pg_subscription_rel (pending tables) + pg_stat_subscription (stats)
   d. Close the connection
4. Detect PG version once and conditionally include apply_error_count/sync_error_count
   (PG 15+ only) in the pg_stat_subscription query
5. Aggregate results into the response struct

Response structs (define in the same file):
```go
type LogicalReplicationResponse struct {
    Subscriptions      []SubscriptionStatus `json:"subscriptions"`
    TotalPendingTables int                  `json:"total_pending_tables"`
}

type SubscriptionStatus struct {
    Database         string             `json:"database"`
    SubscriptionName string             `json:"subscription_name"`
    TablesPending    []PendingTable     `json:"tables_pending"`
    Stats            *SubscriptionStats `json:"stats,omitempty"`
}

type PendingTable struct {
    TableName      string `json:"table_name"`
    SyncState      string `json:"sync_state"`
    SyncStateLabel string `json:"sync_state_label"`
    SyncLSN        string `json:"sync_lsn"`
}

type SubscriptionStats struct {
    PID             int    `json:"pid"`
    ReceivedLSN     string `json:"received_lsn"`
    LatestEndLSN    string `json:"latest_end_lsn"`
    LatestEndTime   string `json:"latest_end_time"`
    ApplyErrorCount *int   `json:"apply_error_count,omitempty"`
    SyncErrorCount  *int   `json:"sync_error_count,omitempty"`
}
```

Map sync states to labels: `i` → "Initializing", `d` → "Data Copy", `s` → "Synchronized",
`f` → "Finalized".

### Task 3: Route Registration

In `internal/api/server.go`, register in the viewer permission group:
```go
r.Get("/instances/{id}/logical-replication", s.handleLogicalReplication)
```

### Task 4: Alert Rule Seed

In `internal/alert/rules.go`, add to the builtin rules list:
```go
{
    Name:        "logical_repl_pending_sync",
    Metric:      "logical_replication_pending_sync_tables",
    Operator:    ">",
    Threshold:   0,
    Severity:    "warning",
    CooldownMin: 10,
    Enabled:     false,
    Source:      "builtin",
    Description: "Logical replication tables not fully synchronized",
}
```

### Important Notes for Collector Agent
- All SQL must use parameterized queries (pgx named args or $1 positional)
- Set `application_name = 'pgpulse_logical_repl'` on temporary connections
- Set `statement_timeout = '5s'` on all queries
- Close temporary connections in defer blocks
- Do NOT modify existing collectors or the replication.go file

---

## FRONTEND AGENT

### Task: Logical Replication Section

Create `web/src/hooks/useLogicalReplication.ts`:
- Fetch `GET /api/v1/instances/{instanceId}/logical-replication`
- 30s refetch interval
- Return: data, isLoading, error

Create `web/src/components/LogicalReplicationSection.tsx`:
- No subscriptions (empty array or total_pending = 0 with no subscriptions):
  → Info card: "No logical subscriptions configured on this instance"
- Has subscriptions with no pending tables:
  → Green success card: "All logical replication tables synchronized"
- Has subscriptions with pending tables:
  - Summary: "{N} subscriptions, {M} tables pending sync"
  - Per-subscription expandable card:
    - Header: subscription name (bold), database badge (blue), PID
    - Stats: received LSN, latest end time, last message time
    - Error badges (PG 15+): red badge if apply_error_count > 0 or sync_error_count > 0
    - Pending tables table:
      - Columns: Table Name | Sync State | Sync LSN
      - State badges:
        - `i` / "Initializing" → blue (`bg-blue-100 text-blue-800`)
        - `d` / "Data Copy" → amber (`bg-amber-100 text-amber-800`)
        - `s` / "Synchronized" → green (`bg-green-100 text-green-800`)
        - `f` / "Finalized" → teal (`bg-teal-100 text-teal-800`)

Integrate into `ServerDetail.tsx`:
- Place after the existing physical replication section
- Always render the section (even if no subscriptions — show the info card)
- Section header: "Logical Replication"

### Important Notes for Frontend Agent
- Use Tailwind core utility classes only
- Use existing patterns (expandable cards, badges, etc.) from other ServerDetail sections
- Handle loading and error states
- No permission gating needed (this is read-only data, viewer-accessible)

---

## QA AGENT

### Verification Tasks

1. **Go build:** `go build ./cmd/pgpulse-server` — success
2. **Go tests:** `go test ./cmd/... ./internal/...` — all pass
3. **Go lint:** `golangci-lint run` — 0 issues
4. **Frontend typecheck:** `cd web && npx tsc --noEmit` — 0 errors
5. **Frontend lint:** `cd web && npm run lint` — 0 errors
6. **Frontend build:** `cd web && npm run build` — success

### Code Review Checks

- Verify the sub-collector handles pg_subscription not existing gracefully (error → log + return 0)
- Verify temporary connections are closed in defer blocks
- Verify application_name and statement_timeout are set on temp connections
- Verify PG version gate for apply_error_count/sync_error_count (PG 15+)
- Verify all SQL is parameterized (no string concatenation)
- Verify the API route is in the viewer permission group (read-only)
- Verify alert rule has `Enabled: false` (disabled by default)
- Verify no `any` types in TypeScript
- Verify sync state label mapping covers all states (i, d, s, f)

---

## Coordination

- Collector Agent creates the sub-collector + API handler + route + alert rule
- Frontend Agent creates the hook + section component + integrates into ServerDetail
- Both can work in parallel (Frontend Agent can stub the TypeScript types from the design doc
  and adjust when Collector Agent's response struct is finalized)
- QA Agent reviews and runs full build verification
- Merge only when all checks pass
