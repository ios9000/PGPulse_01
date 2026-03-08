# Session: 2026-03-03 — M5_03 Live Data Integration: Fleet Overview + Server Detail

## Goal

Replace all mock data in the PGPulse frontend with real backend API data. Build Fleet Overview with live instance cards and Server Detail with 8 sections: header, key metrics, connections chart, cache hit chart, replication, wait events, long transactions, and instance alerts. Add time range selector with presets + custom date picker.

## Agent Team Configuration

- Team Lead: Opus 4.6
- Specialists: Backend Agent + Frontend Agent (2 agents)
- QA performed by Team Lead after agent shutdown
- Implementation duration: ~18 minutes
- Verification duration: ~4 minutes
- Total session: ~22 minutes
- Commits: 7d97fc4 (code), 3c739dc (docs)

## Design Decisions Applied

| # | Decision | Applied |
|---|----------|---------|
| D95 | Polling via TanStack Query refetchInterval; SSE deferred | Yes — 10s live, 30s fleet |
| D96 | Server Detail Tier 1 + Tier 2 (8 sections) | Yes — all 8 sections built |
| D97 | Time range presets + custom date picker | Yes — HTML5 datetime-local |
| D98 | Hybrid API: /metrics/current + /metrics/history | Yes |
| D99 | Fleet uses ?include=metrics,alerts | Yes — single call for fleet |

## Backend Changes (10 files, ~1100 lines)

### Created (6 files)
| File | Purpose |
|------|---------|
| internal/api/connprovider.go | InstanceConnProvider interface for live queries |
| internal/storage/queries.go | CurrentMetrics, HistoryMetrics, CurrentMetricValues, ValidStep |
| internal/api/replication.go | Replication handler + ReplicationResponse types |
| internal/api/activity.go | WaitEvents + LongTransactions handlers |
| internal/api/activity_test.go | 6 tests for activity endpoints |
| internal/api/replication_test.go | 3 tests for replication endpoint |

### Modified (4 files)
| File | Changes |
|------|---------|
| internal/api/server.go | connProvider field, SetConnProvider(), 5 new routes |
| internal/api/metrics.go | MetricsQuerier interface, handleCurrentMetrics, handleMetricsHistory |
| internal/api/instances.go | ?include=metrics,alerts enrichment |
| internal/api/metrics_test.go | 6 new tests |
| internal/api/helpers_test.go | mockMetricsStore for testing |

### New API Endpoints
| Method | Path | Purpose |
|--------|------|---------|
| GET | /api/v1/instances?include=metrics,alerts | Enriched instance list |
| GET | /api/v1/instances/:id/metrics/current | Latest metric snapshot |
| GET | /api/v1/instances/:id/metrics/history | Time-series for charts |
| GET | /api/v1/instances/:id/replication | Live replication data |
| GET | /api/v1/instances/:id/activity/wait-events | Wait event distribution |
| GET | /api/v1/instances/:id/activity/long-transactions | Long-running transactions |

## Frontend Changes (26 files, ~1430 lines)

### Created (20 files)
| File | Purpose |
|------|---------|
| web/src/stores/timeRangeStore.ts | Zustand store for time range (presets + custom) |
| web/src/lib/formatters.ts | formatBytes, formatUptime, formatDuration, etc. |
| web/src/lib/echartsTheme.ts | PGPulse dark theme for ECharts |
| web/src/hooks/useInstances.ts | Fleet data hook with include param |
| web/src/hooks/useMetrics.ts | useCurrentMetrics + useMetricsHistory |
| web/src/hooks/useReplication.ts | Replication data hook |
| web/src/hooks/useActivity.ts | useWaitEvents + useLongTransactions |
| web/src/hooks/useAlerts.ts | useInstanceAlerts |
| web/src/components/shared/TimeRangeSelector.tsx | Preset buttons + custom date picker |
| web/src/components/shared/AlertBadge.tsx | Warning/critical count pill |
| web/src/components/charts/TimeSeriesChart.tsx | Generic ECharts line/area wrapper |
| web/src/components/charts/ConnectionGauge.tsx | Semicircular connection utilization gauge |
| web/src/components/charts/WaitEventsChart.tsx | Horizontal bar chart by wait event type |
| web/src/components/fleet/InstanceCard.tsx | Clickable instance card for fleet grid |
| web/src/components/server/HeaderCard.tsx | Instance header with version, uptime, role |
| web/src/components/server/KeyMetricsRow.tsx | 4 MetricCards row |
| web/src/components/server/ReplicationSection.tsx | Lag table + slots table or WAL receiver |
| web/src/components/server/WaitEventsSection.tsx | Wait events card wrapper |
| web/src/components/server/LongTransactionsTable.tsx | Long transaction table with severity colors |
| web/src/components/server/InstanceAlerts.tsx | Filtered alert list per instance |

### Modified (6 files)
| File | Changes |
|------|---------|
| web/src/App.tsx | ECharts theme registration, /servers/:id route |
| web/src/pages/FleetOverview.tsx | Rewritten — real data via useInstances |
| web/src/pages/ServerDetail.tsx | Rewritten — full 8-section detail layout |
| web/src/components/layout/Sidebar.tsx | Real instance list integration |
| web/src/lib/echarts-setup.ts | Added MarkLineComponent |
| web/src/types/models.ts | All API response types added |

## Verification Results

| Check | Result |
|-------|--------|
| go build ./cmd/... ./internal/... | Pass |
| go vet ./cmd/... ./internal/... | Pass |
| go test ./cmd/... ./internal/... | Pass (all packages) |
| golangci-lint run | 1 pre-existing issue (static.go errcheck) |
| tsc --noEmit | Pass |
| eslint src/ | Pass |
| vite build | Pass |
| No mock data in Fleet/Server Detail | Confirmed |
| Nil connProvider safety | All 3 live-query handlers check |

## Known Limitations

- **InstanceConnProvider is nil at runtime** — the orchestrator does not yet implement ConnFor() and main.go does not call SetConnProvider(). Replication and activity endpoints return 501 until wired. Storage-backed endpoints (metrics/current, metrics/history, instances with include) work immediately.
- **Pre-existing golangci-lint issue** — static.go:28 errcheck on f.Close() (not from M5_03)
- **ECharts chunk size** — not optimized yet (deferred)

## Commit Summary

- **7d97fc4**: feat(api,web): add live data endpoints and wire Fleet/Server Detail pages (M5_03) — 37 files changed, +2530/-124
- **3c739dc**: docs: add M5_03 design, requirements, and team prompt — 3 files, +623

## What's Next

M5_04 candidates:
- pg_stat_statements top queries view (complex table + drill-down)
- Lock tree visualization (tree rendering)
- Progress monitoring (VACUUM, CREATE INDEX — transient data)
- Per-database drill-down
- Wire InstanceConnProvider in main.go (enables live replication/activity endpoints)
