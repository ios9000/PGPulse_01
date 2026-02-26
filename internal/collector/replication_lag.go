package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// replicationLagSQL combines PGAM Q37 (byte lag) and Q38 (time lag) into a single query.
// All nullable columns are COALESCE'd to prevent nil scan panics.
// pg_wal_lsn_diff columns: pending = unsent WAL; write/flush/replay = pipeline stages.
// No version gate needed — all columns exist since PG 10; our minimum is PG 14.
const replicationLagSQL = `
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
FROM pg_stat_replication`

// ReplicationLagCollector collects byte-based and time-based replication lag
// from pg_stat_replication. Runs on primary servers only.
// PGAM source: analiz2.php Q37 (byte lag), Q38 (time lag).
type ReplicationLagCollector struct {
	Base
}

// NewReplicationLagCollector creates a new ReplicationLagCollector for the given instance.
func NewReplicationLagCollector(instanceID string, v version.PGVersion) *ReplicationLagCollector {
	return &ReplicationLagCollector{
		Base: newBase(instanceID, v, 10*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *ReplicationLagCollector) Name() string { return "replication_lag" }

// Collect queries replication lag from pg_stat_replication and returns metric points.
// Returns nil, nil on replica instances — pg_stat_replication is empty on standbys.
// Emits 8 metrics per replica: pending/write/flush/replay/total bytes + write/flush/replay seconds.
// Labels: app_name, client_addr, state (byte metrics); app_name, client_addr (time metrics).
func (c *ReplicationLagCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
	// Primary only — pg_stat_replication is always empty on standbys.
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
			appName, clientAddr, state                string
			pending, write, flush, replay, total      float64
			writeSeconds, flushSeconds, replaySeconds float64
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
