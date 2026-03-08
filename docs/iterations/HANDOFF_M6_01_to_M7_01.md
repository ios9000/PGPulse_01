# PGPulse — Iteration Handoff: M6_01 → M7_01

## DO NOT RE-DISCUSS

- **OS Agent deployment model:** Option C — optional agent with graceful degradation. OSSourceNone returns (nil, nil), never an error. Locked D-M6-01.
- **Patroni/ETCD provider pattern:** FallbackProvider(RESTProvider, ShellProvider). NoOpProvider when not configured. Interface is PatroniProvider / ETCDProvider. Locked D-M6-02.
- **Build tags:** All procfs/sysfs code uses `//go:build linux` with `//go:build !linux` stubs. Dev machine is Windows — this is mandatory, not optional. Locked D-M6-03.
- **ClusterCollector errors:** WARN log only, not returned to caller. Partial data is acceptable. Locked D-M6-05.
- **UserStore interface:** 10 methods as of M5_07 — do not add duplicates.
- **RBAC:** 4 roles (super_admin, roles_admin, dba, app_admin). Not 2 roles. Do not add new roles without discussion.
- **Connection pool:** pgxpool throughout orchestrator/runner.
- **YAML seeding:** INSERT ON CONFLICT DO NOTHING, source='yaml' vs 'manual'.
- **Frontend framework:** React + TypeScript + Tailwind CSS + TanStack Query + Apache ECharts — locked.
- **go test scope:** ALWAYS `./cmd/... ./internal/...` — never `./...` (scans web/node_modules/).

---

## What Exists Now

### Completed Milestones
- **M0:** Project setup
- **M1:** Core collectors (33/76 PGAM queries ported)
- **M2:** Config, orchestrator, storage layer (TimescaleDB)
- **M3:** Auth (JWT + RBAC)
- **M4:** Alert engine (evaluator, rules, notifiers)
- **M5_01–M5_07:** Full web UI MVP
- **M6_01:** OS Agent — pgpulse-agent binary, procfs collectors, Patroni/ETCD Smart Providers

### Key Interfaces

```go
// internal/collector/collector.go
type InstanceContext struct{ IsRecovery bool }

type Collector interface {
    Name() string
    Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
    Interval() time.Duration
}

type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

type AlertEvaluator interface {
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}
```

```go
// internal/auth/store.go (complete as of M5_07)
type UserStore interface {
    GetByUsername(ctx context.Context, username string) (*User, error)
    GetByID(ctx context.Context, id int64) (*User, error)
    Create(ctx context.Context, username, passwordHash, role string) (*User, error)
    Count(ctx context.Context) (int64, error)
    CountActiveByRole(ctx context.Context, role string) (int64, error)
    List(ctx context.Context) ([]*User, error)
    Update(ctx context.Context, id int64, fields UpdateFields) error
    UpdatePassword(ctx context.Context, id int64, passwordHash string) error
    UpdateLastLogin(ctx context.Context, id int64) error
    Delete(ctx context.Context, id int64) error
}
```

```go
// internal/cluster/patroni/provider.go (new in M6_01)
type PatroniProvider interface {
    GetClusterState(ctx context.Context) (*ClusterState, error)
    GetHistory(ctx context.Context) ([]SwitchoverEvent, error)
    GetVersion(ctx context.Context) (string, error)
}

// internal/cluster/etcd/provider.go (new in M6_01)
type ETCDProvider interface {
    GetMembers(ctx context.Context) ([]ETCDMember, error)
    GetEndpointHealth(ctx context.Context) (map[string]bool, error)
}
```

### InstanceConfig (current fields)

```go
type InstanceConfig struct {
    ID             string   `koanf:"id"`
    Name           string   `koanf:"name"`
    DSN            string   `koanf:"dsn"`
    Enabled        bool     `koanf:"enabled"`
    MaxConns       int32    `koanf:"max_conns"`
    AgentURL       string   `koanf:"agent_url"`
    PatroniURL     string   `koanf:"patroni_url"`
    PatroniConfig  string   `koanf:"patroni_config"`
    PatroniCtlPath string   `koanf:"patroni_ctl_path"`
    ETCDEndpoints  []string `koanf:"etcd_endpoints"`
    ETCDCtlPath    string   `koanf:"etcd_ctl_path"`
}
```

### Backend File Structure (key additions from M6_01)

```
cmd/pgpulse-agent/main.go
internal/agent/
  server.go, scraper.go
  osmetrics.go          — types only (OSSnapshot, CPUInfo, MemoryInfo, etc.)
  osmetrics_linux.go    — CollectOS implementation (build tag: linux)
  osmetrics_stub.go     — stub returning ErrOSMetricsUnavailable (build tag: !linux)
internal/cluster/
  patroni/provider.go, rest.go, shell.go, fallback.go, noop.go
  etcd/provider.go, http.go, shell.go, fallback.go, noop.go
internal/collector/
  os.go                 — OSCollector (OSSourceNone/Local/Agent)
  cluster.go            — ClusterCollector (wraps PatroniProvider + ETCDProvider)
configs/pgpulse-agent.example.yml
deploy/systemd/pgpulse-agent.service
```

### Frontend additions from M6_01

```
web/src/components/server/
  DiskSection.tsx        — mount table, usage bars
  IOStatsSection.tsx     — device I/O table, util% coloring
  ClusterSection.tsx     — Patroni + ETCD tables
web/src/pages/ServerDetail.tsx  — wired all 4 new sections
```

### API Endpoints (complete as of M6_01)

```
Auth:         POST /api/v1/auth/login, /auth/refresh, GET /auth/me
              PUT  /api/v1/auth/me/password
Users:        GET/POST/PUT/DELETE /api/v1/auth/users, PUT /api/v1/auth/users/{id}/password
Instances:    GET/POST/PUT/DELETE /api/v1/instances, bulk, test
Metrics:      GET /api/v1/instances/:id/metrics  (pg + os + cluster keys)
Alerts:       GET/POST/PUT/DELETE /api/v1/alerts, /api/v1/alerts/rules
Health:       GET /api/v1/health
```

### Build Verification

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./... && go vet ./... && go test ./cmd/... ./internal/... && golangci-lint run
```

---

## What Was Just Completed (M6_01)

- `pgpulse-agent` binary: procfs OS metrics (CPU delta, memory, disk, iostat delta, hostname, OS release, uptime), chi HTTP server on :9187
- `internal/agent/scraper.go`: main server scrapes agent over HTTP
- `internal/collector/os.go`: OSCollector with three sources, isLocalHost detection, 25 metric point names
- `internal/collector/cluster.go`: ClusterCollector, errors → WARN only
- `internal/cluster/patroni/` + `internal/cluster/etcd/`: full Smart Provider pattern
- `internal/config/`: 7 new InstanceConfig fields + AgentConfig top-level section
- `internal/orchestrator/runner.go`: OS and cluster collectors added to per-instance set
- `internal/api/instances.go`: agent_available in instance detail response
- Frontend: DiskSection, IOStatsSection, ClusterSection, wired in ServerDetail

Build: go build ✅ · all tests ✅ · golangci-lint 0 issues ✅ · npm run build ✅

---

## Known Issues

| Issue | Impact | Status |
|-------|--------|--------|
| Claude Code OOM in Agent Teams | Crash at 4.82GB RSS | Close other terminals before running |
| Go test scope | `./...` scans web/node_modules | Use `./cmd/... ./internal/...` |
| Docker Desktop unavailable | BIOS virtualization disabled | Integration tests CI-only |
| Agent no auth/TLS | Trust network only | M8+ |

---

## PGAM Query Porting Status: 52/76

| Source | Queries | Status |
|--------|---------|--------|
| analiz2.php Q1–Q19 (instance) | 19 | ✅ M1_01 |
| analiz2.php Q20–Q21, Q37–Q38, Q40 (replication) | 5 | ✅ M1_02b |
| analiz2.php Q42–Q47 (progress) | 6 | ✅ M1_03 |
| analiz2.php Q48–Q51 (statements) | 4 | ✅ M1_04 |
| analiz2.php Q53–Q57 (locks) | 5 | ✅ M1_05 |
| analiz2.php Q4–Q8, Q22–Q35 (OS/cluster) | 19 | ✅ M6_01 |
| analiz2.php Q36, Q39 (PG < 10) | 2 | ⏭️ Skipped (below PG 14 min) |
| analiz2.php Q41 (logical replication) | 1 | ⏭️ Deferred |
| analiz2.php Q52 (normalized report) | 1 | ⏭️ Covered by Q50+Q51 |
| analiz_db.php Q1–Q18 (per-DB) | 18 | 🔲 M7 — next |

---

## Next Task: M7_01 — Per-Database Analysis

### Goal
Port `analiz_db.php` Q1–Q18 (18 queries) to `internal/collector/database.go`.
Add the Per-Database Analysis page to the frontend.

### PGAM Queries to Port

| PGAM # | Metric | Target function |
|--------|--------|-----------------|
| Q1 | Recovery state | already in instance collector — skip |
| Q2 | Large object inventory (count, size, owner) | collectLargeObjects() |
| Q3 | Tables with OID/lo columns | collectLargeObjectRefs() |
| Q4 | Function execution stats (pg_stat_user_functions) | collectFunctionStats() |
| Q5 | Sequence list (last_value) | collectSequences() |
| Q6 | Schema sizes (absolute + %) | collectSchemaSizes() |
| Q7 | Unlogged tables/indexes | collectUnloggedObjects() |
| Q8 | Large object sizes > 1GB | collectLargeObjectSizes() |
| Q9 | Partitioned table hierarchy | collectPartitions() |
| Q10 | TOAST table sizes | collectToastSizes() |
| Q11 | Table sizes > 1GB | collectTableSizes() |
| Q12 | Table + index bloat estimate | collectBloat() — complex CTE |
| Q13 | System catalog sizes | collectCatalogSizes() |
| Q14 | Cache hit per table | collectTableCacheHit() |
| Q15 | Tables with autovacuum disabled | collectAutovacuumOptions() |
| Q16 | Vacuum/analyze need analysis | collectVacuumNeed() |
| Q17 | Index usage stats | collectIndexUsage() |
| Q18 | Unused indexes (idx_scan=0) | collectUnusedIndexes() |

### Key Design Questions for M7 Discussion
1. **Connection model:** The database collector connects to a *specific database* (not `postgres`). Current Collector interface passes a single `*pgx.Conn` per instance (connected to the `postgres` DB). Per-DB analysis needs connections to individual user databases. How to handle? Options:
   - Add a second connection parameter to `Collect()` — breaks interface
   - DatabaseCollector opens its own connections per DB from the pool config
   - New interface `DBCollector` with a different signature
2. **Scope:** Collect for all databases in the instance, or only configured ones?
3. **Frontend:** New top-level page "Databases" listing all databases, drill-down to per-DB analysis? Or extend the existing ServerDetail page with a Databases tab?

---

## Roadmap

| Milestone | Name | Status |
|-----------|------|--------|
| M0–M5 | Setup through Web UI MVP | ✅ Done |
| M6 | OS Agent | ✅ Done (M6_01) |
| M7 | Per-Database Analysis | 🔲 Next |
| M8 | P1 Features (query plans, session kill, pg_settings diff) | 🔲 |
| M9 | ML Phase 1 (anomaly detection) | 🔲 |
| M10 | Reports & Export | 🔲 |
| M11 | Polish & Release | 🔲 |

---

## Environment

| Tool | Version |
|------|---------|
| Go | 1.24.0 windows/amd64 |
| Node.js | 22.14.0 |
| Claude Code | 2.1.63+ |
| golangci-lint | v2.10.1 |
| PostgreSQL | 16 (local, TimescaleDB installed) |
