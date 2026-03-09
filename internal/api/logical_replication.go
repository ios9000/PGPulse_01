package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// LogicalReplicationResponse contains logical replication sync status across all databases.
type LogicalReplicationResponse struct {
	Subscriptions      []SubscriptionStatus `json:"subscriptions"`
	TotalPendingTables int                  `json:"total_pending_tables"`
}

// SubscriptionStatus describes one subscription's pending tables and stats.
type SubscriptionStatus struct {
	Database         string             `json:"database"`
	SubscriptionName string             `json:"subscription_name"`
	TablesPending    []PendingTable     `json:"tables_pending"`
	Stats            *SubscriptionStats `json:"stats,omitempty"`
}

// PendingTable describes a table that is not yet fully synchronized.
type PendingTable struct {
	TableName      string `json:"table_name"`
	SyncState      string `json:"sync_state"`
	SyncStateLabel string `json:"sync_state_label"`
	SyncLSN        string `json:"sync_lsn"`
}

// SubscriptionStats contains pg_stat_subscription data for a subscription.
type SubscriptionStats struct {
	PID             int    `json:"pid"`
	ReceivedLSN     string `json:"received_lsn"`
	LatestEndLSN    string `json:"latest_end_lsn"`
	LatestEndTime   string `json:"latest_end_time"`
	ApplyErrorCount *int   `json:"apply_error_count,omitempty"`
	SyncErrorCount  *int   `json:"sync_error_count,omitempty"`
}

// syncStateLabel maps pg_subscription_rel.srsubstate codes to human-readable labels.
func syncStateLabel(state string) string {
	switch state {
	case "i":
		return "Initializing"
	case "d":
		return "Data Copy"
	case "s":
		return "Synchronized"
	case "f":
		return "Finalized"
	default:
		return state
	}
}

// handleLogicalReplication returns logical replication sync status across all databases.
func (s *APIServer) handleLogicalReplication(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.connProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "not_available",
			"instance connection provider not configured")
		return
	}

	conn, err := s.connProvider.ConnFor(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get connection for logical replication",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusBadGateway, "connection_error",
			"failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	// Set statement timeout on the main connection.
	if _, err := conn.Exec(r.Context(), "SET LOCAL statement_timeout = '5s'"); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to set statement_timeout",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to configure connection")
		return
	}

	// Detect PG version — needed for PG 15+ columns in pg_stat_subscription.
	pgVer, err := version.Detect(r.Context(), conn)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to detect PG version",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to detect PostgreSQL version")
		return
	}

	// List user databases.
	dbRows, err := conn.Query(r.Context(),
		"SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate")
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to list databases",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to list databases")
		return
	}
	defer dbRows.Close()

	var databases []string
	for dbRows.Next() {
		var name string
		if err := dbRows.Scan(&name); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to scan database name",
				"instance_id", instanceID, "error", err)
			continue
		}
		databases = append(databases, name)
	}
	if err := dbRows.Err(); err != nil {
		s.logger.ErrorContext(r.Context(), "database list rows error",
			"instance_id", instanceID, "error", err)
	}

	resp := LogicalReplicationResponse{}

	// For each database, open a temporary connection and query logical replication status.
	for _, dbName := range databases {
		subs, err := s.queryDBLogicalReplication(r.Context(), conn, dbName, pgVer, instanceID)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query logical replication for database",
				"instance_id", instanceID, "database", dbName, "error", err)
			continue // skip this database, continue to next
		}
		resp.Subscriptions = append(resp.Subscriptions, subs...)
	}

	// Compute total pending tables.
	for _, sub := range resp.Subscriptions {
		resp.TotalPendingTables += len(sub.TablesPending)
	}

	writeJSON(w, http.StatusOK, Envelope{Data: resp})
}

// queryDBLogicalReplication opens a temporary connection to the given database
// and queries pending logical replication tables and subscription stats.
func (s *APIServer) queryDBLogicalReplication(
	ctx context.Context,
	mainConn *pgx.Conn,
	dbName string,
	pgVer version.PGVersion,
	instanceID string,
) ([]SubscriptionStatus, error) {
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cfg := mainConn.Config().Copy()
	cfg.Database = dbName
	cfg.RuntimeParams["application_name"] = "pgpulse_logical_repl"

	dbConn, err := pgx.ConnectConfig(dbCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", dbName, err)
	}
	defer func() { _ = dbConn.Close(ctx) }()

	// Set statement timeout on the temp connection.
	if _, err := dbConn.Exec(dbCtx, "SET LOCAL statement_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("set timeout on %s: %w", dbName, err)
	}

	// Query pending tables.
	const pendingSQL = `
		SELECT s.subname,
		       r.srrelid::regclass::text AS table_name,
		       r.srsubstate,
		       COALESCE(r.srsublsn::text, '') AS sync_lsn
		FROM pg_subscription_rel r
		JOIN pg_subscription s ON s.oid = r.srsubid
		WHERE r.srsubstate <> 'r'`

	rows, err := dbConn.Query(dbCtx, pendingSQL)
	if err != nil {
		// pg_subscription may not exist — skip this database silently.
		return nil, nil //nolint:nilerr // expected when no logical subscriptions exist
	}
	defer rows.Close()

	// Group pending tables by subscription name.
	subMap := make(map[string]*SubscriptionStatus)
	for rows.Next() {
		var subname, tableName, syncState, syncLSN string
		if err := rows.Scan(&subname, &tableName, &syncState, &syncLSN); err != nil {
			s.logger.ErrorContext(ctx, "failed to scan pending table",
				"instance_id", instanceID, "database", dbName, "error", err)
			continue
		}
		sub, ok := subMap[subname]
		if !ok {
			sub = &SubscriptionStatus{
				Database:         dbName,
				SubscriptionName: subname,
			}
			subMap[subname] = sub
		}
		sub.TablesPending = append(sub.TablesPending, PendingTable{
			TableName:      tableName,
			SyncState:      syncState,
			SyncStateLabel: syncStateLabel(syncState),
			SyncLSN:        syncLSN,
		})
	}
	if err := rows.Err(); err != nil {
		s.logger.ErrorContext(ctx, "pending tables rows error",
			"instance_id", instanceID, "database", dbName, "error", err)
	}

	// Query stats for each subscription.
	for subname, sub := range subMap {
		stats, err := s.querySubscriptionStats(dbCtx, dbConn, subname, pgVer)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to query subscription stats",
				"instance_id", instanceID, "database", dbName,
				"subscription", subname, "error", err)
			continue
		}
		sub.Stats = stats
	}

	// Collect results.
	var result []SubscriptionStatus
	for _, sub := range subMap {
		result = append(result, *sub)
	}
	return result, nil
}

// querySubscriptionStats queries pg_stat_subscription for a single subscription.
func (s *APIServer) querySubscriptionStats(
	ctx context.Context,
	conn *pgx.Conn,
	subname string,
	pgVer version.PGVersion,
) (*SubscriptionStats, error) {
	stats := &SubscriptionStats{}

	if pgVer.AtLeast(15, 0) {
		err := conn.QueryRow(ctx,
			`SELECT pid,
			        COALESCE(received_lsn::text, ''),
			        COALESCE(latest_end_lsn::text, ''),
			        COALESCE(latest_end_time::text, ''),
			        apply_error_count,
			        sync_error_count
			 FROM pg_stat_subscription
			 WHERE subname = $1 LIMIT 1`, subname,
		).Scan(
			&stats.PID, &stats.ReceivedLSN, &stats.LatestEndLSN,
			&stats.LatestEndTime, &stats.ApplyErrorCount, &stats.SyncErrorCount,
		)
		if err != nil {
			return nil, fmt.Errorf("stat_subscription (PG15+) for %s: %w", subname, err)
		}
	} else {
		err := conn.QueryRow(ctx,
			`SELECT pid,
			        COALESCE(received_lsn::text, ''),
			        COALESCE(latest_end_lsn::text, ''),
			        COALESCE(latest_end_time::text, '')
			 FROM pg_stat_subscription
			 WHERE subname = $1 LIMIT 1`, subname,
		).Scan(
			&stats.PID, &stats.ReceivedLSN, &stats.LatestEndLSN, &stats.LatestEndTime,
		)
		if err != nil {
			return nil, fmt.Errorf("stat_subscription for %s: %w", subname, err)
		}
	}

	return stats, nil
}
