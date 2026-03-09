# M8_03 Team Prompt
## Instance Lister Fix + Session Kill API + ML Model Persistence

**Read before spawning agents:**
- `.claude/CLAUDE.md` — current iteration, shared interfaces
- `docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/M8_03_design.md` — full design
- `docs/iterations/M8_03_03092026_lister-sessionkill-mlpersist/M8_03_requirements.md` — requirements

---

## Team Lead Instructions

Read CLAUDE.md and the M8_03 design doc. Create a team of 3 specialists.

Dependency order:

```
Migration (API Agent) ────────────────────► schema exists before any store code runs
Collector Agent ──────────────────────────► DBInstanceLister + ML persistence (no API deps)
API Agent ────────────────────────────────► session handlers + migration (no ML deps)
QA Agent ────────────────────────────────► tests for all three features as code lands

Merge order:
  1. Migration 010 (API Agent)
  2. Collector Agent (lister + persistence)
  3. API Agent (session handlers)
  4. QA Agent (tests)
  5. main.go wiring (Team Lead)
```

Merge only after QA Agent confirms all tests pass and
`go build ./cmd/pgpulse-server` + `golangci-lint run` are clean.

---

## Collector Agent

**Your scope:** `internal/ml/lister.go`, `internal/ml/persistence.go`,
modifications to `internal/ml/baseline.go` and `internal/ml/detector.go`

**Do NOT touch:** `internal/api/`, `migrations/`, `internal/auth/`

---

### Task 1: DBInstanceLister

Create `internal/ml/lister.go` exactly as specified in design doc section 1.
The query is: `SELECT id FROM instances WHERE enabled = true ORDER BY id`

---

### Task 2: BaselineSnapshot + STLBaseline methods

In `internal/ml/baseline.go`, add:

- `BaselineSnapshot` struct with JSON tags (defined in design doc section 3)
- `func (b *STLBaseline) Snapshot(instanceID string) BaselineSnapshot`
- `func LoadFromSnapshot(s BaselineSnapshot) *STLBaseline`

Key implementation note for `Snapshot()`: export only the *live* residuals from
the ring buffer, not the full pre-allocated slice with stale slots. Use the
ring head and count to extract in chronological order:

```go
liveResiduals := make([]float64, b.resCount)
for i := 0; i < b.resCount; i++ {
    idx := (b.resHead - b.resCount + i + b.windowSize) % b.windowSize
    liveResiduals[i] = b.residuals[idx]
}
```

Key implementation note for `LoadFromSnapshot()`: restore the residuals into
the ring buffer starting at position 0, set `resHead = resCount % windowSize`.
This is equivalent to the ring being filled from the front — scoring will work
correctly because `Score()` only reads `residuals[:resCount]`.

---

### Task 3: PersistenceStore

Create `internal/ml/persistence.go` exactly as specified in design doc section 3.
The interface:

```go
type PersistenceStore interface {
    Save(ctx context.Context, snap BaselineSnapshot) error
    Load(ctx context.Context, instanceID, metricKey string) (*BaselineSnapshot, error)
    LoadAll(ctx context.Context) ([]BaselineSnapshot, error)
}
```

`DBPersistenceStore` serializes `BaselineSnapshot` to JSON for the `state` JSONB column.
Upsert on `(instance_id, metric_key)` — one row per metric per instance.

---

### Task 4: Modify Detector

In `internal/ml/detector.go`:

**Add `persist PersistenceStore` field to `Detector` struct.**

**Update `NewDetector` signature** to accept `persist PersistenceStore` as the
final parameter (may be nil — all code must guard `if d.persist != nil`):

```go
func NewDetector(cfg DetectorConfig, store collector.MetricStore,
    lister InstanceLister, evaluator collector.AlertEvaluator,
    persist PersistenceStore) *Detector
```

**Update `Bootstrap`** to implement the two-phase sequence from design doc section 3:
1. If `d.persist != nil`: call `LoadAll`, load non-stale snapshots via `LoadFromSnapshot`
2. For metrics not loaded from snapshot: existing TimescaleDB replay (unchanged)
3. Log counts: how many loaded from snapshot vs replayed

Staleness check per metric: `time.Since(snap.UpdatedAt) > time.Duration(2*mc.Period) * d.config.CollectionInterval`
Find the matching `MetricConfig` for each snapshot by key. If no matching config found, skip.

**Update `Evaluate`** to persist after processing all points:
```go
// After all scoring/dispatch: persist each baseline that has been updated
if d.persist != nil {
    d.mu.RLock()
    for key, b := range d.baselines {
        parts := strings.SplitN(key, ":", 2)
        if len(parts) != 2 { continue }
        snap := b.Snapshot(parts[0])
        if err := d.persist.Save(ctx, snap); err != nil {
            slog.Warn("ML baseline persist failed", "key", key, "err", err)
        }
    }
    d.mu.RUnlock()
}
```

**Important:** All existing tests pass `nil` for the persist param. Ensure
`NewDetector(..., nil)` works correctly — no panics.

---

## API Agent

**Your scope:** `internal/api/session_actions.go`, `internal/api/server.go`
(route registration only), `migrations/010_ml_baseline_snapshots.sql`

**Do NOT touch:** `internal/ml/`, `internal/collector/`

---

### Task 1: Migration

Create `migrations/010_ml_baseline_snapshots.sql` exactly as specified in
design doc section 3:

```sql
CREATE TABLE IF NOT EXISTS ml_baseline_snapshots (
    id           BIGSERIAL    PRIMARY KEY,
    instance_id  TEXT         NOT NULL,
    metric_key   TEXT         NOT NULL,
    period       INT          NOT NULL,
    state        JSONB        NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (instance_id, metric_key)
);

CREATE INDEX IF NOT EXISTS idx_ml_baseline_snapshots_instance
    ON ml_baseline_snapshots(instance_id);
```

---

### Task 2: Session Kill Handlers

Create `internal/api/session_actions.go` implementing `handleSessionCancel`
and `handleSessionTerminate` as specified in design doc section 2.

Full implementation requirements:

**Validation:**
- Parse `{pid}` URL param as integer. If non-integer or <= 0: return 400 `{"error": "invalid pid"}`
- Get connection pool from `s.orchestrator.PoolForInstance(instanceID)`. If error: 404

**Safety guard 1 — own PID:**
```go
var ownPID int
_ = pool.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&ownPID)
if pid == ownPID {
    writeError(w, http.StatusBadRequest, "cannot target PGPulse's own backend")
    return
}
```

**Safety guard 2 — superuser check:**
```go
var isSuper bool
err = pool.QueryRow(ctx, `
    SELECT COALESCE(u.usesuper, false)
    FROM pg_stat_activity a
    LEFT JOIN pg_user u ON u.usename = a.usename
    WHERE a.pid = $1
`, pid).Scan(&isSuper)
if err != nil {
    // pid not found — already gone, treat as success=false
    writeJSON(w, http.StatusOK, sessionActionResponse{PID: pid, Action: action, Success: false})
    return
}
if isSuper {
    writeError(w, http.StatusForbidden, "cannot terminate superuser session")
    return
}
```

**Execute:**
```go
var fn string
switch action {
case "cancel":
    fn = "pg_cancel_backend($1)"
default:
    fn = "pg_terminate_backend($1)"
}
var success bool
_ = pool.QueryRow(ctx, "SELECT "+fn, pid).Scan(&success)
```

Note: `"SELECT " + fn` with `fn` being a hardcoded string (not user input) is
the one deliberate Sprintf-equivalent pattern here. Add a comment:
`// fn is a hardcoded function name, not user input — safe`

**Audit log:**
```go
subject := auth.SubjectFromContext(r.Context())
slog.Info("session action",
    "instance", instanceID, "pid", pid,
    "action", action, "success", success, "user", subject)
```

Check whether `auth.SubjectFromContext` already exists. If not, add it to
`internal/auth/` — it reads the JWT claims from context and returns the subject
string, or "unknown" if not present. Match the exact pattern used by other
handlers in the codebase for JWT claim extraction.

**Response:**
```go
type sessionActionResponse struct {
    PID     int    `json:"pid"`
    Action  string `json:"action"`
    Success bool   `json:"success"`
}
writeJSON(w, http.StatusOK, sessionActionResponse{PID: pid, Action: action, Success: success})
```

---

### Task 3: Register Routes

In `internal/api/server.go`, register the two new routes in the **admin-only**
middleware group (match the pattern used by other mutation endpoints):

```go
r.Post("/instances/{id}/sessions/{pid}/cancel",    s.handleSessionCancel)
r.Post("/instances/{id}/sessions/{pid}/terminate", s.handleSessionTerminate)
```

Find the existing admin route group by looking at how `POST /api/v1/instances`
(add instance) is registered — use the same group.

---

### Task 4: configs/pgpulse.example.yml

Add `persistence` subsection under the existing `ml:` block:

```yaml
ml:
  # ... existing fields ...
  persistence:
    enabled: true
    staleness_multiplier: 2
```

---

## QA Agent

**Your scope:** All `*_test.go` files for this iteration.
Write stubs immediately; fill assertions as agents commit code.

---

### Task 1: DBInstanceLister Tests

Create `internal/ml/lister_test.go` (testcontainers PG 16):

- `TestDBInstanceLister_Empty`: empty instances table → returns `[]string{}`, no error
- `TestDBInstanceLister_ReturnsEnabledOnly`: insert one enabled + one disabled instance → returns only enabled ID
- `TestDBInstanceLister_OrderedByID`: multiple instances → returned in ID order

---

### Task 2: BaselineSnapshot Round-Trip Tests

Create `internal/ml/persistence_test.go`:

**Unit tests (no DB):**
- `TestBaselineSnapshot_RoundTrip`: create `STLBaseline`, feed 100 points, call `Snapshot()`, call `LoadFromSnapshot()`, verify all exported fields match (EWMA, seasonal slice, SeasonN, ResCount, TotalSeen, SumAll)
- `TestBaselineSnapshot_ScoringContinuity`: fit baseline with 200 points; call `Score(outlierValue)` → get `(z1, iqr1)`; snapshot + reload; call `Score(outlierValue)` again → `(z2, iqr2)`; assert `math.Abs(z1-z2) < 0.01` (scores are deterministic after reload)
- `TestBaselineSnapshot_LiveResidualsOnly`: fit baseline with windowSize=10, feed 15 points; snapshot; assert `len(snap.Residuals) == 10` (not 15 or internal windowSize)
- `TestLoadFromSnapshot_Ready`: snapshot from a Ready() baseline → `LoadFromSnapshot().Ready() == true`

**Store tests (testcontainers PG 16):**
- `TestDBPersistenceStore_SaveAndLoad`: save snapshot, load by instanceID+metricKey, assert fields match
- `TestDBPersistenceStore_Upsert`: save same (instanceID, metricKey) twice with different EWMA → one row in DB, second EWMA value wins
- `TestDBPersistenceStore_LoadAll`: save 3 snapshots for 2 instances → LoadAll returns all 3

---

### Task 3: Detector Bootstrap Tests

Add to `internal/ml/detector_test.go`:

- `TestDetector_Bootstrap_LoadsFromSnapshot`: pre-populate `ml_baseline_snapshots` with a fresh snapshot; call Bootstrap with a mock store that would return points if queried — assert the mock store's Query is NOT called for that metric (snapshot was used)
- `TestDetector_Bootstrap_SkipsStaleSnapshot`: pre-populate with snapshot where `UpdatedAt = now() - 72h` (well beyond staleness); assert the mock store IS queried for replay
- `TestDetector_Bootstrap_FallbackOnEmptyDB`: no snapshots in DB, no TimescaleDB points → metric skipped with warning, no panic

Use a mock `PersistenceStore` (in-memory map) for these tests to avoid needing testcontainers.

---

### Task 4: Session Kill Handler Tests

Create `internal/api/session_actions_test.go` (httptest, mock pool):

- `TestSessionCancel_RequiresAdmin`: POST with viewer JWT → 403
- `TestSessionCancel_RequiresAuth`: POST without JWT → 401
- `TestSessionCancel_InvalidPID`: POST with pid="abc" → 400
- `TestSessionCancel_Success`: mock `pg_cancel_backend` returns true → 200 `{success: true}`
- `TestSessionCancel_PIDGone`: mock `pg_cancel_backend` returns false → 200 `{success: false}`
- `TestSessionTerminate_SuperuserBlocked`: mock superuser check returns true → 403
- `TestSessionTerminate_OwnPIDBlocked`: mock `pg_backend_pid()` returns same pid → 400
- `TestSessionTerminate_Success`: mock returns false for superuser, true for terminate → 200 `{success: true}`

For the mock pool: use a test helper that returns predictable rows for the
specific queries (`pg_backend_pid()`, superuser check, `pg_cancel_backend`,
`pg_terminate_backend`). Match the pattern used in other API handler tests
in the codebase.

---

### Task 5: Lint + Verification

After all code is committed:

1. `golangci-lint run ./internal/ml/... ./internal/api/...` → 0 issues
2. `grep -rn 'fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT' internal/` → 0 results
   (the `"SELECT " + fn` in session_actions.go uses string concatenation with
   a hardcoded string — verify the comment is present: `// fn is a hardcoded function name`)
3. `go test -race ./internal/ml/... ./internal/api/...` → all pass
4. `go build ./cmd/pgpulse-server` → clean

---

## Shared: main.go Wiring

Team Lead: after all agents complete, update `cmd/pgpulse-server/main.go`:

1. **Replace `configInstanceLister`** with `ml.NewDBInstanceLister(storagePool)`.
   Delete the old `configInstanceLister` struct and constructor.

2. **Initialize persistence store:**
   ```go
   var persistStore ml.PersistenceStore
   if cfg.ML.Enabled && cfg.ML.Persistence.Enabled {
       persistStore = ml.NewDBPersistenceStore(storagePool)
   }
   ```

3. **Update `ml.NewDetector` call** to pass `persistStore` as final argument.

4. **Verify session routes** are registered (API Agent handles this in server.go —
   just confirm the routes appear in the router when server starts).

5. **Update `.claude/CLAUDE.md`** current iteration section to M8_03.

---

## Build Verification Sequence

```bash
cd web && npm run build && npm run typecheck && npm run lint
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
golangci-lint run
```

All agents: do NOT commit if `go build ./cmd/pgpulse-server` fails.
QA Agent: report final test count and pass rate before Team Lead merges.
