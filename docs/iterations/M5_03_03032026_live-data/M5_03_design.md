# M5_03 Technical Design — Live Data Integration: Fleet Overview + Server Detail

**Iteration:** M5_03
**Date:** 2026-03-03
**Author:** Claude.ai (Brain contour)

---

## Design Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D95 | Polling via TanStack Query refetchInterval; SSE deferred | Zero backend changes for refresh; visual experience identical to SSE at 10s intervals |
| D96 | Server Detail scope: Tier 1 + Tier 2 (8 sections) | Enough to demonstrate full monitoring value; defers complex views to M5_04 |
| D97 | Time range: presets + custom date picker with HTML5 datetime-local | Covers forensic analysis; native inputs avoid new library dependency |
| D98 | Hybrid API: /metrics/current for snapshots, /metrics/history for charts | One call powers cards/headers; per-metric calls for charts |
| D99 | Fleet uses ?include=metrics,alerts on instance list | Single API call for fleet instead of N+1 per instance |

---

## 1. Backend API Additions

### 1.1 GET /api/v1/instances (Updated)

Add optional include query parameter. When include=metrics,alerts, join latest metric values and active alert counts per instance.

```go
// internal/api/instances.go — updated handler
func (h *InstanceHandler) List(w http.ResponseWriter, r *http.Request) {
    instances, err := h.store.ListInstances(r.Context())
    includes := r.URL.Query()["include"]
    includeMetrics := slices.Contains(includes, "metrics")
    includeAlerts := slices.Contains(includes, "alerts")

    type EnrichedInstance struct {
        Instance
        Metrics     map[string]float64 `json:"metrics,omitempty"`
        AlertCounts map[string]int     `json:"alert_counts,omitempty"`
    }
    // enrich each instance as needed
}
```

Key metrics to embed: pgpulse.connections.utilization_pct, pgpulse.cache.hit_ratio, pgpulse.replication.max_lag_bytes, pgpulse.server.version_num, pgpulse.server.is_in_recovery.

### 1.2 GET /api/v1/instances/:id/metrics/current

Latest value of every metric for one instance.

```go
// internal/storage/queries.go
func (s *MetricStore) CurrentMetrics(ctx context.Context, instanceID string) (*CurrentMetricsResponse, error) {
    query := `SELECT DISTINCT ON (metric) metric, value, labels, collected_at
              FROM metrics WHERE instance_id = $1
              ORDER BY metric, collected_at DESC`
}

type CurrentMetricsResponse struct {
    InstanceID  string                 `json:"instance_id"`
    CollectedAt time.Time              `json:"collected_at"`
    Metrics     map[string]MetricValue `json:"metrics"`
}

type MetricValue struct {
    Value  float64           `json:"value"`
    Labels map[string]string `json:"labels,omitempty"`
}
```

### 1.3 GET /api/v1/instances/:id/metrics/history

Time-series for charts. Params: metric (repeatable), from, to, step (optional: 1m/5m/15m/1h/1d).

```go
func (s *MetricStore) HistoryMetrics(ctx context.Context, req HistoryRequest) (*HistoryResponse, error) {
    if req.Step != "" {
        // time_bucket (TimescaleDB) or date_trunc fallback
        query := `SELECT time_bucket($1, collected_at) AS bucket, metric, AVG(value) AS value
                  FROM metrics
                  WHERE instance_id = $2 AND metric = ANY($3)
                    AND collected_at >= $4 AND collected_at <= $5
                  GROUP BY bucket, metric ORDER BY bucket`
    } else {
        // Raw points capped at 1000 per metric
    }
}

type HistoryResponse struct {
    InstanceID string                          `json:"instance_id"`
    From       time.Time                       `json:"from"`
    To         time.Time                       `json:"to"`
    Step       string                          `json:"step,omitempty"`
    Series     map[string][]TimeSeriesPoint    `json:"series"`
}

type TimeSeriesPoint struct {
    T time.Time `json:"t"`
    V float64   `json:"v"`
}
```

Step validation: only 1m, 5m, 15m, 1h, 1d. Reject others with 400.

### 1.4 GET /api/v1/instances/:id/replication

Live query to monitored instance via InstanceConnProvider. Returns structured replication data.

```go
type ReplicationResponse struct {
    InstanceID  string           `json:"instance_id"`
    Role        string           `json:"role"`
    Replicas    []ReplicaInfo    `json:"replicas,omitempty"`
    Slots       []SlotInfo       `json:"slots,omitempty"`
    WALReceiver *WALReceiverInfo `json:"wal_receiver,omitempty"`
}

type ReplicaInfo struct {
    ClientAddr      string  `json:"client_addr"`
    ApplicationName string  `json:"application_name"`
    State           string  `json:"state"`
    SyncState       string  `json:"sync_state"`
    Lag             LagInfo `json:"lag"`
}

type LagInfo struct {
    PendingBytes int64  `json:"pending_bytes"`
    WriteBytes   int64  `json:"write_bytes"`
    FlushBytes   int64  `json:"flush_bytes"`
    ReplayBytes  int64  `json:"replay_bytes"`
    WriteLag     string `json:"write_lag,omitempty"`
    FlushLag     string `json:"flush_lag,omitempty"`
    ReplayLag    string `json:"replay_lag,omitempty"`
}

type SlotInfo struct {
    SlotName         string `json:"slot_name"`
    SlotType         string `json:"slot_type"`
    Active           bool   `json:"active"`
    WALRetainedBytes int64  `json:"wal_retained_bytes"`
}

type WALReceiverInfo struct {
    SenderHost string `json:"sender_host"`
    SenderPort int    `json:"sender_port"`
    Status     string `json:"status"`
}
```

SQL for primary: pg_stat_replication with pg_wal_lsn_diff for lag by phase, pg_replication_slots for slot health. SQL for replica: pg_stat_wal_receiver.

### 1.5 GET /api/v1/instances/:id/activity/wait-events

Live query. Returns wait event distribution.

```go
type WaitEventsResponse struct {
    Events        []WaitEvent `json:"events"`
    TotalBackends int         `json:"total_backends"`
}
type WaitEvent struct {
    WaitEventType string `json:"wait_event_type"`
    WaitEvent     string `json:"wait_event"`
    Count         int    `json:"count"`
}
```

SQL: SELECT wait_event_type, wait_event, count(*) FROM pg_stat_activity WHERE wait_event IS NOT NULL AND pid != pg_backend_pid() GROUP BY 1,2 ORDER BY 3 DESC.

### 1.6 GET /api/v1/instances/:id/activity/long-transactions

Live query. Threshold default 5s, configurable via ?threshold_seconds= param.

```go
type LongTransaction struct {
    PID             int       `json:"pid"`
    Username        string    `json:"username"`
    Database        string    `json:"database"`
    State           string    `json:"state"`
    Waiting         bool      `json:"waiting"`
    DurationSeconds float64   `json:"duration_seconds"`
    Query           string    `json:"query"`
    XactStart       time.Time `json:"xact_start"`
}
```

SQL: SELECT pid, usename, datname, state, wait_event IS NOT NULL, EXTRACT(EPOCH FROM (now()-xact_start)), LEFT(query, 200), xact_start FROM pg_stat_activity WHERE xact_start IS NOT NULL AND state != 'idle' AND duration > threshold ORDER BY duration DESC.

### 1.7 InstanceConnProvider Interface

```go
type InstanceConnProvider interface {
    ConnFor(ctx context.Context, instanceID string) (*pgx.Conn, error)
}
```

Orchestrator implements this. Handler borrows connection, queries, releases, then writes response. statement_timeout 5s on borrowed connections.

### 1.8 Route Wiring

All new endpoints under authenticated routes with view_all permission:

```go
r.Route("/api/v1/instances/{id}", func(r chi.Router) {
    r.Use(auth.RequirePermission(auth.PermViewAll))
    r.Get("/metrics/current", metricsHandler.Current)
    r.Get("/metrics/history", metricsHandler.History)
    r.Get("/replication", replicationHandler.Get)
    r.Get("/activity/wait-events", activityHandler.WaitEvents)
    r.Get("/activity/long-transactions", activityHandler.LongTransactions)
    r.Get("/alerts", alertsHandler.ListForInstance)
})
```

---

## 2. Frontend Architecture

### 2.1 TimeRangeStore

```typescript
// web/src/stores/timeRangeStore.ts
type PresetKey = '15m' | '1h' | '6h' | '24h' | '7d';
interface TimeRange {
  from: Date; to: Date; preset: PresetKey | 'custom';
}
interface TimeRangeState {
  range: TimeRange;
  setPreset: (preset: PresetKey) => void;
  setCustomRange: (from: Date, to: Date) => void;
}
```

For presets: from/to recalculated at query time (not stale). Store stores preset key; hook computes dates.

### 2.2 ECharts Theme

Dark theme registered globally via echarts.registerTheme('pgpulse-dark', theme). Transparent bg, slate-300 text, slate-600 axes, slate-700/50 grid. Palette: blue-500, emerald-500, amber-500, rose-500, violet-500, cyan-500. Tooltip: slate-800 bg.

### 2.3 Formatters

formatBytes, formatUptime, formatDuration, formatPercent, formatPGVersion (160004 to 16.4), formatTimestamp, thresholdColor.

### 2.4 TanStack Query Hooks

- useInstances({ include }) — refetch 30s
- useCurrentMetrics(id) — refetch 10s
- useMetricsHistory(id, metrics[], { step }) — reads timeRangeStore, refetch 10s for presets, disabled for custom
- useReplication(id) — refetch 10s
- useWaitEvents(id) — refetch 10s
- useLongTransactions(id) — refetch 10s
- useInstanceAlerts(id) — refetch 30s

### 2.5 Chart Components

**TimeSeriesChart**: Generic ECharts line/area wrapper. Props: series[], referenceLine, yAxisLabel, loading, height. ResizeObserver, loading overlay, empty state.

**ConnectionGauge**: ECharts semicircular gauge. Green 0-70%, amber 70-90%, red 90-100%. Shows current/max center.

**WaitEventsChart**: Horizontal bars by wait_event_type. Lock=rose, IO=blue, LWLock=amber, Client=slate, Activity=emerald.

### 2.6 Fleet Overview Page

useInstances with include metrics,alerts. Grid of InstanceCard components. Each card: status dot, name, host:port, PG version badge, role badge, connection%, cache hit%, repl lag, alert badge. Clickable to /servers/:id. Responsive 1/2/3/4 columns.

Status logic: green (metric < 60s old), yellow (> 60s), red (> 300s or no metrics).

### 2.7 Server Detail Page

Layout:
```
Breadcrumb: Fleet Overview > instance-name
[HeaderCard — full width]
[MetricCard x4 row]
[TimeRangeSelector]
[ConnectionsChart | CacheHitChart] — 2-col grid
[ReplicationSection — full width]
[WaitEventsSection | LongTransactionsTable] — 2-col grid
[InstanceAlerts — full width]
```

**HeaderCard**: name, host:port, PG version, role badge, uptime, status dot, collected_at.

**KeyMetricsRow**: Connections (active/max + util%), Cache Hit (color-coded), Active Txns (count + waiting), Repl Lag (formatted or N/A).

**ConnectionsChart**: useMetricsHistory with connections.active/idle/total + max_connections reference line. Area style for active.

**CacheHitChart**: Single series, 95% reference line, y-axis zoomed 90-100%.

**ReplicationSection**: Primary shows lag table + slots table. Replica shows WAL receiver. Inactive slots red.

**WaitEventsSection**: WaitEventsChart in a card.

**LongTransactionsTable**: DataTable with PID, User, DB, State, Duration, Query. Yellow >60s, red >300s. Query truncated with expand.

**InstanceAlerts**: Filtered alert list with severity, rule, metric vs threshold.

### 2.8 TimeRangeSelector Component

```
[15m] [1h] [6h] [24h] [7d] [Custom]
                               From: [datetime-local]
                               To:   [datetime-local]
                               [Apply]
```

Active preset: blue-500/20 bg, blue-500 border. Custom dates applied on Apply click only.

---

## 3. File Inventory

### Backend (10 files, ~675 lines)

| File | Action |
|------|--------|
| internal/api/connprovider.go | Create |
| internal/storage/queries.go | Update |
| internal/api/metrics.go | Update |
| internal/api/replication.go | Create |
| internal/api/activity.go | Create |
| internal/api/instances.go | Update |
| internal/api/router.go | Update |
| internal/api/metrics_test.go | Update |
| internal/api/activity_test.go | Create |
| internal/api/replication_test.go | Create |

### Frontend (24 files, ~1395 lines)

| File | Action |
|------|--------|
| web/src/stores/timeRangeStore.ts | Create |
| web/src/lib/echartsTheme.ts | Create |
| web/src/lib/formatters.ts | Create |
| web/src/components/shared/TimeRangeSelector.tsx | Create |
| web/src/components/shared/AlertBadge.tsx | Create |
| web/src/components/charts/TimeSeriesChart.tsx | Create |
| web/src/components/charts/ConnectionGauge.tsx | Create |
| web/src/components/charts/WaitEventsChart.tsx | Create |
| web/src/components/fleet/InstanceCard.tsx | Create |
| web/src/pages/FleetOverviewPage.tsx | Rewrite |
| web/src/pages/ServerDetailPage.tsx | Rewrite |
| web/src/components/server/HeaderCard.tsx | Create |
| web/src/components/server/KeyMetricsRow.tsx | Create |
| web/src/components/server/ReplicationSection.tsx | Create |
| web/src/components/server/WaitEventsSection.tsx | Create |
| web/src/components/server/LongTransactionsTable.tsx | Create |
| web/src/components/server/InstanceAlerts.tsx | Create |
| web/src/hooks/useInstances.ts | Update |
| web/src/hooks/useMetrics.ts | Create |
| web/src/hooks/useReplication.ts | Create |
| web/src/hooks/useActivity.ts | Create |
| web/src/hooks/useAlerts.ts | Update |
| web/src/App.tsx | Update |
| web/src/components/layout/Sidebar.tsx | Update |

### Total: 34 files, ~2070 lines
