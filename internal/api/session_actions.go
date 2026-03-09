package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/auth"
)

// sessionActionResponse is the response body for session cancel/terminate.
type sessionActionResponse struct {
	PID     int    `json:"pid"`
	Action  string `json:"action"`
	Success bool   `json:"success"`
}

// handleSessionCancel cancels the current query on the specified backend PID.
// POST /instances/{id}/sessions/{pid}/cancel
func (s *APIServer) handleSessionCancel(w http.ResponseWriter, r *http.Request) {
	s.handleSessionAction(w, r, "cancel")
}

// handleSessionTerminate terminates the backend with the specified PID.
// POST /instances/{id}/sessions/{pid}/terminate
func (s *APIServer) handleSessionTerminate(w http.ResponseWriter, r *http.Request) {
	s.handleSessionAction(w, r, "terminate")
}

func (s *APIServer) handleSessionAction(w http.ResponseWriter, r *http.Request, action string) {
	instanceID := chi.URLParam(r, "id")
	pidStr := chi.URLParam(r, "pid")

	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid pid")
		return
	}

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found", "instance not found")
		return
	}

	if s.connProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "connection provider not available")
		return
	}

	ctx := r.Context()

	conn, err := s.connProvider.ConnFor(ctx, instanceID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get connection for session action",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusBadGateway, "connection_error", "failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(ctx) }()

	// Safety guard 1: never kill PGPulse's own backend
	var ownPID int
	_ = conn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&ownPID)
	if pid == ownPID {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot target PGPulse's own backend")
		return
	}

	// Safety guard 2: never kill superuser session
	var isSuper bool
	err = conn.QueryRow(ctx, `
		SELECT COALESCE(u.usesuper, false)
		FROM pg_stat_activity a
		LEFT JOIN pg_user u ON u.usename = a.usename
		WHERE a.pid = $1
	`, pid).Scan(&isSuper)
	if err != nil {
		// pid not found — already gone, treat as success=false
		writeJSON(w, http.StatusOK, Envelope{Data: sessionActionResponse{
			PID: pid, Action: action, Success: false,
		}})
		return
	}
	if isSuper {
		writeError(w, http.StatusForbidden, "forbidden", "cannot terminate superuser session")
		return
	}

	// Execute the action
	// fn is a hardcoded function name, not user input — safe
	var fn string
	switch action {
	case "cancel":
		fn = "pg_cancel_backend($1)"
	default:
		fn = "pg_terminate_backend($1)"
	}
	var success bool
	_ = conn.QueryRow(ctx, "SELECT "+fn, pid).Scan(&success)

	// Audit log
	username := "anonymous"
	if claims := auth.ClaimsFromContext(ctx); claims != nil {
		username = claims.Username
	}
	slog.Info("session action",
		"instance", instanceID,
		"pid", pid,
		"action", action,
		"success", success,
		"user", username)

	writeJSON(w, http.StatusOK, Envelope{Data: sessionActionResponse{
		PID: pid, Action: action, Success: success,
	}})
}
