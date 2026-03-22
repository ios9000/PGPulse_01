# D506 — Recommendation Schema Evolution: Unified Upsert for RCA→Adviser Bridge

**Decision:** D506 (part of M14_03 — Expansion, Calibration, and Knowledge Integration)
**Date:** 2026-03-22
**Status:** Proposed — pending lock

---

## 1. Design Rationale

The RCA engine (M14_01) identifies root causes and attaches remediation hooks to causal chains. The Adviser subsystem (M10) independently scans metrics and produces recommendations. Today these two systems are disconnected — an RCA incident that identifies "checkpoint_completion_target is too low" cannot surface that finding in the Adviser dashboard, and the Adviser has no awareness that its recommendation was confirmed by a real production incident.

The **Unified Upsert** pattern solves this by making the Adviser the single owner of recommendation state. The RCA engine never writes to the recommendation store directly. Instead, it calls `Engine.EvaluateHook()`, which either creates a new recommendation (tagged with the incident) or enriches an existing one (bumping urgency and linking the incident). This guarantees:

- **No duplicate recommendations** for the same issue on the same instance
- **Single source of truth** — one record per (rule, instance) pair
- **Two consumption paths** — firefighting DBA sees advice inline on the Incident Detail page; proactive engineer sees it in the global Adviser feed with an "incident-linked" badge
- **Future-proof** — M15 (Maintenance Forecasting) will use the same `EvaluateHook()` path with `source: "forecast"`

---

## 2. Current State (Pre-M14_03)

### 2.1 PostgreSQL Table: `recommendations`

From migration `009_remediation.sql`:

```sql
CREATE TABLE IF NOT EXISTS recommendations (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     TEXT        NOT NULL,
    rule_id         TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    description     TEXT        NOT NULL,
    priority        TEXT        NOT NULL DEFAULT 'info',
    category        TEXT        NOT NULL DEFAULT 'general',
    status          TEXT        NOT NULL DEFAULT 'active',
    metric_key      TEXT        NOT NULL DEFAULT '',
    metric_value    FLOAT8      NOT NULL DEFAULT 0,
    threshold       FLOAT8      NOT NULL DEFAULT 0,
    remediation     TEXT        NOT NULL DEFAULT '',
    alert_event_id  BIGINT      REFERENCES alert_events(id) ON DELETE SET NULL,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_recommendations_instance ON recommendations(instance_id);
CREATE INDEX idx_recommendations_status ON recommendations(status);
CREATE INDEX idx_recommendations_alert ON recommendations(alert_event_id);
```

### 2.2 Go Struct: `Recommendation`

From `internal/remediation/rule.go`:

```go
type Recommendation struct {
    ID             int64      `json:"id"`
    InstanceID     string     `json:"instance_id"`
    RuleID         string     `json:"rule_id"`
    Title          string     `json:"title"`
    Description    string     `json:"description"`
    Priority       string     `json:"priority"`
    Category       string     `json:"category"`
    Status         string     `json:"status"`
    MetricKey      string     `json:"metric_key"`
    MetricValue    float64    `json:"metric_value"`
    Threshold      float64    `json:"threshold"`
    Remediation    string     `json:"remediation"`
    AlertEventID   *int64     `json:"alert_event_id,omitempty"`
    AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
    AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`
}
```

### 2.3 Store Interface: `RecommendationStore`

From `internal/remediation/store.go`:

```go
type RecommendationStore interface {
    Write(ctx context.Context, recs []Recommendation) error
    ListByInstance(ctx context.Context, instanceID string, opts ListOpts) ([]Recommendation, error)
    ListAll(ctx context.Context, opts ListOpts) ([]Recommendation, error)
    ListByAlertEvent(ctx context.Context, alertEventID int64) ([]Recommendation, error)
    Acknowledge(ctx context.Context, id int64, username string) error
    CleanOld(ctx context.Context, olderThan time.Duration) (int64, error)
    ResolveStale(ctx context.Context, validRuleIDs []string) (int64, error)
}
```

### 2.4 Frontend Type: `Recommendation`

From `web/src/types/remediation.ts`:

```typescript
export interface Recommendation {
  id: number;
  instance_id: string;
  rule_id: string;
  title: string;
  description: string;
  priority: string;       // "critical" | "warning" | "info"
  category: string;
  status: string;         // "active" | "acknowledged" | "resolved"
  metric_key: string;
  metric_value: number;
  threshold: number;
  remediation: string;
  alert_event_id?: number;
  acknowledged_at?: string;
  acknowledged_by?: string;
  created_at: string;
  updated_at: string;
}
```

---

## 3. Schema Changes (M14_03)

### 3.1 New Columns

| Column | PG Type | Go Type | Default | Purpose |
|--------|---------|---------|---------|---------|
| `source` | `TEXT NOT NULL` | `string` | `'background'` | Origin subsystem: `"background"`, `"rca"`, `"alert"`, `"forecast"` (future) |
| `urgency_score` | `FLOAT8 NOT NULL` | `float64` | `0.0` | Continuous ranking signal. Background scan sets from rule priority; RCA upsert bumps by configurable delta. Used for sorting the Adviser feed. |
| `incident_ids` | `BIGINT[] NOT NULL` | `[]int64` | `'{}'` | Array of linked RCA incident IDs. Empty for background-only recommendations. Appended on each RCA upsert. |
| `last_incident_at` | `TIMESTAMPTZ` | `*time.Time` | `NULL` | Timestamp of the most recently linked incident. Enables "Caused an incident on Friday" display without joining the incidents table. |

### 3.2 Migration: `017_recommendation_rca_bridge.sql`

```sql
-- M14_03: Add RCA→Adviser bridge columns to recommendations table

-- Source subsystem that created or last enriched this recommendation
ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'background';

-- Continuous urgency score for ranking in the Adviser feed
-- Background scan: mapped from priority (critical=3.0, warning=2.0, info=1.0)
-- RCA upsert: bumps by +1.0 per incident linkage (configurable)
ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS urgency_score FLOAT8 NOT NULL DEFAULT 0.0;

-- Array of linked RCA incident IDs (native PG array — simpler queries than JSONB)
ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS incident_ids BIGINT[] NOT NULL DEFAULT '{}';

-- Timestamp of the most recently linked incident (denormalized for display)
ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS last_incident_at TIMESTAMPTZ;

-- Index for querying recommendations linked to a specific incident
-- Uses GIN because incident_ids is an array and we need @> (contains) queries
CREATE INDEX IF NOT EXISTS idx_recommendations_incident_ids
    ON recommendations USING GIN (incident_ids);

-- Index for sorting by urgency in the Adviser feed
CREATE INDEX IF NOT EXISTS idx_recommendations_urgency
    ON recommendations (urgency_score DESC)
    WHERE status = 'active';

-- Partial index for source-filtered queries (e.g., "show me all RCA-driven advice")
CREATE INDEX IF NOT EXISTS idx_recommendations_source
    ON recommendations (source)
    WHERE status = 'active';

-- Backfill: set urgency_score for existing recommendations based on priority
UPDATE recommendations SET urgency_score = CASE priority
    WHEN 'critical' THEN 3.0
    WHEN 'warning'  THEN 2.0
    WHEN 'info'     THEN 1.0
    ELSE 0.5
END WHERE urgency_score = 0.0;
```

### 3.3 Updated Go Struct: `Recommendation`

```go
type Recommendation struct {
    // --- Existing fields (unchanged) ---
    ID             int64      `json:"id"`
    InstanceID     string     `json:"instance_id"`
    RuleID         string     `json:"rule_id"`
    Title          string     `json:"title"`
    Description    string     `json:"description"`
    Priority       string     `json:"priority"`
    Category       string     `json:"category"`
    Status         string     `json:"status"`
    MetricKey      string     `json:"metric_key"`
    MetricValue    float64    `json:"metric_value"`
    Threshold      float64    `json:"threshold"`
    Remediation    string     `json:"remediation"`
    AlertEventID   *int64     `json:"alert_event_id,omitempty"`
    AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
    AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`

    // --- New fields (M14_03) ---
    Source         string     `json:"source"`                     // "background", "rca", "alert", "forecast"
    UrgencyScore   float64    `json:"urgency_score"`              // Continuous ranking signal
    IncidentIDs    []int64    `json:"incident_ids"`               // Linked RCA incident IDs
    LastIncidentAt *time.Time `json:"last_incident_at,omitempty"` // Most recent incident timestamp
}
```

**pgx scanning note:** The `IncidentIDs` field maps to a `BIGINT[]` column. pgx/v5 natively scans PostgreSQL arrays into Go slices — no custom scanner needed. Use `pgtype.FlatArray[int64]` if issues arise, but `[]int64` should work directly with pgx/v5.

### 3.4 Updated Frontend Type: `Recommendation`

```typescript
export interface Recommendation {
  // --- Existing fields (unchanged) ---
  id: number;
  instance_id: string;
  rule_id: string;
  title: string;
  description: string;
  priority: string;
  category: string;
  status: string;
  metric_key: string;
  metric_value: number;
  threshold: number;
  remediation: string;
  alert_event_id?: number;
  acknowledged_at?: string;
  acknowledged_by?: string;
  created_at: string;
  updated_at: string;

  // --- New fields (M14_03) ---
  source: RecommendationSource;     // Origin subsystem
  urgency_score: number;            // Continuous ranking signal
  incident_ids: number[];           // Linked RCA incident IDs (empty array if none)
  last_incident_at?: string;        // ISO timestamp of most recent incident
}

export type RecommendationSource = 'background' | 'rca' | 'alert' | 'forecast';
```

---

## 4. Store Interface Changes

### 4.1 New Method: `Upsert`

```go
type RecommendationStore interface {
    // --- Existing methods (unchanged) ---
    Write(ctx context.Context, recs []Recommendation) error
    ListByInstance(ctx context.Context, instanceID string, opts ListOpts) ([]Recommendation, error)
    ListAll(ctx context.Context, opts ListOpts) ([]Recommendation, error)
    ListByAlertEvent(ctx context.Context, alertEventID int64) ([]Recommendation, error)
    Acknowledge(ctx context.Context, id int64, username string) error
    CleanOld(ctx context.Context, olderThan time.Duration) (int64, error)
    ResolveStale(ctx context.Context, validRuleIDs []string) (int64, error)

    // --- New methods (M14_03) ---

    // Upsert creates or enriches a recommendation for (rule_id, instance_id).
    // If an active recommendation exists: appends incident_id, bumps urgency, updates source.
    // If none exists: creates a new recommendation with the given fields.
    // Returns the upserted recommendation (with ID populated).
    Upsert(ctx context.Context, rec Recommendation) (*Recommendation, error)

    // ListByIncident returns all active recommendations linked to a specific RCA incident.
    // Used by the Incident Detail page: GET /api/v1/recommendations?incident_id=123
    ListByIncident(ctx context.Context, incidentID int64) ([]Recommendation, error)
}
```

### 4.2 Updated `ListOpts`

```go
type ListOpts struct {
    Status   string   // Filter by status ("active", "acknowledged", "resolved", "")
    Category string   // Filter by category ("", "connections", "storage", ...)
    Source   string   // NEW: Filter by source ("", "background", "rca", "alert", "forecast")
    OrderBy  string   // NEW: Sort field ("urgency_score", "created_at", "updated_at")
}
```

### 4.3 SQL Implementation: `Upsert`

```sql
-- The upsert query (executed by PGRecommendationStore.Upsert)
-- Uses INSERT ... ON CONFLICT with a unique constraint on (rule_id, instance_id, status)

-- Step 0: Require a partial unique index (added in migration 017)
CREATE UNIQUE INDEX IF NOT EXISTS idx_recommendations_rule_instance_active
    ON recommendations (rule_id, instance_id)
    WHERE status = 'active';

-- Step 1: The upsert
INSERT INTO recommendations (
    instance_id, rule_id, title, description, priority, category,
    status, metric_key, metric_value, threshold, remediation,
    source, urgency_score, incident_ids, last_incident_at,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    'active', $7, $8, $9, $10,
    $11, $12, ARRAY[$13::BIGINT], $14,
    NOW(), NOW()
)
ON CONFLICT (rule_id, instance_id) WHERE status = 'active'
DO UPDATE SET
    -- Always update description/metric values to reflect latest state
    description    = EXCLUDED.description,
    metric_key     = EXCLUDED.metric_key,
    metric_value   = EXCLUDED.metric_value,
    threshold      = EXCLUDED.threshold,
    remediation    = EXCLUDED.remediation,
    -- Bump urgency (additive, not replacement)
    urgency_score  = recommendations.urgency_score + $15,
    -- Append incident ID if not already present
    incident_ids   = CASE
        WHEN $13::BIGINT = 0 THEN recommendations.incident_ids
        WHEN $13::BIGINT = ANY(recommendations.incident_ids) THEN recommendations.incident_ids
        ELSE array_append(recommendations.incident_ids, $13::BIGINT)
    END,
    -- Update last_incident_at if this incident is newer
    last_incident_at = CASE
        WHEN $14 IS NOT NULL AND ($14 > COALESCE(recommendations.last_incident_at, '1970-01-01'::TIMESTAMPTZ))
        THEN $14
        ELSE recommendations.last_incident_at
    END,
    -- Escalate source: rca > alert > background (rca always wins)
    source = CASE
        WHEN EXCLUDED.source = 'rca' THEN 'rca'
        WHEN EXCLUDED.source = 'forecast' THEN 'forecast'
        WHEN recommendations.source = 'rca' THEN 'rca'
        ELSE EXCLUDED.source
    END,
    -- Escalate priority if the new recommendation has higher severity
    priority = CASE
        WHEN EXCLUDED.priority = 'critical' THEN 'critical'
        WHEN EXCLUDED.priority = 'warning' AND recommendations.priority != 'critical' THEN 'warning'
        ELSE recommendations.priority
    END,
    updated_at = NOW()
RETURNING *;
```

### 4.4 SQL Implementation: `ListByIncident`

```sql
-- Used by Incident Detail page
SELECT * FROM recommendations
WHERE $1 = ANY(incident_ids)
  AND status = 'active'
ORDER BY urgency_score DESC;
```

This leverages the GIN index on `incident_ids` for efficient array containment queries.

---

## 5. Engine Interface: `EvaluateHook`

### 5.1 Method Signature

```go
// On internal/remediation/engine.go (existing Engine type)

// EvaluateHook is called by the RCA engine when a fired causal chain
// has a RemediationHook. It evaluates the corresponding adviser rule
// and upserts the recommendation, linking it to the incident.
//
// Flow:
//   1. Look up the remediation rule by hookID
//   2. If no matching rule exists, log and return nil (not all hooks have rules yet)
//   3. Evaluate the rule against current metrics (same as background scan)
//   4. If the rule condition is met, call store.Upsert() with source="rca"
//   5. If the rule condition is NOT met, still create/upsert with a note
//      that RCA identified this as a contributing factor even though
//      the metric has since recovered
//
// Parameters:
//   - hookID: the ontology hook constant (e.g., HookCheckpointTuning)
//   - instanceID: the affected instance
//   - incidentID: the RCA incident that identified this root cause
//   - incidentTime: when the incident's trigger fired (for last_incident_at)
//
// Returns the upserted Recommendation, or nil if no matching rule exists.
func (e *Engine) EvaluateHook(
    ctx context.Context,
    hookID string,
    instanceID string,
    incidentID int64,
    incidentTime time.Time,
) (*Recommendation, error)
```

### 5.2 Hook-to-Rule Mapping

The mapping from RCA ontology hook constants to remediation rule IDs is maintained in a registry within the remediation engine. This is a static map — no database lookup needed.

```go
// internal/remediation/hooks.go (new file)

// hookToRuleID maps ontology Hook* constants to remediation rule IDs.
// Not every hook has a corresponding rule — some are Tier B stubs.
var hookToRuleID = map[string]string{
    // From internal/rca/ontology.go Hook* constants
    "remediation.checkpoint_completion_target": "pg_checkpoint_completion",
    "remediation.shared_buffers":              "pg_shared_buffers",
    "remediation.work_mem":                    "pg_work_mem",
    "remediation.max_connections":             "pg_connection_saturation",
    "remediation.autovacuum_tuning":           "pg_vacuum_health",
    "remediation.wal_size":                    "pg_wal_config",
    "remediation.lock_timeout":                "pg_lock_contention",
    "remediation.statement_timeout":           "pg_long_transactions",
    "remediation.index_recommendation":        "pg_missing_index",
    "remediation.disk_capacity":               "os_disk_usage",
    "remediation.memory_pressure":             "os_memory_pressure",
    // Add more as chains are activated
}
```

### 5.3 Urgency Score Calculation

```go
// internal/remediation/urgency.go (new file)

const (
    // Base urgency scores mapped from priority levels
    UrgencyBaseCritical = 3.0
    UrgencyBaseWarning  = 2.0
    UrgencyBaseInfo     = 1.0

    // Delta added per RCA incident linkage
    // Configurable in remediation config; this is the default
    UrgencyDeltaRCAIncident = 1.0

    // Future: delta for forecast-driven recommendations
    UrgencyDeltaForecast = 0.5
)

// UrgencyFromPriority returns the base urgency score for a given priority level.
func UrgencyFromPriority(priority string) float64 {
    switch priority {
    case "critical":
        return UrgencyBaseCritical
    case "warning":
        return UrgencyBaseWarning
    case "info":
        return UrgencyBaseInfo
    default:
        return 0.5
    }
}
```

---

## 6. API Changes

### 6.1 Modified Endpoint: `GET /api/v1/recommendations`

Add query parameter support:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `instance_id` | string | (all) | Filter by instance |
| `incident_id` | int64 | (none) | Filter by linked RCA incident ID |
| `source` | string | (all) | Filter by source: `background`, `rca`, `alert`, `forecast` |
| `order_by` | string | `urgency_score` | Sort field: `urgency_score`, `created_at`, `updated_at` |
| `status` | string | `active` | Status filter |

The `incident_id` parameter is the key addition — the Incident Detail page uses this to render inline recommendations:

```
GET /api/v1/recommendations?incident_id=123
```

### 6.2 Response Shape (unchanged envelope, new fields in items)

```json
{
  "recommendations": [
    {
      "id": 42,
      "instance_id": "primary",
      "rule_id": "pg_checkpoint_completion",
      "title": "Checkpoint completion target too low",
      "description": "checkpoint_completion_target is 0.5, causing I/O spikes...",
      "priority": "warning",
      "category": "storage",
      "status": "active",
      "metric_key": "pg.checkpoint.write_pct",
      "metric_value": 0.95,
      "threshold": 0.8,
      "remediation": "SET checkpoint_completion_target = 0.9;",
      "source": "rca",
      "urgency_score": 3.0,
      "incident_ids": [7, 12],
      "last_incident_at": "2026-03-21T14:23:00Z",
      "created_at": "2026-03-20T10:00:00Z",
      "updated_at": "2026-03-21T14:23:00Z"
    }
  ]
}
```

---

## 7. Frontend Changes

### 7.1 New Component: `RCABadge`

Displayed next to recommendations that have `incident_ids.length > 0`:

```
🔗 Linked to 2 incidents (latest: Mar 21)
```

Clicking navigates to the most recent incident detail page.

### 7.2 Incident Detail Page: Inline Recommendations

Below the timeline section, add a "Recommended Actions" panel that queries:

```
GET /api/v1/recommendations?incident_id={incidentId}
```

Renders the same `RecommendationCard` component used in the Adviser Dashboard, ensuring visual consistency. If no recommendations are linked, show a subtle "No automated remediation available for this root cause" message.

### 7.3 Adviser Dashboard: Sort by Urgency

Default sort order changes from `created_at DESC` to `urgency_score DESC, updated_at DESC`. RCA-linked recommendations naturally float to the top because their urgency score has been bumped.

### 7.4 New React Query Hook

```typescript
// web/src/hooks/useRecommendationsByIncident.ts
export function useRecommendationsByIncident(incidentId: number) {
  return useQuery({
    queryKey: ['recommendations', 'incident', incidentId],
    queryFn: () => api.get(`/recommendations?incident_id=${incidentId}`),
    enabled: incidentId > 0,
  });
}
```

---

## 8. Data Flow Diagram

```
┌──────────────────────┐
│   RCA Engine          │
│   Analyze() completes │
│   Chain fires with    │
│   RemediationHook     │
└──────────┬───────────┘
           │
           │ EvaluateHook(hookID, instanceID, incidentID, incidentTime)
           ▼
┌──────────────────────┐
│  Remediation Engine   │
│                       │
│  1. hookToRuleID map  │──── hook not found? → log warning, return nil
│  2. Evaluate rule     │
│  3. Build Recommendation
│     source = "rca"    │
│     urgency = base    │
│       + RCA delta     │
│     incident_ids =    │
│       [incidentID]    │
│                       │
│  4. Store.Upsert()    │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────────────────────────────────────┐
│  PGRecommendationStore.Upsert()                       │
│                                                       │
│  INSERT ... ON CONFLICT (rule_id, instance_id)        │
│    WHERE status = 'active'                            │
│  DO UPDATE SET                                        │
│    urgency_score += delta,                            │
│    incident_ids = array_append(incident_ids, $id),    │
│    last_incident_at = GREATEST(existing, new),        │
│    source = escalate(existing, new)                   │
│                                                       │
│  RETURNING *                                          │
└──────────────────────────────────────────────────────┘
           │
           │ Single normalized record
           ▼
┌──────────────────────────────────────────────────────┐
│  Two UI consumption paths                             │
│                                                       │
│  Path A (Reactive / Firefighting):                    │
│    Incident Detail page                               │
│    GET /api/v1/recommendations?incident_id=123        │
│    → Renders RecommendationCard inline                │
│                                                       │
│  Path B (Proactive / Monday Morning):                 │
│    Global Adviser Dashboard                           │
│    GET /api/v1/recommendations?order_by=urgency_score │
│    → RCA-linked items float to top                    │
│    → RCABadge shows "Linked to 2 incidents"           │
└──────────────────────────────────────────────────────┘
```

---

## 9. NullStore Implementation

`NullRecommendationStore` (used in live/no-storage mode) needs the two new methods:

```go
func (n *NullRecommendationStore) Upsert(ctx context.Context, rec Recommendation) (*Recommendation, error) {
    rec.ID = 0
    return &rec, nil
}

func (n *NullRecommendationStore) ListByIncident(ctx context.Context, incidentID int64) ([]Recommendation, error) {
    return nil, nil
}
```

---

## 10. Configuration Additions

```yaml
# In pgpulse.yml under remediation:
remediation:
  enabled: true
  background_interval: 5m
  retention_days: 30
  # NEW: RCA bridge configuration
  rca_urgency_delta: 1.0      # Urgency bump per incident linkage (default: 1.0)
  forecast_urgency_delta: 0.5  # Future: urgency bump for forecast-driven advice
```

Go config struct addition:

```go
// In internal/config/config.go, RemediationConfig struct
type RemediationConfig struct {
    Enabled            bool          `yaml:"enabled"`
    BackgroundInterval time.Duration `yaml:"background_interval"`
    RetentionDays      int           `yaml:"retention_days"`
    // NEW
    RCAUrgencyDelta      float64 `yaml:"rca_urgency_delta"`      // Default: 1.0
    ForecastUrgencyDelta float64 `yaml:"forecast_urgency_delta"` // Default: 0.5
}
```

---

## 11. Migration Safety

**Backward compatibility:** All new columns have defaults. The `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` pattern ensures idempotency. Existing recommendations get `source='background'`, `urgency_score` backfilled from priority, empty `incident_ids`, and NULL `last_incident_at`.

**Partial unique index:** The `idx_recommendations_rule_instance_active` index is critical for the upsert. It only covers `status = 'active'` rows, so acknowledged/resolved recommendations don't block new ones for the same (rule, instance) pair. This means if a DBA acknowledges a recommendation and the same issue recurs, a new active recommendation is created — which is the correct behavior.

**GIN index on `incident_ids`:** Supports the `$1 = ANY(incident_ids)` query pattern efficiently. GIN is the standard choice for array containment queries in PostgreSQL.

---

## 12. Testing Strategy

| Test | Scope | Validates |
|------|-------|-----------|
| `TestUpsert_NewRecommendation` | pgstore | INSERT path: creates record with source, urgency, incident_ids |
| `TestUpsert_ExistingRecommendation` | pgstore | UPDATE path: appends incident_id, bumps urgency, escalates source |
| `TestUpsert_DuplicateIncidentID` | pgstore | Idempotency: same incident_id not appended twice |
| `TestUpsert_PriorityEscalation` | pgstore | warning → critical escalation when RCA identifies critical path |
| `TestListByIncident` | pgstore | Returns only recommendations linked to given incident |
| `TestListByIncident_Empty` | pgstore | Returns empty slice when no recommendations linked |
| `TestEvaluateHook_MatchingRule` | engine | Full flow: hook → rule lookup → evaluate → upsert |
| `TestEvaluateHook_NoMatchingRule` | engine | Returns nil when hook has no corresponding rule |
| `TestEvaluateHook_ExistingRecommendation` | engine | Upsert enrichment: urgency bump + incident append |
| `TestUrgencyFromPriority` | unit | Correct base scores for each priority level |
| `TestListAll_OrderByUrgency` | pgstore | Default ordering puts highest urgency first |
| `TestListAll_FilterBySource` | pgstore | Source filter returns only matching records |
| `TestNullStore_Upsert` | nullstore | No-op returns populated struct |
| `TestNullStore_ListByIncident` | nullstore | No-op returns empty slice |

---

## 13. Open Questions (For Discussion)

**Q1: Urgency score cap?** Should we cap `urgency_score` at a maximum (e.g., 10.0) to prevent runaway escalation if a flapping alert triggers many incidents? Current design has no cap — relies on the 15-min cooldown in AutoTrigger to rate-limit. Recommendation: add a soft cap of 10.0 in the upsert SQL.

**Q2: Should `Write()` also set urgency_score?** The existing `Write()` method (used by background scan) currently doesn't set urgency. Should we update it to call `UrgencyFromPriority()` on write, or handle it only in the backfill migration? Recommendation: update `Write()` to set `urgency_score = UrgencyFromPriority(priority)` for all new writes, ensuring the field is always populated going forward.

**Q3: Resolved recommendation re-creation.** If a recommendation was resolved (DBA fixed the issue), then the same issue recurs and RCA detects it — the partial unique index allows a new `active` record. Should the new record inherit any context from the old one (e.g., "Previously resolved on DATE")? Recommendation: defer to M15 — keep it simple for now.
