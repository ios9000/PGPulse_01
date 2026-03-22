# M14_03 — Design: Expansion, Calibration, and Knowledge Integration

**Iteration:** M14_03
**Date:** 2026-03-22
**Companion:** M14_03_requirements.md, D506_recommendation_schema_evolution.md
**Locked Decisions:** D500–D508, Q1–Q3

---

## 1. Architecture Overview

M14_03 modifies 5 packages and adds new files to 2 packages:

```
internal/rca/          — W2 (threshold hardening), W3 (WhileEffective), W4 (statement source),
                         W5 (tier B activation), W6 (summaries), W7 (confidence), W9 (json tags)
internal/remediation/  — W8 (Unified Upsert bridge, urgency, hooks)
internal/config/       — W2 (new RCA config fields), W8 (new remediation config fields)
internal/api/          — W8 (recommendations query params), W10 (review endpoint)
web/                   — W8 (RCABadge, inline recs), W9 (types update), W10 (review widget)
migrations/            — W8 (017_recommendation_rca_bridge.sql)
pgpulse.yml (demo)     — W1 (comprehensive ML metrics config)
```

Data flow after M14_03:

```
Collector → MetricStore → ML Detector (now tracking all RCA-relevant keys)
                               ↓
                       AnomalySource (ML primary, Threshold 4h+calm fallback)
                               ↓
                          RCA Engine.Analyze()
                            ├─ StatementDiffSource (chains 12, 13)
                            ├─ SettingsProvider (chain 19 / WhileEffective)
                            ├─ All 20 chains (Tier B filter removed)
                            ├─ Improved summaries with metric values
                            ├─ Refined confidence scoring
                            ↓
                          Incident stored
                            ├─ EvaluateHook() → Remediation Engine → Upsert()
                            ↓
                       Two UI paths:
                         Path A: Incident Detail → inline recommendations
                         Path B: Adviser Dashboard → urgency-sorted feed
```

---

## 2. W1 — ML Metric Key Alignment

### 2.1 Comprehensive Metric-to-Chain Mapping

The agent must perform a pre-flight grep on `chains.go` to extract every `MetricKeys` slice from every `CausalNode`. Then cross-reference each key against CODEBASE_DIGEST Section 3 to confirm it exists and identify the emitting collector.

**Expected output:** A table in the corrections doc of the form:

```
Chain 1: "Connection Saturation → ..."
  Node "connection_pressure": MetricKeys = ["pg.connections.utilization_pct", "pg.connections.total"]
    → ConnectionsCollector (high freq, 10s) ✓
  Node "backend_waiting": MetricKeys = ["pg.wait_events.count"]
    → WaitEventsCollector (high freq, 10s) ✓
  ...
```

Any metric key referenced by a chain node but NOT present in the collector catalog is a bug that must be fixed (either by correcting the chain definition or adding the metric key alias).

### 2.2 Demo YAML Config Block

The comprehensive `ml.metrics[]` block. Structure:

```yaml
ml:
  enabled: true
  collection_interval: 30s
  zscore_threshold_warning: 2.5
  zscore_threshold_critical: 4.0
  anomaly_logic: "or"
  persistence:
    enabled: true
  metrics:
    # === HIGH FREQUENCY (10s collectors) ===
    - key: pg.connections.utilization_pct
      enabled: true
      period: 360     # 1h at 10s intervals = seasonal period for daily pattern
    - key: pg.connections.total
      enabled: true
      period: 360
    - key: pg.cache.hit_ratio
      enabled: true
      period: 360
    - key: pg.locks.blocker_count
      enabled: true
      period: 360
    - key: pg.locks.blocked_count
      enabled: true
      period: 360
    - key: pg.locks.max_chain_depth
      enabled: true
      period: 360
    - key: pg.long_transactions.oldest_seconds
      enabled: true
      period: 360
    - key: pg.long_transactions.count
      enabled: true
      period: 360
    - key: pg.wait_events.count
      enabled: true
      period: 360

    # === MEDIUM FREQUENCY (60s collectors) ===
    - key: pg.checkpoint.timed
      enabled: true
      period: 60      # 1h at 60s intervals
    - key: pg.checkpoint.requested
      enabled: true
      period: 60
    - key: pg.checkpoint.write_time_ms
      enabled: true
      period: 60
    - key: pg.checkpoint.sync_time_ms
      enabled: true
      period: 60
    - key: pg.checkpoint.buffers_written
      enabled: true
      period: 60
    - key: pg.bgwriter.buffers_backend
      enabled: true
      period: 60
    - key: pg.bgwriter.buffers_clean
      enabled: true
      period: 60
    - key: pg.bgwriter.buffers_alloc
      enabled: true
      period: 60
    - key: pg.replication.lag.total_bytes
      enabled: true
      period: 60
    - key: pg.replication.lag.replay_seconds
      enabled: true
      period: 60
    - key: pg.replication.slot.retained_bytes
      enabled: true
      period: 60
    - key: pg.statements.fill_pct
      enabled: true
      period: 60
    - key: pg.transactions.commit_ratio
      enabled: true
      period: 60
    - key: pg.transactions.deadlocks
      enabled: true
      period: 60

    # === OS METRICS ===
    - key: os.cpu.user_pct
      enabled: true
      period: 120     # 1h at 30s intervals (OS collector medium freq)
    - key: os.cpu.iowait_pct
      enabled: true
      period: 120
    - key: os.cpu.system_pct
      enabled: true
      period: 120
    - key: os.memory.available_kb
      enabled: true
      period: 120
    - key: os.memory.used_kb
      enabled: true
      period: 120
    - key: os.disk.util_pct
      enabled: true
      period: 120
    - key: os.disk.free_bytes
      enabled: true
      period: 120
    - key: os.load.1m
      enabled: true
      period: 120
    - key: os.load.5m
      enabled: true
      period: 120

    # === LOW FREQUENCY (300s collectors) — longer seasonal period ===
    - key: pg.server.wraparound_pct
      enabled: true
      period: 12      # 1h at 300s intervals
    - key: pg.database.size_bytes
      enabled: true
      period: 12

    # === ADDITIONAL COMPREHENSIVE COVERAGE ===
    # Not currently referenced by chains but valuable for anomaly detection
    - key: pg.connections.by_state
      enabled: true
      period: 360
    - key: pg.bgwriter.maxwritten_clean
      enabled: true
      period: 60
    - key: pg.replication.active_replicas
      enabled: true
      period: 60
    - key: pg.statements.count
      enabled: true
      period: 60
    - key: os.disk.read_bytes_per_sec
      enabled: true
      period: 120
    - key: os.disk.write_bytes_per_sec
      enabled: true
      period: 120
```

**Note:** The agent MUST verify every key above against `chains.go` MetricKeys AND the collector catalog. If any chain references a key not in this list, add it. If any key in this list doesn't exist in collectors, flag it.

### 2.3 Demo VM Deployment

After updating `pgpulse.yml` on the demo VM:

```bash
ssh ml4dbs@185.159.111.139
sudo systemctl stop pgpulse
# Edit /opt/pgpulse/pgpulse.yml — replace ml: block with the comprehensive version
sudo systemctl start pgpulse
# Wait 2 minutes for baseline warm-up
curl -s http://localhost:8989/api/v1/ml/models | python3 -m json.tool | head -50
```

---

## 3. W2 — Threshold Fallback Hardening

### 3.1 Config Additions

In `internal/rca/config.go`, add to `RCAConfig`:

```go
type RCAConfig struct {
    // ... existing fields ...
    ThresholdBaselineWindow time.Duration `yaml:"threshold_baseline_window"` // Default: 4h
    ThresholdCalmPeriod     time.Duration `yaml:"threshold_calm_period"`     // Default: 15m
    ThresholdCalmSigma      float64       `yaml:"threshold_calm_sigma"`      // Default: 1.5
}

func DefaultRCAConfig() RCAConfig {
    return RCAConfig{
        // ... existing defaults ...
        ThresholdBaselineWindow: 4 * time.Hour,
        ThresholdCalmPeriod:     15 * time.Minute,
        ThresholdCalmSigma:      1.5,
    }
}
```

### 3.2 ThresholdAnomalySource Changes

In `internal/rca/anomaly.go`, modify `ThresholdAnomalySource`:

```go
// Change the stats query window from hardcoded 1h to configurable
func (t *ThresholdAnomalySource) GetAnomalies(...) (...) {
    // OLD: from = to.Add(-1 * time.Hour)
    // NEW: from = to.Add(-t.baselineWindow)  // 4h default
    
    stats, err := t.statsProvider.GetMetricStats(ctx, instanceID, metricKey, from, to)
    if err != nil { ... }
    
    // NEW: calm period check
    if !t.isBaselineCalm(ctx, instanceID, metricKey, to, stats) {
        // Mark as unreliable, return with reduced confidence
        // Set anomaly.Source = "threshold_unreliable"
    }
}
```

### 3.3 Calm Period Check Implementation

```go
// isBaselineCalm checks if the most recent CalmPeriod of data
// falls within CalmSigma standard deviations of the rolling mean.
func (t *ThresholdAnomalySource) isBaselineCalm(
    ctx context.Context,
    instanceID, metricKey string,
    to time.Time,
    fullStats MetricStats,
) bool {
    // Query just the recent window
    recentFrom := to.Add(-t.calmPeriod)
    recentStats, err := t.statsProvider.GetMetricStats(ctx, instanceID, metricKey, recentFrom, to)
    if err != nil || recentStats.Count < 3 {
        return false // Not enough data — not calm
    }
    
    // Check if recent mean is within CalmSigma of the full baseline
    if fullStats.StdDev == 0 {
        return true // No variance at all — definitionally calm
    }
    deviation := math.Abs(recentStats.Mean - fullStats.Mean) / fullStats.StdDev
    return deviation <= t.calmSigma
}
```

### 3.4 QualityStatus Integration

When the calm check fails, append to `QualityStatus.ScopeLimitations`:

```go
quality.ScopeLimitations = append(quality.ScopeLimitations,
    "Threshold baseline unreliable for metric "+metricKey+
    " (recent data volatile, σ-deviation: "+fmt.Sprintf("%.1f", deviation)+")")
```

---

## 4. W3 — WhileEffective Temporal Semantics

### 4.1 Engine Dependencies

The RCA engine needs access to settings data. Add an optional `SettingsProvider` interface:

```go
// internal/rca/settings.go (new file)

// SettingsProvider supplies current and historical settings data for
// WhileEffective temporal semantics.
type SettingsProvider interface {
    // GetSettingValue returns the current value of a PostgreSQL setting
    // for the given instance. Returns ("", false) if unknown.
    GetSettingValue(ctx context.Context, instanceID, settingName string) (string, bool, error)
    
    // GetSettingChanges returns settings that changed within the time window.
    // Each change includes the setting name, old value, new value, and timestamp.
    GetSettingChanges(ctx context.Context, instanceID string, from, to time.Time) ([]SettingChange, error)
}

type SettingChange struct {
    Name      string
    OldValue  string
    NewValue  string
    Timestamp time.Time
}
```

### 4.2 Implementation: Settings Snapshot Adapter

```go
// internal/rca/settings_adapter.go (new file)

// SnapshotSettingsProvider implements SettingsProvider by querying
// the settings snapshot store and optionally the live connection.
type SnapshotSettingsProvider struct {
    store    settings.SnapshotStore  // internal/settings/store.go
    connProv InstanceConnProvider    // For live queries when snapshots are stale
}
```

The adapter queries `settings_snapshots` table for the two most recent snapshots within the window, diffs them, and returns changes. If no snapshots exist, it falls back to live `SHOW` queries via `connProv`.

### 4.3 evaluateEdge() WhileEffective Implementation

In `internal/rca/graph.go`:

```go
case WhileEffective:
    // The edge fires if the source node's condition is still in effect.
    // For settings chains: a config change happened AND the new value persists.
    if e.settingsProvider == nil {
        return 0, nil, false
    }
    
    changes, err := e.settingsProvider.GetSettingChanges(ctx, instanceID, from, to)
    if err != nil {
        return 0, nil, false
    }
    
    // Check if any of the source node's relevant settings changed
    // and the change is still effective (current value == changed value)
    for _, change := range changes {
        if isRelevantSetting(edge, change.Name) {
            currentVal, ok, err := e.settingsProvider.GetSettingValue(ctx, instanceID, change.Name)
            if err != nil || !ok {
                continue
            }
            if currentVal == change.NewValue {
                // Change is still in effect — edge fires
                score := edge.BaseConfidence
                evidence := &AnomalyEvent{
                    MetricKey:   "pg.settings." + change.Name,
                    Timestamp:   change.Timestamp,
                    Value:       0, // Settings are string-valued; store hash or 0
                    Description: fmt.Sprintf("%s changed from %s to %s (still effective)",
                        change.Name, change.OldValue, change.NewValue),
                }
                return score, evidence, true
            }
        }
    }
    return 0, nil, false
```

### 4.4 Relevant Settings Mapping

The `isRelevantSetting()` function maps edge context to PostgreSQL setting names. For chain 19, the mapping is derived from the edge's metadata or the target node's mechanism key. Example mappings:

```go
var settingsByMechanism = map[string][]string{
    MechCheckpointStorm:    {"checkpoint_completion_target", "max_wal_size", "checkpoint_timeout"},
    MechMemoryPressure:     {"shared_buffers", "work_mem", "maintenance_work_mem", "effective_cache_size"},
    MechConnectionExhaust:  {"max_connections", "superuser_reserved_connections"},
    MechVacuumFailing:      {"autovacuum_max_workers", "autovacuum_naptime", "autovacuum_vacuum_cost_limit"},
    MechWALBloat:           {"max_wal_size", "wal_keep_size", "max_slot_wal_keep_size"},
}
```

---

## 5. W4 — Statement Snapshot Diff Integration

### 5.1 StatementDiffSource

```go
// internal/rca/statement_source.go (new file)

// StatementDiffSource provides anomaly events derived from
// pg_stat_statements snapshot diffs for chains 12 and 13.
type StatementDiffSource struct {
    store    statements.SnapshotStore
    logger   *slog.Logger
}

// Implements a method called by the RCA engine during the detect step.
func (s *StatementDiffSource) GetStatementAnomalies(
    ctx context.Context,
    instanceID string,
    from, to time.Time,
) ([]AnomalyEvent, error) {
    // 1. Get the two most recent snapshots for this instance within [from, to]
    snapshots, err := s.store.GetLatestSnapshots(ctx, instanceID, 2)
    if err != nil || len(snapshots) < 2 {
        return nil, nil // No snapshots — not an error
    }
    
    // 2. Compute diff
    oldEntries, _ := s.store.GetSnapshotEntries(ctx, snapshots[1].ID)
    newEntries, _ := s.store.GetSnapshotEntries(ctx, snapshots[0].ID)
    diff := statements.ComputeDiff(oldEntries, newEntries)
    
    var anomalies []AnomalyEvent
    
    // 3. Detect query regressions (chain 12)
    for _, d := range diff.Regressions {
        if d.MeanExecTimeRatio > 2.0 { // >2× regression
            anomalies = append(anomalies, AnomalyEvent{
                InstanceID:  instanceID,
                MetricKey:   "pg.statements.regression",
                Timestamp:   snapshots[0].CapturedAt,
                Value:       d.MeanExecTimeNew,
                BaselineVal: d.MeanExecTimeOld,
                ZScore:      d.MeanExecTimeRatio, // Use ratio as pseudo-zscore
                Strength:    min(d.MeanExecTimeRatio/5.0, 1.0), // Normalize to [0,1]
                Source:      "statement_diff",
                Description: fmt.Sprintf("Query %d regressed %.1f× (%.2fms → %.2fms)",
                    d.QueryID, d.MeanExecTimeRatio, d.MeanExecTimeOld*1000, d.MeanExecTimeNew*1000),
            })
        }
    }
    
    // 4. Detect new queries (chain 13)
    for _, d := range diff.NewQueries {
        if d.TotalTimePct > 0.05 { // >5% of total workload
            anomalies = append(anomalies, AnomalyEvent{
                InstanceID:  instanceID,
                MetricKey:   "pg.statements.new_query",
                Timestamp:   snapshots[0].CapturedAt,
                Value:       d.TotalTimePct,
                BaselineVal: 0,
                ZScore:      d.TotalTimePct * 20, // Scale for scoring
                Strength:    min(d.TotalTimePct*5, 1.0),
                Source:      "statement_diff",
                Description: fmt.Sprintf("New query %d consuming %.1f%% of workload",
                    d.QueryID, d.TotalTimePct*100),
            })
        }
    }
    
    return anomalies, nil
}
```

### 5.2 Integration Point

In `engine.go`, during the "detect anomalies" step (step 4 of the 9-step algorithm), add:

```go
// After standard anomaly detection from ML/Threshold sources
if e.statementSource != nil {
    stmtAnomalies, err := e.statementSource.GetStatementAnomalies(ctx, req.InstanceID, window.From, window.To)
    if err != nil {
        e.logger.Warn("statement diff source failed", "error", err)
    } else {
        for _, sa := range stmtAnomalies {
            anomalyMap[sa.MetricKey] = append(anomalyMap[sa.MetricKey], sa)
        }
    }
} else {
    quality.UnavailableDeps = append(quality.UnavailableDeps, "statement_snapshots")
}
```

### 5.3 Statement Diff Types Needed

The `statements.ComputeDiff()` function already exists in `internal/statements/diff.go`. However, the RCA source needs access to structured regression and new-query data. Check if `DiffEntry` already provides `MeanExecTimeRatio`. If not, add a helper:

```go
// In internal/statements/diff.go or a new helper
type RegressionInfo struct {
    QueryID           int64
    MeanExecTimeOld   float64
    MeanExecTimeNew   float64
    MeanExecTimeRatio float64
    TotalTimePct      float64
}
```

The agent must grep `internal/statements/diff.go` to confirm the exact shape of `DiffEntry` and adapt accordingly.

---

## 6. W5 — Activate Tier B Chains

### 6.1 Remove Tier Filter

In `internal/rca/ontology.go` or `engine.go`, find where chains are filtered by tier:

```go
// BEFORE (M14_01):
if TierOf(chainID) == TierB {
    continue // Skip Tier B chains
}

// AFTER (M14_03):
// Remove the filter entirely. All 20 chains participate.
```

### 6.2 Chain-Specific Wiring

| Chain | ID | Tier B Reason | M14_03 Resolution |
|-------|----|---------------|-------------------|
| 3 | (identify from chains.go) | Unknown — agent must grep | Likely just needs filter removal if all MetricKeys are emitted by collectors |
| 12 | Query Regression | No statement diff source | Wire StatementDiffSource (W4) — detects `pg.statements.regression` anomalies |
| 13 | New Query Pattern | No statement diff source | Wire StatementDiffSource (W4) — detects `pg.statements.new_query` anomalies |
| 19 | Config Change → Behavioral Shift | No WhileEffective impl | Wire SettingsProvider (W3) — implements WhileEffective temporal mode |

The agent MUST grep `chains.go` for the exact chain IDs and node structure of chains 3, 12, 13, 19 before implementation.

---

## 7. W6 — Improved Summary Generation

### 7.1 Summary Builder Changes

In `internal/rca/incident.go`, the `IncidentBuilder` constructs the summary. Modify to include:

```go
func (b *IncidentBuilder) BuildSummary() string {
    var sb strings.Builder
    
    // 1. Primary chain narrative
    if b.primaryChain != nil {
        sb.WriteString(fmt.Sprintf("Primary chain: %s (confidence: %s, score: %.2f). ",
            b.primaryChain.ChainName,
            confidenceBucket(b.primaryChain.Score),
            b.primaryChain.Score))
    }
    
    // 2. Root cause with specific value
    if root := b.primaryChain.RootEvent(); root != nil {
        sb.WriteString(fmt.Sprintf("%s %s to %.2f at %s (baseline: %.2f, z-score: %.1f). ",
            root.NodeName,
            directionVerb(root.Value, root.BaselineVal), // "rose" or "dropped"
            root.Value,
            root.Timestamp.Format("15:04 UTC"),
            root.BaselineVal,
            root.ZScore))
    }
    
    // 3. Trigger metric context
    sb.WriteString(fmt.Sprintf("Triggered by %s = %.2f at %s. ",
        b.triggerMetric, b.triggerValue,
        b.triggerTime.Format("15:04 UTC")))
    
    // 4. Alternative chains
    if b.alternativeChain != nil {
        sb.WriteString(fmt.Sprintf("Alternative explanation: %s (confidence: %s, score: %.2f).",
            b.alternativeChain.ChainName,
            confidenceBucket(b.alternativeChain.Score),
            b.alternativeChain.Score))
    }
    
    return sb.String()
}

func directionVerb(current, baseline float64) string {
    if current > baseline {
        return "rose"
    }
    return "dropped"
}
```

### 7.2 Timeline Event Descriptions

In the event building step, ensure each `TimelineEvent.Description` includes:

```go
event.Description = fmt.Sprintf("%s %s to %.4g at %s (baseline: %.4g, z-score: %.1f)",
    event.MetricKey,
    directionVerb(event.Value, event.BaselineVal),
    event.Value,
    event.Timestamp.Format("15:04:05 UTC"),
    event.BaselineVal,
    event.ZScore)
```

---

## 8. W7 — Confidence Model Refinement

### 8.1 Temporal Proximity Weighting

In the edge evaluation, replace flat temporal scoring with a lag-window-aware curve:

```go
func temporalWeight(anomalyTime, triggerTime time.Time, minLag, maxLag time.Duration) float64 {
    elapsed := triggerTime.Sub(anomalyTime)
    if elapsed < 0 {
        elapsed = -elapsed // anomaly after trigger — reverse causal, small penalty
    }
    
    // Perfect window: anomaly arrived within [minLag, maxLag] before trigger
    if elapsed >= minLag && elapsed <= maxLag {
        return 1.0 // Full weight
    }
    
    // Outside window: exponential decay
    var distance time.Duration
    if elapsed < minLag {
        distance = minLag - elapsed
    } else {
        distance = elapsed - maxLag
    }
    
    // Decay constant: score halves every 2 minutes outside the window
    halfLife := 2 * time.Minute
    return math.Exp(-0.693 * float64(distance) / float64(halfLife))
}
```

### 8.2 Evidence Strength Multiplier

```go
func evidenceMultiplier(zScore float64) float64 {
    // Strong anomalies (z > 5) get a small boost, capped at 1.1×
    if zScore > 5.0 {
        return min(1.0 + (zScore-5.0)*0.02, 1.1)
    }
    // Weak anomalies (z < 2) get a penalty
    if zScore < 2.0 {
        return max(0.5 + zScore*0.25, 0.5)
    }
    return 1.0
}
```

### 8.3 Combined Edge Score

```go
edgeScore := edge.BaseConfidence *
    temporalWeight(anomaly.Timestamp, triggerTime, edge.MinLag, edge.MaxLag) *
    anomaly.Strength *
    evidenceMultiplier(anomaly.ZScore)
```

### 8.4 BaseConfidence Tuning

The agent should review and document the reasoning for each edge's BaseConfidence. General guidelines:

| Causal Link Type | Suggested Range | Reasoning |
|-----------------|-----------------|-----------|
| Near-deterministic (e.g., checkpoint storm → backend writes) | 0.85–0.95 | PostgreSQL internals guarantee this causal path |
| Strong correlation (e.g., lock contention → connection queueing) | 0.70–0.85 | High probability but other factors can contribute |
| Statistical (e.g., memory pressure → cache eviction) | 0.50–0.70 | OS-level causality is probabilistic |
| Circumstantial (e.g., new query → CPU spike) | 0.30–0.50 | Correlation, not proven causation |

---

## 9. W8 — RCA→Adviser Bridge

Full specification in `D506_recommendation_schema_evolution.md`. Implementation summary:

### 9.1 New Files

| File | Purpose |
|------|---------|
| `migrations/017_recommendation_rca_bridge.sql` | Schema migration (Section 3.2 of D506) |
| `internal/remediation/hooks.go` | `hookToRuleID` registry map |
| `internal/remediation/urgency.go` | `UrgencyFromPriority()`, constants |

### 9.2 Modified Files

| File | Change |
|------|--------|
| `internal/remediation/rule.go` | Add `Source`, `UrgencyScore`, `IncidentIDs`, `LastIncidentAt` to Recommendation struct |
| `internal/remediation/store.go` | Add `Upsert()`, `ListByIncident()` to interface; add `Source`, `OrderBy` to ListOpts |
| `internal/remediation/pgstore.go` | Implement `Upsert()` (INSERT ON CONFLICT), `ListByIncident()` (GIN query); update `Write()` to set urgency_score |
| `internal/remediation/nullstore.go` | Add no-op `Upsert()`, `ListByIncident()` |
| `internal/remediation/engine.go` | Add `EvaluateHook()` method |
| `internal/config/config.go` | Add `RCAUrgencyDelta`, `ForecastUrgencyDelta` to RemediationConfig |
| `internal/rca/engine.go` | Call `EvaluateHook()` after chain fires with RemediationHook |
| `internal/api/remediation.go` | Add `incident_id`, `source`, `order_by` query params to list endpoints |
| `web/src/types/remediation.ts` | Add new fields to Recommendation interface |
| `web/src/types/models.ts` | If Recommendation is defined here, update here instead |
| `web/src/hooks/useRecommendations.ts` | Add `useRecommendationsByIncident()` hook |
| `web/src/pages/RCAIncidentDetail.tsx` | Add inline recommendations section |
| `web/src/pages/Advisor.tsx` | Sort by urgency_score, add RCABadge |
| `web/src/components/advisor/AdvisorRow.tsx` | Render RCABadge when `incident_ids.length > 0` |

### 9.3 Upsert SQL (with soft cap)

```sql
INSERT INTO recommendations (
    instance_id, rule_id, title, description, priority, category,
    status, metric_key, metric_value, threshold, remediation,
    source, urgency_score, incident_ids, last_incident_at,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $8, $9, $10,
          $11, $12, ARRAY[$13::BIGINT], $14, NOW(), NOW())
ON CONFLICT (rule_id, instance_id) WHERE status = 'active'
DO UPDATE SET
    description    = EXCLUDED.description,
    metric_key     = EXCLUDED.metric_key,
    metric_value   = EXCLUDED.metric_value,
    threshold      = EXCLUDED.threshold,
    remediation    = EXCLUDED.remediation,
    urgency_score  = LEAST(recommendations.urgency_score + $15, 10.0),  -- Soft cap Q1
    incident_ids   = CASE
        WHEN $13::BIGINT = 0 THEN recommendations.incident_ids
        WHEN $13::BIGINT = ANY(recommendations.incident_ids) THEN recommendations.incident_ids
        ELSE array_append(recommendations.incident_ids, $13::BIGINT)
    END,
    last_incident_at = CASE
        WHEN $14 IS NOT NULL AND ($14 > COALESCE(recommendations.last_incident_at, '1970-01-01'::TIMESTAMPTZ))
        THEN $14
        ELSE recommendations.last_incident_at
    END,
    source = CASE
        WHEN EXCLUDED.source = 'rca' THEN 'rca'
        WHEN EXCLUDED.source = 'forecast' THEN 'forecast'
        WHEN recommendations.source = 'rca' THEN 'rca'
        ELSE EXCLUDED.source
    END,
    priority = CASE
        WHEN EXCLUDED.priority = 'critical' THEN 'critical'
        WHEN EXCLUDED.priority = 'warning' AND recommendations.priority != 'critical' THEN 'warning'
        ELSE recommendations.priority
    END,
    updated_at = NOW()
RETURNING *;
```

### 9.4 RCA Engine Integration Point

In `engine.go`, after building the incident:

```go
// After incident is stored
if e.remediationEngine != nil && incident.PrimaryChain != nil {
    for _, edge := range e.graph.EdgesForChain(incident.PrimaryChain.ChainID) {
        if edge.RemediationHook != "" {
            rec, err := e.remediationEngine.EvaluateHook(ctx,
                edge.RemediationHook,
                req.InstanceID,
                incident.ID,
                req.TriggerTime)
            if err != nil {
                e.logger.Warn("EvaluateHook failed",
                    "hook", edge.RemediationHook, "error", err)
            } else if rec != nil {
                e.logger.Info("RCA→Adviser bridge created recommendation",
                    "hook", edge.RemediationHook, "rec_id", rec.ID)
            }
        }
    }
}
```

---

## 10. W9 — JSON Tag Cleanup

### 10.1 CausalNode Tags

```go
type CausalNode struct {
    ID           string   `json:"id"`
    Name         string   `json:"name"`
    MetricKeys   []string `json:"metric_keys"`
    Layer        string   `json:"layer"`
    SymptomKey   string   `json:"symptom_key"`
    MechanismKey string   `json:"mechanism_key"`
}
```

### 10.2 CausalEdge Tags

```go
type CausalEdge struct {
    FromNode        string        `json:"from_node"`
    ToNode          string        `json:"to_node"`
    MinLag          time.Duration `json:"-"`            // Exclude raw duration
    MaxLag          time.Duration `json:"-"`            // Exclude raw duration
    MinLagSeconds   float64       `json:"min_lag_seconds"`  // Computed in MarshalJSON or handler
    MaxLagSeconds   float64       `json:"max_lag_seconds"`  // Computed in MarshalJSON or handler
    Temporal        string        `json:"temporal"`
    Evidence        string        `json:"evidence"`
    BaseConfidence  float64       `json:"base_confidence"`
    ChainID         string        `json:"chain_id"`
    RemediationHook string        `json:"remediation_hook,omitempty"`
}
```

**Implementation choice:** Rather than adding a custom `MarshalJSON`, add computed `MinLagSeconds`/`MaxLagSeconds` fields in the API handler when building the response. The graph response handler already wraps the raw `CausalGraph` — add the conversion there:

```go
// In internal/api/rca.go handleRCAGetGraph
type edgeResponse struct {
    FromNode        string  `json:"from_node"`
    ToNode          string  `json:"to_node"`
    MinLagSeconds   float64 `json:"min_lag_seconds"`
    MaxLagSeconds   float64 `json:"max_lag_seconds"`
    Temporal        string  `json:"temporal"`
    Evidence        string  `json:"evidence"`
    BaseConfidence  float64 `json:"base_confidence"`
    ChainID         string  `json:"chain_id"`
    RemediationHook string  `json:"remediation_hook,omitempty"`
}
```

### 10.3 Frontend Type Updates

```typescript
// web/src/types/rca.ts
export interface RCACausalNode {
  id: string;
  name: string;
  metric_keys: string[];
  layer: string;
  symptom_key: string;
  mechanism_key: string;
}

export interface RCACausalEdge {
  from_node: string;
  to_node: string;
  min_lag_seconds: number;
  max_lag_seconds: number;
  temporal: string;
  evidence: string;
  base_confidence: number;
  chain_id: string;
  remediation_hook?: string;
}
```

---

## 11. W10 — Review Instrumentation Stubs

### 11.1 API Endpoint

```go
// PUT /api/v1/rca/incidents/{id}/review
type ReviewRequest struct {
    Status string `json:"status"` // "confirmed", "false_positive", "inconclusive"
    Notes  string `json:"notes"`
}
```

Handler updates the `review_status` and `review_notes` columns (already in migration 016's `rca_incidents` table).

### 11.2 Store Method

Add to `IncidentStore` interface:

```go
UpdateReview(ctx context.Context, id int64, status, notes string) error
```

### 11.3 Frontend Widget

Small component on `RCAIncidentDetail.tsx`:

```typescript
// Three buttons: Confirmed | False Positive | Inconclusive
// Optional notes textarea (collapsed by default)
// Uses PUT /api/v1/rca/incidents/{id}/review
```

---

## 12. Agent Team Structure

### Agent 1: Backend (Go)

**Owns:** W1 (config), W2, W3, W4, W5, W7, W8 (backend only), W9 (Go structs + API handler), W10 (backend)

**Files created:**
- `internal/rca/settings.go` — SettingsProvider interface
- `internal/rca/settings_adapter.go` — SnapshotSettingsProvider
- `internal/rca/statement_source.go` — StatementDiffSource
- `internal/remediation/hooks.go` — hookToRuleID registry
- `internal/remediation/urgency.go` — urgency scoring
- `migrations/017_recommendation_rca_bridge.sql`

**Files modified:**
- `internal/rca/config.go` — new threshold config fields
- `internal/rca/anomaly.go` — 4h window + calm period
- `internal/rca/graph.go` — WhileEffective implementation + json tags
- `internal/rca/engine.go` — statement source integration, tier B filter removal, EvaluateHook call, summary improvements
- `internal/rca/incident.go` — improved BuildSummary()
- `internal/rca/chains.go` — BaseConfidence tuning (W7), verify MetricKeys (W1)
- `internal/rca/ontology.go` — remove Tier B classification if filter is there
- `internal/remediation/rule.go` — Recommendation struct new fields
- `internal/remediation/store.go` — Upsert, ListByIncident, updated ListOpts
- `internal/remediation/pgstore.go` — Upsert SQL, ListByIncident SQL, Write urgency init
- `internal/remediation/nullstore.go` — new method stubs
- `internal/remediation/engine.go` — EvaluateHook method
- `internal/config/config.go` — RCAConfig + RemediationConfig additions
- `internal/api/rca.go` — review endpoint, graph response with seconds conversion
- `internal/api/remediation.go` — incident_id/source/order_by query params

**Tests to add/update:**
- `internal/rca/anomaly_test.go` — calm period tests
- `internal/rca/engine_test.go` — statement source, WhileEffective, tier B
- `internal/rca/settings_adapter_test.go` — new
- `internal/rca/statement_source_test.go` — new
- `internal/remediation/pgstore_test.go` — Upsert, ListByIncident
- `internal/remediation/engine_test.go` — EvaluateHook

### Agent 2: Frontend + Summary + Demo Config

**Owns:** W1 (demo VM deployment), W6 (summary verification), W8 (frontend), W9 (TypeScript types), W10 (frontend widget)

**Files created:**
- `web/src/components/advisor/RCABadge.tsx`
- `web/src/hooks/useRecommendationsByIncident.ts`
- `web/src/components/rca/ReviewWidget.tsx`

**Files modified:**
- `web/src/types/rca.ts` — lowercase field names (W9)
- `web/src/types/remediation.ts` or `web/src/types/models.ts` — new Recommendation fields
- `web/src/components/rca/CausalGraphView.tsx` — updated field names
- `web/src/pages/RCAIncidentDetail.tsx` — inline recommendations, review widget
- `web/src/pages/Advisor.tsx` — urgency sort, source filter
- `web/src/components/advisor/AdvisorRow.tsx` — RCABadge rendering
- `web/src/hooks/useRecommendations.ts` — add incident query hook
- `web/src/hooks/useRCA.ts` — add review mutation

**Demo VM:**
- Update `/opt/pgpulse/pgpulse.yml` with comprehensive ML config (W1)
- Deploy updated binary
- Run chaos test to verify chains fire

---

## 13. Dependency Order

```
Phase 1 (parallel, no deps):
  Agent 1: W2 (threshold hardening)
  Agent 1: W3 (WhileEffective)
  Agent 1: W4 (statement source)
  Agent 1: W9 (json tags on Go structs)
  Agent 2: W9 (frontend type updates)

Phase 2 (depends on Phase 1):
  Agent 1: W5 (activate Tier B — needs W3, W4)
  Agent 1: W7 (confidence model — can start in Phase 1 but needs W5 for validation)
  Agent 1: W8 backend (migration, store, engine, API)
  Agent 2: W8 frontend (RCABadge, inline recs, urgency sort)

Phase 3 (depends on Phase 2):
  Agent 1: W6 (summary generation — needs chains to fire)
  Agent 2: W10 (review widget)
  Agent 1: W10 (review API endpoint)
  Agent 1: W1 config verification (after all backend changes)
  Agent 2: W1 demo VM deployment + chaos test
```

---

## 14. DO NOT RE-DISCUSS

All D400–D408 (M14_01/02) decisions remain locked. Additionally:

- D500: Tier ordering (W1–W10 as specified)
- D501: Comprehensive ML metric key mapping (not minimum-to-fire)
- D502: 2 agents
- D503: 4h baseline window + 15min calm period (both)
- D504: JSON tags in M14_03
- D505: Option (c) via Unified Upsert
- D506: BIGINT[] + urgency_score FLOAT8
- D507: EvaluateHook on existing remediation.Engine
- D508: Migration 017 with partial unique + GIN indexes
- Q1: Soft cap 10.0 via LEAST()
- Q2: Write() must initialize urgency_score — mandatory
- Q3: Defer recurrence lineage
