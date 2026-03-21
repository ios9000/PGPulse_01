# PGPulse M14 — RCA Engine — Requirements (Revised)

**Date:** 2026-03-21
**Iteration:** M14 (3 sub-iterations: M14_01, M14_02, M14_03)
**Depends on:** M12 (complete), M8 (ML anomaly detection), M10 (advisor/remediation)

> This milestone establishes a trustworthy reasoning foundation for operational intelligence in PGPulse. M14 prioritizes evidence-backed causality, bounded execution, explainability, shared knowledge structures, and future compatibility with Adviser integration, human feedback, and cluster-aware evolution.

---

## 1. Goal

Build a Root Cause Analysis engine that automatically correlates metric anomalies across PostgreSQL and OS layers to produce incident timelines explaining *why* an issue occurred — not just *what* happened.

When a DBA sees "replication lag spiked at 14:32," the RCA engine answers: "At 14:27, a bulk INSERT appeared in pg_stat_statements. WAL generation rate jumped 4x at 14:28. A checkpoint triggered at 14:29. Disk I/O saturated at 14:30. Replication lag followed at 14:32."

RCA in PGPulse must be designed as a **trustworthy causal reasoning subsystem**, not merely a correlation viewer. Its purpose is to identify the most likely initiating factor and the probable cascade path behind a trigger event, while explicitly communicating uncertainty, evidence quality, and alternative explanations. RCA must remain self-hosted, PostgreSQL-native, and compatible with PGPulse's existing adviser/remediation model.

**No competitor in the PostgreSQL monitoring space offers this.** Datadog does cross-layer correlation but requires their full SaaS ecosystem. PGPulse does it self-hosted, single-binary, with no application instrumentation.

---

## 2. Locked Decisions

| ID | Decision | Choice | Rationale |
|----|----------|--------|-----------|
| D400 | RCA output | **Incident Timeline + Lightweight Causal Graph** | Graph is the internal reasoning model; timeline is the user-facing explanation. Graph traversal supports required-evidence pruning and state-aware temporal semantics. |
| D401 | Trigger | **Auto on CRITICAL + on-demand for any** | CRITICAL alerts get automatic RCA; user can trigger for WARNING or manual investigation |
| D402 | Time window | **30-min lookback** (configurable to 2h) | Most PG incident chains fit in 30 min; slow-developing issues need longer |
| D403 | UI output | **Inline summary on alert detail + dedicated RCA page** | Inline is cheap (alert panel exists), full page for deep investigation |
| D404 | ML dependency | **ML-enhanced, not required** | With ML: use Z-scores. Without: rate-of-change threshold spikes as fallback. RCA works in both modes. |
| D405 | Sub-iterations | **3: Engine Reliability → UI → Expansion/Calibration** | M14_01 includes core reasoning reliability (negative evidence pruning, basic confidence, state-aware causality). M14_02 adds UI. M14_03 expands rules, snapshot-driven chains, calibration. |
| D406 | Initial chains | **20 causal chains**, tiered A/B | Core PG failure modes; Tier A fully supported in M14_01, Tier B dependent on later integrations |
| D407 | Negative evidence | **Required in M14_01** | RCA must prune unsupported branches when mandatory intermediate evidence is absent; otherwise false positives are unacceptable |
| D408 | Knowledge base strategy | **Shared RCA + Adviser ontology** | RCA chains and adviser rules evolve from a shared, versioned knowledge base with stable identifiers and remediation hooks |

---

## 3. Architecture Overview

### 3.1 Components

```
┌──────────────────────────────────────────────────────┐
│                   RCA Engine                          │
│                                                       │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────┐ │
│  │ Causal Graph │  │  Correlation │  │  Incident  │ │
│  │  (knowledge) │→ │   Engine     │→ │  Builder   │ │
│  │  graph.go    │  │  engine.go   │  │ incident.go│ │
│  └──────────────┘  └──────┬───────┘  └────────────┘ │
│                           │                           │
│            ┌──────────────┼──────────────┐            │
│            ▼              ▼              ▼            │
│     ┌──────────┐  ┌──────────┐  ┌──────────────┐    │
│     │MetricStore│ │ML Detector│ │ Alert History │    │
│     │(time-range│ │(Z-scores) │ │ (events)     │    │
│     │ queries)  │ │ optional  │ │              │    │
│     └──────────┘  └──────────┘  └──────────────┘    │
└──────────────────────────────────────────────────────┘
```

### 3.2 Reasoning Reliability Principles

The RCA engine must follow these rules:

1. **Trigger-first traversal** — investigation always starts from a trigger symptom and walks upstream through the causal graph.
2. **Graph for reasoning, timeline for explanation** — the causal graph is the internal reasoning model, not the primary UI artifact. The timeline is the user-facing output.
3. **Required-evidence pruning** — a causal branch cannot survive if its mandatory intermediate evidence is absent. If the chain says "WAL → checkpoint → disk I/O → replication lag" but checkpoints were normal, the branch is killed.
4. **State-aware causality** — not all causes are short-lived events. Some are effective states: `shared_buffers = 256MB` is a persistent condition, not a timestamped event. Configuration-related chains must reason over effective state, not only change timestamps.
5. **Confidence is mandatory** — every RCA result must produce a confidence score and evidence-quality assessment. The engine must not present causality as mathematical proof.
6. **Alternative explanations** — RCA retains the top chain plus up to 1 alternative when evidence is not dominant. Capped at 2 total chains per incident.
7. **Bounded execution** — traversal depth, candidate chain count, and metric query volume are all bounded by configuration. RCA must not scale unbounded with fleet size or metric cardinality.

### 3.3 Shared Knowledge Base

RCA and Adviser must use a shared, versioned knowledge layer containing:

- **Symptom keys** — stable identifiers for observable symptoms (e.g., `symptom.replication_lag_high`)
- **Mechanism keys** — intermediate causal mechanisms (e.g., `mechanism.checkpoint_storm`)
- **Root cause keys** — identified root causes (e.g., `root_cause.bulk_insert_workload`)
- **Chain identifiers** — stable IDs for each causal chain (e.g., `chain.wal_checkpoint_io_repllag`)
- **Remediation hook IDs** — mapping to existing adviser rules (e.g., `remediation.increase_checkpoint_completion_target`)
- **Tier classification** — `stable` (Tier A) vs `experimental` (Tier B)

This layer is a Go struct with stable string constants in M14_01. It does not require a database schema or runtime registry — it's compiled-in knowledge. Future iterations may make it configurable.

### 3.4 Data Flow

1. **Trigger** → CRITICAL alert fires (auto) or user clicks "Investigate" (on-demand)
2. **Context** → Engine receives: instance ID, trigger metric, trigger timestamp, trigger value
3. **Window** → Engine defines analysis window: [trigger_time - lookback, trigger_time + forward_tail]
4. **Scope** → Engine selects relevant chain candidates based on trigger metric's domain pack (not all 20 chains)
5. **Query** → Engine queries MetricStore only for metrics referenced by active chain candidates (bounded by `max_metrics_per_run`)
6. **Detect** → For each metric in the window:
   - If ML enabled: use existing anomaly Z-scores
   - If ML disabled: compute rate-of-change, flag values exceeding 2σ from recent baseline
7. **Correlate** → Walk the causal graph starting from the trigger metric:
   - For each incoming edge: check required evidence (anomaly in expected time lag)
   - **Prune** branches where required evidence is absent
   - **Score** edges based on evidence strength (Z-score magnitude, temporal proximity)
   - Retain supporting evidence where present (strengthens confidence)
   - Recurse upstream for edges that pass
8. **Rank** → Score complete chains by cumulative edge confidence. Retain top chain + best alternative (max 2)
9. **Build** → Produce `Incident` with timeline, summary (qualified language), confidence, quality markers
10. **Store** → Persist the incident for later review
11. **Present** → Return via API, show in UI

---

## 4. Causal Graph Model

### 4.1 Core Types

```go
type CausalNode struct {
    ID          string   // stable identifier: "wal_generation"
    Name        string   // human-readable: "WAL Generation Rate"
    MetricKeys  []string // pg.checkpoint.buffers_written_per_second, etc.
    Layer       string   // "db", "os", "workload", "config"
    SymptomKey  string   // shared ontology: "symptom.wal_spike"
}

type EvidenceRequirement int
const (
    EvidenceRequired  EvidenceRequirement = iota // branch dies without it
    EvidenceSupporting                            // strengthens confidence if present
)

type TemporalSemantics int
const (
    BoundedLag     TemporalSemantics = iota // event → event within min/max lag
    PersistentState                          // ongoing condition (e.g., low shared_buffers)
    WhileEffective                           // active at incident time (config state)
)

type CausalEdge struct {
    FromNode     string
    ToNode       string
    MinLag       time.Duration       // for BoundedLag
    MaxLag       time.Duration       // for BoundedLag
    Temporal     TemporalSemantics
    Evidence     EvidenceRequirement
    Description  string              // "WAL spike causes checkpoint within 1-3 min"
    BaseConfidence float64           // 0.0-1.0 prior confidence of this link
    ChainID      string              // which chain(s) this edge belongs to
    RemediationHook string           // adviser rule ID, if applicable
}

type CausalGraph struct {
    Nodes map[string]*CausalNode
    Edges []CausalEdge
}
```

### 4.2 Chain Evaluation Model

Each causal chain declares:
- Upstream and downstream nodes
- Temporal semantics per edge (bounded lag, persistent state, while effective)
- Required evidence vs supporting evidence per edge
- Scope assumptions (single-instance, cluster-local)
- Remediation hook IDs (link to adviser rules)
- Rollout tier: **Tier A** (fully supported M14_01) or **Tier B** (dependent on later integrations)

### 4.3 Temporal Semantics

| Type | Meaning | Example |
|------|---------|---------|
| `BoundedLag` | Event A → Event B within min/max time | WAL spike → checkpoint within 1-3 min |
| `PersistentState` | Ongoing condition that persists | Dead tuple accumulation over hours |
| `WhileEffective` | Configuration state active at incident time | `shared_buffers = 256MB` was in effect |

Configuration-related chains must distinguish between configuration *change events* and *effective configuration state at incident time*. RCA reasons primarily over effective state: "shared_buffers was 256MB when the cache hit ratio dropped" — not "shared_buffers was changed 3 days ago."

### 4.4 The 20 Initial Causal Chains

#### Replication Chains (4)

| # | Tier | Chain | Path | Temporal | Scope |
|---|------|-------|------|----------|-------|
| 1 | A | Bulk workload → WAL → checkpoint → disk I/O → replication lag | workload_change → wal_generation → checkpoint → disk_io → replication_lag | BoundedLag (1-5 min) | single-instance |
| 2 | A | Inactive slot → WAL retention → disk fill | slot_inactive → wal_retention → disk_space | PersistentState | single-instance |
| 3 | B | Network issue → WAL receiver disconnect → lag | network_issue → wal_receiver → replication_lag | BoundedLag (0-1 min) | cluster-local (limited) |
| 4 | A | Long transaction on primary → replication apply delay | long_transaction → replication_apply_delay → replication_lag | BoundedLag (0-5 min) | single-instance |

**Replication scope note:** Chains 1, 2, 4 operate within a single monitored instance. Chain 3 requires evidence from both primary and replica — marked Tier B with "cluster-local (limited)" scope. If both instances are monitored by PGPulse, bounded cross-node correlation is permitted. Otherwise, the chain is marked incomplete.

#### I/O and Checkpoint Chains (4)

| # | Tier | Chain | Path | Temporal |
|---|------|-------|------|----------|
| 5 | A | WAL spike → checkpoint storm → disk I/O | wal_generation → checkpoint → disk_io | BoundedLag (1-3 min) |
| 6 | A | Autovacuum storm → disk I/O → query latency | autovacuum_activity → disk_io → query_latency | BoundedLag (0-2 min) |
| 7 | A | Temp file spike → disk I/O → query slowdown | temp_files → disk_io → query_latency | BoundedLag (0-1 min) |
| 8 | A | Shared buffers eviction → backend writes → I/O | buffers_backend → disk_io → query_latency | BoundedLag (0-1 min) |

#### Connection Chains (3)

| # | Tier | Chain | Path | Temporal |
|---|------|-------|------|----------|
| 9 | A | Lock contention → blocked queries → connection pileup | lock_contention → blocked_queries → connection_exhaustion | BoundedLag (0-5 min) |
| 10 | A | Connection spike → memory pressure → OS OOM risk | connection_spike → memory_pressure → os_oom | BoundedLag (1-10 min) |
| 11 | A | Long transactions → MVCC bloat → connection holding | long_transaction → mvcc_bloat → connection_resources | PersistentState |

#### Statement and Workload Chains (4)

| # | Tier | Chain | Path | Temporal |
|---|------|-------|------|----------|
| 12 | B | Query regression → CPU spike → latency | query_regression → cpu_spike → query_latency | BoundedLag (0-2 min) |
| 13 | B | New query (deployment) → resource shift | new_query → resource_shift → (various) | BoundedLag (0-5 min) |
| 14 | A | Missing index → seq scans → disk reads → I/O | missing_index → seq_scans → disk_reads → disk_io | BoundedLag (0-1 min) |
| 15 | A | pg_stat_statements fill → eviction | pgss_fill → pgss_eviction | BoundedLag (0-1 min) |

Chains 12-13 are Tier B: they require statement snapshot diffs to detect query regressions/new queries, which is M14_03 scope. Specifically, detecting a "new query" requires a set-difference operation between the current pg_stat_statements snapshot and a baseline snapshot — a query isn't "new" because it executed, but because its `queryid` wasn't present in the prior snapshot. This goes beyond Z-score or threshold analysis and depends on the M11 snapshot infrastructure.

#### Vacuum and Maintenance Chains (3)

| # | Tier | Chain | Path | Temporal |
|---|------|-------|------|----------|
| 16 | A | Dead tuple accumulation → bloat → scan degradation | dead_tuples → table_bloat → query_latency | PersistentState |
| 17 | A | Wraparound approaching → aggressive vacuum → I/O | wraparound_risk → aggressive_vacuum → disk_io | BoundedLag (0-5 min) |
| 18 | A | Long tx blocking vacuum → dead tuple growth | long_transaction → vacuum_blocked → dead_tuples | PersistentState |

#### Configuration and System Chains (2)

| # | Tier | Chain | Path | Temporal |
|---|------|-------|------|----------|
| 19 | B | Settings change → behavioral shift | config_change → (various symptoms) | WhileEffective |
| 20 | A | OS memory pressure → OOM killer → PG crash | memory_pressure → os_oom → pg_crash → connection_reset | BoundedLag (0-1 min) |

Chain 19 is Tier B: requires settings effective-state reasoning from M14_03.

**Tier summary:** 16 Tier A chains (M14_01), 4 Tier B chains (M14_03: chains 3, 12, 13, 19).

### 4.5 Metric Key Mapping

| Node ID | Metric Keys | Source |
|---------|-------------|--------|
| `replication_lag` | `pg.replication.lag.replay_bytes`, `pg.replication.lag.replay_seconds` | ReplicationLagCollector |
| `wal_generation` | `pg.checkpoint.buffers_written_per_second` | CheckpointCollector |
| `checkpoint` | `pg.checkpoint.requested_per_second`, `pg.checkpoint.timed_per_second`, `pg.checkpoint.sync_time_ms` | CheckpointCollector |
| `disk_io` | `os.disk.util_pct`, `os.disk.read_bytes_per_sec`, `os.disk.write_bytes_per_sec` | OSCollector |
| `connection_exhaustion` | `pg.connections.utilization_pct`, `pg.connections.total` | ConnectionsCollector |
| `lock_contention` | `pg.wait_events.count` (Lock type) | WaitEventsCollector |
| `long_transaction` | `pg.long_transactions.count`, `pg.long_transactions.oldest_seconds` | LongTransactionsCollector |
| `cpu_spike` | `os.cpu.user_pct`, `os.cpu.system_pct`, `os.cpu.iowait_pct` | OSCollector |
| `memory_pressure` | `os.memory.available_kb`, `os.memory.used_kb` | OSCollector |
| `query_latency` | `pg.statements.top.avg_time_ms` | StatementsTopCollector |
| `autovacuum_activity` | `pg.progress.vacuum.completion_pct` (count active) | VacuumProgressCollector |
| `dead_tuples` | `pg.db.vacuum.dead_tuples`, `pg.db.vacuum.dead_pct` | DatabaseCollector |
| `table_bloat` | `pg.db.bloat.table_ratio`, `pg.db.bloat.table_wasted_bytes` | DatabaseCollector |
| `temp_files` | `pg.transactions.temp_bytes` | TransactionsCollector |
| `buffers_backend` | `pg.bgwriter.buffers_backend_per_second` | CheckpointCollector |
| `pgss_fill` | `pg.statements.fill_pct` | StatementsConfigCollector |
| `wraparound_risk` | `pg.server.wraparound_pct` | ServerInfoCollector |
| `slot_inactive` | `pg.replication.slot.active` (value=0) | ReplicationSlotsCollector |
| `disk_space` | `os.disk.used_bytes`, `os.disk.free_bytes` | OSCollector |
| `config_change` | (settings snapshot diff — Tier B, M14_03) | SettingsCollector |
| `query_regression` | (statement snapshot diff — Tier B, M14_03) | StatementsTopCollector |
| `new_query` | (statement snapshot diff — Tier B, M14_03) | StatementsTopCollector |

---

## 5. Anomaly Detection: Dual Mode

### 5.1 ML Mode (ml.enabled: true)

Use the existing `internal/ml/Detector` which computes per-metric Z-scores. The RCA engine queries the detector for anomaly scores in the analysis window.

### 5.2 Threshold Fallback Mode (ml.enabled: false)

When ML is disabled, the RCA engine does lightweight on-demand anomaly detection:

1. Query the metric's recent history (lookback window + 1 hour baseline before)
2. Compute mean and standard deviation of the baseline period
3. Flag points in the analysis window exceeding 2σ from baseline mean
4. Flag points where rate-of-change exceeds 3x baseline rate

### 5.3 AnomalySource Interface

```go
type AnomalySource interface {
    GetAnomalies(ctx context.Context, instanceID string, from, to time.Time) ([]AnomalyEvent, error)
    GetMetricAnomaly(ctx context.Context, instanceID, metricKey string, from, to time.Time) (*AnomalyEvent, error)
}

type AnomalyEvent struct {
    InstanceID string
    MetricKey  string
    Timestamp  time.Time
    Value      float64
    ZScore     float64     // anomaly magnitude (ML mode)
    RateChange float64     // rate of change (fallback mode)
    Source     string      // "ml" or "threshold"
}
```

### 5.4 Evidence Model

RCA must treat anomaly signals as one form of evidence. In both ML and threshold modes, the engine supports:

- **Required evidence checks** — mandatory intermediate node must show anomaly
- **Supporting evidence checks** — optional node that strengthens confidence
- **Evidence strength scoring** — Z-score magnitude, temporal proximity to expected lag
- **Missing-evidence penalties** — absent required evidence kills the branch; absent supporting evidence reduces confidence

### 5.5 Fuzzy Window for Collection Jitter ("Phantom Event" Problem)

If a metric is polled every 60 seconds, a transient event lasting 10 seconds (e.g., an instantaneous pg_stat_statements eviction spike) can fall between polling cycles. If a required edge relies on that metric, the valid causal chain is pruned — a false negative.

**Mitigation:** The engine applies a "time smear" to evidence windows. If the expected lag for an edge is [0, 1 min], the engine evaluates data points from [trigger - max_lag - collection_interval, trigger - min_lag + collection_interval]. This accounts for collection jitter without overly broadening the window. The smear width is derived from the metric's actual collection frequency (high=10s, medium=60s, low=300s).

### 5.6 Batch Stats Query for Threshold Fallback

In threshold fallback mode, computing mean and standard deviation for every metric individually would generate O(nodes × metrics_per_node) queries — potentially 60+ MetricStore calls for one RCA run.

**Mitigation:** The `MetricStore` interface must support a batch statistics method:

```go
// GetMetricStats computes mean, stddev, min, max for multiple metrics in a single SQL query
GetMetricStats(ctx context.Context, instanceID string, metricKeys []string, from, to time.Time) (map[string]MetricStats, error)
```

This executes one SQL query using `avg()` and `stddev_samp()` aggregates grouped by metric key, rather than pulling raw data into Go. The RCA engine calls this once per analysis run with all needed metric keys, then evaluates anomalies in-memory.

### 5.7 Trigger-Scoped Metric Retrieval

The engine must not query all 220+ metrics for every RCA run. Metric retrieval is restricted by:

- **Trigger class** — the trigger metric determines which domain pack is relevant
- **Active chain set** — only chains whose terminal node matches the trigger are activated
- **Graph reachability** — only metrics referenced by reachable upstream nodes are queried
- **Configured cap** — `max_metrics_per_run` hard limit (default 50)

This keeps RCA latency predictable and protects MetricStore under alert storms.

---

## 6. Incident Timeline Output

### 6.1 Core Types

```go
type Incident struct {
    ID                int64
    InstanceID        string
    TriggerMetric     string
    TriggerValue      float64
    TriggerTime       time.Time
    TriggerKind       string              // "alert", "manual", "anomaly"
    AnalysisWindow    TimeWindow
    PrimaryChain      *CausalChainResult  // strongest evidence-backed chain
    AlternativeChain  *CausalChainResult  // second-best, if evidence is close (nil if dominant)
    Timeline          []TimelineEvent     // ordered by timestamp
    Summary           string              // qualified language: "likely caused by..."
    Confidence        float64             // 0.0-1.0
    Quality           QualityStatus       // evidence completeness + scope markers
    RemediationHooks  []string            // adviser rule IDs for recommended next steps
    AutoTriggered     bool
    CreatedAt         time.Time
    // Future-ready feedback fields (nullable, not populated in M14_01)
    ReviewStatus      *string             // "correct", "incorrect", "partial", nil
    ReviewedBy        *string
    ReviewedAt        *time.Time
    ReviewComment     *string
}

type CausalChainResult struct {
    ChainID    string           // stable chain identifier
    Score      float64          // cumulative edge confidence
    Events     []TimelineEvent  // events in this chain
    RootCause  string           // root cause key from shared ontology
}

type TimelineEvent struct {
    Timestamp   time.Time
    NodeID      string
    NodeName    string
    MetricKey   string
    Value       float64
    ZScore      float64
    Layer       string         // "db", "os", "workload", "config"
    Role        string         // "root_cause", "intermediate", "symptom"
    Evidence    string         // "required", "supporting"
    Description string         // "WAL generation rate jumped to 4x normal"
    EdgeDesc    string         // causal edge description (nil for trigger)
}

type QualityStatus struct {
    TelemetryCompleteness float64  // 0.0-1.0: what % of needed metrics had data
    AnomalySourceMode     string   // "ml" or "threshold"
    ScopeLimitations      []string // e.g., "single-instance only", "OS metrics unavailable"
    UnavailableDeps       []string // e.g., "settings snapshots not enabled"
}

type TimeWindow struct {
    From time.Time
    To   time.Time
}
```

### 6.2 Summary Language

Incident summaries must use qualified language:

- "Likely caused by..."
- "Consistent with..."
- "Supported by evidence from..."
- "Alternative explanations remain possible"

The engine must not present causality as certainty.

### 6.3 Feedback Schema (Future-Ready)

The `Incident` struct includes nullable review fields (`ReviewStatus`, `ReviewedBy`, `ReviewedAt`, `ReviewComment`). These are not populated in M14_01 but the schema supports them. M14_03 may prepare the capture path; actual supervised calibration is deferred beyond M14.

---

## 7. API Endpoints (New)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/instances/{id}/rca/analyze` | viewer+ | On-demand RCA: `{metric, timestamp, window_minutes}` |
| GET | `/instances/{id}/rca/incidents` | viewer+ | List incidents for instance (paginated) |
| GET | `/instances/{id}/rca/incidents/{incidentId}` | viewer+ | Full incident with timeline |
| GET | `/rca/incidents` | viewer+ | All incidents across fleet |
| GET | `/rca/graph` | viewer+ | Causal graph definition (for UI visualization) |

### 7.1 Trigger Abstraction

The on-demand API accepts `{metric, timestamp, window_minutes}` in M14_01. The design must remain compatible with future trigger types: alert ID, anomaly ID, SQL fingerprint, database-scoped incident.

### 7.2 Review API (Future-Ready)

Not shipped in M14_01, but the incident model must not block future endpoints for: mark correct/incorrect/partial, attach reviewer comment, select alternative chain.

---

## 8. Storage

### 8.1 Migration

```sql
CREATE TABLE rca_incidents (
    id                    BIGSERIAL PRIMARY KEY,
    instance_id           TEXT NOT NULL,
    trigger_metric        TEXT NOT NULL,
    trigger_value         DOUBLE PRECISION NOT NULL,
    trigger_time          TIMESTAMPTZ NOT NULL,
    trigger_kind          TEXT NOT NULL DEFAULT 'alert',
    window_from           TIMESTAMPTZ NOT NULL,
    window_to             TIMESTAMPTZ NOT NULL,
    primary_chain_id      TEXT,                          -- stable chain identifier
    primary_root_cause    TEXT,                          -- root cause key from ontology
    confidence            DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence_bucket     TEXT,                          -- "high", "medium", "low"
    quality_status        TEXT NOT NULL DEFAULT 'unknown',
    timeline_json         JSONB NOT NULL,                -- full incident artifact
    summary               TEXT NOT NULL,
    auto_triggered        BOOLEAN NOT NULL DEFAULT false,
    remediation_hooks     TEXT[],                        -- adviser rule IDs
    review_status         TEXT,                          -- future: correct/incorrect/partial
    reviewed_by           TEXT,
    reviewed_at           TIMESTAMPTZ,
    review_comment        TEXT,
    chain_version         TEXT,                          -- knowledge base version tag
    anomaly_source_mode   TEXT,                          -- "ml" or "threshold"
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rca_incidents_instance ON rca_incidents(instance_id, created_at DESC);
CREATE INDEX idx_rca_incidents_trigger ON rca_incidents(trigger_metric, trigger_time);
CREATE INDEX idx_rca_incidents_chain ON rca_incidents(primary_chain_id);
CREATE INDEX idx_rca_incidents_review ON rca_incidents(review_status) WHERE review_status IS NOT NULL;
```

Timeline events stored as JSONB — always read/written as a unit. Normalized summary fields (`primary_chain_id`, `primary_root_cause`, `confidence_bucket`, `quality_status`, `review_status`) support filtering, analytics, and deduplication without parsing JSONB.

---

## 9. Configuration

```yaml
rca:
  enabled: true
  lookback_window: 30m                 # how far back from trigger
  auto_trigger_severity: critical      # auto-trigger on this severity+
  max_incidents_per_hour: 10           # rate limit per instance
  retention_days: 90
  max_traversal_depth: 5               # hard cap on backward graph walk
  max_candidate_chains: 5              # chain candidates before ranking
  max_metrics_per_run: 50              # protects MetricStore
  min_edge_score: 0.25                 # prune weak edges early
  min_chain_score: 0.40                # hide weak chains from output
  deferred_forward_tail: 5m            # post-trigger tail for auto-triggered RCA
  quality_banner_enabled: true         # expose evidence quality in results
  remediation_hooks_enabled: true      # emit adviser hook IDs in incidents
```

Bounded execution: traversal depth, candidate chains, metric queries, and forward-tail are all configurable. RCA does not scale unbounded with fleet size or metric cardinality.

---

## 10. Sub-Iteration Breakdown

### M14_01 — Causal Graph + Reliable Correlation Engine (backend)

**Scope:** `internal/rca/` package, 16 Tier A chains, anomaly source abstraction (ML + fallback), required-evidence pruning, basic confidence scoring, state-aware temporal semantics, incident types, storage (pgstore + nullstore), migration, 5 new API endpoints, auto-trigger hook on alert dispatcher, shared knowledge identifiers.

**Explicitly includes:**
- Negative evidence pruning
- Minimal confidence model (edge scores → chain score)
- Bounded traversal (depth, metrics, candidates)
- Quality metadata in incident results
- Remediation hook IDs in result model
- Shared RCA/Adviser knowledge identifiers (stable string constants)
- Tier A/B classification of all chains

**Explicitly excludes:**
- Advanced contradiction reasoning
- Learned confidence weights
- Full cross-instance correlation
- Rich review workflow UI
- Statement snapshot integration (Tier B chains)
- Settings effective-state reasoning (Tier B chains)

**Estimated:** ~1,500–2,000 lines, 10–12 new files, 2–3 modified files
**Agent team:** 3 agents (RCA Engine + API + QA)

### M14_02 — RCA UI (frontend)

**Scope:** New RCA Incidents page, incident timeline visualization (vertical event chain with severity colors, causal arrows, confidence markers, quality banner), inline RCA summary on alert detail panel, "Investigate" button, sidebar navigation.

**Estimated:** ~800–1,200 lines, 6–8 new components, 2–3 modified pages
**Agent team:** 2 agents (Frontend + QA)

### M14_03 — Expansion, Calibration, and Knowledge Integration

**Scope:** Integrate settings effective-state reasoning (chain 19), integrate statement snapshot diffs (chains 12, 13), activate Tier B chains, improve summary generation, strengthen confidence model, add rule-quality instrumentation stubs, prepare incident feedback capture path, RCA→Adviser bridge.

**Estimated:** ~500–800 lines, 3–5 modified files
**Agent team:** 2 agents (RCA Engine + QA)

---

## 11. Non-Requirements (Explicitly Out of Scope)

| Item | Reason |
|------|--------|
| APM / application trace correlation | Requires app instrumentation — outside PGPulse philosophy |
| User-defined custom causal chains | Complexity; 20 built-in chains cover common cases first |
| Real-time streaming RCA | On-demand + auto-trigger is sufficient |
| Full fleet-wide cross-instance correlation | Requires topology-aware reasoning. Bounded cluster-local correlation for selected replication chains is permitted if explicitly implemented. |
| Automatic remediation (self-healing) | RCA diagnoses and emits remediation hooks to Adviser, but does not execute changes automatically |
| Online learning from RCA feedback | Feedback capture may be prepared in M14, but rule reweighting is deferred to a later milestone |

---

## 12. Risk Register

| Risk | Impact | Mitigation |
|------|--------|------------|
| MetricStore lacks data for low-frequency metrics in 30-min window | Incomplete timeline | Use available points; flag low-confidence nodes; quality banner |
| ML detector not tracking all metrics the graph needs | Gaps in anomaly detection | Threshold fallback fills gaps; ML is enhancement, not requirement |
| Too many auto-triggered incidents (noisy) | Incident flood | Rate limit: max 10/instance/hour; cooldown per trigger metric |
| Causal chains produce false positives (coincidental correlation) | Misleading RCA | Required-evidence pruning in M14_01, confidence scoring, limited alternatives, quality banner, human review path |
| Configuration causality modeled as events only, not effective state | Incorrect RCA for settings-driven incidents | State-aware temporal semantics (WhileEffective) for config chains |
| Replication chains require upstream evidence outside instance scope | Partial or misleading lag RCA | Bounded cluster-local scope for chain 3; other repl chains are single-instance; limitations documented |
| Threshold fallback becomes expensive under alert storms | High latency, MetricStore pressure | Bounded metrics per run, domain-pack filtering, bounded traversal, max_incidents_per_hour |
| Rule base grows without governance | Inconsistent RCA/Adviser behavior | Shared knowledge identifiers, Tier A/B classification, stable chain IDs |
| JSONB timeline grows large for complex incidents | Storage bloat | Cap timeline at 50 events; retention cleanup at 90 days |
| Phantom events between polling cycles cause false negatives | Valid chains pruned because required evidence fell between collection intervals | Fuzzy window / time-smear: extend evidence windows by ± collection_interval to account for jitter |
| Threshold fallback generates excessive MetricStore queries | High latency per RCA run | Batch stats via `GetMetricStats()` — single SQL with `avg()`/`stddev_samp()` grouped by metric key |

---

## 13. Success Criteria

1. A CRITICAL replication lag alert auto-produces an incident timeline showing the strongest evidence-backed causal chain within supported scope.
2. A DBA can click "Investigate" on any alert and see a timeline distinguishing likely root cause, intermediate mechanisms, and symptom.
3. RCA works both with ML enabled and with threshold fallback mode.
4. RCA does not retain unsupported branches when required intermediate evidence is missing.
5. RCA output includes confidence and quality markers, not just a human-readable summary.
6. RCA results carry remediation hook IDs compatible with the existing Adviser layer.
7. The initial chain set is explicitly tiered and testable, with clear Tier A vs Tier B distinction.
8. Zero existing tests break — RCA is an additive package with clean interfaces and bounded runtime.

---

## 14. Future-Ready Foundations

M14 design prepares for later capabilities even where not fully implemented:

1. **Shared Knowledge Base** — RCA chains and Adviser rules use shared identifiers from a common ontology. Future: make it configurable, versionable, exportable.

2. **Incident Feedback Loop** — Schema supports human review (correct/incorrect/partial). Future: supervised confidence calibration, rule quality analytics.

3. **RCA → Adviser Bridge** — RCA emits root cause keys and remediation hook IDs. Future: focused next-step recommendations triggered by RCA findings.

4. **Cluster-Aware Evolution** — Incident model retains instance ID and topology context. Future: bounded cross-instance correlation for primary↔replica chains.

5. **Rule Quality Analytics** — Stable chain IDs and review fields enable future reporting: most-confirmed chains, most-rejected chains, best-performing remediation mappings.
