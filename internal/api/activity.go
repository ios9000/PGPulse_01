package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// WaitEventsResponse contains wait event statistics for an instance.
type WaitEventsResponse struct {
	Events        []WaitEvent `json:"events"`
	TotalBackends int         `json:"total_backends"`
}

// WaitEvent represents a single wait event type with its count.
type WaitEvent struct {
	WaitEventType string `json:"wait_event_type"`
	WaitEvent     string `json:"wait_event"`
	Count         int    `json:"count"`
}

// LongTransactionsResponse contains long-running transactions for an instance.
type LongTransactionsResponse struct {
	Transactions []LongTransaction `json:"transactions"`
}

// LongTransaction describes a long-running transaction.
type LongTransaction struct {
	PID             int       `json:"pid"`
	Username        string    `json:"username"`
	Database        string    `json:"database"`
	ApplicationName string    `json:"application_name"`
	State           string    `json:"state"`
	Waiting         bool      `json:"waiting"`
	DurationSeconds float64   `json:"duration_seconds"`
	Query           string    `json:"query"`
	XactStart       time.Time `json:"xact_start"`
}

// handleWaitEvents returns wait event statistics for a monitored instance.
func (s *APIServer) handleWaitEvents(w http.ResponseWriter, r *http.Request) {
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
		s.logger.ErrorContext(r.Context(), "failed to get connection for wait events",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusBadGateway, "connection_error",
			"failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	if _, err := conn.Exec(r.Context(), "SET LOCAL statement_timeout = '5s'"); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to set statement_timeout",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to configure connection")
		return
	}

	// Get total backends.
	var totalBackends int
	if err := conn.QueryRow(r.Context(),
		"SELECT count(*) FROM pg_stat_activity WHERE pid != pg_backend_pid()").Scan(&totalBackends); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to count backends",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to count backends")
		return
	}

	// Get wait events.
	rows, err := conn.Query(r.Context(), `SELECT wait_event_type, wait_event, count(*) AS count
FROM pg_stat_activity
WHERE wait_event IS NOT NULL AND pid != pg_backend_pid()
GROUP BY 1, 2 ORDER BY 3 DESC`)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to query wait events",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query wait events")
		return
	}
	defer rows.Close()

	events := []WaitEvent{}
	for rows.Next() {
		var e WaitEvent
		if err := rows.Scan(&e.WaitEventType, &e.WaitEvent, &e.Count); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to scan wait event",
				"instance_id", instanceID, "error", err)
			continue
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		s.logger.ErrorContext(r.Context(), "wait events rows error",
			"instance_id", instanceID, "error", err)
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: WaitEventsResponse{
			Events:        events,
			TotalBackends: totalBackends,
		},
	})
}

// handleLongTransactions returns long-running transactions for a monitored instance.
func (s *APIServer) handleLongTransactions(w http.ResponseWriter, r *http.Request) {
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

	thresholdSeconds := 5.0
	if ts := r.URL.Query().Get("threshold_seconds"); ts != "" {
		v, err := strconv.ParseFloat(ts, 64)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "bad_request",
				"invalid 'threshold_seconds' parameter")
			return
		}
		thresholdSeconds = v
	}

	conn, err := s.connProvider.ConnFor(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get connection for long transactions",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusBadGateway, "connection_error",
			"failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	if _, err := conn.Exec(r.Context(), "SET LOCAL statement_timeout = '5s'"); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to set statement_timeout",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to configure connection")
		return
	}

	rows, err := conn.Query(r.Context(), `SELECT pid, usename, datname, COALESCE(application_name, '') AS application_name, state, wait_event IS NOT NULL AS waiting,
       EXTRACT(EPOCH FROM (now() - xact_start)) AS duration_seconds,
       LEFT(query, 200) AS query, xact_start
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
  AND pid != pg_backend_pid()
  AND EXTRACT(EPOCH FROM (now() - xact_start)) > $1
ORDER BY duration_seconds DESC`, thresholdSeconds)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to query long transactions",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query long transactions")
		return
	}
	defer rows.Close()

	txns := []LongTransaction{}
	for rows.Next() {
		var lt LongTransaction
		if err := rows.Scan(
			&lt.PID, &lt.Username, &lt.Database, &lt.ApplicationName, &lt.State, &lt.Waiting,
			&lt.DurationSeconds, &lt.Query, &lt.XactStart,
		); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to scan long transaction",
				"instance_id", instanceID, "error", err)
			continue
		}
		txns = append(txns, lt)
	}
	if err := rows.Err(); err != nil {
		s.logger.ErrorContext(r.Context(), "long transactions rows error",
			"instance_id", instanceID, "error", err)
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: LongTransactionsResponse{Transactions: txns},
	})
}
