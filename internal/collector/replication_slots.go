package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// replicationSlotsGate selects the appropriate SQL for pg_replication_slots based on PG version.
// All three variants SELECT the same 6 columns, using empty string literals for
// columns that don't exist in older versions. This allows a single Scan() call
// regardless of PG version — no conditional scanning logic needed.
//
// Version differences:
//   PG 14: no two_phase or conflicting columns
//   PG 15: adds two_phase (boolean)
//   PG 16+: adds conflicting (boolean, null when slot is active)
//
// PGAM source: analiz2.php Q40.
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
			// PG 15: adds two_phase column
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
			// PG 16+: adds conflicting column
			Range: version.VersionRange{MinMajor: 16, MinMinor: 0, MaxMajor: 99, MaxMinor: 99},
			SQL: `SELECT
				slot_name,
				slot_type,
				active,
				coalesce(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn), 0) AS retained_bytes,
				two_phase::text AS two_phase,
				coalesce(conflicting::text, '') AS conflicting
			FROM pg_replication_slots`,
		},
	},
}

// ReplicationSlotsCollector collects replication slot health metrics.
// Runs on both primary and replica servers.
// PGAM source: analiz2.php Q40.
type ReplicationSlotsCollector struct {
	Base
	sqlGate version.Gate
}

// NewReplicationSlotsCollector creates a new ReplicationSlotsCollector for the given instance.
func NewReplicationSlotsCollector(instanceID string, v version.PGVersion) *ReplicationSlotsCollector {
	return &ReplicationSlotsCollector{
		Base:    newBase(instanceID, v, 60*time.Second),
		sqlGate: replicationSlotsGate,
	}
}

// Name returns the collector's identifier.
func (c *ReplicationSlotsCollector) Name() string { return "replication_slots" }

// Collect queries pg_replication_slots and returns metric points.
// Runs on both primary and replica instances.
// Emits 2 metrics per slot: retained_bytes and active (1.0/0.0).
// Labels: slot_name, slot_type, active; conditionally two_phase and conflicting if non-empty.
func (c *ReplicationSlotsCollector) Collect(ctx context.Context, conn *pgx.Conn, _ InstanceContext) ([]MetricPoint, error) {
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
			slotName, slotType    string
			active                bool
			retainedBytes         float64
			twoPhase, conflicting string
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

		// Add version-conditional labels only when the column is present (non-empty string).
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
