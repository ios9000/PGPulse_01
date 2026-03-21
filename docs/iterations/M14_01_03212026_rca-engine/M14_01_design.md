# PGPulse M14_01 — Causal Graph + Reliable Correlation Engine — Design Document

**Date:** 2026-03-21
**Iteration:** M14_01
**Parent:** M14_requirements_v2.md
**Scope:** `internal/rca/` package, 16 Tier A chains, anomaly source (ML + fallback), required-evidence pruning, basic confidence scoring, state-aware temporal semantics, incident types, storage, migration, 5 API endpoints, auto-trigger hook

---

## 1. Integration Points in Existing Codebase

### 1.1 MetricStore (what RCA reads from)

```go
// internal/collector/collector.go — existing, unchanged
type MetricStore interface {
    Write(ctx context.Context, points []MetricPoint) error
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    Close() error
}

type MetricQuery struct {
    InstanceID string
    Metric     string
    Labels     map[string]string
    Start      time.Time
    End        time.Time
    Limit      int
}
```

RCA uses `MetricStore.Query()` to fetch historical metric data within the analysis window. Each metric key referenced by the causal graph generates one query call.

**New method needed for threshold fallback:** `GetMetricStats` for batch statistics. Rather than modify the existing `MetricStore` interface (which would break all implementations), we define a new optional interface:

```go
// internal/rca/statsource.go
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

The RCA engine checks if the provided `MetricStore` implements `MetricStatsProvider` (type assertion). If yes, it uses the batch method. If no (e.g., `MemoryStore` in live mode), it falls back to fetching raw points via `Query()` and computing stats in Go. Both `PGMetricStore` and `MemoryStore` will get `GetMetricStats` implementations.

### 1.2 Alert Dispatcher (auto-trigger hook)

```go
// internal/alert/dispatcher.go — M12_02 added OnAlert
func (d *AlertDispatcher) OnAlert(fn func(AlertEvent))
```

RCA registers a hook via `OnAlert` that fires when a CRITICAL alert event occurs. The hook checks the configured auto-trigger severity and rate limits, then calls `engine.Analyze()` asynchronously.

### 1.3 ML Detector (optional anomaly source)

```go
// internal/ml/detector.go — existing
type Detector struct { ... }
func (d *Detector) Detect(instanceID, metricKey string) (*AnomalyResult, error)
```

RCA wraps the ML detector behind the `AnomalySource` interface. If ML is disabled (`ml.enabled: false`), RCA uses the threshold fallback instead. Both paths produce the same `AnomalyEvent` type.

### 1.4 Remediation Engine (pattern reference)

`internal/remediation/` follows a similar architecture to what RCA needs: rules → engine → store. RCA mirrors this pattern:

| Remediation | RCA |
|---|---|
| `rule.go` (Rule interface) | `graph.go` (CausalGraph + edges) |
| `engine.go` (evaluate rules) | `engine.go` (traverse graph, correlate) |
| `pgstore.go` (store recommendations) | `pgstore.go` (store incidents) |
| `nullstore.go` (live mode) | `nullstore.go` (live mode) |
| `background.go` (periodic) | auto-trigger via `OnAlert` hook |

### 1.5 API Server (route registration)

```go
// internal/api/server.go — APIServer.Routes()
// New RCA routes registered alongside existing remediation routes
r.Route("/instances/{id}/rca", func(r chi.Router) { ... })
r.Route("/rca", func(r chi.Router) { ... })
```

The `APIServer` struct needs a new field: `rcaEngine *rca.Engine` (or interface). Passed via constructor in `main.go`.

### 1.6 Migration Numbering

Latest migration: `015_pgss_snapshots.sql` (M11). RCA migration: **016_rca_incidents.sql**.

---

## 2. Package Design: `internal/rca/`

### 2.1 File Layout

| File | Lines Est. | Purpose |
|------|-----------|---------|
| `ontology.go` | ~120 | Shared knowledge constants: SymptomKey, MechanismKey, RootCauseKey, ChainID, RemediationHookID. Used by both RCA and Adviser. |
| `graph.go` | ~150 | `CausalNode`, `CausalEdge`, `CausalGraph` types. `NewDefaultGraph()` builds the 20-chain graph. Edge traversal methods. |
| `chains.go` | ~300 | 20 chain definitions with all edge metadata: temporal semantics, evidence requirements, base confidence, remediation hooks. |
| `engine.go` | ~350 | `Engine` struct. `Analyze()` method: trigger → scope → query → detect → traverse → prune → rank → build. Core algorithm. |
| `anomaly.go` | ~180 | `AnomalySource` interface + `MLAnomalySource` (wraps detector) + `ThresholdAnomalySource` (fallback). Fuzzy window logic. |
| `statsource.go` | ~40 | `MetricStatsProvider` interface for batch stats. |
| `incident.go` | ~100 | `Incident`, `CausalChainResult`, `TimelineEvent`, `QualityStatus` types. `IncidentBuilder` constructs incidents from traversal results. |
| `store.go` | ~20 | `IncidentStore` interface. |
| `pgstore.go` | ~200 | PostgreSQL-backed `IncidentStore`: Create, Get, List, ListAll, Cleanup. |
| `nullstore.go` | ~30 | No-op `IncidentStore` for live mode. |
| `trigger.go` | ~80 | `AutoTrigger`: registers `OnAlert` hook, rate-limits, dispatches `Analyze()` in goroutine. |
| `config.go` | ~40 | `RCAConfig` struct matching YAML schema. |
| `engine_test.go` | ~400 | Tests for correlation engine: chain traversal, pruning, scoring, bounded execution. |
| `graph_test.go` | ~150 | Tests for graph construction and edge lookup. |
| `chains_test.go` | ~200 | Tests for each Tier A chain: verify nodes, edges, temporal semantics, evidence requirements. |
| `anomaly_test.go` | ~200 | Tests for ML adapter and threshold fallback. |
| `pgstore_test.go` | ~150 | Tests for incident CRUD and cleanup. |

**Estimated total:** ~2,100 lines across 17 files.

---

## 3. Core Types

### 3.1 Shared Ontology (`ontology.go`)

```go
package rca

// Symptom keys — what the DBA observes
const (
    SymReplicationLagHigh     = "symptom.replication_lag_high"
    SymDiskIOSaturated        = "symptom.disk_io_saturated"
    SymConnectionExhaustion   = "symptom.connection_exhaustion"
    SymQueryLatencyHigh       = "symptom.query_latency_high"
    SymDiskSpaceLow           = "symptom.disk_space_low"
    SymOOMRisk                = "symptom.oom_risk"
    SymPGSSEviction           = "symptom.pgss_eviction"
    // ... all symptom constants
)

// Mechanism keys — intermediate causal steps
const (
    MechCheckpointStorm       = "mechanism.checkpoint_storm"
    MechWALSpike              = "mechanism.wal_spike"
    MechBufferBackendWrites   = "mechanism.buffer_backend_writes"
    MechLockContention        = "mechanism.lock_contention"
    MechAutovacuumStorm       = "mechanism.autovacuum_storm"
    MechTempFileSpike         = "mechanism.temp_file_spike"
    MechMVCCBloat             = "mechanism.mvcc_bloat"
    MechVacuumBlocked         = "mechanism.vacuum_blocked"
    // ... all mechanism constants
)

// Root cause keys — the initiating factor
const (
    RCBulkWorkload            = "root_cause.bulk_workload"
    RCInactiveSlot            = "root_cause.inactive_replication_slot"
    RCLongTransaction         = "root_cause.long_transaction"
    RCMissingIndex            = "root_cause.missing_index"
    RCWraparoundRisk          = "root_cause.wraparound_approaching"
    RCMemoryPressure          = "root_cause.memory_pressure"
    RCConfigChange            = "root_cause.config_change"
    // ... all root cause constants
)

// Chain IDs — stable identifiers for each causal chain
const (
    ChainBulkWALCheckpointIOReplLag    = "chain.bulk_wal_checkpoint_io_repllag"    // #1
    ChainInactiveSlotWALDisk           = "chain.inactive_slot_wal_disk"             // #2
    // ... all 20 chain ID constants
)

// Remediation hook IDs — link to adviser rules
const (
    HookCheckpointTuning       = "remediation.checkpoint_completion_target"
    HookVacuumTuning           = "remediation.vacuum_cost_settings"
    HookConnectionPooling      = "remediation.connection_pooling"
    HookIndexCreation          = "remediation.create_missing_index"
    // ... mapping RCA root causes to adviser actions
)

// Tier classification
const (
    TierA = "stable"       // fully supported in M14_01
    TierB = "experimental" // dependent on later integrations
)

// KnowledgeVersion — tracks the chain definition version
const KnowledgeVersion = "1.0.0"
```

These constants are the shared RCA↔Adviser ontology. The same string identifiers appear in RCA incident results and can be matched to adviser remediation rules. No database table — compiled-in constants.

### 3.2 Causal Graph Types (`graph.go`)

```go
type TemporalSemantics int
const (
    BoundedLag      TemporalSemantics = iota // event → event within MinLag..MaxLag
    PersistentState                           // ongoing condition persisting before trigger
    WhileEffective                            // configuration state active at incident time
)

type EvidenceRequirement int
const (
    EvidenceRequired   EvidenceRequirement = iota // branch dies without it
    EvidenceSupporting                             // strengthens confidence if present
)

type CausalNode struct {
    ID           string   // "wal_generation", "disk_io", "replication_lag"
    Name         string   // "WAL Generation Rate"
    MetricKeys   []string // actual metric keys from collector catalog
    Layer        string   // "db", "os", "workload", "config"
    SymptomKey   string   // shared ontology key (may be empty for intermediate nodes)
    MechanismKey string   // shared ontology key (may be empty for leaf nodes)
}

type CausalEdge struct {
    FromNode        string              // upstream node ID (cause)
    ToNode          string              // downstream node ID (effect)
    MinLag          time.Duration       // for BoundedLag: minimum expected delay
    MaxLag          time.Duration       // for BoundedLag: maximum expected delay
    Temporal        TemporalSemantics   // how time relates cause to effect
    Evidence        EvidenceRequirement // required or supporting
    Description     string              // human-readable: "WAL spike causes checkpoint within 1-3 min"
    BaseConfidence  float64             // 0.0-1.0 prior confidence of this link
    ChainID         string              // which chain this edge belongs to
    RemediationHook string              // adviser rule ID (may be empty)
}

type CausalGraph struct {
    Nodes    map[string]*CausalNode
    Edges    []CausalEdge
    ChainIDs []string // all registered chain IDs
}

// IncomingEdges returns all edges pointing to nodeID
func (g *CausalGraph) IncomingEdges(nodeID string) []CausalEdge

// ChainsForTrigger returns chain IDs whose terminal node matches the trigger metric
func (g *CausalGraph) ChainsForTrigger(metricKey string) []string

// ReachableNodes returns all upstream nodes reachable from nodeID within maxDepth
func (g *CausalGraph) ReachableNodes(nodeID string, maxDepth int) []*CausalNode

// MetricKeysForChains returns the deduplicated set of metric keys needed for a set of chains
func (g *CausalGraph) MetricKeysForChains(chainIDs []string) []string
```

### 3.3 Anomaly Types (`anomaly.go`)

```go
type AnomalySource interface {
    // GetAnomalies returns all anomalies for an instance within a window
    GetAnomalies(ctx context.Context, instanceID string, from, to time.Time) ([]AnomalyEvent, error)
    // GetMetricAnomaly checks a specific metric for anomalies in a window
    // Uses fuzzy window internally (extends by ±collectionInterval for jitter)
    GetMetricAnomaly(ctx context.Context, instanceID, metricKey string, from, to time.Time) (*AnomalyEvent, error)
}

type AnomalyEvent struct {
    InstanceID  string
    MetricKey   string
    Timestamp   time.Time
    Value       float64
    BaselineVal float64  // mean/expected value for context
    ZScore      float64  // anomaly magnitude (ML mode; 0 in threshold mode)
    RateChange  float64  // rate of change vs baseline (threshold mode)
    Strength    float64  // normalized 0.0-1.0 evidence strength
    Source      string   // "ml" or "threshold"
}
```

**MLAnomalySource:** Wraps `*ml.Detector`. Queries the detector's baseline data for the metric in the window. Returns anomalies with Z-scores.

**ThresholdAnomalySource:** Uses `MetricStatsProvider.GetMetricStats()` (or raw query fallback) to compute baseline stats for a 1-hour period before the analysis window. Flags values exceeding 2σ or rate-of-change exceeding 3x baseline. Applies fuzzy window (±collection interval) to evidence checks.

### 3.4 Incident Types (`incident.go`)

```go
type Incident struct {
    ID               int64
    InstanceID       string
    TriggerMetric    string
    TriggerValue     float64
    TriggerTime      time.Time
    TriggerKind      string              // "alert", "manual"
    AnalysisWindow   TimeWindow
    PrimaryChain     *CausalChainResult  // strongest chain
    AlternativeChain *CausalChainResult  // second-best (nil if dominant)
    Timeline         []TimelineEvent     // all events, ordered by timestamp
    Summary          string              // qualified language: "Likely caused by..."
    Confidence       float64             // 0.0-1.0
    ConfidenceBucket string              // "high" (>0.7), "medium" (0.4-0.7), "low" (<0.4)
    Quality          QualityStatus
    RemediationHooks []string            // adviser rule IDs
    AutoTriggered    bool
    ChainVersion     string              // KnowledgeVersion
    AnomalyMode      string              // "ml" or "threshold"
    CreatedAt        time.Time
    // Future-ready (nullable, not populated in M14_01)
    ReviewStatus     *string
    ReviewedBy       *string
    ReviewedAt       *time.Time
    ReviewComment    *string
}

type CausalChainResult struct {
    ChainID      string           // stable chain identifier
    ChainName    string           // human-readable
    Score        float64          // cumulative confidence
    RootCauseKey string           // ontology key
    Events       []TimelineEvent
}

type TimelineEvent struct {
    Timestamp    time.Time
    NodeID       string    // causal graph node
    NodeName     string    // human-readable
    MetricKey    string    // specific metric key
    Value        float64
    BaselineVal  float64   // expected value for context
    ZScore       float64
    Strength     float64   // normalized evidence strength
    Layer        string    // "db", "os", "workload", "config"
    Role         string    // "root_cause", "intermediate", "symptom"
    Evidence     string    // "required", "supporting"
    Description  string    // "WAL generation rate jumped to 4x normal"
    EdgeDesc     string    // causal edge description
}

type QualityStatus struct {
    TelemetryCompleteness float64  // % of needed metrics that had data
    AnomalySourceMode     string   // "ml" or "threshold"
    ScopeLimitations      []string // e.g., "single-instance only"
    UnavailableDeps       []string // e.g., "settings snapshots not enabled"
}

type TimeWindow struct {
    From time.Time
    To   time.Time
}
```

---

## 4. Core Algorithm: `engine.go`

### 4.1 Engine struct

```go
type Engine struct {
    graph       *CausalGraph
    anomaly     AnomalySource
    store       IncidentStore
    metricStore collector.MetricStore
    cfg         RCAConfig
    mu          sync.Mutex
}

type AnalyzeRequest struct {
    InstanceID    string
    TriggerMetric string
    TriggerValue  float64
    TriggerTime   time.Time
    WindowMinutes int    // 0 = use default from config
    TriggerKind   string // "alert" or "manual"
}

func (e *Engine) Analyze(ctx context.Context, req AnalyzeRequest) (*Incident, error)
```

### 4.2 Analyze Algorithm (Step by Step)

```
func (e *Engine) Analyze(ctx, req):

    1. DEFINE WINDOW
       from = req.TriggerTime - cfg.LookbackWindow
       to   = req.TriggerTime + cfg.DeferredForwardTail  (if auto-triggered)
            = req.TriggerTime                             (if manual)
       window = TimeWindow{from, to}

    2. SCOPE: Select candidate chains
       chainIDs = graph.ChainsForTrigger(req.TriggerMetric)
       if len(chainIDs) > cfg.MaxCandidateChains:
           chainIDs = chainIDs[:cfg.MaxCandidateChains]  // cap
       // Filter to Tier A only (Tier B chains return no results in M14_01)

    3. QUERY: Determine needed metrics
       metricKeys = graph.MetricKeysForChains(chainIDs)
       if len(metricKeys) > cfg.MaxMetricsPerRun:
           metricKeys = metricKeys[:cfg.MaxMetricsPerRun]  // cap + log warning

    4. DETECT: Get anomalies for all needed metrics
       anomalies = anomaly.GetAnomalies(ctx, req.InstanceID, from, to)
       // Index by metricKey for O(1) lookup
       anomalyMap = indexByMetricKey(anomalies)

    5. TRAVERSE + PRUNE: Walk graph backward from trigger
       for each chainID in chainIDs:
           chain = traverseChain(ctx, chainID, req.TriggerMetric, window, anomalyMap)
           // traverseChain walks upstream edges:
           //   - For each edge: check evidence based on temporal semantics
           //   - BoundedLag: anomaly exists in [trigger - maxLag - jitter, trigger - minLag + jitter]
           //   - PersistentState: anomaly/high-value exists at any point in window
           //   - WhileEffective: NOT IMPLEMENTED in M14_01 (Tier B)
           //   - If edge.Evidence == EvidenceRequired AND no anomaly found: KILL branch
           //   - If edge.Evidence == EvidenceSupporting AND no anomaly found: reduce score
           //   - Score edge: baseConfidence * anomalyStrength * temporalProximityFactor
           //   - Recurse upstream if depth < cfg.MaxTraversalDepth

    6. RANK: Score complete chains
       Sort chains by cumulative score descending.
       Filter out chains with score < cfg.MinChainScore.
       primaryChain = chains[0] (if any survive)
       altChain = chains[1] (if score close to primary, i.e., within 20%)

    7. BUILD INCIDENT
       timeline = merge all events from primary + alt chain, sort by timestamp
       summary = generateSummary(primaryChain)  // qualified language
       quality = assessQuality(metricKeys, anomalyMap, chainIDs)
       confidence = primaryChain.Score (or 0 if no chain survived)
       confidenceBucket = bucketize(confidence)  // high/medium/low
       remediationHooks = collectHooks(primaryChain)

    8. STORE
       incident = buildIncident(...)
       store.Create(ctx, incident)

    9. RETURN incident
```

### 4.3 Evidence Evaluation Detail

```go
func (e *Engine) evaluateEdge(
    edge CausalEdge,
    triggerTime time.Time,
    anomalyMap map[string][]AnomalyEvent,
    depth int,
) (score float64, event *TimelineEvent, found bool) {

    node := e.graph.Nodes[edge.FromNode]

    switch edge.Temporal {
    case BoundedLag:
        // Look for anomaly in [triggerTime - maxLag - jitter, triggerTime - minLag + jitter]
        // jitter = collectionInterval for the node's metric group (10s/60s/300s)
        jitter := e.collectionJitter(node)
        searchFrom := triggerTime.Add(-edge.MaxLag - jitter)
        searchTo := triggerTime.Add(-edge.MinLag + jitter)
        anomaly := findStrongestAnomaly(anomalyMap, node.MetricKeys, searchFrom, searchTo)
        if anomaly == nil {
            if edge.Evidence == EvidenceRequired { return 0, nil, false } // PRUNE
            return edge.BaseConfidence * 0.3, nil, true // supporting, absent → penalty
        }
        proximity := temporalProximity(anomaly.Timestamp, triggerTime, edge.MinLag, edge.MaxLag)
        score = edge.BaseConfidence * anomaly.Strength * proximity
        event = buildTimelineEvent(node, anomaly, edge)
        return score, event, true

    case PersistentState:
        // Look for elevated/anomalous value at ANY point in the full window
        anomaly := findStrongestAnomaly(anomalyMap, node.MetricKeys, window.From, window.To)
        if anomaly == nil {
            if edge.Evidence == EvidenceRequired { return 0, nil, false }
            return edge.BaseConfidence * 0.3, nil, true
        }
        score = edge.BaseConfidence * anomaly.Strength
        event = buildTimelineEvent(node, anomaly, edge)
        return score, event, true

    case WhileEffective:
        // NOT IMPLEMENTED in M14_01 — Tier B chains only
        // Returns "no evidence" which prunes the branch (all WhileEffective edges are on Tier B chains)
        return 0, nil, false
    }
}
```

### 4.4 Temporal Proximity Factor

Rewards anomalies that appear at the expected time lag, penalizes those at the edge of the window:

```go
func temporalProximity(anomalyTime, triggerTime time.Time, minLag, maxLag time.Duration) float64 {
    actualLag := triggerTime.Sub(anomalyTime)
    expectedCenter := (minLag + maxLag) / 2
    deviation := math.Abs(float64(actualLag - expectedCenter))
    maxDeviation := float64(maxLag - minLag) / 2
    if maxDeviation == 0 { return 1.0 }
    proximity := 1.0 - (deviation / (maxDeviation * 1.5)) // gradual falloff
    if proximity < 0.2 { proximity = 0.2 } // floor
    return proximity
}
```

### 4.5 Summary Generation

```go
func generateSummary(chain *CausalChainResult, confidence float64) string {
    if chain == nil {
        return "No probable causal chain identified. Manual investigation recommended."
    }

    qualifier := "Likely caused by"
    if confidence < 0.4 {
        qualifier = "Possibly related to"
    } else if confidence > 0.8 {
        qualifier = "Strongly consistent with"
    }

    rootEvent := chain.Events[0] // earliest event
    symptomEvent := chain.Events[len(chain.Events)-1]

    return fmt.Sprintf("%s %s. %s was detected at %s, leading to %s at %s.",
        qualifier,
        rootEvent.Description,
        rootEvent.NodeName,
        rootEvent.Timestamp.Format("15:04:05"),
        symptomEvent.NodeName,
        symptomEvent.Timestamp.Format("15:04:05"),
    )
}
```

### 4.6 Collection Jitter Table

```go
func (e *Engine) collectionJitter(node *CausalNode) time.Duration {
    // Derive from the node's metric collection frequency
    // High-frequency metrics (connections, locks): 10s → 15s jitter
    // Medium-frequency (checkpoints, replication): 60s → 90s jitter
    // Low-frequency (server info, sizes): 300s → 450s jitter
    // Use 1.5x the collection interval as jitter
    switch nodeCollectionGroup(node) {
    case "high":   return 15 * time.Second
    case "medium": return 90 * time.Second
    case "low":    return 450 * time.Second
    default:       return 90 * time.Second
    }
}
```

---

## 5. Auto-Trigger: `trigger.go`

```go
type AutoTrigger struct {
    engine      *Engine
    cfg         RCAConfig
    lastFired   map[string]time.Time // instanceID:metric → last trigger time
    mu          sync.Mutex
}

func NewAutoTrigger(engine *Engine, cfg RCAConfig) *AutoTrigger

// Register hooks on the alert dispatcher
func (t *AutoTrigger) RegisterHook(dispatcher interface{ OnAlert(func(alert.AlertEvent)) }) {
    dispatcher.OnAlert(func(event alert.AlertEvent) {
        if !t.shouldTrigger(event) { return }
        go t.fire(event) // non-blocking
    })
}

func (t *AutoTrigger) shouldTrigger(event alert.AlertEvent) bool {
    // 1. Check severity >= configured auto_trigger_severity
    if severityRank(event.Severity) < severityRank(t.cfg.AutoTriggerSeverity) {
        return false
    }
    // 2. Rate limit: max N incidents per instance per hour
    // 3. Cooldown: same metric/instance not re-triggered within 15 min
    key := event.InstanceID + ":" + event.Metric
    t.mu.Lock()
    defer t.mu.Unlock()
    if last, ok := t.lastFired[key]; ok && time.Since(last) < 15*time.Minute {
        return false
    }
    t.lastFired[key] = time.Now()
    return true
}

func (t *AutoTrigger) fire(event alert.AlertEvent) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    _, err := t.engine.Analyze(ctx, AnalyzeRequest{
        InstanceID:    event.InstanceID,
        TriggerMetric: event.Metric,
        TriggerValue:  event.Value,
        TriggerTime:   event.FiredAt,
        TriggerKind:   "alert",
    })
    if err != nil {
        slog.Error("auto-triggered RCA failed", "instance", event.InstanceID, "error", err)
    }
}
```

---

## 6. Storage: `pgstore.go`

### 6.1 Migration 016

```sql
-- migrations/016_rca_incidents.sql

CREATE TABLE rca_incidents (
    id                    BIGSERIAL PRIMARY KEY,
    instance_id           TEXT NOT NULL,
    trigger_metric        TEXT NOT NULL,
    trigger_value         DOUBLE PRECISION NOT NULL,
    trigger_time          TIMESTAMPTZ NOT NULL,
    trigger_kind          TEXT NOT NULL DEFAULT 'alert',
    window_from           TIMESTAMPTZ NOT NULL,
    window_to             TIMESTAMPTZ NOT NULL,
    primary_chain_id      TEXT,
    primary_root_cause    TEXT,
    confidence            DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence_bucket     TEXT,
    quality_status        TEXT NOT NULL DEFAULT 'unknown',
    timeline_json         JSONB NOT NULL,
    summary               TEXT NOT NULL,
    auto_triggered        BOOLEAN NOT NULL DEFAULT false,
    remediation_hooks     TEXT[],
    chain_version         TEXT,
    anomaly_source_mode   TEXT,
    review_status         TEXT,
    reviewed_by           TEXT,
    reviewed_at           TIMESTAMPTZ,
    review_comment        TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rca_incidents_instance ON rca_incidents(instance_id, created_at DESC);
CREATE INDEX idx_rca_incidents_trigger ON rca_incidents(trigger_metric, trigger_time);
CREATE INDEX idx_rca_incidents_chain ON rca_incidents(primary_chain_id);
CREATE INDEX idx_rca_incidents_review ON rca_incidents(review_status) WHERE review_status IS NOT NULL;
```

### 6.2 IncidentStore Interface

```go
type IncidentStore interface {
    Create(ctx context.Context, incident *Incident) (int64, error)
    Get(ctx context.Context, id int64) (*Incident, error)
    ListByInstance(ctx context.Context, instanceID string, limit, offset int) ([]Incident, int, error)
    ListAll(ctx context.Context, limit, offset int) ([]Incident, int, error)
    Cleanup(ctx context.Context, olderThan time.Duration) (int64, error)
}
```

`PGIncidentStore.Create()` serializes `Timeline`, `PrimaryChain`, `AlternativeChain`, `Quality`, and `RemediationHooks` into `timeline_json` as a single JSONB document. Normalized fields (`primary_chain_id`, `confidence_bucket`, etc.) are extracted and stored in their dedicated columns for filtering.

`PGIncidentStore.Get()` deserializes the JSONB back into the full `Incident` struct.

`NullIncidentStore` returns empty results for all methods (live mode).

---

## 7. API Handlers: `internal/api/rca.go`

### 7.1 New File

```go
// internal/api/rca.go

type RCAHandler struct {
    engine *rca.Engine
    store  rca.IncidentStore
}

func NewRCAHandler(engine *rca.Engine, store rca.IncidentStore) *RCAHandler
```

### 7.2 Endpoints

| Method | Path | Handler | Request Body | Response |
|--------|------|---------|-------------|----------|
| POST | `/instances/{id}/rca/analyze` | `handleAnalyze` | `{"metric":"pg.replication.lag.replay_bytes","timestamp":"...","window_minutes":30}` | `Incident` JSON |
| GET | `/instances/{id}/rca/incidents` | `handleListIncidents` | — (query: `?limit=20&offset=0`) | `{incidents: [], total: N}` |
| GET | `/instances/{id}/rca/incidents/{incidentId}` | `handleGetIncident` | — | `Incident` JSON |
| GET | `/rca/incidents` | `handleListAllIncidents` | — (query: `?limit=20&offset=0`) | `{incidents: [], total: N}` |
| GET | `/rca/graph` | `handleGetGraph` | — | CausalGraph JSON (nodes + edges) |

### 7.3 Route Registration

In `internal/api/server.go`, add to `Routes()`:

```go
// RCA routes
if s.rcaEngine != nil {
    rcaHandler := NewRCAHandler(s.rcaEngine, s.rcaStore)
    r.Route("/instances/{id}/rca", func(r chi.Router) {
        r.Post("/analyze", rcaHandler.handleAnalyze)
        r.Get("/incidents", rcaHandler.handleListIncidents)
        r.Get("/incidents/{incidentId}", rcaHandler.handleGetIncident)
    })
    r.Get("/rca/incidents", rcaHandler.handleListAllIncidents)
    r.Get("/rca/graph", rcaHandler.handleGetGraph)
}
```

---

## 8. Wiring in `main.go`

After existing component initialization (orchestrator, alert dispatcher, remediation engine):

```go
// RCA Engine (requires MetricStore + optional ML Detector)
var rcaEngine *rca.Engine
var rcaStore rca.IncidentStore
var rcaTrigger *rca.AutoTrigger

if cfg.RCA.Enabled && persistentStore != nil {
    rcaStore = rca.NewPGIncidentStore(pool)
    anomalySource := rca.NewThresholdAnomalySource(persistentStore) // default
    if cfg.ML.Enabled && mlDetector != nil {
        anomalySource = rca.NewMLAnomalySource(mlDetector, persistentStore)
    }
    rcaEngine = rca.NewEngine(rca.EngineOptions{
        Graph:       rca.NewDefaultGraph(),
        Anomaly:     anomalySource,
        Store:       rcaStore,
        MetricStore: persistentStore,
        Config:      cfg.RCA,
    })
    // Auto-trigger on CRITICAL alerts
    rcaTrigger = rca.NewAutoTrigger(rcaEngine, cfg.RCA)
    rcaTrigger.RegisterHook(alertDispatcher)

    slog.Info("RCA engine enabled", "chains", 16, "auto_trigger", cfg.RCA.AutoTriggerSeverity)
} else {
    rcaStore = rca.NewNullIncidentStore()
}

// Pass to API server
apiServer := api.New(api.Options{
    // ... existing fields ...
    RCAEngine: rcaEngine,
    RCAStore:  rcaStore,
})
```

---

## 9. Configuration Additions

### 9.1 Config Struct (`internal/config/config.go`)

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

### 9.2 Defaults

```go
func rcaDefaults() RCAConfig {
    return RCAConfig{
        Enabled:              false,
        LookbackWindow:       30 * time.Minute,
        AutoTriggerSeverity:  "critical",
        MaxIncidentsPerHour:  10,
        RetentionDays:        90,
        MaxTraversalDepth:    5,
        MaxCandidateChains:   5,
        MaxMetricsPerRun:     50,
        MinEdgeScore:         0.25,
        MinChainScore:        0.40,
        DeferredForwardTail:  5 * time.Minute,
        QualityBannerEnabled: true,
        RemediationHooksEnabled: true,
    }
}
```

---

## 10. Modified Existing Files

| File | Change | Lines Est. |
|------|--------|-----------|
| `cmd/pgpulse-server/main.go` | RCA engine init, auto-trigger hook, pass to APIServer | +30 |
| `internal/api/server.go` | Add `RCAEngine`, `RCAStore` to Options/struct, register routes | +15 |
| `internal/config/config.go` | Add `RCAConfig` to `Config` struct | +5 |
| `internal/config/load.go` | Add RCA defaults | +10 |
| `internal/storage/pgstore.go` | Implement `MetricStatsProvider` (batch stats query) | +40 |
| `internal/storage/memory.go` | Implement `MetricStatsProvider` (in-memory calculation) | +30 |
| `go.mod` | No new dependencies expected (pure Go) | 0 |

---

## 11. Agent Team: 3 Agents

### Agent 1 — RCA Engine

**Owns:** `internal/rca/*` (all new files)

**Tasks (in order):**
1. Create `ontology.go` — shared knowledge constants
2. Create `graph.go` — CausalNode, CausalEdge, CausalGraph with traversal methods
3. Create `chains.go` — 20 chain definitions (16 Tier A active, 4 Tier B stubbed)
4. Create `anomaly.go` — AnomalySource interface, MLAnomalySource, ThresholdAnomalySource with fuzzy window
5. Create `statsource.go` — MetricStatsProvider interface
6. Create `incident.go` — all output types + IncidentBuilder + summary generation
7. Create `config.go` — RCAConfig struct
8. Create `engine.go` — Engine struct with Analyze() implementing the full algorithm
9. Create `trigger.go` — AutoTrigger with OnAlert hook, rate limiting
10. Create `store.go` — IncidentStore interface
11. Create `pgstore.go` — PostgreSQL implementation
12. Create `nullstore.go` — no-op implementation

### Agent 2 — API + Integration

**Owns:** `internal/api/rca.go`, modifications to `server.go`, `main.go`, `config/`, `storage/`

**Tasks (in order):**
1. Create `migrations/016_rca_incidents.sql`
2. Create `internal/api/rca.go` — 5 endpoint handlers
3. Modify `internal/api/server.go` — add RCA fields, register routes
4. Modify `internal/config/config.go` — add RCAConfig to Config
5. Modify `internal/config/load.go` — add RCA defaults
6. Implement `MetricStatsProvider` on `internal/storage/pgstore.go`
7. Implement `MetricStatsProvider` on `internal/storage/memory.go`
8. Modify `cmd/pgpulse-server/main.go` — RCA engine wiring

### Agent 3 — QA

**Owns:** All `*_test.go` files

**Tasks (in order):**
1. Create `internal/rca/graph_test.go` — graph construction, edge lookup, reachability
2. Create `internal/rca/chains_test.go` — verify all 20 chains: correct nodes, edges, temporal semantics, evidence requirements, tier classification
3. Create `internal/rca/engine_test.go` — core algorithm:
   - Test chain traversal with mock anomaly data → correct timeline
   - Test required-evidence pruning → branch killed when evidence absent
   - Test supporting-evidence penalty → score reduced but branch survives
   - Test bounded traversal → respects MaxTraversalDepth
   - Test no chains match → empty incident with "manual investigation" summary
   - Test confidence bucketing → high/medium/low correct
4. Create `internal/rca/anomaly_test.go` — threshold fallback stats, fuzzy window, ML adapter
5. Create `internal/rca/pgstore_test.go` — CRUD + cleanup + JSONB round-trip
6. Run full test suite: `go test ./cmd/... ./internal/... -count=1`
7. Run lint: `golangci-lint run ./cmd/... ./internal/...`
8. Verify: `cd web && npm run build && npm run typecheck && npm run lint`
9. Verify: standard build (no tags) still passes, zero Wails symbols
10. Verify: `internal/api/server.go`, `internal/alert/dispatcher.go` — minimal changes, existing tests pass

---

## 12. DO NOT RE-DISCUSS

| Decision | Status |
|----------|--------|
| D400-D408 | All locked per M14_requirements_v2.md |
| Temporal semantics | `BoundedLag`, `PersistentState`, `WhileEffective` — WhileEffective NOT implemented in M14_01 |
| Evidence model | Required kills branch, Supporting reduces score |
| Negative evidence pruning | Required in M14_01, not deferred |
| Confidence scoring | Required in M14_01, not deferred |
| Shared ontology | Compiled-in string constants, not a database table |
| Tier A/B | 16 Tier A (active), 4 Tier B (stubbed). Tier B returns no results. |
| MetricStatsProvider | New optional interface, does NOT modify existing MetricStore |
| Fuzzy window | ±1.5x collection interval to handle polling jitter |
| Summary language | Qualified: "Likely caused by", "Possibly related to", "Strongly consistent with" |
| Frontend | **ZERO changes** in M14_01 — UI comes in M14_02 |
| Max alternatives | Top chain + 1 alternative (capped at 2) |

---

## 13. Watch-List (Expected Files)

**New files (17):**
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
- [ ] `internal/storage/pgstore.go` (+40 lines: MetricStatsProvider)
- [ ] `internal/storage/memory.go` (+30 lines: MetricStatsProvider)

**Unchanged (verify):**
- [ ] `internal/alert/dispatcher.go` — OnAlert hook already exists from M12_02
- [ ] `internal/ml/detector.go` — wrapped, not modified
- [ ] `web/src/*` — zero changes
