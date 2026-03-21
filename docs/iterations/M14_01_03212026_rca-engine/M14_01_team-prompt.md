# PGPulse M14_01 — Team Prompt

**Iteration:** M14_01 — Causal Graph + Reliable Correlation Engine
**Date:** 2026-03-21
**Agent team size:** 3 (RCA Engine + API/Integration + QA)
**Team lead model:** Opus

---

## Context

Read these files before starting:
- `docs/iterations/M14_01_03212026_rca-engine/design.md` — full architecture, algorithm, code samples
- `docs/iterations/M14_01_03212026_rca-engine/requirements.md` — requirements with all amendments
- `CLAUDE.md` — project conventions, build commands
- `docs/CODEBASE_DIGEST.md` — current file inventory, interfaces, metric keys

PGPulse is a PostgreSQL monitoring platform. M14 adds a Root Cause Analysis engine that correlates metric anomalies across DB + OS layers to produce incident timelines explaining *why* an issue occurred. M14_01 builds the backend engine; M14_02 will add the UI.

**This is the most architecturally complex feature PGPulse has built.** The design doc (Section 4) contains the full algorithm. Read it carefully before starting.

---

## DO NOT RE-DISCUSS

| Decision | Locked Value |
|----------|-------------|
| D400 | Incident Timeline + Lightweight Causal Graph |
| D401 | Auto CRITICAL + on-demand any |
| D402 | 30-min lookback, configurable to 2h |
| D404 | ML-enhanced, not required (threshold fallback) |
| D407 | Negative evidence pruning **required in M14_01** — branches die when required evidence absent |
| D408 | Shared RCA + Adviser ontology — compiled-in string constants in `ontology.go` |
| Temporal semantics | `BoundedLag`, `PersistentState` implemented. `WhileEffective` NOT implemented (Tier B only). |
| Evidence model | `EvidenceRequired` kills branch. `EvidenceSupporting` reduces score (0.3x penalty). |
| Tier A/B | 16 Tier A chains active, 4 Tier B chains stubbed (return no results). |
| MetricStatsProvider | New optional interface via type assertion. Does NOT modify `MetricStore`. |
| Fuzzy window | ±1.5x collection interval per metric group. Prevents phantom event false negatives. |
| Summary language | Qualified: "Likely caused by", "Possibly related to", "Strongly consistent with". Never certainty. |
| Frontend | **ZERO changes** — UI comes in M14_02 |
| Max alternatives | Primary chain + 1 alternative. Capped at 2 chains per incident. |
| Build scope | `./cmd/... ./internal/...` — never `./...` |
| Git branch | `master` |
| Migration | `016_rca_incidents.sql` (latest is 015) |

---

## Team Structure

### Agent 1 — RCA Engine

**Owns:** ALL files in `internal/rca/` (new package)

**Does NOT touch:** `internal/api/*`, `internal/storage/*`, `internal/alert/*`, `internal/ml/*`, `internal/config/*`, `cmd/*`, `web/src/*`, `migrations/*`

**Tasks (in order):**

#### Task 1 — Shared Ontology (`ontology.go`)

Create `internal/rca/ontology.go`. Define string constants for the shared RCA↔Adviser knowledge layer:

- `Sym*` constants — symptom keys (e.g., `SymReplicationLagHigh = "symptom.replication_lag_high"`)
- `Mech*` constants — mechanism keys (e.g., `MechCheckpointStorm = "mechanism.checkpoint_storm"`)
- `RC*` constants — root cause keys (e.g., `RCBulkWorkload = "root_cause.bulk_workload"`)
- `Chain*` constants — stable chain identifiers (e.g., `ChainBulkWALCheckpointIOReplLag = "chain.bulk_wal_checkpoint_io_repllag"`)
- `Hook*` constants — remediation hook IDs linking to adviser rules
- `TierA = "stable"`, `TierB = "experimental"` classification constants
- `KnowledgeVersion = "1.0.0"` version tag

Cover all 20 chains from the requirements. These are just `const` strings — no logic.

#### Task 2 — Causal Graph Types (`graph.go`)

Create `internal/rca/graph.go`. Define:

- `TemporalSemantics` enum: `BoundedLag`, `PersistentState`, `WhileEffective`
- `EvidenceRequirement` enum: `EvidenceRequired`, `EvidenceSupporting`
- `CausalNode` struct: ID, Name, MetricKeys (from collector catalog), Layer, SymptomKey, MechanismKey
- `CausalEdge` struct: FromNode, ToNode, MinLag, MaxLag, Temporal, Evidence, Description, BaseConfidence, ChainID, RemediationHook
- `CausalGraph` struct: Nodes map, Edges slice, ChainIDs slice

Implement these methods on `CausalGraph`:
- `IncomingEdges(nodeID string) []CausalEdge` — all edges pointing to nodeID
- `ChainsForTrigger(metricKey string) []string` — chain IDs whose terminal node contains metricKey
- `ReachableNodes(nodeID string, maxDepth int) []*CausalNode` — all upstream nodes within depth limit
- `MetricKeysForChains(chainIDs []string) []string` — deduplicated metric keys needed for given chains

#### Task 3 — Chain Definitions (`chains.go`)

Create `internal/rca/chains.go`. Implement `NewDefaultGraph() *CausalGraph` that builds the full 20-chain graph.

**Critical: Each chain must define:**
- All nodes with correct metric keys from the collector catalog (see design doc Section 4.5 for the mapping)
- All edges with correct temporal semantics (`BoundedLag` with min/max lag, or `PersistentState`)
- Evidence requirements: which edges are `EvidenceRequired` vs `EvidenceSupporting`
- Base confidence per edge (0.0–1.0)
- Chain ID from ontology constants
- Remediation hook ID where applicable
- Tier classification (A or B)

**Tier A chains (16):** 1, 2, 4, 5, 6, 7, 8, 9, 10, 11, 14, 15, 16, 17, 18, 20
**Tier B chains (4):** 3 (cluster-local), 12 (query regression), 13 (new query), 19 (config state)

Tier B chains should be defined in the graph (nodes + edges exist) but **must be filtered out** during chain selection in the engine. Mark them with their chain ID containing the Tier B constant.

**Metric key accuracy is critical.** Use exact metric keys from `docs/CODEBASE_DIGEST.md` Section 3 (Metric Key Catalog). Common ones:
- `pg.replication.lag.replay_bytes`, `pg.replication.lag.replay_seconds`
- `pg.checkpoint.requested_per_second`, `pg.checkpoint.sync_time_ms`
- `pg.connections.utilization_pct`, `pg.connections.total`
- `os.disk.util_pct`, `os.disk.read_bytes_per_sec`, `os.disk.write_bytes_per_sec`
- `os.cpu.user_pct`, `os.cpu.system_pct`, `os.cpu.iowait_pct`
- `os.memory.available_kb`
- `pg.bgwriter.buffers_backend_per_second`
- `pg.wait_events.count`
- `pg.long_transactions.count`, `pg.long_transactions.oldest_seconds`
- `pg.statements.fill_pct`
- `pg.server.wraparound_pct`
- `pg.replication.slot.active`
- `pg.db.vacuum.dead_tuples`, `pg.db.vacuum.dead_pct`
- `pg.db.bloat.table_ratio`
- `pg.progress.vacuum.completion_pct`

**Verify every metric key against the digest before committing.** Metric key mismatches are a recurring failure mode.

#### Task 4 — Anomaly Source (`anomaly.go`)

Create `internal/rca/anomaly.go`. Define:

- `AnomalySource` interface: `GetAnomalies()`, `GetMetricAnomaly()`
- `AnomalyEvent` struct: InstanceID, MetricKey, Timestamp, Value, BaselineVal, ZScore, RateChange, Strength, Source

Implement two sources:

**`ThresholdAnomalySource`:**
- Accepts a `collector.MetricStore`
- For baseline: queries 1 hour before analysis window
- Computes mean + stddev (uses `MetricStatsProvider` if available via type assertion, else raw query + Go math)
- Flags anomalies: value > mean + 2*stddev, OR rate-of-change > 3x baseline rate
- Normalizes `Strength` to 0.0–1.0 based on deviation magnitude
- **Fuzzy window:** extends search windows by ±jitter (passed as parameter, derived from collection frequency by caller)

**`MLAnomalySource`:**
- Wraps `*ml.Detector`
- Falls back to `ThresholdAnomalySource` for metrics the ML detector doesn't track
- Uses Z-scores from detector for `Strength` normalization

**Important:** Both sources must be able to handle metric keys that have no data in the window (return nil, not error). Missing data is expected — not all metrics are collected at all frequencies.

#### Task 5 — Stats Source Interface (`statsource.go`)

Create `internal/rca/statsource.go`:

```go
type MetricStatsProvider interface {
    GetMetricStats(ctx context.Context, instanceID string, keys []string, from, to time.Time) (map[string]MetricStats, error)
}

type MetricStats struct {
    Mean   float64
    StdDev float64
    Min    float64
    Max    float64
    Count  int
}
```

This is imported by `anomaly.go` for the batch stats optimization.

#### Task 6 — Incident Types (`incident.go`)

Create `internal/rca/incident.go`. Define all output types per design doc Section 3.4:
- `Incident`, `CausalChainResult`, `TimelineEvent`, `QualityStatus`, `TimeWindow`
- `IncidentBuilder` — helper that assembles an `Incident` from traversal results
- `generateSummary()` — qualified language based on confidence level
- `bucketizeConfidence()` — high (>0.7), medium (0.4–0.7), low (<0.4)
- `assessQuality()` — compute telemetry completeness from available vs needed metrics

Include future-ready nullable fields: `ReviewStatus`, `ReviewedBy`, `ReviewedAt`, `ReviewComment` (all pointer types, nil in M14_01).

#### Task 7 — Config (`config.go`)

Create `internal/rca/config.go`:

```go
type RCAConfig struct {
    Enabled              bool          `koanf:"enabled"`
    LookbackWindow       time.Duration `koanf:"lookback_window"`
    AutoTriggerSeverity  string        `koanf:"auto_trigger_severity"`
    MaxIncidentsPerHour  int           `koanf:"max_incidents_per_hour"`
    RetentionDays        int           `koanf:"retention_days"`
    MaxTraversalDepth    int           `koanf:"max_traversal_depth"`
    MaxCandidateChains   int           `koanf:"max_candidate_chains"`
    MaxMetricsPerRun     int           `koanf:"max_metrics_per_run"`
    MinEdgeScore         float64       `koanf:"min_edge_score"`
    MinChainScore        float64       `koanf:"min_chain_score"`
    DeferredForwardTail  time.Duration `koanf:"deferred_forward_tail"`
    QualityBannerEnabled bool          `koanf:"quality_banner_enabled"`
    RemediationHooksEnabled bool       `koanf:"remediation_hooks_enabled"`
}
```

#### Task 8 — Core Engine (`engine.go`)

Create `internal/rca/engine.go`. This is the most critical file. Implement the full algorithm from design doc Section 4.2:

```go
type Engine struct {
    graph       *CausalGraph
    anomaly     AnomalySource
    store       IncidentStore
    metricStore collector.MetricStore
    cfg         RCAConfig
}

type EngineOptions struct {
    Graph       *CausalGraph
    Anomaly     AnomalySource
    Store       IncidentStore
    MetricStore collector.MetricStore
    Config      RCAConfig
}

func NewEngine(opts EngineOptions) *Engine
func (e *Engine) Analyze(ctx context.Context, req AnalyzeRequest) (*Incident, error)
```

The `Analyze` method implements the 9-step algorithm:
1. Define window
2. Scope: select candidate chains (Tier A only, filter by trigger metric)
3. Query: determine needed metric keys (bounded by `MaxMetricsPerRun`)
4. Detect: get anomalies for all needed metrics
5. Traverse + Prune: walk graph backward, evaluate edges, kill branches on missing required evidence
6. Rank: score chains, filter by `MinChainScore`, keep top 2
7. Build incident with qualified summary, confidence, quality
8. Store
9. Return

**Key implementation details:**
- `evaluateEdge()` — per design doc Section 4.3. Handles BoundedLag and PersistentState. WhileEffective returns false (Tier B).
- `temporalProximity()` — per design doc Section 4.4. Rewards anomalies at expected lag, penalizes edges.
- `collectionJitter()` — per design doc Section 4.6. High=15s, Medium=90s, Low=450s.
- Recursion bounded by `MaxTraversalDepth`.
- Empty result (no chains survive pruning) → incident with confidence=0, summary="No probable causal chain identified. Manual investigation recommended."

#### Task 9 — Auto-Trigger (`trigger.go`)

Create `internal/rca/trigger.go`. Implement `AutoTrigger` per design doc Section 5:
- `NewAutoTrigger(engine *Engine, cfg RCAConfig) *AutoTrigger`
- `RegisterHook(dispatcher)` — registers via `OnAlert` callback
- `shouldTrigger()` — severity check + rate limit (max N/hour) + cooldown (15 min per metric/instance)
- `fire()` — calls `engine.Analyze()` in a goroutine with 30-second timeout

Import `internal/alert` for `AlertEvent` type only. Do not import the full dispatcher package — the hook is registered via the interface `{ OnAlert(func(alert.AlertEvent)) }`.

#### Task 10 — Store Interface + Implementations

Create `internal/rca/store.go`:
```go
type IncidentStore interface {
    Create(ctx context.Context, incident *Incident) (int64, error)
    Get(ctx context.Context, id int64) (*Incident, error)
    ListByInstance(ctx context.Context, instanceID string, limit, offset int) ([]Incident, int, error)
    ListAll(ctx context.Context, limit, offset int) ([]Incident, int, error)
    Cleanup(ctx context.Context, olderThan time.Duration) (int64, error)
}
```

Create `internal/rca/pgstore.go`:
- `PGIncidentStore` backed by `rca_incidents` table
- `Create()` serializes Timeline, PrimaryChain, AlternativeChain, Quality into `timeline_json` JSONB. Extracts `primary_chain_id`, `confidence_bucket`, `quality_status` into normalized columns.
- `Get()` deserializes JSONB back into full struct
- `ListByInstance()` and `ListAll()` return paginated results with total count (no JSONB deserialization for list — only summary fields)
- `Cleanup()` deletes incidents older than retention period

Create `internal/rca/nullstore.go`:
- Returns empty results for all methods. Used in live mode.

---

### Agent 2 — API + Integration

**Owns:** `internal/api/rca.go`, `migrations/016_rca_incidents.sql`, modifications to `internal/api/server.go`, `cmd/pgpulse-server/main.go`, `internal/config/config.go`, `internal/config/load.go`, `internal/storage/pgstore.go`, `internal/storage/memory.go`

**Does NOT touch:** `internal/rca/*` (Agent 1's territory), `internal/alert/*`, `internal/ml/*`, `web/src/*`

**Tasks (in order):**

#### Task 1 — Migration

Create `migrations/016_rca_incidents.sql` with the full schema from design doc Section 6.1. Include all columns: trigger fields, chain/root cause summary, confidence/quality, timeline JSONB, review fields, version tracking. Include all 4 indexes.

#### Task 2 — MetricStatsProvider on PGMetricStore

Add `GetMetricStats` method to `internal/storage/pgstore.go`:

```go
func (s *PGMetricStore) GetMetricStats(ctx context.Context, instanceID string, keys []string, from, to time.Time) (map[string]rca.MetricStats, error)
```

Implementation: single SQL query using `SELECT metric, avg(value), stddev_samp(value), min(value), max(value), count(*) FROM metrics WHERE instance_id=$1 AND metric = ANY($2) AND timestamp BETWEEN $3 AND $4 GROUP BY metric`.

**Do NOT import `internal/rca` in `internal/storage`.** Define a local `MetricStats` struct with the same fields if needed, or use the interface from `internal/rca/statsource.go`. The type assertion happens in the RCA engine, not in storage.

**Alternative approach (preferred):** Define `MetricStatsProvider` and `MetricStats` in `internal/rca/statsource.go` (Agent 1 does this). Storage package implements the method with matching signature. The RCA engine does a type assertion at runtime: `if provider, ok := metricStore.(rca.MetricStatsProvider); ok { ... }`.

To avoid circular imports: Agent 2 can define a local identical interface in storage and have the method satisfy both. Or: the RCA engine passes a wrapper. **Use your judgment — the key constraint is no circular import between `internal/storage` and `internal/rca`.**

#### Task 3 — MetricStatsProvider on MemoryStore

Add `GetMetricStats` method to `internal/storage/memory.go`. Compute mean/stddev in Go from the in-memory ring buffer.

#### Task 4 — Config additions

Modify `internal/config/config.go`: add `RCA RCAConfig` field to `Config` struct. Define `RCAConfig` locally (duplicate the struct from `internal/rca/config.go` to avoid circular import — or have RCA config in the config package directly). Use your judgment on the cleanest approach.

Modify `internal/config/load.go`: add defaults for all RCA config fields per design doc Section 9.2.

#### Task 5 — API handlers

Create `internal/api/rca.go` with 5 handlers per design doc Section 7:
- `handleAnalyze` — POST, parses request body, calls `engine.Analyze()`, returns incident JSON
- `handleListIncidents` — GET, paginated list for instance
- `handleGetIncident` — GET, single incident with full timeline
- `handleListAllIncidents` — GET, paginated list across fleet
- `handleGetGraph` — GET, returns the causal graph definition as JSON

All handlers follow existing patterns in `internal/api/`: use `writeJSON`, `writeError`, parse chi URL params.

#### Task 6 — Route registration

Modify `internal/api/server.go`:
- Add `RCAEngine` and `RCAStore` fields to the `APIServer` struct (or Options)
- Register RCA routes in `Routes()` method, gated by `rcaEngine != nil`
- Follow the exact pattern from design doc Section 7.3

#### Task 7 — main.go wiring

Modify `cmd/pgpulse-server/main.go`:
- After existing component init (orchestrator, alert dispatcher, ML detector, remediation):
- If `cfg.RCA.Enabled && persistentStore != nil`: create RCA engine, store, anomaly source, auto-trigger
- Register auto-trigger hook on alert dispatcher
- Pass RCA engine and store to APIServer constructor
- Follow the exact pattern from design doc Section 8

**Keep changes minimal.** ~30 lines in the existing startup flow.

---

### Agent 3 — QA

**Owns:** All `*_test.go` files in `internal/rca/`. Build verification.

**Does NOT touch:** Production code.

**Tasks (in order):**

#### Task 1 — Graph tests (`graph_test.go`)

- `TestNewDefaultGraph` — verify 20 chains loaded, correct node/edge counts
- `TestIncomingEdges` — verify edges for known nodes
- `TestChainsForTrigger` — verify correct chains returned for `pg.replication.lag.replay_bytes`
- `TestReachableNodes` — verify upstream traversal respects maxDepth
- `TestMetricKeysForChains` — verify deduplication

#### Task 2 — Chain tests (`chains_test.go`)

For each of the 20 chains, verify:
- All nodes exist in graph
- All edges have valid from/to node references
- Temporal semantics are set (not zero value)
- Evidence requirement is set on every edge
- BaseConfidence is between 0.0 and 1.0
- ChainID matches an ontology constant
- Tier A chains: at least one edge is EvidenceRequired
- Tier B chains: classified correctly
- Metric keys on nodes are non-empty and look like real metric keys (contain dots)

#### Task 3 — Engine tests (`engine_test.go`)

**These are the most important tests.** Use mock `AnomalySource` and mock `IncidentStore`.

- **TestAnalyze_FullChain** — provide mock anomalies matching chain #1 (bulk → WAL → checkpoint → disk I/O → replication lag). Verify: incident produced, primary chain populated, timeline has correct event order, summary uses qualified language.
- **TestAnalyze_RequiredEvidencePruning** — provide mock anomalies for chain #1 but OMIT checkpoint anomaly (which is required). Verify: chain #1 does NOT appear in results. This is the D407 test.
- **TestAnalyze_SupportingEvidenceAbsent** — provide anomalies but omit a supporting (not required) edge. Verify: chain survives but score is lower than when all evidence present.
- **TestAnalyze_BoundedTraversal** — set `MaxTraversalDepth=2`. Verify: engine doesn't traverse deeper even if graph allows it.
- **TestAnalyze_NoMatchingChains** — trigger with a metric that no chain covers. Verify: incident has confidence=0 and "No probable causal chain" summary.
- **TestAnalyze_AlternativeChain** — provide anomalies matching two chains equally. Verify: primary + alternative both populated.
- **TestAnalyze_ConfidenceBuckets** — verify high/medium/low bucketing logic.
- **TestAnalyze_QualityStatus** — verify telemetry completeness when some metrics lack data.
- **TestAnalyze_MaxMetricsPerRun** — set low cap, verify engine doesn't exceed it.

#### Task 4 — Anomaly tests (`anomaly_test.go`)

- **TestThresholdAnomalySource** — provide metric data with a clear spike. Verify: anomaly detected with correct strength.
- **TestThresholdAnomalySource_NoData** — metric has no data in window. Verify: returns nil, no error.
- **TestThresholdAnomalySource_FuzzyWindow** — anomaly at edge of window. Verify: found when jitter applied, missed when not.
- **TestMLAnomalySource_Fallback** — ML detector doesn't track the metric. Verify: falls back to threshold.

#### Task 5 — Store tests (`pgstore_test.go`)

- **TestPGIncidentStore_Create** — create incident, verify ID returned
- **TestPGIncidentStore_Get** — create + get, verify JSONB round-trip (timeline, chains, quality all preserved)
- **TestPGIncidentStore_ListByInstance** — create 3 incidents, verify pagination
- **TestPGIncidentStore_Cleanup** — create old incident, verify cleanup removes it

Note: If testcontainers are not available, these can be integration test stubs. Mark with `//go:build integration` if needed.

#### Task 6 — Full verification

```bash
# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...

# Frontend (must be unchanged)
cd web && npm run build && npm run typecheck && npm run lint && cd ..

# Standard build
go build ./cmd/pgpulse-server

# Desktop build
go build -tags desktop ./cmd/pgpulse-server

# Verify no Wails in standard binary
go tool nm pgpulse-server.exe 2>/dev/null | grep -ci wails
# Expected: 0
```

#### Task 7 — Commit

```bash
git add -A
git commit -m "feat(rca): M14_01 — causal graph + reliable correlation engine

- Create internal/rca/ package: 16 Tier A causal chains, 4 Tier B stubs
- Shared RCA↔Adviser ontology with stable identifiers
- Correlation engine with required-evidence pruning and bounded traversal
- Dual anomaly source: ML Z-scores or threshold fallback with fuzzy window
- Incident timeline with confidence scoring and quality markers
- Qualified summary language (never presents causality as certainty)
- Auto-trigger on CRITICAL alerts via OnAlert hook
- 5 new API endpoints for RCA analysis and incident retrieval
- PostgreSQL incident storage with normalized summary columns
- Migration 016: rca_incidents table with JSONB timeline
- MetricStatsProvider for batch stats optimization in threshold mode"
```

---

## Coordination Notes

- Agent 1 (RCA Engine) works independently — `internal/rca/` is a new package with no dependencies on other agents' work.
- Agent 2 (API/Integration) can start Task 1 (migration) and Tasks 4-5 (config, API handlers) immediately. Tasks 2-3 (MetricStatsProvider) and Tasks 6-7 (server.go, main.go wiring) can proceed in parallel with Agent 1.
- Agent 3 (QA) starts writing test stubs immediately. Fills in assertions as Agent 1's types compile.
- **Circular import risk:** `internal/rca` imports `internal/collector` (for MetricStore, MetricPoint, MetricQuery), `internal/alert` (for AlertEvent type in trigger.go), and `internal/ml` (for Detector wrapper). None of these import `internal/rca` back. The API package imports `internal/rca` for handlers. `internal/storage` does NOT import `internal/rca` — the MetricStatsProvider interface satisfaction happens via structural typing (duck typing).
- **If agents encounter Wails v3 alpha.74 API issues** in the desktop build test, ignore — desktop is M12's territory. Just verify `go build -tags desktop` compiles.
- **Metric key verification is critical.** Agent 1 must grep the codebase digest or source files to confirm every metric key in chains.go matches what collectors actually emit. Agent 3 must verify this in chains_test.go.

---

## Build Commands Reference

```bash
# Frontend
cd web && npm run build && cd ..

# Standard server build
go build ./cmd/pgpulse-server

# Desktop build
go build -tags desktop ./cmd/pgpulse-server

# Tests
go test ./cmd/... ./internal/... -count=1

# Lint
golangci-lint run ./cmd/... ./internal/...

# Cross-compile for Linux (demo VM)
export GOOS=linux && export GOARCH=amd64 && export CGO_ENABLED=0
go build -ldflags="-s -w" -o dist/pgpulse-server-linux-amd64 ./cmd/pgpulse-server
unset GOOS GOARCH CGO_ENABLED
```

---

## Watch-List (Expected Files After Completion)

**New files (19):**
- [ ] `internal/rca/ontology.go`
- [ ] `internal/rca/graph.go`
- [ ] `internal/rca/chains.go`
- [ ] `internal/rca/engine.go`
- [ ] `internal/rca/anomaly.go`
- [ ] `internal/rca/statsource.go`
- [ ] `internal/rca/incident.go`
- [ ] `internal/rca/trigger.go`
- [ ] `internal/rca/config.go`
- [ ] `internal/rca/store.go`
- [ ] `internal/rca/pgstore.go`
- [ ] `internal/rca/nullstore.go`
- [ ] `internal/rca/engine_test.go`
- [ ] `internal/rca/graph_test.go`
- [ ] `internal/rca/chains_test.go`
- [ ] `internal/rca/anomaly_test.go`
- [ ] `internal/rca/pgstore_test.go`
- [ ] `internal/api/rca.go`
- [ ] `migrations/016_rca_incidents.sql`

**Modified files (6):**
- [ ] `cmd/pgpulse-server/main.go` (+30 lines)
- [ ] `internal/api/server.go` (+15 lines)
- [ ] `internal/config/config.go` (+5 lines)
- [ ] `internal/config/load.go` (+10 lines)
- [ ] `internal/storage/pgstore.go` (+40 lines)
- [ ] `internal/storage/memory.go` (+30 lines)

**Unchanged (verify):**
- [ ] `internal/alert/dispatcher.go`
- [ ] `internal/ml/detector.go`
- [ ] `web/src/*`
