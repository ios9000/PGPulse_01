# Codebase Digest — Generation Rules

> **Location:** `.claude/rules/codebase-digest.md`
> **Purpose:** Instructions for Claude Code to generate `docs/CODEBASE_DIGEST.md`
> **When:** End of every iteration, after build verification passes

---

## Task

Read the entire PGPulse codebase and produce `docs/CODEBASE_DIGEST.md` with the 7 sections below. This document is consumed by Claude.ai (Brain contour) during planning sessions — it must be accurate, complete, and self-contained.

**Rules:**
- Regenerate the ENTIRE file from scratch each time (do not patch)
- Include only what EXISTS in the code — never speculate or include planned features
- Use exact function names, exact metric keys, exact file paths
- Keep descriptions to one line per item — this is a reference index, not documentation
- Sort alphabetically within each section unless another order is specified

---

## Section 1 — File Inventory

List every `.go`, `.ts`, `.tsx`, `.css` source file (exclude `node_modules/`, `.git/`, `vendor/`, test fixtures, generated files in `web/dist/`).

Format:
```
### internal/collector/
| File | Lines | Purpose |
|------|-------|---------|
| cache.go | 87 | CacheCollector — buffer cache hit ratio from pg_stat_database |
| checkpoint.go | 156 | CheckpointCollector — stateful, version-gated PG16/17 split |
```

Group by directory. Include line count (wc -l).

---

## Section 2 — Interface & Type Registry

List every exported interface and key exported struct that crosses module boundaries.

Format:
```
### Interfaces
| Interface | File | Methods | Used By |
|-----------|------|---------|---------|
| Collector | internal/collector/collector.go | Name(), Collect(), Interval() | All collectors, orchestrator |
| MetricStore | internal/collector/collector.go | Write(), Query(), Close() | storage/timescale.go, api/ |

### Key Structs
| Struct | File | Fields (summary) | Used By |
|--------|------|-------------------|---------|
| MetricPoint | internal/collector/collector.go | InstanceID, Metric, Value, Labels, Timestamp | Everywhere |
| InstanceContext | internal/collector/collector.go | IsRecovery bool | Orchestrator → all collectors |
```

Include the FULL current signature for each interface (copy from source).

---

## Section 3 — Metric Key Catalog

List every metric key string emitted by every collector.

Format:
```
| Metric Key | Collector | Group | Type | Labels |
|------------|-----------|-------|------|--------|
| pgpulse.connections.active | ConnectionsCollector | high | gauge | — |
| pgpulse.connections.idle | ConnectionsCollector | high | gauge | — |
| pgpulse.replication.lag.replay_bytes | ReplicationLagCollector | medium | gauge | replica |
| os.memory.total_kb | OSSQLCollector | medium | gauge | — |
```

Sort by metric key. Include ALL keys — do not summarize or group.

---

## Section 4 — API Endpoint Table

List every route registered in the chi router.

Format:
```
| Method | Path | Handler | Auth | Response Summary |
|--------|------|---------|------|------------------|
| GET | /api/v1/instances | ListInstances | viewer+ | []Instance with optional metrics/alerts |
| POST | /api/v1/instances | CreateInstance | admin+ | Instance |
| GET | /api/v1/instances/{id}/metrics/current | GetCurrentMetrics | viewer+ | map[string]MetricPoint |
```

Include middleware notes (e.g., "rate-limited", "CORS").

---

## Section 5 — Frontend Component Map

List every page and its data dependencies.

Format:
```
### Pages
| Page | Route | File | Sections |
|------|-------|------|----------|
| FleetOverview | /fleet | pages/FleetOverview.tsx | InstanceCard[] |
| ServerDetail | /servers/:id | pages/ServerDetail.tsx | HeaderCard, KeyMetrics, ConnectionsChart, ... |

### Data Flow
| Component | API Endpoint(s) | Metric Keys Consumed | Refresh Interval |
|-----------|----------------|---------------------|-----------------|
| ConnectionsChart | /instances/{id}/metrics/history | pgpulse.connections.active, .idle, .total | 10s |
| OSSystemSection | /instances/{id}/metrics/current | os.memory.*, os.load.*, os.cpu.* | 30s |
```

---

## Section 6 — Collector Registry

Copy the actual collector assignments from the orchestrator's `buildCollectors()` or equivalent registration code.

Format:
```
### High Frequency (default 10s)
| Collector | Constructor | Stateful | Version-Gated | Notes |
|-----------|-------------|----------|---------------|-------|
| ConnectionsCollector | NewConnectionsCollector | no | no | — |
| WaitEventsCollector | NewWaitEventsCollector | no | no | — |

### Medium Frequency (default 60s)
...

### Low Frequency (default 300s)
...
```

---

## Section 7 — Configuration Schema

List every config key from the YAML config structs and example config.

Format:
```
| Key Path | Type | Default | Component | Description |
|----------|------|---------|-----------|-------------|
| server.port | int | 8989 | main.go | HTTP listen port |
| server.cors_enabled | bool | false | api/router.go | Enable CORS headers |
| storage.dsn | string | required | storage/ | PGPulse metadata DB connection |
| instances[].dsn | string | required | orchestrator | Monitored instance connection |
| instances[].os_metrics_method | string | "sql" | orchestrator | "sql" or "agent" |
```

---

## Header

Every generated digest starts with:

```markdown
# PGPulse — Codebase Digest

> **Auto-generated by Claude Code** — do not edit manually
> **Date:** YYYY-MM-DD
> **Commit:** {git rev-parse HEAD}
> **Iteration:** M{X}_{NN}
> **Go files:** {count} ({total lines})
> **TypeScript files:** {count} ({total lines})
> **Metric keys:** {count}
> **API endpoints:** {count}
> **Collectors:** {count}
```
