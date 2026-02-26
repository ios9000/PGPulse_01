package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// backupStateGate gates the pg_is_in_backup() call to PG 14 only.
// The function was removed in PostgreSQL 15.
var backupStateGate = version.Gate{
	Name: "backup_state",
	Variants: []version.SQLVariant{
		{
			Range: version.VersionRange{MinMajor: 14, MinMinor: 0, MaxMajor: 14, MaxMinor: 99},
			SQL:   "SELECT pg_is_in_backup()::int AS is_backup",
		},
	},
}

// ServerInfoCollector collects PostgreSQL server identity and state metrics.
// It covers PGAM queries Q2, Q3, Q9, and Q10.
type ServerInfoCollector struct {
	Base
}

// NewServerInfoCollector creates a new ServerInfoCollector for the given PostgreSQL instance.
func NewServerInfoCollector(instanceID string, v version.PGVersion) *ServerInfoCollector {
	return &ServerInfoCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *ServerInfoCollector) Name() string { return "server_info" }

// Collect executes server identity queries and returns metric points.
// Emits: server.start_time_unix, server.uptime_seconds,
// server.is_in_recovery, server.is_in_backup.
func (c *ServerInfoCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
	var points []MetricPoint

	// Q2: postmaster start time
	tctx, cancel := queryContext(ctx)
	var startEpoch int64
	err := conn.QueryRow(tctx, "SELECT extract(epoch FROM pg_postmaster_start_time())::bigint").Scan(&startEpoch)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("server_info collect start_time: %w", err)
	}
	points = append(points, c.point("server.start_time_unix", float64(startEpoch), nil))
	points = append(points, c.point("server.uptime_seconds", float64(time.Now().Unix()-startEpoch), nil))

	// Q9: recovery state — read from InstanceContext (queried once per cycle by orchestrator).
	recoveryVal := 0.0
	if ic.IsRecovery {
		recoveryVal = 1.0
	}
	points = append(points, c.point("server.is_in_recovery", recoveryVal, nil))

	// Q10: backup state — PG 14 only; removed in PG 15
	if sql, ok := backupStateGate.Select(c.pgVersion); ok {
		tctx3, cancel3 := queryContext(ctx)
		var isBackup int64
		err = conn.QueryRow(tctx3, sql).Scan(&isBackup)
		cancel3()
		if err != nil {
			return nil, fmt.Errorf("server_info collect is_backup: %w", err)
		}
		points = append(points, c.point("server.is_in_backup", float64(isBackup), nil))
	} else {
		// PG 15+: pg_is_in_backup() was removed; emit 0 to indicate no backup tracking
		points = append(points, c.point("server.is_in_backup", 0.0, nil))
	}

	return points, nil
}
