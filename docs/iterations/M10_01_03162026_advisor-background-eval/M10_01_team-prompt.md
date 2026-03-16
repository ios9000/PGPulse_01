# M10_01 — Advisor Auto-Population (Background Eval + Create Alert Rule)

Read `docs/iterations/M10_01_03162026_advisor-background-eval/design.md` for full technical design.
Read `docs/CODEBASE_DIGEST.md` for current codebase state.

Create a team of 2 specialists:

## BACKEND AGENT

### Task A: Background Evaluation Worker

1. Read the existing remediation package:
   - `internal/remediation/engine.go` — `Diagnose()` method
   - `internal/remediation/pgstore.go` — `RecommendationStore` implementation
   - `internal/remediation/nullstore.go` — NullStore for live mode
   - `internal/remediation/metricsource.go` — `StoreMetricSource`
   - `internal/remediation/store.go` — `RecommendationStore` interface

2. Create `internal/remediation/background.go`:
   - `BackgroundEvaluator` struct with engine, store, metricSource, interval, retention
   - `Start(ctx context.Context)` — starts a goroutine with ticker
   - `Stop()` — cancels the goroutine
   - `runCycle(ctx context.Context)`:
     a. Get list of active instance IDs (need an InstanceLister — check if `internal/ml/lister.go` has one, or get instance IDs from the orchestrator/store)
     b. For each instance: build snapshot → call Diagnose → resolve stale → write new results
     c. Run CleanOld at end of cycle
     d. Log summary with instance count, recommendation count, duration

3. Create `internal/remediation/background_test.go`:
   - Test that runCycle calls Diagnose for each instance
   - Test that stale recommendations get resolved
   - Test that retention cleanup runs

### Task B: Database Schema

1. Read `migrations/013_remediation_recommendations.sql` — check what columns already exist
2. If `status`, `evaluated_at`, `resolved_at` columns are missing, create `migrations/014_remediation_status.sql`:
   ```sql
   ALTER TABLE remediation_recommendations
       ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active',
       ADD COLUMN IF NOT EXISTS evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
       ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;
   CREATE INDEX IF NOT EXISTS idx_remediation_status ON remediation_recommendations (instance_id, status);
   CREATE INDEX IF NOT EXISTS idx_remediation_evaluated ON remediation_recommendations (evaluated_at);
   ```
3. If columns already exist, skip migration — do NOT create duplicate.

### Task C: Config

1. Read `internal/config/config.go` — add `RemediationConfig` struct:
   ```go
   type RemediationConfig struct {
       Enabled            bool          `koanf:"enabled"`
       BackgroundInterval time.Duration `koanf:"background_interval"`
       RetentionDays      int           `koanf:"retention_days"`
   }
   ```
2. Add `Remediation RemediationConfig` field to `Config` struct
3. Add defaults in `internal/config/load.go`:
   - `remediation.enabled`: false
   - `remediation.background_interval`: "5m"
   - `remediation.retention_days`: 30

### Task D: Wiring in main.go

1. Read `cmd/pgpulse-server/main.go` — find where remediation engine is created
2. After existing remediation setup, add:
   ```go
   if cfg.Remediation.Enabled && persistentStore != nil {
       // Create BackgroundEvaluator
       // Start it
       // Defer Stop()
       // Log "remediation background evaluator started"
   }
   ```
3. The guard `persistentStore != nil` ensures it NEVER runs in live/MemoryStore mode
4. Find or create an InstanceLister that provides active instance IDs. Check:
   - Does the orchestrator expose a method to list instance IDs?
   - Does `internal/ml/lister.go` define an interface?
   - If neither works, use `internal/storage/instances.go` InstanceStore.List()

### Task E: PGStore Updates

1. Read `internal/remediation/pgstore.go` and `internal/remediation/store.go`
2. Add `ResolveStale` method to `RecommendationStore` interface:
   ```go
   ResolveStale(ctx context.Context, instanceID string, currentRuleIDs []string) error
   ```
3. Implement in PGStore:
   - `UPDATE remediation_recommendations SET status = 'resolved', resolved_at = NOW() WHERE instance_id = $1 AND status = 'active' AND rule_id != ALL($2::text[])`
4. Implement in NullStore (no-op, return nil)
5. Update `Write()` to use upsert logic:
   - If active recommendation exists for same rule+instance, update `evaluated_at`
   - If resolved recommendation exists and fires again, set status back to `active`
   - New recommendations get status `active`
6. Add `status` filter to `ListAll()` and `ListByInstance()`:
   - Check the existing `ListOptions` struct — add `Status string` field if missing
   - Apply `WHERE status = $N` when status filter is provided

### Task F: API Response Enhancement

1. Read `internal/api/remediation.go` — check how recommendations are serialized
2. Ensure `status`, `evaluated_at`, `resolved_at`, `metric_key`, `metric_value` fields are in the JSON response
3. Add `status` query parameter to `handleListAllRecommendations` and `handleListRecommendations`:
   - Read `?status=active` from query string
   - Pass to store's list method via ListOptions

### Build Verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/... ./internal/...
go test ./cmd/... ./internal/... -count=1
golangci-lint run ./cmd/... ./internal/...
```

---

## FRONTEND AGENT

### Task G1: Advisor Page Auto-Refresh

1. Read `web/src/pages/Advisor.tsx` and `web/src/hooks/useRecommendations.ts`
2. Add `refetchInterval: 30000` (30s) to the recommendations query hook
3. Show "Last evaluated: {timestamp}" using the most recent `evaluated_at` from results
4. Format timestamp as relative time ("2 minutes ago")

### Task G2: "Create Alert Rule" Button

1. Read `web/src/components/advisor/AdvisorRow.tsx` — add a "Create Alert Rule" button
2. Read `web/src/components/alerts/RuleFormModal.tsx` — understand its props for pre-filling
3. Import `RuleFormModal` into AdvisorRow (or Advisor page)
4. On button click, open RuleFormModal with:
   - `metric`: recommendation's `metric_key`
   - `name`: `"Auto: " + recommendation.title`
   - `operator`: `">"`
   - `threshold`: recommendation's `metric_value`
   - `severity`: map priority → severity:
     - `action_required` → `critical`
     - `suggestion` → `warning`
     - `info` → `info`
5. Use `PermissionGate` or `useAuth().can('alert_management')` to hide button for non-dba users
6. After successful creation, show toast via `toastStore`
7. Read `web/src/hooks/useAlertRules.ts` for the create mutation hook

### Task G3: Sidebar Badge

1. Read `web/src/components/layout/Sidebar.tsx`
2. Add a recommendation count badge on the "Advisor" nav item
3. Use the recommendations hook to get count of `status=active` recommendations
4. Show badge as a small number pill (e.g., red circle with white text)
5. Hide badge when count is 0
6. Keep the badge lightweight — only fetch count, not full recommendation list
   - Option: add a `GET /api/v1/recommendations/count?status=active` endpoint, OR
   - Option: piggyback on existing data if Advisor page is loaded, OR
   - Option: use the list endpoint with `limit=0` and read the total from response headers/metadata

### Task G4: TypeScript Model Updates

1. Read `web/src/types/models.ts` — find `Recommendation` type
2. Add fields if missing:
   - `status: 'active' | 'resolved' | 'acknowledged'`
   - `evaluated_at: string` (ISO timestamp)
   - `resolved_at: string | null`
   - `metric_key: string`
   - `metric_value: number`

### Task G5: Advisor Filters Enhancement

1. Read `web/src/components/advisor/AdvisorFilters.tsx`
2. Add "Status" filter: Active / Resolved / Acknowledged / All
3. Pass status filter to the API call via query parameter `?status=active`
4. Default to "Active" so the page shows current issues by default

### Build Verification

```bash
cd web && npm run build && npm run typecheck && npm run lint
```

---

## Coordination

- Backend Agent and Frontend Agent work in PARALLEL
- Backend Agent: Tasks A → B → C → D → E → F (dependency chain)
- Frontend Agent: Tasks G4 → G5 → G1 → G2 → G3 (types first, then features)
- Frontend Agent should verify the API response shape matches after Backend Agent finishes Task F
- Both agents commit independently
- Merge only when full build + test suite passes

## DO NOT RE-DISCUSS

- Remediation engine architecture (25 rules, Engine.Diagnose, EvaluateMetric) — settled in REM_01
- Recommendation struct fields (MetricKey, MetricValue) — settled in M9_01
- NullStore pattern for live mode — settled in REM_01
- Setter pattern for APIServer — settled in REM_01
- Alert rules and their metric keys — settled in M9_01
