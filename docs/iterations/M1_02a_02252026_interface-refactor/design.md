# M1_02a — Design: InstanceContext Interface Refactor

**Iteration:** M1_02a
**Date:** 2026-02-26
**Depends on:** M1_01 completed codebase
**Produces:** Updated Collector interface consumed by M1_02b

---

## 1. InstanceContext Struct

**File:** `internal/collector/collector.go`

Add after the `MetricPoint` struct, before the `Collector` interface:

```go
// InstanceContext holds per-scrape-cycle metadata about the PostgreSQL
// instance. The orchestrator queries this data once per cycle and passes
// it to all collectors, avoiding redundant queries and ensuring a single
// source of truth for instance-level state that may change at runtime
// (e.g., primary/replica role after a Patroni failover).
type InstanceContext struct {
	// IsRecovery is true when the instance is a replica (standby).
	// Derived from: SELECT pg_is_in_recovery()
	// Queried once per scrape cycle by the orchestrator.
	IsRecovery bool
}
```

## 2. Updated Collector Interface

**File:** `internal/collector/collector.go`

Change from:
```go
type Collector interface {
	Name() string
	Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error)
	Interval() time.Duration
}
```

Change to:
```go
type Collector interface {
	Name() string
	Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error)
	Interval() time.Duration
}
```

## 3. Mechanical Collector Updates (6 files)

Each of these collectors adds `_ InstanceContext` to the signature. No other changes.

### connections.go
```go
// Before:
func (c *ConnectionsCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (c *ConnectionsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
```

### cache.go
```go
// Before:
func (c *CacheCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (c *CacheCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
```

### transactions.go
```go
// Before:
func (c *TransactionsCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (c *TransactionsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
```

### database_sizes.go
```go
// Before:
func (c *DatabaseSizesCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (c *DatabaseSizesCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
```

### settings.go
```go
// Before:
func (c *SettingsCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (c *SettingsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
```

### extensions.go
```go
// Before:
func (c *ExtensionsCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (c *ExtensionsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
```

## 4. ServerInfoCollector — Logic Change

**File:** `internal/collector/server_info.go`

This is the only collector with a real behavior change. It currently queries
`pg_is_in_recovery()` directly. After refactor, it reads from `ic.IsRecovery`.

### Signature change:
```go
// Before:
func (c *ServerInfoCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (c *ServerInfoCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
```

### Recovery metric emission change:

**Before** (queries PG directly):
```go
// Somewhere in the Collect method body:
var isRecovery bool
err := conn.QueryRow(qCtx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)
if err != nil {
    return nil, fmt.Errorf("server_info recovery check: %w", err)
}
recoveryVal := 0.0
if isRecovery {
    recoveryVal = 1.0
}
points = append(points, b.point("server.is_in_recovery", recoveryVal, nil))
```

**After** (reads from InstanceContext):
```go
// Replace the above block with:
recoveryVal := 0.0
if ic.IsRecovery {
    recoveryVal = 1.0
}
points = append(points, b.point("server.is_in_recovery", recoveryVal, nil))
```

The `pg_is_in_recovery()` query is removed entirely from this collector.
The orchestrator is now responsible for calling it.

**pg_is_in_backup() is NOT changed** — it remains a direct query in
ServerInfoCollector because it is version-gated (PG < 15 only), rarely
needed by other collectors, and doesn't change as frequently as recovery state.

## 5. Registry Update

**File:** `internal/collector/registry.go`

### CollectAll signature:

```go
// Before:
func CollectAll(ctx context.Context, conn *pgx.Conn, collectors []Collector) ([]MetricPoint, []error) {

// After:
func CollectAll(ctx context.Context, conn *pgx.Conn, ic InstanceContext, collectors []Collector) ([]MetricPoint, []error) {
```

### Inside CollectAll — pass ic to each collector:

```go
// Before:
points, err := c.Collect(ctx, conn)

// After:
points, err := c.Collect(ctx, conn, ic)
```

No other logic changes — partial-failure behavior stays identical.

## 6. Test File Updates

### 6.1 testutil_test.go

If `testutil_test.go` contains helper functions that invoke `Collect()`,
update them to pass `InstanceContext{}`. Example:

```go
// If there's a helper like:
func collectAndAssert(t *testing.T, c Collector, conn *pgx.Conn) []MetricPoint {
    points, err := c.Collect(context.Background(), conn)
    // ...
}

// Change to:
func collectAndAssert(t *testing.T, c Collector, conn *pgx.Conn) []MetricPoint {
    points, err := c.Collect(context.Background(), conn, InstanceContext{})
    // ...
}
```

### 6.2 Mechanical test updates (6 files)

In `connections_test.go`, `cache_test.go`, `transactions_test.go`,
`database_sizes_test.go`, `settings_test.go`, `extensions_test.go`:

Every call to `c.Collect(ctx, conn)` becomes `c.Collect(ctx, conn, InstanceContext{})`.

Default `InstanceContext{}` has `IsRecovery: false` (zero value), which means
"primary." This is correct for these tests — they test collectors that work
identically on primary and replica.

### 6.3 server_info_test.go — Logic change

Tests must be updated to reflect that ServerInfoCollector no longer queries
`pg_is_in_recovery()` directly.

**Test for primary:**
```go
ic := InstanceContext{IsRecovery: false}
points, err := c.Collect(ctx, conn, ic)
// Assert pgpulse.server.is_in_recovery == 0.0
```

**Test for replica:**
```go
ic := InstanceContext{IsRecovery: true}
points, err := c.Collect(ctx, conn, ic)
// Assert pgpulse.server.is_in_recovery == 1.0
```

If the existing test uses a mock that expects a `pg_is_in_recovery()` query,
that expectation must be removed.

### 6.4 registry_test.go

Mock collectors must implement the updated interface:

```go
type mockCollector struct {
    name   string
    points []MetricPoint
    err    error
}

// Before:
func (m *mockCollector) Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {

// After:
func (m *mockCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
```

Calls to `CollectAll()` add `InstanceContext{}`:

```go
// Before:
points, errs := CollectAll(ctx, nil, collectors)

// After:
points, errs := CollectAll(ctx, nil, InstanceContext{}, collectors)
```

## 7. File Impact Summary

| File | Change Type | Complexity |
|------|-------------|------------|
| `collector.go` | Add struct + update interface | Low |
| `connections.go` | Signature only | Trivial |
| `cache.go` | Signature only | Trivial |
| `transactions.go` | Signature only | Trivial |
| `database_sizes.go` | Signature only | Trivial |
| `settings.go` | Signature only | Trivial |
| `extensions.go` | Signature only | Trivial |
| `server_info.go` | Signature + remove query + read from ic | Medium |
| `registry.go` | Signature + pass-through | Low |
| `testutil_test.go` | Pass InstanceContext{} | Low |
| `connections_test.go` | Pass InstanceContext{} | Trivial |
| `cache_test.go` | Pass InstanceContext{} | Trivial |
| `transactions_test.go` | Pass InstanceContext{} | Trivial |
| `database_sizes_test.go` | Pass InstanceContext{} | Trivial |
| `settings_test.go` | Pass InstanceContext{} | Trivial |
| `extensions_test.go` | Pass InstanceContext{} | Trivial |
| `server_info_test.go` | Update mock + test both roles | Medium |
| `registry_test.go` | Update mock signature + CollectAll call | Low |

**Total: 18 files, ~50 lines changed**

## 8. Validation Checklist

After all changes:

```bash
go build ./...              # Must pass — no compilation errors
go vet ./...                # Must pass — no vet warnings
golangci-lint run           # Must pass — 0 issues (v2.10.1)
go test ./internal/collector/...  # Must pass — all unit tests green
go test ./internal/version/...    # Must pass — unchanged but verify
```

Zero new dependencies. Zero new files (except this design doc). Clean refactor.
