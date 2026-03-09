package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// replicationStatusPrimarySQL enumerates active physical replicas connected via replication slots.
// JOIN to pg_replication_slots ensures we only count replicas with an active slot.
// PGAM source: analiz2.php Q20.
const replicationStatusPrimarySQL = `
SELECT
    coalesce(r.application_name, '') AS app_name,
    coalesce(r.client_addr::text, '') AS client_addr,
    coalesce(r.state, '') AS state,
    coalesce(r.sync_state, '') AS sync_state
FROM pg_stat_replication r
JOIN pg_replication_slots s ON r.pid = s.active_pid
WHERE s.slot_type = 'physical' AND s.active = true`

// replicationStatusReplicaSQL reads the WAL receiver state on a standby.
// lag_bytes = difference between the latest WAL end sent by primary and what's been received.
// PGAM source: analiz2.php Q21.
const replicationStatusReplicaSQL = `
SELECT
    coalesce(status, '') AS status,
    coalesce(sender_host, '') AS sender_host,
    coalesce(sender_port::text, '0') AS sender_port,
    coalesce(pg_wal_lsn_diff(latest_end_lsn, flushed_lsn), 0) AS lag_bytes
FROM pg_stat_wal_receiver`

// ReplicationStatusCollector collects replication topology information.
// On a primary: enumerates active physical replicas (Q20).
// On a replica: reports WAL receiver connection status (Q21).
type ReplicationStatusCollector struct {
	Base
}

// NewReplicationStatusCollector creates a new ReplicationStatusCollector for the given instance.
func NewReplicationStatusCollector(instanceID string, v version.PGVersion) *ReplicationStatusCollector {
	return &ReplicationStatusCollector{
		Base: newBase(instanceID, v, 60*time.Second),
	}
}

// Name returns the collector's identifier.
func (c *ReplicationStatusCollector) Name() string { return "replication_status" }

// Collect delegates to collectPrimary or collectReplica based on ic.IsRecovery.
func (c *ReplicationStatusCollector) Collect(ctx context.Context, conn *pgx.Conn, ic InstanceContext) ([]MetricPoint, error) {
	if ic.IsRecovery {
		return c.collectReplica(ctx, conn)
	}
	return c.collectPrimary(ctx, conn)
}

// collectPrimary queries pg_stat_replication for active physical replicas.
// Emits replication.replica.connected (1 per replica) and replication.active_replicas (count).
// Always emits active_replicas=0 when no replicas are connected.
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
			"app_name":    appName,
			"client_addr": clientAddr,
			"state":       state,
			"sync_state":  syncState,
		}))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("replication_status primary rows: %w", err)
	}

	// Always emit aggregate count — 0 when no replicas are connected.
	points = append(points, c.point("replication.active_replicas", count, nil))

	return points, nil
}

// collectReplica queries pg_stat_wal_receiver for WAL receiver state.
// Emits wal_receiver.connected (1 if status='streaming', 0 otherwise)
// and wal_receiver.lag_bytes with sender_host/sender_port labels.
// Emits connected=0 with nil labels if no WAL receiver row exists (disconnected standby).
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

	// No WAL receiver row → standby is not connected to a primary.
	if !hasReceiver {
		points = append(points, c.point("replication.wal_receiver.connected", 0, nil))
	}

	return points, nil
}
