# M0_01: Project Setup — Design

**Milestone:** M0 — Project Setup  
**Iteration:** M0_01_02262026_project-setup  
**Date:** 2026-02-26

---

## Directory Structure to Create

```
pgpulse/
├── cmd/
│   ├── pgpulse-server/
│   │   └── main.go                 # Server entrypoint (placeholder)
│   └── pgpulse-agent/
│       └── main.go                 # Agent entrypoint (placeholder)
│
├── internal/
│   ├── collector/
│   │   └── collector.go            # Shared interfaces: MetricPoint, Collector, MetricStore
│   ├── version/
│   │   ├── version.go              # PG version detection + comparison
│   │   └── gate.go                 # Version-gated SQL template registry
│   ├── storage/
│   │   └── .gitkeep
│   ├── api/
│   │   └── .gitkeep
│   ├── auth/
│   │   └── .gitkeep
│   ├── alert/
│   │   ├── .gitkeep
│   │   └── notifier/
│   │       └── .gitkeep
│   ├── config/
│   │   └── .gitkeep
│   ├── ml/
│   │   └── .gitkeep
│   └── rca/
│       └── .gitkeep
│
├── web/
│   └── .gitkeep
│
├── migrations/
│   └── .gitkeep
│
├── configs/
│   └── pgpulse.example.yml
│
├── deploy/
│   ├── docker/
│   │   ├── Dockerfile
│   │   └── docker-compose.yml
│   ├── helm/
│   │   └── .gitkeep
│   └── systemd/
│       └── .gitkeep
│
├── docs/                            # Already created, copy from scaffold
│
├── .github/
│   └── workflows/
│       └── ci.yml
│
├── .golangci.yml
├── .gitignore
├── Makefile
├── README.md
├── go.mod
└── go.sum
```

## Key Files Design

### cmd/pgpulse-server/main.go

```go
package main

import (
    "fmt"
    "os"
)

func main() {
    fmt.Println("PGPulse Server — PostgreSQL Health & Activity Monitor")
    fmt.Println("Version: 0.1.0-dev")
    // TODO M2: Initialize config, storage, API router
    // TODO M1: Initialize collectors
    // TODO M3: Initialize auth
    // TODO M4: Initialize alert engine
    os.Exit(0)
}
```

### internal/collector/collector.go

```go
package collector

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5"
)

// MetricPoint represents a single metric data point collected from PostgreSQL.
type MetricPoint struct {
    InstanceID string
    Metric     string
    Value      float64
    Labels     map[string]string
    Timestamp  time.Time
}

// Collector is the interface that all metric collectors must implement.
type Collector interface {
    // Name returns the collector's identifier (e.g., "instance", "replication").
    Name() string
    // Collect executes queries and returns metric points.
    Collect(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error)
    // Interval returns how often this collector should run.
    Interval() time.Duration
}

// MetricQuery defines parameters for querying stored metrics.
type MetricQuery struct {
    InstanceID string
    Metric     string
    Labels     map[string]string
    From       time.Time
    To         time.Time
    Limit      int
}

// MetricStore is the interface for time-series metric storage.
type MetricStore interface {
    // Write persists a batch of metric points.
    Write(ctx context.Context, points []MetricPoint) error
    // Query retrieves metric points matching the query parameters.
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)
    // Close releases storage resources.
    Close() error
}

// AlertEvaluator processes metric values against alert rules.
type AlertEvaluator interface {
    // Evaluate checks a metric value against configured thresholds.
    Evaluate(ctx context.Context, metric string, value float64, labels map[string]string) error
}
```

### internal/version/version.go

```go
package version

import (
    "context"
    "fmt"
    "strconv"

    "github.com/jackc/pgx/v5"
)

// PGVersion represents a parsed PostgreSQL version.
type PGVersion struct {
    Major int // e.g., 16
    Minor int // e.g., 4
    Num   int // e.g., 160004 (raw server_version_num)
    Full  string // e.g., "16.4 (Ubuntu 16.4-1.pgdg22.04+1)"
}

// String returns the version as "Major.Minor".
func (v PGVersion) String() string {
    return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// AtLeast returns true if this version is >= the given major.minor.
func (v PGVersion) AtLeast(major, minor int) bool {
    if v.Major != major {
        return v.Major > major
    }
    return v.Minor >= minor
}

// Detect queries the PostgreSQL server for its version.
func Detect(ctx context.Context, conn *pgx.Conn) (PGVersion, error) {
    var numStr string
    var full string

    err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&numStr)
    if err != nil {
        return PGVersion{}, fmt.Errorf("detect PG version num: %w", err)
    }

    err = conn.QueryRow(ctx, "SHOW server_version").Scan(&full)
    if err != nil {
        return PGVersion{}, fmt.Errorf("detect PG version full: %w", err)
    }

    num, err := strconv.Atoi(numStr)
    if err != nil {
        return PGVersion{}, fmt.Errorf("parse server_version_num %q: %w", numStr, err)
    }

    return PGVersion{
        Major: num / 10000,
        Minor: num % 10000 / 100,
        Num:   num,
        Full:  full,
    }, nil
}
```

### internal/version/gate.go

```go
package version

// VersionRange defines a minimum and maximum PG version for a SQL variant.
type VersionRange struct {
    MinMajor int // inclusive
    MinMinor int // inclusive
    MaxMajor int // inclusive, 0 means no upper bound
    MaxMinor int // inclusive
}

// Contains returns true if the given PGVersion falls within this range.
func (r VersionRange) Contains(v PGVersion) bool {
    if !v.AtLeast(r.MinMajor, r.MinMinor) {
        return false
    }
    if r.MaxMajor == 0 {
        return true // no upper bound
    }
    if v.Major > r.MaxMajor {
        return false
    }
    if v.Major == r.MaxMajor && v.Minor > r.MaxMinor {
        return false
    }
    return true
}

// SQLVariant pairs a version range with a SQL query string.
type SQLVariant struct {
    Range VersionRange
    SQL   string
}

// Gate holds version-gated SQL variants for a single metric query.
// Variants are checked in order; the first matching range wins.
type Gate struct {
    Name     string
    Variants []SQLVariant
}

// Select returns the SQL query appropriate for the given PG version.
// Returns empty string and false if no variant matches.
func (g Gate) Select(v PGVersion) (string, bool) {
    for _, variant := range g.Variants {
        if variant.Range.Contains(v) {
            return variant.SQL, true
        }
    }
    return "", false
}
```

### configs/pgpulse.example.yml

```yaml
# PGPulse Configuration
# Copy to pgpulse.yml and adjust for your environment.

server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

# PGPulse metadata storage (where PGPulse stores its own data)
storage:
  host: "localhost"
  port: 5432
  database: "pgpulse"
  user: "pgpulse"
  password: "${PGPULSE_DB_PASSWORD}"  # Use env var
  sslmode: "prefer"

# PostgreSQL instances to monitor
instances:
  - name: "production-primary"
    host: "db-primary.example.com"
    port: 5432
    user: "pgpulse_monitor"  # Must have pg_monitor role
    password: "${MONITOR_PASSWORD}"
    sslmode: "require"
    databases:
      - "app_production"
      - "analytics"

  - name: "production-replica"
    host: "db-replica.example.com"
    port: 5432
    user: "pgpulse_monitor"
    password: "${MONITOR_PASSWORD}"
    sslmode: "require"

# Collection intervals
collection:
  high_frequency: 10s     # connections, locks, wait events
  medium_frequency: 60s   # statements, replication
  low_frequency: 5m       # per-DB analysis, bloat

# Alert configuration
alerts:
  enabled: true
  evaluation_interval: 30s
  
  channels:
    telegram:
      enabled: false
      bot_token: "${TELEGRAM_BOT_TOKEN}"
      chat_id: "${TELEGRAM_CHAT_ID}"
    
    slack:
      enabled: false
      webhook_url: "${SLACK_WEBHOOK_URL}"
    
    email:
      enabled: false
      smtp_host: "smtp.example.com"
      smtp_port: 587
      from: "pgpulse@example.com"
      to:
        - "dba-team@example.com"
    
    webhook:
      enabled: false
      url: "https://pagerduty.example.com/webhook"

# Logging
logging:
  level: "info"    # debug, info, warn, error
  format: "json"   # json, text
```

### Dockerfile Design

Multi-stage build:
1. **Builder stage**: `golang:1.23-alpine` → compile binary
2. **Runtime stage**: `alpine:3.19` → copy binary, minimal footprint

### docker-compose.yml Design

Services:
1. **pgpulse**: built from Dockerfile, depends on postgres
2. **postgres**: timescale/timescaledb:latest-pg16 with pgpulse database
3. Shared network, volume for postgres data

### CI Pipeline Design

Trigger: push to main, pull requests
Steps:
1. Checkout code
2. Setup Go 1.23
3. Cache Go modules
4. Run golangci-lint
5. Run tests with race detector
6. Build binaries (server + agent)

---

## Agent Allocation

| Specialist | Responsibilities |
|---|---|
| **Scaffold Agent** | Go module, directory tree, main.go placeholders, Makefile, Dockerfile, docker-compose, CI, .golangci.yml, .gitignore, README.md |
| **Interfaces & Config Agent** | collector.go interfaces, version.go + gate.go, pgpulse.example.yml, verify compilation |
