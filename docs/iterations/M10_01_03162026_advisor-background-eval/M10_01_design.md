# M10_01 — Design Document

**Iteration:** M10_01
**Milestone:** M10 — Advisor Auto-Population
**Date:** 2026-03-16
**Module:** advisor-background-eval

---

## Architecture Overview

```
┌──────────────────────────────────────────────────┐
│  Orchestrator (existing)                         │
│  ├── instanceRunner per instance                 │
│  │   ├── high/medium/low collectors              │
│  │   └── MetricStore.Write()                     │
│  └── BackgroundEvaluator (NEW)                   │
│      ├── ticker: every background_interval       │
│      ├── for each instance:                      │
│      │   ├── MetricSource.CurrentSnapshot()      │
│      │   ├── Engine.Diagnose(snapshot)            │
│      │   └── RecommendationStore.Write(results)  │
│      └── RecommendationStore.CleanOld(retention) │
└──────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────────────────────────────────────┐
│  remediation_recommendations table               │
│  (migration 013 — already exists)                │
│  + status column (active/resolved/acknowledged)  │
│  + evaluated_at column                           │
└──────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────────────────────────────────────┐
│  GET /api/v1/recommendations                     │
│  → Advisor page auto-refreshes                   │
│  → "Create Alert Rule" button per row            │
│  → Opens RuleFormModal pre-filled                │
│  → POST /api/v1/alerts/rules (existing)          │
└──────────────────────────────────────────────────┘
```

---

## A. Background Evaluation Worker

### New File: `internal/remediation/background.go`

```go
type BackgroundEvaluator struct {
    engine       *Engine
    store        RecommendationStore
    metricSource MetricSource
    instances    InstanceLister       // reuse from ml/ or define new
    interval     time.Duration
    retention    time.Duration
    logger       *slog.Logger
}

func NewBackgroundEvaluator(
    engine *Engine,
    store RecommendationStore,
    metricSource MetricSource,
    instances InstanceLister,
    interval time.Duration,
    retention time.Duration,
) *BackgroundEvaluator

func (b *BackgroundEvaluator) Start(ctx context.Context)
func (b *BackgroundEvaluator) Stop()
func (b *BackgroundEvaluator) runCycle(ctx context.Context)
```

### Evaluation Cycle Logic (`runCycle`)

1. Get list of active instance IDs from `InstanceLister`
2. For each instance:
   a. Build `MetricSnapshot` via `MetricSource.CurrentSnapshot(ctx, instanceID)`
   b. If snapshot is empty (instance unreachable or no data yet), skip with WARN log
   c. Call `Engine.Diagnose(ctx, instanceID, snapshot)`
   d. Mark previous active recommendations for this instance as `resolved` if not in current results
   e. Write new/updated recommendations via `RecommendationStore.Write()`
3. Run `RecommendationStore.CleanOld(ctx, time.Now().Add(-retention))` at end of cycle
4. Log summary: `"background evaluation complete" instances=3 recommendations=7 duration=1.2s`

### Instance Listing

The `ml/lister.go` already has an `InstanceLister` interface that returns instance IDs. Reuse this or define an equivalent in the remediation package. Check if `ml.InstanceLister` is importable without circular dependency — if not, define a simple interface:

```go
type InstanceLister interface {
    ListInstanceIDs(ctx context.Context) ([]string, error)
}
```

### Recommendation Status Lifecycle

```
┌─────────┐    next cycle finds it    ┌─────────┐
│  active  │◄──────────────────────────│  active  │
└────┬─────┘                           └──────────┘
     │ next cycle doesn't find it
     ▼
┌──────────┐    DBA clicks acknowledge
│ resolved │─────────────────────────► (stays resolved)
└──────────┘

┌─────────┐    DBA clicks acknowledge  ┌──────────────┐
│  active  │──────────────────────────►│ acknowledged  │
└─────────┘                            └──────────────┘
```

---

## B. Database Schema Changes

### Migration 014: Add status and evaluation tracking

Check if migration 013 (`remediation_recommendations`) already has `status` and timestamp columns. If not, add:

```sql
-- 014_remediation_status.sql
ALTER TABLE remediation_recommendations
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_remediation_status
    ON remediation_recommendations (instance_id, status);

CREATE INDEX IF NOT EXISTS idx_remediation_evaluated
    ON remediation_recommendations (evaluated_at);
```

**AGENT INSTRUCTION:** Read migration 013 first. If these columns already exist, skip migration 014. If the table schema already has `status`, `evaluated_at`, etc., just verify and move on.

---

## C. Config Changes

### File: `internal/config/config.go`

Add new struct:

```go
type RemediationConfig struct {
    Enabled            bool          `koanf:"enabled"`
    BackgroundInterval time.Duration `koanf:"background_interval"`
    RetentionDays      int           `koanf:"retention_days"`
}
```

Add to `Config` struct:

```go
type Config struct {
    // ... existing fields
    Remediation RemediationConfig `koanf:"remediation"`
}
```

Default values in loader:

```go
k.Load(confmap.Provider(map[string]interface{}{
    // ... existing defaults
    "remediation.enabled":             false,
    "remediation.background_interval": "5m",
    "remediation.retention_days":      30,
}, "."), nil)
```

---

## D. Wiring in main.go

```go
// After existing remediation engine setup:
if cfg.Remediation.Enabled && persistentStore != nil {
    bgEval := remediation.NewBackgroundEvaluator(
        remediationEngine,
        remediationStore,  // PGStore, not NullStore
        metricSource,
        instanceLister,    // from orchestrator or ML
        cfg.Remediation.BackgroundInterval,
        time.Duration(cfg.Remediation.RetentionDays) * 24 * time.Hour,
    )
    bgEval.Start(ctx)
    defer bgEval.Stop()
    slog.Info("remediation background evaluator started",
        "interval", cfg.Remediation.BackgroundInterval)
}
```

### Live Mode Guard

The key check is `persistentStore != nil`. In live mode (MemoryStore), `remediationStore` is a `NullStore`. The `BackgroundEvaluator` should NOT be created when using `NullStore` — the `cfg.Remediation.Enabled && persistentStore != nil` guard handles this.

---

## E. PGStore Updates

### Resolve Stale Recommendations

Add method to `RecommendationStore` interface and `PGStore`:

```go
// ResolveStale marks active recommendations as resolved if they weren't
// found in the latest evaluation cycle for the given instance.
ResolveStale(ctx context.Context, instanceID string, currentRuleIDs []string) error
```

Implementation: `UPDATE remediation_recommendations SET status = 'resolved', resolved_at = NOW() WHERE instance_id = $1 AND status = 'active' AND rule_id != ALL($2)`

### Upsert Logic

When writing recommendations from background eval, use upsert behavior:
- If an active recommendation already exists for this rule+instance, update `evaluated_at` timestamp
- If it was previously resolved and now fires again, set status back to `active`
- New recommendations get status `active`

### CleanOld Enhancement

Existing `CleanOld` removes by age. Keep this — it applies to all statuses. Resolved and acknowledged recommendations older than retention_days get deleted.

---

## F. API Changes

### Existing Endpoints (no changes needed)

- `GET /api/v1/recommendations` — already returns all recommendations
- `GET /api/v1/instances/{id}/recommendations` — already returns per-instance
- `PUT /api/v1/recommendations/{id}/acknowledge` — already exists
- `GET /api/v1/recommendations/rules` — already returns rule definitions

### Response Enhancement

Ensure the recommendation JSON response includes:
- `status` field (active / resolved / acknowledged)
- `evaluated_at` field (timestamp of last evaluation)
- `resolved_at` field (nullable timestamp)
- `metric_key` and `metric_value` fields (from M9_01)

**AGENT INSTRUCTION:** Check the existing `Recommendation` struct and API serialization. If these fields are already present, just verify. If `status`/`evaluated_at` are missing from the struct, add them.

### Query Parameter Enhancement

Add `status` filter to the list endpoints:

```
GET /api/v1/recommendations?status=active
GET /api/v1/recommendations?status=resolved
GET /api/v1/recommendations?status=acknowledged
```

The existing `ListOptions` struct may already support filtering — extend if needed.

---

## G. Frontend Changes

### G1: Advisor Page Auto-Refresh

In `web/src/pages/Advisor.tsx`:
- Add auto-refresh using React Query's `refetchInterval` (30s default)
- Show "Last evaluated: {timestamp}" from the most recent recommendation's `evaluated_at`
- Add status filter to AdvisorFilters (Active / Resolved / Acknowledged / All)

### G2: "Create Alert Rule" Button

In `web/src/components/advisor/AdvisorRow.tsx`:
- Add "Create Alert Rule" button (visible to dba+ role only)
- On click: open the existing `RuleFormModal` from `web/src/components/alerts/RuleFormModal.tsx`
- Pre-fill the modal with:
  - `metric`: recommendation's `metric_key`
  - `name`: `"Auto: " + recommendation.title`
  - `operator`: `">"` (default, DBA can change)
  - `threshold`: recommendation's `metric_value` (the value that triggered the rule)
  - `severity`: map priority → severity (`action_required` → `critical`, `suggestion` → `warning`, `info` → `info`)
- After successful creation, show toast: "Alert rule created"

### G3: Sidebar Badge

In `web/src/components/layout/Sidebar.tsx`:
- Show a count badge on the "Advisor" nav item
- Badge shows count of `active` recommendations
- Use a lightweight hook that polls the recommendations count (or piggyback on existing data)
- Badge hidden when count is 0

### G4: TypeScript Model Updates

In `web/src/types/models.ts`:
- Add `status`, `evaluated_at`, `resolved_at` fields to `Recommendation` type (if not already present)

### G5: Advisor Filters Enhancement

In `web/src/components/advisor/AdvisorFilters.tsx`:
- Add "Status" filter group: Active / Resolved / Acknowledged / All
- Wire to query parameter on the recommendations API call

---

## Agent Team Sizing

**2 agents:** Backend + Frontend

| Agent | Owns | Work |
|---|---|---|
| **Backend Agent** | `internal/remediation/`, `internal/config/`, `cmd/pgpulse-server/`, `migrations/` | A (background worker), B (migration), C (config), D (wiring), E (store updates), F (API filter) |
| **Frontend Agent** | `web/src/` | G1-G5 (auto-refresh, create alert rule button, sidebar badge, filters, types) |

Both agents write their own tests. Backend can start immediately. Frontend starts in parallel — the API shape doesn't change, just new fields on existing responses.

---

## Risk Register

| Risk | Mitigation |
|---|---|
| Background eval takes too long with many instances | Each instance runs sequentially with timeout; log duration; skip slow instances |
| Snapshot empty after restart | Skip instance with WARN, don't create "no issues" records |
| RecommendationStore.Write() conflicts with Diagnose-on-demand | Background eval and on-demand Diagnose use the same store; upsert logic prevents duplicates |
| RuleFormModal import from alerts/ creates coupling | Import is frontend-only (React component); acceptable coupling |
| Migration 014 on existing data | ALTER ADD COLUMN IF NOT EXISTS is safe; existing rows get defaults |
