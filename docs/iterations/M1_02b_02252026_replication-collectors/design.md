# M1_02b — Design: Replication Collectors

**Iteration:** M1_02b
**Date:** 2026-02-26
**Depends on:** M1_02a committed (InstanceContext in Collector interface)

---

## 1. ReplicationLagCollector

**File:** `internal/collector/replication_lag.go`

### Constructor

```go
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ios9000/PGPulse_01/internal/version"
)

// ReplicationLagCollector collects byte-based and time-based replication lag
// from pg_stat_replication. Runs on primary servers only.
// PGAM source: analiz2.php Q37 (byte lag), Q38 (time lag).
type ReplicationLagCollector struct {
	Base
}

func NewReplicationLagCollector(instanceID string, v version.PGVersion) *ReplicationLagCollector {
	return &ReplicationLagCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

func (c *ReplicationLagCollector) Name() string { return "replication_lag" }
```

### SQL Query (single query combines Q37 + Q38)

No version gate needed — all columns exist since PG 10, and our minimum is PG 14.

```sql
SELECT
    coalesce(application_name, '') AS app_name,
    coalesce(client_addr::text, '') AS client_addr,
    coalesce(state, '') AS state,
    coalesce(pg_wal_lsn_diff(pg_current_wal_lsn(), sent_lsn), 0) AS pending_bytes,
    coalesce(pg_wal_lsn_diff(sent_lsn, write_lsn), 0) AS write_bytes,
    coalesce(pg_wal_lsn_diff(write_lsn, flush_lsn), 0) AS flush_bytes,
    coalesce(pg_wal_lsn_diff(flush_lsn, replay_lsn), 0) AS replay_bytes,
    coalesce(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS total_bytes,
    coalesce(EXTRACT(EPOCH FROM write_lag), 0) AS write_seconds,
    coalesce(EXTRACT(EPOCH FROM flush_lag), 0) AS flush_seconds,
    coalesce(EXTRACT(EPOCH FROM replay_lag), 0) AS replay_seconds
FROM pg_stat_replication
```

### Collect Method

```go
func (c *ReplicationLagCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
	// Primary only — replicas don't have pg_stat_replication data
	if ic.IsRecovery {
		return nil, nil
	}

	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, replicationLagSQL)
	if err != nil {
		return nil, fmt.Errorf("replication_lag: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			appName, clientAddr, state                          string
			pending, write, flush, replay, total                float64
			writeSeconds, flushSeconds, replaySeconds           float64
		)
		if err := rows.Scan(
			&appName, &clientAddr, &state,
			&pending, &write, &flush, &replay, &total,
			&writeSeconds, &flushSeconds, &replaySeconds,
		); err != nil {
			return nil, fmt.Errorf("replication_lag scan: %w", err)
		}

		labels := map[string]string{
			"app_name":    appName,
			"client_addr": clientAddr,
			"state":       state,
		}
		timeLbls := map[string]string{
			"app_name":    appName,
			"client_addr": clientAddr,
		}

		points = append(points,
			c.point("replication.lag.pending_bytes", pending, labels),
			c.point("replication.lag.write_bytes", write, labels),
			c.point("replication.lag.flush_bytes", flush, labels),
			c.point("replication.lag.replay_bytes", replay, labels),
			c.point("replication.lag.total_bytes", total, labels),
			c.point("replication.lag.write_seconds", writeSeconds, timeLbls),
			c.point("replication.lag.flush_seconds", flushSeconds, timeLbls),
			c.point("replication.lag.replay_seconds", replaySeconds, timeLbls),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("replication_lag rows: %w", err)
	}

	return points, nil
}

func init() {
	RegisterCollector(func(id string, v version.PGVersion) Collector {
		return NewReplicationLagCollector(id, v)
	})
}
```

---

## 2. ReplicationSlotsCollector

**File:** `internal/collector/replication_slots.go`

### Constructor

```go
// ReplicationSlotsCollector collects replication slot health metrics.
// Runs on both primary and replica servers.
// PGAM source: analiz2.php Q40.
type ReplicationSlotsCollector struct {
	Base
	sqlGate version.Gate
}

func NewReplicationSlotsCollector(instanceID string, v version.PGVersion) *ReplicationSlotsCollector {
	return &ReplicationSlotsCollector{
		Base:    newBase(instanceID, v, 60*time.Second),
		sqlGate: replicationSlotsGate,
	}
}

func (c *ReplicationSlotsCollector) Name() string { return "replication_slots" }
```

### Version-Gated SQL

```go
var replicationSlotsGate = version.Gate{
	Name: "replication_slots",
	Variants: []version.SQLVariant{
		{
			// PG 14: base columns only
			Range: version.VersionRange{MinMajor: 14, MinMinor: 0, MaxMajor: 14, MaxMinor: 99},
			SQL: `SELECT
				slot_name,
				slot_type,
				active,
				coalesce(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn), 0) AS retained_bytes,
				'' AS two_phase,
				'' AS conflicting
			FROM pg_replication_slots`,
		},
		{
			// PG 15: adds two_phase
			Range: version.VersionRange{MinMajor: 15, MinMinor: 0, MaxMajor: 15, MaxMinor: 99},
			SQL: `SELECT
				slot_name,
				slot_type,
				active,
				coalesce(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn), 0) AS retained_bytes,
				two_phase::text AS two_phase,
				'' AS conflicting
			FROM pg_replication_slots`,
		},
		{
			// PG 16+: adds conflicting
			Range: version.VersionRange{MinMajor: 16, MinMinor: 0, MaxMajor: 99, MaxMinor: 99},
			SQL: `SELECT
				slot_name,
				slot_type,
				active,
				coalesce(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn), 0) AS retained_bytes,
				two_phase::text AS two_phase,
				conflicting::text AS conflicting
			FROM pg_replication_slots`,
		},
	},
}
```

**Design note:** All three variants SELECT the same 6 columns (slot_name,
slot_type, active, retained_bytes, two_phase, conflicting). Missing columns
are replaced with empty string literals. This allows a single `Scan()` call
regardless of PG version — no conditional scanning logic needed.

### Collect Method

```go
func (c *ReplicationSlotsCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
	sql, ok := c.sqlGate.Select(c.pgVersion)
	if !ok {
		return nil, fmt.Errorf("replication_slots: no SQL variant for PG %s", c.pgVersion.Full)
	}

	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, sql)
	if err != nil {
		return nil, fmt.Errorf("replication_slots: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint

	for rows.Next() {
		var (
			slotName, slotType     string
			active                 bool
			retainedBytes          float64
			twoPhase, conflicting  string
		)
		if err := rows.Scan(&slotName, &slotType, &active, &retainedBytes, &twoPhase, &conflicting); err != nil {
			return nil, fmt.Errorf("replication_slots scan: %w", err)
		}

		activeStr := "false"
		if active {
			activeStr = "true"
		}

		labels := map[string]string{
			"slot_name": slotName,
			"slot_type": slotType,
			"active":    activeStr,
		}

		// Add version-conditional labels (non-empty only)
		if twoPhase != "" {
			labels["two_phase"] = twoPhase
		}
		if conflicting != "" {
			labels["conflicting"] = conflicting
		}

		activeVal := 0.0
		if active {
			activeVal = 1.0
		}

		points = append(points,
			c.point("replication.slot.retained_bytes", retainedBytes, labels),
			c.point("replication.slot.active", activeVal, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("replication_slots rows: %w", err)
	}

	return points, nil
}

func init() {
	RegisterCollector(func(id string, v version.PGVersion) Collector {
		return NewReplicationSlotsCollector(id, v)
	})
}
```

---

## 3. ReplicationStatusCollector

**File:** `internal/collector/replication_status.go`

### Constructor

```go
// ReplicationStatusCollector collects replication topology information.
// On a primary: enumerates active physical replicas (Q20).
// On a replica: reports WAL receiver connection status (Q21).
type ReplicationStatusCollector struct {
	Base
}

func NewReplicationStatusCollector(instanceID string, v version.PGVersion) *ReplicationStatusCollector {
	return &ReplicationStatusCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

func (c *ReplicationStatusCollector) Name() string { return "replication_status" }
```

### SQL — Primary Side (Q20)

```go
const replicationStatusPrimarySQL = `
SELECT
    coalesce(r.application_name, '') AS app_name,
    coalesce(r.client_addr::text, '') AS client_addr,
    coalesce(r.state, '') AS state,
    coalesce(r.sync_state, '') AS sync_state
FROM pg_stat_replication r
JOIN pg_replication_slots s ON r.pid = s.active_pid
WHERE s.slot_type = 'physical' AND s.active = true`
```

### SQL — Replica Side (Q21)

```go
const replicationStatusReplicaSQL = `
SELECT
    coalesce(status, '') AS status,
    coalesce(sender_host, '') AS sender_host,
    coalesce(sender_port::text, '0') AS sender_port,
    coalesce(pg_wal_lsn_diff(latest_end_lsn, received_lsn), 0) AS lag_bytes
FROM pg_stat_wal_receiver`
```

### Collect Method

```go
func (c *ReplicationStatusCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
	if ic.IsRecovery {
		return c.collectReplica(ctx, conn)
	}
	return c.collectPrimary(ctx, conn)
}

func (c *ReplicationStatusCollector) collectPrimary(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, replicationStatusPrimarySQL)
	if err != nil {
		return nil, fmt.Errorf("replication_status primary: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	var count float64

	for rows.Next() {
		var appName, clientAddr, state, syncState string
		if err := rows.Scan(&appName, &clientAddr, &state, &syncState); err != nil {
			return nil, fmt.Errorf("replication_status primary scan: %w", err)
		}

		count++
		points = append(points, c.point("replication.replica.connected", 1, map[string]string{
			"app_name":   appName,
			"client_addr": clientAddr,
			"state":       state,
			"sync_state":  syncState,
		}))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("replication_status primary rows: %w", err)
	}

	// Always emit aggregate count (0 if no replicas)
	points = append(points, c.point("replication.active_replicas", count, nil))

	return points, nil
}

func (c *ReplicationStatusCollector) collectReplica(ctx context.Context, conn *pgx.Conn) ([]MetricPoint, error) {
	qCtx, cancel := queryContext(ctx)
	defer cancel()

	rows, err := conn.Query(qCtx, replicationStatusReplicaSQL)
	if err != nil {
		return nil, fmt.Errorf("replication_status replica: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	hasReceiver := false

	for rows.Next() {
		var status, senderHost, senderPort string
		var lagBytes float64
		if err := rows.Scan(&status, &senderHost, &senderPort, &lagBytes); err != nil {
			return nil, fmt.Errorf("replication_status replica scan: %w", err)
		}

		hasReceiver = true
		labels := map[string]string{
			"sender_host": senderHost,
			"sender_port": senderPort,
		}

		connectedVal := 0.0
		if status == "streaming" {
			connectedVal = 1.0
		}

		points = append(points,
			c.point("replication.wal_receiver.connected", connectedVal, labels),
			c.point("replication.wal_receiver.lag_bytes", lagBytes, labels),
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("replication_status replica rows: %w", err)
	}

	// If no WAL receiver exists, emit disconnected status
	if !hasReceiver {
		points = append(points, c.point("replication.wal_receiver.connected", 0, nil))
	}

	return points, nil
}

func init() {
	RegisterCollector(func(id string, v version.PGVersion) Collector {
		return NewReplicationStatusCollector(id, v)
	})
}
```

---

## 4. Test Design

### 4.1 replication_lag_test.go

| Test | InstanceContext | Setup | Assertion |
|------|----------------|-------|-----------|
| `TestReplicationLag_SkipsOnReplica` | `{IsRecovery: true}` | — | Returns nil, nil |
| `TestReplicationLag_EmptyOnPrimary` | `{IsRecovery: false}` | No replicas | Returns empty slice, no error |
| `TestReplicationLag_Integration` | `{IsRecovery: false}` | testcontainers PG 16 with standby | Returns 8 metrics per replica |

Integration test tagged `//go:build integration` (requires Docker).
Unit tests should test the skip-on-replica logic and empty-result handling
without needing a real PG connection.

### 4.2 replication_slots_test.go

| Test | Focus | Assertion |
|------|-------|-----------|
| `TestReplicationSlots_GateSelectPG14` | Version gate | Selects PG 14 variant (no two_phase/conflicting) |
| `TestReplicationSlots_GateSelectPG15` | Version gate | Selects PG 15 variant (two_phase present) |
| `TestReplicationSlots_GateSelectPG16` | Version gate | Selects PG 16+ variant (both columns) |
| `TestReplicationSlots_EmptyResult` | No slots | Returns empty slice, no error |
| `TestReplicationSlots_Integration` | Full cycle | Returns retained_bytes + active per slot |

Version gate unit tests don't need a PG connection — test `Gate.Select()` directly.

### 4.3 replication_status_test.go

| Test | InstanceContext | Focus | Assertion |
|------|----------------|-------|-----------|
| `TestReplicationStatus_PrimaryNoReplicas` | `{IsRecovery: false}` | Empty pg_stat_replication | active_replicas=0 |
| `TestReplicationStatus_ReplicaNoReceiver` | `{IsRecovery: true}` | Empty pg_stat_wal_receiver | wal_receiver.connected=0 |
| `TestReplicationStatus_Integration_Primary` | `{IsRecovery: false}` | testcontainers with replica | active_replicas>0 |
| `TestReplicationStatus_Integration_Replica` | `{IsRecovery: true}` | testcontainers standby | wal_receiver.connected=1 |

---

## 5. File Summary

### New Files

| File | Lines (est.) | Agent |
|------|-------------|-------|
| `internal/collector/replication_lag.go` | ~100 | Collector Agent |
| `internal/collector/replication_slots.go` | ~140 | Collector Agent |
| `internal/collector/replication_status.go` | ~140 | Collector Agent |
| `internal/collector/replication_lag_test.go` | ~60 | QA Agent |
| `internal/collector/replication_slots_test.go` | ~80 | QA Agent |
| `internal/collector/replication_status_test.go` | ~80 | QA Agent |

### No Modified Files

M1_02a already landed the interface change. These new files implement the
updated interface from day one — no existing files need modification.

---

## 6. Validation Checklist

```bash
go build ./...
go vet ./...
golangci-lint run
go test ./internal/collector/...       # unit tests
go test ./internal/version/...         # verify no breakage
# CI only: go test -tags=integration ./internal/collector/...
```

All new SQL must be:
- ✅ String constants (not built with fmt.Sprintf)
- ✅ Using COALESCE for nullable columns
- ✅ No user input in queries (parameterized where needed)
- ✅ application_name not set per-collector (set at connection level — future M2)
