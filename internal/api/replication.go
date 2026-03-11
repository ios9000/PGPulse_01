package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ReplicationResponse contains replication status for an instance.
type ReplicationResponse struct {
	InstanceID  string           `json:"instance_id"`
	Role        string           `json:"role"` // "primary" or "replica"
	Replicas    []ReplicaInfo    `json:"replicas,omitempty"`
	Slots       []SlotInfo       `json:"slots,omitempty"`
	WALReceiver *WALReceiverInfo `json:"wal_receiver,omitempty"`
}

// ReplicaInfo describes a connected replica.
type ReplicaInfo struct {
	ClientAddr      string  `json:"client_addr"`
	ApplicationName string  `json:"application_name"`
	State           string  `json:"state"`
	SyncState       string  `json:"sync_state"`
	Lag             LagInfo `json:"lag"`
}

// LagInfo contains replication lag metrics in bytes and time.
type LagInfo struct {
	PendingBytes int64  `json:"pending_bytes"`
	WriteBytes   int64  `json:"write_bytes"`
	FlushBytes   int64  `json:"flush_bytes"`
	ReplayBytes  int64  `json:"replay_bytes"`
	WriteLag     string `json:"write_lag,omitempty"`
	FlushLag     string `json:"flush_lag,omitempty"`
	ReplayLag    string `json:"replay_lag,omitempty"`
}

// SlotInfo describes a replication slot.
type SlotInfo struct {
	SlotName         string `json:"slot_name"`
	SlotType         string `json:"slot_type"`
	Active           bool   `json:"active"`
	WALRetainedBytes int64  `json:"wal_retained_bytes"`
}

// WALReceiverInfo describes the WAL receiver on a replica.
type WALReceiverInfo struct {
	SenderHost string `json:"sender_host"`
	SenderPort int    `json:"sender_port"`
	Status     string `json:"status"`
}

// handleReplication returns replication status for a monitored instance.
func (s *APIServer) handleReplication(w http.ResponseWriter, r *http.Request) {
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
		s.logger.ErrorContext(r.Context(), "failed to get connection for replication",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusBadGateway, "connection_error",
			"failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	// Set statement timeout.
	if _, err := conn.Exec(r.Context(), "SET LOCAL statement_timeout = '5s'"); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to set statement_timeout",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to configure connection")
		return
	}

	// Check if primary or replica.
	var isRecovery bool
	if err := conn.QueryRow(r.Context(), "SELECT pg_is_in_recovery()").Scan(&isRecovery); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to check recovery status",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to check recovery status")
		return
	}

	resp := ReplicationResponse{
		InstanceID: instanceID,
	}

	if isRecovery {
		resp.Role = "replica"

		// Query WAL receiver info.
		rows, err := conn.Query(r.Context(),
			"SELECT sender_host, sender_port, status FROM pg_stat_wal_receiver")
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query wal receiver",
				"instance_id", instanceID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error",
				"failed to query WAL receiver")
			return
		}
		defer rows.Close()

		if rows.Next() {
			var wr WALReceiverInfo
			if err := rows.Scan(&wr.SenderHost, &wr.SenderPort, &wr.Status); err != nil {
				s.logger.ErrorContext(r.Context(), "failed to scan wal receiver",
					"instance_id", instanceID, "error", err)
				writeError(w, http.StatusInternalServerError, "internal_error",
					"failed to read WAL receiver data")
				return
			}
			resp.WALReceiver = &wr
		}
		if err := rows.Err(); err != nil {
			s.logger.ErrorContext(r.Context(), "wal receiver rows error",
				"instance_id", instanceID, "error", err)
		}
	} else {
		resp.Role = "primary"

		// Query replicas.
		replRows, err := conn.Query(r.Context(), `SELECT client_addr::text, application_name, state, sync_state,
       pg_wal_lsn_diff(pg_current_wal_insert_lsn(), sent_lsn) AS pending_bytes,
       pg_wal_lsn_diff(sent_lsn, write_lsn) AS write_bytes,
       pg_wal_lsn_diff(write_lsn, flush_lsn) AS flush_bytes,
       pg_wal_lsn_diff(flush_lsn, replay_lsn) AS replay_bytes,
       COALESCE(write_lag::text, '') AS write_lag,
       COALESCE(flush_lag::text, '') AS flush_lag,
       COALESCE(replay_lag::text, '') AS replay_lag
FROM pg_stat_replication`)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query replication",
				"instance_id", instanceID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error",
				"failed to query replication status")
			return
		}
		defer replRows.Close()

		for replRows.Next() {
			var ri ReplicaInfo
			var clientAddr *string
			if err := replRows.Scan(
				&clientAddr,
				&ri.ApplicationName, &ri.State, &ri.SyncState,
				&ri.Lag.PendingBytes, &ri.Lag.WriteBytes,
				&ri.Lag.FlushBytes, &ri.Lag.ReplayBytes,
				&ri.Lag.WriteLag, &ri.Lag.FlushLag, &ri.Lag.ReplayLag,
			); err != nil {
				s.logger.ErrorContext(r.Context(), "failed to scan replication row",
					"instance_id", instanceID, "error", err)
				continue
			}
			if clientAddr != nil {
				ri.ClientAddr = *clientAddr
			}
			resp.Replicas = append(resp.Replicas, ri)
		}
		if err := replRows.Err(); err != nil {
			s.logger.ErrorContext(r.Context(), "replication rows error",
				"instance_id", instanceID, "error", err)
		}

		// Query replication slots.
		slotRows, err := conn.Query(r.Context(), `SELECT slot_name, slot_type, active,
       pg_wal_lsn_diff(pg_current_wal_insert_lsn(), restart_lsn) AS wal_retained_bytes
FROM pg_replication_slots`)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query replication slots",
				"instance_id", instanceID, "error", err)
			// Non-fatal: still return what we have.
		} else {
			defer slotRows.Close()
			for slotRows.Next() {
				var si SlotInfo
				if err := slotRows.Scan(&si.SlotName, &si.SlotType, &si.Active, &si.WALRetainedBytes); err != nil {
					s.logger.ErrorContext(r.Context(), "failed to scan slot row",
						"instance_id", instanceID, "error", err)
					continue
				}
				resp.Slots = append(resp.Slots, si)
			}
			if err := slotRows.Err(); err != nil {
				s.logger.ErrorContext(r.Context(), "slot rows error",
					"instance_id", instanceID, "error", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, Envelope{Data: resp})
}
