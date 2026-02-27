# M2_01 Team Prompt — Configuration & Orchestrator

> **Paste this into Claude Code Agent Teams.**
> Agents create files only — developer runs bash manually (Windows bash bug).

---

Build the configuration loader and collection orchestrator for PGPulse.
Read `.claude/CLAUDE.md` for project context and interfaces.
Read `docs/iterations/M2_01_.../design.md` for detailed specifications.

**⚠️ CRITICAL: Agents CANNOT run shell commands on this platform.**
Do NOT attempt `go build`, `go test`, `go get`, `git commit`, or any bash commands.
Create and edit files only. Developer will run all bash commands manually.

Create a team of 2 specialists:

---

## IMPLEMENTATION AGENT

You create all production code. Work in this order (config is a dependency of orchestrator):

### Step 1: Update `internal/collector/collector.go`

Add the `MetricQuery` struct needed by the MetricStore interface. Insert it after the existing MetricStore interface definition:

```go
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

Ensure `"time"` is in the imports.

### Step 2: Create `internal/config/config.go`

Define configuration structs:

```go
package config

type Config struct {
    Server    ServerConfig     `koanf:"server"`
    Storage   StorageConfig    `koanf:"storage"`
    Instances []InstanceConfig `koanf:"instances"`
}

type ServerConfig struct {
    Listen   string `koanf:"listen"`
    LogLevel string `koanf:"log_level"`
}

type StorageConfig struct {
    DSN            string `koanf:"dsn"`
    UseTimescaleDB bool   `koanf:"use_timescaledb"`
    RetentionDays  int    `koanf:"retention_days"`
}

type InstanceConfig struct {
    ID        string         `koanf:"id"`
    DSN       string         `koanf:"dsn"`
    Enabled   *bool          `koanf:"enabled"` // pointer to detect omission, default true
    Intervals IntervalConfig `koanf:"intervals"`
}

type IntervalConfig struct {
    High   time.Duration `koanf:"high"`
    Medium time.Duration `koanf:"medium"`
    Low    time.Duration `koanf:"low"`
}
```

Note: `Enabled` is `*bool` so we can distinguish "not set" (nil → default true) from "explicitly false".

### Step 3: Create `internal/config/load.go`

Implement `Load(path string) (Config, error)`:

1. Create `koanf.New(".")`
2. Load YAML file: `k.Load(file.Provider(path), yaml.Parser())`
3. Load env overrides: `k.Load(env.Provider("PGPULSE_", ".", transformEnvKey), nil)`
   - `transformEnvKey`: strips "PGPULSE_" prefix, lowercases, replaces "_" with "."
   - Example: `PGPULSE_SERVER_LISTEN` → `server.listen`
4. Unmarshal into Config struct
5. Call `validate(&cfg)` — check and apply defaults

`validate()` rules:
- `cfg.Server.Listen` default `":8080"`
- `cfg.Server.LogLevel` default `"info"`, must be one of: debug, info, warn, error
- `cfg.Storage.RetentionDays` default `30`
- `cfg.Instances` must have at least 1 entry
- Each instance: DSN must not be empty, ID must not be empty
- Each instance: if `Enabled` is nil, set to pointer-to-true
- Each instance intervals: default High=10s, Medium=60s, Low=300s (if zero)
- Return descriptive error messages: `"instance[0]: dsn is required"`

**Imports needed:**
```go
import (
    "fmt"
    "strings"
    "time"

    "github.com/knadh/koanf/v2"
    "github.com/knadh/koanf/parsers/yaml"
    "github.com/knadh/koanf/providers/env"
    "github.com/knadh/koanf/providers/file"
)
```

### Step 4: Create `internal/orchestrator/orchestrator.go`

```go
package orchestrator

type Orchestrator struct {
    cfg     config.Config
    store   collector.MetricStore
    runners []*instanceRunner
    wg      sync.WaitGroup
    cancel  context.CancelFunc
    logger  *slog.Logger
}

func New(cfg config.Config, store collector.MetricStore, logger *slog.Logger) *Orchestrator

func (o *Orchestrator) Start(ctx context.Context) error
// - Create child context with cancel (store cancel func)
// - For each enabled instance config:
//   - Create instanceRunner
//   - runner.connect(ctx) — if error, log warn, skip
//   - runner.buildCollectors()
//   - runner.start(ctx, &o.wg) — launches goroutines
// - If zero runners started → return error
// - Log: "orchestrator started", instance_count=N

func (o *Orchestrator) Stop()
// - Call o.cancel()
// - o.wg.Wait()
// - For each runner: runner.close()
// - Log: "orchestrator stopped"
```

### Step 5: Create `internal/orchestrator/runner.go`

```go
type instanceRunner struct {
    cfg       config.InstanceConfig
    conn      *pgx.Conn
    pgVersion version.PGVersion
    store     collector.MetricStore
    groups    []*intervalGroup
    logger    *slog.Logger
}

func (r *instanceRunner) connect(ctx context.Context) error
// - pgx.ParseConfig(r.cfg.DSN)
// - Set ConnectTimeout = 5s
// - Set RuntimeParams["application_name"] = "pgpulse_orchestrator"
// - pgx.ConnectConfig(ctx, connConfig)
// - version.Detect(ctx, r.conn) → r.pgVersion
// - Log: "connected to instance", id=..., pg_version=...

func (r *instanceRunner) buildCollectors()
// - Instantiate ALL collectors with r.cfg.ID and r.pgVersion
// - Group into high/medium/low (see assignment table below)
// - Create 3 intervalGroup instances

func (r *instanceRunner) start(ctx context.Context, wg *sync.WaitGroup)
// - For each group: wg.Add(1), go group.run(ctx, wg)

func (r *instanceRunner) close()
// - r.conn.Close(context.Background())
```

**Collector assignment (hardcoded in buildCollectors):**

HIGH (default 10s):
- NewConnectionsCollector
- NewCacheCollector
- NewWaitEventsCollector
- NewLockTreeCollector
- NewLongTransactionsCollector

MEDIUM (default 60s):
- NewReplicationStatusCollector
- NewReplicationLagCollector
- NewReplicationSlotsCollector
- NewStatementsConfigCollector
- NewStatementsTopCollector
- NewCheckpointCollector
- NewProgressVacuumCollector
- NewProgressMaintenanceCollector
- NewProgressOperationsCollector

LOW (default 300s):
- NewServerInfoCollector
- NewDatabaseSizesCollector
- NewSettingsCollector
- NewExtensionsCollector
- NewTransactionsCollector
- NewIOStatsCollector

### Step 6: Create `internal/orchestrator/group.go`

```go
type intervalGroup struct {
    name       string
    interval   time.Duration
    collectors []collector.Collector
    conn       *pgx.Conn
    store      collector.MetricStore
    logger     *slog.Logger
}

func newIntervalGroup(...) *intervalGroup

func (g *intervalGroup) run(ctx context.Context, wg *sync.WaitGroup)
// - defer wg.Done()
// - ticker := time.NewTicker(g.interval)
// - defer ticker.Stop()
// - g.collect(ctx) // run once immediately
// - for { select case <-ctx.Done(): return; case <-ticker.C: g.collect(ctx) }

func (g *intervalGroup) collect(ctx context.Context)
// - queryInstanceContext(ctx, g.conn) → ic, err
//   - On error: log warn, return (skip cycle)
// - For each collector: Collect(ctx, g.conn, ic)
//   - On error: log warn, continue
//   - Append points to batch
// - If batch non-empty: store.Write(ctx, batch)
//   - On error: log error
//   - On success: log debug with point count

func queryInstanceContext(ctx context.Context, conn *pgx.Conn) (collector.InstanceContext, error)
// - 5s timeout context
// - SELECT pg_is_in_recovery()
// - Return InstanceContext{IsRecovery: result}
```

### Step 7: Create `internal/orchestrator/logstore.go`

```go
type LogStore struct {
    logger *slog.Logger
}

func NewLogStore(logger *slog.Logger) *LogStore

func (s *LogStore) Write(ctx context.Context, points []collector.MetricPoint) error
// - Log at debug level: "stored N metric points"
// - If len > 0, log first metric name as sample
// - Return nil

func (s *LogStore) Query(ctx context.Context, query collector.MetricQuery) ([]collector.MetricPoint, error)
// - Return nil, nil

func (s *LogStore) Close() error
// - Return nil
```

### Step 8: Rewrite `cmd/pgpulse-server/main.go`

Replace the existing placeholder with the real main function:
- Parse `-config` flag
- Set up slog logger
- Load config via config.Load()
- Parse log level from config, reconfigure logger
- Create LogStore
- Create Orchestrator
- Start orchestrator
- Signal handling (SIGINT, SIGTERM)
- Graceful shutdown with 10s timeout

See design.md section 4 for the full main.go code.

### Step 9: Create `configs/pgpulse.example.yml`

See design.md section 5 for the full YAML content.

### Final checklist for Implementation Agent:
- [ ] All files compile (no syntax errors in import paths)
- [ ] collector.go updated with MetricQuery struct
- [ ] config package has no dependency on collector or orchestrator
- [ ] orchestrator imports config, collector, and version packages
- [ ] main.go imports config and orchestrator packages
- [ ] All collector constructors referenced in buildCollectors() match the actual constructor names in internal/collector/
- [ ] koanf import paths are correct: `github.com/knadh/koanf/v2` etc.
- [ ] No bash commands attempted

---

## QA AGENT

You create test files. Read existing test patterns in `internal/collector/testutil_test.go` for mock helpers.

**Before writing tests**, read the Implementation Agent's files to verify struct names, function signatures, and package names.

### File 1: `internal/config/config_test.go`

Create test YAML files as string constants (not files on disk) using `os.CreateTemp` or write to a temp file in each test.

| Test | Description |
|------|-------------|
| TestLoad_ValidConfig | Write full valid YAML to temp file → Load → verify all fields |
| TestLoad_Defaults | Write minimal YAML (one instance, no server/storage) → verify defaults: listen=":8080", log_level="info", retention=30, intervals=10s/60s/300s, enabled=true |
| TestLoad_MissingFile | Load non-existent path → error |
| TestLoad_InvalidYAML | Write "{{invalid" to temp file → error |
| TestLoad_NoInstances | Write YAML with empty instances list → validation error |
| TestLoad_EmptyDSN | Write YAML with instance missing DSN → validation error |
| TestLoad_EnabledExplicitFalse | Write YAML with enabled: false → verify it stays false |

Use `t.TempDir()` for temp files. Each test writes its own YAML and calls `config.Load()`.

### File 2: `internal/orchestrator/group_test.go`

Create mock collectors and mock store for testing group logic.

```go
// mockCollector implements collector.Collector for testing.
type mockCollector struct {
    name     string
    interval time.Duration
    points   []collector.MetricPoint
    err      error
    called   bool
}

// mockStore implements collector.MetricStore for testing.
type mockStore struct {
    written [][]collector.MetricPoint
    err     error
}
```

| Test | Description |
|------|-------------|
| TestIntervalGroup_Collect_AllSuccess | 3 mock collectors returning points → store.Write called once with all points |
| TestIntervalGroup_Collect_PartialFailure | 3 collectors, middle one errors → other 2 points still written |
| TestIntervalGroup_Collect_AllFail | All collectors error → store.Write not called (no points) |
| TestIntervalGroup_Collect_NilPoints | Collectors return nil, nil → no store write |

**Note:** These test `collect()` directly, NOT `run()` (which uses a ticker and blocks). You'll need to make `collect()` accessible for testing — either by keeping it on the exported struct or by testing through a short-lived `run()` with immediate context cancellation.

Since `collect()` calls `queryInstanceContext()` which needs a real pgx.Conn, you have two options:
- **(a)** Extract the InstanceContext query into an interface/function that can be mocked
- **(b)** Test at a higher level with integration tests only

Recommend **(a)**: add a field `icFunc func(ctx, conn) (InstanceContext, error)` to intervalGroup that defaults to `queryInstanceContext` in production but can be replaced in tests.

### File 3: `internal/orchestrator/logstore_test.go`

| Test | Description |
|------|-------------|
| TestLogStore_Write | Write 10 points → no error returned |
| TestLogStore_Write_Empty | Write 0 points → no error |
| TestLogStore_Query | Returns nil, nil |
| TestLogStore_Close | Returns nil |

### File 4: `internal/orchestrator/orchestrator_test.go`

| Test | Description |
|------|-------------|
| TestNew | Verify orchestrator created with correct config and store |

Note: Full Start/Stop testing requires real PG connections → integration test only. Unit tests focus on construction and buildCollectors logic.

### Final checklist for QA Agent:
- [ ] All test files compile
- [ ] Config tests use t.TempDir() for temp YAML files
- [ ] Group tests use mock collectors and mock store
- [ ] No `//go:build integration` tags — these are unit tests
- [ ] No bash commands attempted

---

## Coordination

- Implementation Agent works **sequentially**: config first, then orchestrator, then main.go
- QA Agent can start config tests immediately (config has no dependencies)
- QA Agent writes orchestrator test stubs, fills in once orchestrator structs are visible
- If collector constructor names don't match, coordinate via task list

## Dependencies

Developer must run these manually BEFORE `go build`:
```bash
go get github.com/knadh/koanf/v2
go get github.com/knadh/koanf/parsers/yaml
go get github.com/knadh/koanf/providers/file
go get github.com/knadh/koanf/providers/env
go mod tidy
```

## Output

When both agents are done, list all files created/modified so the developer can run:
```bash
go get github.com/knadh/koanf/v2
go get github.com/knadh/koanf/parsers/yaml
go get github.com/knadh/koanf/providers/file
go get github.com/knadh/koanf/providers/env
go mod tidy
go build ./...
go vet ./...
golangci-lint run
go test -v ./internal/config/ ./internal/orchestrator/
```
