# M2_01 Design — Configuration & Orchestrator

**Iteration:** M2_01
**Date:** 2026-02-26

---

## 1. Package: internal/config/

### File: `internal/config/config.go`

```go
package config

import "time"

// Config is the top-level PGPulse configuration.
type Config struct {
    Server   ServerConfig    `koanf:"server"`
    Storage  StorageConfig   `koanf:"storage"`
    Instances []InstanceConfig `koanf:"instances"`
}

type ServerConfig struct {
    Listen   string `koanf:"listen"`    // e.g. ":8080"
    LogLevel string `koanf:"log_level"` // debug, info, warn, error
}

type StorageConfig struct {
    DSN            string `koanf:"dsn"`              // PGPulse's own database
    UseTimescaleDB bool   `koanf:"use_timescaledb"`
    RetentionDays  int    `koanf:"retention_days"`
}

type InstanceConfig struct {
    ID        string          `koanf:"id"`
    DSN       string          `koanf:"dsn"`
    Enabled   bool            `koanf:"enabled"`
    Intervals IntervalConfig  `koanf:"intervals"`
}

type IntervalConfig struct {
    High   time.Duration `koanf:"high"`    // default 10s
    Medium time.Duration `koanf:"medium"`  // default 60s
    Low    time.Duration `koanf:"low"`     // default 300s
}
```

### File: `internal/config/load.go`

```go
package config

// Load reads configuration from a YAML file and applies environment
// variable overrides. Environment variables use PGPULSE_ prefix with
// underscore-delimited paths (e.g., PGPULSE_SERVER_LISTEN).
//
// Returns error if file not found, YAML is invalid, or validation fails.
func Load(path string) (Config, error)

// validate checks required fields and sets defaults.
// - At least one instance with non-empty DSN
// - Defaults: listen=":8080", log_level="info", retention_days=30
// - Defaults: intervals high=10s, medium=60s, low=300s
// - enabled defaults to true if omitted
func validate(cfg *Config) error
```

**Implementation approach:**
1. `koanf.New(".")` as the base
2. `koanf.Load(file.Provider(path), yaml.Parser())` for YAML
3. `koanf.Load(env.Provider("PGPULSE_", ".", func(s string) string {...}), nil)` for env overrides
4. `koanf.Unmarshal("", &cfg)` into Config struct
5. Call `validate(&cfg)` to check required fields and apply defaults

**Duration parsing:** koanf's YAML parser handles Go duration strings natively when the struct field is `time.Duration`. Config values like `"10s"`, `"60s"`, `"5m"` parse correctly.

---

## 2. Package: internal/orchestrator/

### Architecture

```
Orchestrator
├── instanceRunner ("prod-main")
│   ├── pgx.Conn → monitored PG instance
│   ├── PGVersion (detected once)
│   ├── intervalGroup (high=10s)
│   │   ├── connections
│   │   ├── cache
│   │   ├── wait_events
│   │   ├── lock_tree
│   │   └── long_transactions
│   ├── intervalGroup (medium=60s)
│   │   ├── replication_status
│   │   ├── replication_lag
│   │   ├── replication_slots
│   │   ├── statements_config
│   │   ├── statements_top
│   │   ├── checkpoint
│   │   └── progress_*
│   └── intervalGroup (low=300s)
│       ├── server_info
│       ├── database_sizes
│       ├── settings
│       ├── extensions
│       ├── transactions
│       └── io_stats
│
├── instanceRunner ("prod-replica")
│   └── ... (same structure)
│
└── MetricStore (LogStore placeholder)
```

### File: `internal/orchestrator/orchestrator.go`

```go
package orchestrator

import (
    "context"
    "log/slog"
    "sync"

    "github.com/ios9000/PGPulse_01/internal/collector"
    "github.com/ios9000/PGPulse_01/internal/config"
)

// Orchestrator manages the lifecycle of all instance runners.
type Orchestrator struct {
    cfg     config.Config
    store   collector.MetricStore
    runners []*instanceRunner
    wg      sync.WaitGroup
    logger  *slog.Logger
}

// New creates an Orchestrator from config. Does not connect yet.
func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger) *Orchestrator

// Start connects to all enabled instances, detects versions, starts
// collection goroutines. Returns error only if ALL instances fail to
// connect (partial success is OK — failed instances are logged and skipped).
func (o *Orchestrator) Start(ctx context.Context) error

// Stop cancels collection, waits for goroutines, closes connections.
func (o *Orchestrator) Stop()
```

**Start() logic:**
1. Filter enabled instances from config
2. For each instance config:
   a. Create instanceRunner
   b. Call runner.connect(ctx) — if fails, log warn, skip this instance
   c. Call runner.start(ctx) — launches interval group goroutines
3. If zero runners started successfully → return error
4. Log: "started N/M instances"

**Stop() logic:**
1. Cancel the context passed to Start (caller manages this)
2. `o.wg.Wait()` — blocks until all goroutines exit
3. For each runner: `runner.close()` — closes pgx.Conn

### File: `internal/orchestrator/runner.go`

```go
package orchestrator

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"

    "github.com/jackc/pgx/v5"

    "github.com/ios9000/PGPulse_01/internal/collector"
    "github.com/ios9000/PGPulse_01/internal/config"
    "github.com/ios9000/PGPulse_01/internal/version"
)

// instanceRunner manages collection for a single monitored PG instance.
type instanceRunner struct {
    cfg       config.InstanceConfig
    conn      *pgx.Conn
    pgVersion version.PGVersion
    store     collector.MetricStore
    groups    []*intervalGroup
    logger    *slog.Logger
}

// connect establishes a pgx connection with connect_timeout and
// application_name, then detects PG version.
func (r *instanceRunner) connect(ctx context.Context) error {
    // Parse DSN, add application_name=pgpulse_orchestrator, connect_timeout=5
    // pgx.Connect(ctx, connString)
    // version.Detect(ctx, r.conn)
    // Log: instance "prod-main" connected, PG 16.4
}

// buildCollectors creates all collector instances for this PG version
// and assigns them to interval groups.
func (r *instanceRunner) buildCollectors() {
    // Create all collectors with r.cfg.ID, r.pgVersion
    // Group by interval into high/medium/low
    // See collector assignment table in design
}

// start launches goroutines for each interval group.
func (r *instanceRunner) start(ctx context.Context, wg *sync.WaitGroup)

// close closes the pgx connection.
func (r *instanceRunner) close()
```

**connect() DSN manipulation:**
```go
connConfig, err := pgx.ParseConfig(r.cfg.DSN)
if err != nil {
    return fmt.Errorf("parse DSN for %s: %w", r.cfg.ID, err)
}
connConfig.ConnectTimeout = 5 * time.Second
connConfig.RuntimeParams["application_name"] = "pgpulse_orchestrator"
r.conn, err = pgx.ConnectConfig(ctx, connConfig)
```

**buildCollectors() — collector instantiation:**

```go
func (r *instanceRunner) buildCollectors() {
    id := r.cfg.ID
    v := r.pgVersion

    high := []collector.Collector{
        collector.NewConnectionsCollector(id, v),
        collector.NewCacheCollector(id, v),
        collector.NewWaitEventsCollector(id, v),
        collector.NewLockTreeCollector(id, v),
        collector.NewLongTransactionsCollector(id, v),
    }

    medium := []collector.Collector{
        collector.NewReplicationStatusCollector(id, v),
        collector.NewReplicationLagCollector(id, v),
        collector.NewReplicationSlotsCollector(id, v),
        collector.NewStatementsConfigCollector(id, v),
        collector.NewStatementsTopCollector(id, v),
        collector.NewCheckpointCollector(id, v),
        collector.NewProgressVacuumCollector(id, v),
        collector.NewProgressMaintenanceCollector(id, v),
        collector.NewProgressOperationsCollector(id, v),
    }

    low := []collector.Collector{
        collector.NewServerInfoCollector(id, v),
        collector.NewDatabaseSizesCollector(id, v),
        collector.NewSettingsCollector(id, v),
        collector.NewExtensionsCollector(id, v),
        collector.NewTransactionsCollector(id, v),
        collector.NewIOStatsCollector(id, v),
    }

    r.groups = []*intervalGroup{
        newIntervalGroup("high", r.cfg.Intervals.High, high, r.conn, r.store, r.logger),
        newIntervalGroup("medium", r.cfg.Intervals.Medium, medium, r.conn, r.store, r.logger),
        newIntervalGroup("low", r.cfg.Intervals.Low, low, r.conn, r.store, r.logger),
    }
}
```

**Note:** This is explicit registration matching decision D9. No init() magic. If a collector constructor doesn't exist, the build fails — which is what we want.

### File: `internal/orchestrator/group.go`

```go
package orchestrator

import (
    "context"
    "log/slog"
    "sync"
    "time"

    "github.com/jackc/pgx/v5"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

// intervalGroup runs a set of collectors at a fixed interval.
type intervalGroup struct {
    name       string // "high", "medium", "low"
    interval   time.Duration
    collectors []collector.Collector
    conn       *pgx.Conn
    store      collector.MetricStore
    logger     *slog.Logger
}

func newIntervalGroup(
    name string,
    interval time.Duration,
    collectors []collector.Collector,
    conn *pgx.Conn,
    store collector.MetricStore,
    logger *slog.Logger,
) *intervalGroup

// run executes the collection loop. Blocks until ctx is cancelled.
// Called as a goroutine.
func (g *intervalGroup) run(ctx context.Context, wg *sync.WaitGroup) {
    defer wg.Done()
    
    ticker := time.NewTicker(g.interval)
    defer ticker.Stop()

    // Run once immediately on start.
    g.collect(ctx)

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            g.collect(ctx)
        }
    }
}

// collect runs all collectors in the group sequentially, passing
// results to the store. Handles partial failure.
func (g *intervalGroup) collect(ctx context.Context) {
    // 1. Query pg_is_in_recovery() → InstanceContext
    //    (each group does this at the start of its cycle)
    ic, err := queryInstanceContext(ctx, g.conn)
    if err != nil {
        g.logger.Warn("failed to query instance context", "group", g.name, "error", err)
        return // skip this cycle — connection may be dead
    }

    var allPoints []collector.MetricPoint

    // 2. Run each collector
    for _, c := range g.collectors {
        points, err := c.Collect(ctx, g.conn, ic)
        if err != nil {
            g.logger.Warn("collector error",
                "collector", c.Name(),
                "group", g.name,
                "error", err,
            )
            continue // partial failure — other collectors still run
        }
        if points != nil {
            allPoints = append(allPoints, points...)
        }
    }

    // 3. Write batch to store
    if len(allPoints) > 0 {
        if err := g.store.Write(ctx, allPoints); err != nil {
            g.logger.Error("store write failed",
                "group", g.name,
                "points", len(allPoints),
                "error", err,
            )
        } else {
            g.logger.Debug("collected metrics",
                "group", g.name,
                "points", len(allPoints),
            )
        }
    }
}
```

**queryInstanceContext() — shared helper:**
```go
// queryInstanceContext queries pg_is_in_recovery() and returns InstanceContext.
// Uses a 5s timeout.
func queryInstanceContext(ctx context.Context, conn *pgx.Conn) (collector.InstanceContext, error) {
    qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    var isRecovery bool
    err := conn.QueryRow(qCtx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)
    if err != nil {
        return collector.InstanceContext{}, fmt.Errorf("pg_is_in_recovery: %w", err)
    }
    return collector.InstanceContext{IsRecovery: isRecovery}, nil
}
```

**Design note on InstanceContext refresh:** Each interval group queries pg_is_in_recovery() at the start of its own cycle. This means the high group refreshes every 10s, low group every 300s. A failover event would be detected within 10s by the high group. This is simpler than a shared lock-protected context, and the cost of one extra lightweight query per cycle is negligible.

### File: `internal/orchestrator/logstore.go`

```go
package orchestrator

import (
    "context"
    "log/slog"

    "github.com/ios9000/PGPulse_01/internal/collector"
)

// LogStore implements collector.MetricStore by logging metrics.
// Used as a placeholder until M2_02 adds real PG-backed storage.
type LogStore struct {
    logger *slog.Logger
}

func NewLogStore(logger *slog.Logger) *LogStore

// Write logs the point count and a sample of metric names.
func (s *LogStore) Write(ctx context.Context, points []collector.MetricPoint) error {
    // Log at debug level: "stored 47 metric points", sample first 5 metric names
    return nil
}

// Query is not implemented — returns nil, nil.
func (s *LogStore) Query(ctx context.Context, query collector.MetricQuery) ([]collector.MetricPoint, error) {
    return nil, nil
}

// Close is a no-op.
func (s *LogStore) Close() error {
    return nil
}
```

---

## 3. MetricQuery Type

We need to add `MetricQuery` to collector.go to satisfy the MetricStore interface:

```go
// Add to internal/collector/collector.go

// MetricQuery defines parameters for querying stored metrics.
type MetricQuery struct {
    InstanceID string
    Metric     string            // optional: filter by metric name prefix
    Labels     map[string]string // optional: filter by label values
    Start      time.Time         // time range start
    End        time.Time         // time range end
    Limit      int               // max results (0 = no limit)
}
```

This is a minimal query struct. The real storage implementation in M2_02 will use it. LogStore ignores it.

---

## 4. cmd/pgpulse-server/main.go

```go
package main

import (
    "context"
    "flag"
    "log/slog"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/ios9000/PGPulse_01/internal/config"
    "github.com/ios9000/PGPulse_01/internal/orchestrator"
)

func main() {
    configPath := flag.String("config", "configs/pgpulse.yml", "path to config file")
    flag.Parse()

    // 1. Set up logger
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug, // will be configurable from cfg.Server.LogLevel
    }))

    // 2. Load config
    cfg, err := config.Load(*configPath)
    if err != nil {
        logger.Error("failed to load config", "path", *configPath, "error", err)
        os.Exit(1)
    }
    logger.Info("config loaded",
        "instances", len(cfg.Instances),
        "listen", cfg.Server.Listen,
    )

    // 3. Configure log level from config
    // Parse cfg.Server.LogLevel → slog.Level, reconfigure handler

    // 4. Create store (placeholder)
    store := orchestrator.NewLogStore(logger)

    // 5. Create and start orchestrator
    orch := orchestrator.New(cfg, store, logger)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    if err := orch.Start(ctx); err != nil {
        logger.Error("failed to start orchestrator", "error", err)
        os.Exit(1)
    }

    // 6. Wait for shutdown signal
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    sig := <-sigCh
    logger.Info("received signal, shutting down", "signal", sig)

    // 7. Graceful shutdown with timeout
    cancel()

    done := make(chan struct{})
    go func() {
        orch.Stop()
        close(done)
    }()

    select {
    case <-done:
        logger.Info("shutdown complete")
    case <-time.After(10 * time.Second):
        logger.Error("shutdown timed out, forcing exit")
        os.Exit(1)
    }
}
```

---

## 5. configs/pgpulse.example.yml

```yaml
# PGPulse Configuration
# Copy to pgpulse.yml and customize.
# Environment variables override with PGPULSE_ prefix:
#   PGPULSE_SERVER_LISTEN=":9090"
#   PGPULSE_STORAGE_DSN="postgres://..."

server:
  listen: ":8080"
  log_level: "info"   # debug, info, warn, error

storage:
  dsn: ""              # PGPulse internal DB (empty = log-only mode)
  use_timescaledb: false
  retention_days: 30

instances:
  - id: "local"
    dsn: "postgres://pg_monitor@localhost:5432/postgres"
    enabled: true
    intervals:
      high: "10s"
      medium: "60s"
      low: "300s"

  # Example: add more instances
  # - id: "prod-replica"
  #   dsn: "postgres://pg_monitor@10.0.0.2:5432/postgres"
  #   enabled: true
  #   intervals:
  #     high: "10s"
  #     medium: "60s"
  #     low: "300s"
```

---

## 6. Connection Resilience

The orchestrator must handle a monitored instance becoming unavailable mid-run.

**Strategy: reconnect on next cycle.**

In `intervalGroup.collect()`, if `queryInstanceContext()` returns a connection error, the group logs a warning and skips the cycle. On the next ticker fire, it tries again. If the connection is truly dead (closed by server), pgx will return an error, and we need to reconnect.

```go
// In intervalGroup.collect():
ic, err := queryInstanceContext(ctx, g.conn)
if err != nil {
    g.logger.Warn("instance context query failed, attempting reconnect",
        "group", g.name, "error", err)
    if reconnErr := g.reconnect(ctx); reconnErr != nil {
        g.logger.Warn("reconnect failed, skipping cycle",
            "group", g.name, "error", reconnErr)
        return
    }
    // Retry after reconnect
    ic, err = queryInstanceContext(ctx, g.conn)
    if err != nil {
        g.logger.Warn("still failing after reconnect, skipping cycle",
            "group", g.name, "error", err)
        return
    }
}
```

The reconnect logic lives in the runner (which owns the connection). Groups need a reference back to the runner for reconnection. This adds a bit of complexity — the simpler approach for M2_01 is:

**Simplified: just skip the cycle on error.** If the connection recovers on its own (transient network issue), the next cycle works. If the PG instance is truly down, we log warnings every cycle. Automatic reconnect can be added in a follow-up.

I recommend the simplified approach for M2_01 — skip cycle on error, no reconnect logic. Add reconnection in M2_02 or M2_03 when we have the full system running and can test it properly.

---

## 7. Test Strategy

### File: `internal/config/config_test.go`

| Test | Description |
|------|-------------|
| TestLoad_ValidConfig | Load sample YAML → verify all fields parsed correctly |
| TestLoad_Defaults | Load minimal YAML (just one instance) → verify defaults applied |
| TestLoad_MissingFile | Non-existent path → error |
| TestLoad_InvalidYAML | Malformed YAML → error |
| TestLoad_NoInstances | Empty instances list → validation error |
| TestLoad_EmptyDSN | Instance with empty DSN → validation error |
| TestLoad_EnabledDefault | Instance without enabled field → defaults to true |

### File: `internal/orchestrator/group_test.go`

| Test | Description |
|------|-------------|
| TestIntervalGroup_CollectorsRunSequentially | Mock collectors → verify all called in order |
| TestIntervalGroup_PartialFailure | One collector returns error → others still run |
| TestIntervalGroup_StoreReceivesPoints | Mock store → verify Write called with collected points |
| TestIntervalGroup_EmptyCollectorList | No collectors → no store writes, no errors |

### File: `internal/orchestrator/logstore_test.go`

| Test | Description |
|------|-------------|
| TestLogStore_Write | Write points → no error (just logs) |
| TestLogStore_Query | Returns nil, nil |
| TestLogStore_Close | Returns nil |

### File: `internal/orchestrator/orchestrator_test.go`

| Test | Description |
|------|-------------|
| TestNew | Creates orchestrator with config and store |
| TestBuildCollectors_AllCreated | Verify correct number of collectors per group |

---

## 8. File Summary

| File | Lines (est.) | Agent |
|------|-------------|-------|
| `internal/config/config.go` | ~50 | Implementation |
| `internal/config/load.go` | ~90 | Implementation |
| `internal/orchestrator/orchestrator.go` | ~80 | Implementation |
| `internal/orchestrator/runner.go` | ~120 | Implementation |
| `internal/orchestrator/group.go` | ~100 | Implementation |
| `internal/orchestrator/logstore.go` | ~40 | Implementation |
| `cmd/pgpulse-server/main.go` | ~70 | Implementation |
| `configs/pgpulse.example.yml` | ~30 | Implementation |
| Update: `internal/collector/collector.go` | +10 (MetricQuery) | Implementation |
| `internal/config/config_test.go` | ~120 | QA |
| `internal/orchestrator/group_test.go` | ~150 | QA |
| `internal/orchestrator/logstore_test.go` | ~40 | QA |
| `internal/orchestrator/orchestrator_test.go` | ~60 | QA |
| **Total** | **~960** | |

---

## 9. Dependencies to Add

```
go get github.com/knadh/koanf/v2
go get github.com/knadh/koanf/providers/file
go get github.com/knadh/koanf/parsers/yaml
go get github.com/knadh/koanf/providers/env
```

These need to be run manually (bash bug). The implementation agent should include the correct import paths but cannot run `go get`.

---

## 10. What M2_01 Does NOT Include

| Not included | Why | When |
|-------------|-----|------|
| PG-backed MetricStore | Separate concern | M2_02 |
| REST API endpoints | Separate concern | M2_03 |
| Automatic reconnection | Adds complexity, test later | M2_02 or M2_03 |
| Dynamic instance add/remove | Requires API + auth | M3+ |
| Connection pooling (pgxpool) | Single conn sufficient for now | Future optimization |
| Collector enable/disable per instance | All collectors run for all instances | Future config option |
| Metric buffering / batching across cycles | Write per cycle is sufficient | Future optimization |
