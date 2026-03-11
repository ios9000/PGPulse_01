package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type explainRequest struct {
	Database string `json:"database"`
	Query    string `json:"query"`
	Analyze  bool   `json:"analyze"`
	Buffers  bool   `json:"buffers"`
}

// handleExplain runs EXPLAIN on a user-supplied query against the specified database.
// POST /instances/{id}/explain
func (s *APIServer) handleExplain(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")

	var req explainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Database == "" || req.Query == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "database and query are required")
		return
	}

	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found", "instance not found")
		return
	}

	if s.connProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "not_available",
			"instance connection provider not configured")
		return
	}

	ctx := r.Context()

	conn, err := s.connProvider.ConnForDB(ctx, instanceID, req.Database)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to connect for explain",
			"instance_id", instanceID, "database", req.Database, "error", err)
		writeError(w, http.StatusBadGateway, "connection_error",
			fmt.Sprintf("cannot connect to database %q: %v", req.Database, err))
		return
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, "SET statement_timeout = '30s'"); err != nil {
		s.logger.ErrorContext(ctx, "failed to set statement_timeout for explain",
			"instance_id", instanceID, "error", err)
	}
	if _, err := conn.Exec(ctx, "SET application_name = 'pgpulse_explain'"); err != nil {
		s.logger.ErrorContext(ctx, "failed to set application_name for explain",
			"instance_id", instanceID, "error", err)
	}

	// Build EXPLAIN command.
	// NOTE: The query is intentionally not parameterized — EXPLAIN cannot use $1
	// for the target SQL statement. The auth gate (instance_management permission)
	// is the protection layer; only DBA/super_admin roles can reach this endpoint.
	opts := []string{"FORMAT JSON"}
	if req.Analyze {
		opts = append(opts, "ANALYZE")
	}
	if req.Buffers {
		opts = append(opts, "BUFFERS")
	}
	explainSQL := fmt.Sprintf("EXPLAIN (%s) %s", strings.Join(opts, ", "), req.Query)

	var planJSON []byte
	if err := conn.QueryRow(ctx, explainSQL).Scan(&planJSON); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "explain_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(planJSON)
}
