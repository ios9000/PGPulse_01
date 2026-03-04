package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/storage"
)

// InstanceCRUDResponse is the JSON shape returned for instance CRUD operations.
type InstanceCRUDResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	DSN       string `json:"dsn"` // password masked
	Enabled   bool   `json:"enabled"`
	Source    string `json:"source"`
	MaxConns  int    `json:"max_conns"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// toInstanceCRUDResponse converts an InstanceRecord to an API response with masked DSN.
func toInstanceCRUDResponse(r storage.InstanceRecord) InstanceCRUDResponse {
	return InstanceCRUDResponse{
		ID:        r.ID,
		Name:      r.Name,
		Host:      r.Host,
		Port:      r.Port,
		DSN:       maskDSN(r.DSN),
		Enabled:   r.Enabled,
		Source:    r.Source,
		MaxConns:  r.MaxConns,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
}

// maskDSN hides the password in a postgres:// URL DSN.
// Key=value DSNs have their password field masked.
func maskDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		// Try key=value format: mask the password= part.
		if strings.Contains(dsn, "password=") {
			parts := strings.Fields(dsn)
			for i, p := range parts {
				if strings.HasPrefix(p, "password=") {
					parts[i] = "password=*****"
				}
			}
			return strings.Join(parts, " ")
		}
		return dsn
	}

	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), "*****")
		}
	}
	return u.String()
}

// createInstanceRequest is the JSON body for creating an instance.
type createInstanceRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	DSN      string `json:"dsn"`
	Enabled  *bool  `json:"enabled"`
	MaxConns int    `json:"max_conns"`
}

func (s *APIServer) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	if s.instanceStore == nil {
		writeError(w, http.StatusServiceUnavailable, "no_store", "instance store not available")
		return
	}

	var req createInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "id is required")
		return
	}
	if req.DSN == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "dsn is required")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.MaxConns == 0 {
		req.MaxConns = 5
	}

	host, port := parseHostPort(req.DSN)

	rec := storage.InstanceRecord{
		ID:       req.ID,
		Name:     req.Name,
		DSN:      req.DSN,
		Host:     host,
		Port:     port,
		Enabled:  enabled,
		Source:   "manual",
		MaxConns: req.MaxConns,
	}

	created, err := s.instanceStore.Create(r.Context(), rec)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "duplicate", fmt.Sprintf("instance %q already exists", req.ID))
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to create instance", "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to create instance")
		return
	}

	writeJSON(w, http.StatusCreated, Envelope{Data: toInstanceCRUDResponse(*created)})
}

// updateInstanceRequest is the JSON body for updating an instance.
type updateInstanceRequest struct {
	Name     *string `json:"name"`
	DSN      *string `json:"dsn"`
	Enabled  *bool   `json:"enabled"`
	MaxConns *int    `json:"max_conns"`
}

func (s *APIServer) handleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	if s.instanceStore == nil {
		writeError(w, http.StatusServiceUnavailable, "no_store", "instance store not available")
		return
	}

	id := chi.URLParam(r, "id")

	existing, err := s.instanceStore.Get(r.Context(), id)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get instance for update", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to get instance")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("instance %q not found", id))
		return
	}

	var req updateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.DSN != nil {
		existing.DSN = *req.DSN
		existing.Host, existing.Port = parseHostPort(*req.DSN)
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.MaxConns != nil {
		existing.MaxConns = *req.MaxConns
	}

	updated, err := s.instanceStore.Update(r.Context(), *existing)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to update instance", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to update instance")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: toInstanceCRUDResponse(*updated)})
}

func (s *APIServer) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	if s.instanceStore == nil {
		writeError(w, http.StatusServiceUnavailable, "no_store", "instance store not available")
		return
	}

	id := chi.URLParam(r, "id")

	if err := s.instanceStore.Delete(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("instance %q not found", id))
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to delete instance", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "server_error", "failed to delete instance")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: map[string]string{"id": id, "status": "deleted"}})
}

// handleBulkImport accepts CSV with columns: id, name, dsn, enabled, max_conns.
// Skips rows with errors and returns a summary.
func (s *APIServer) handleBulkImport(w http.ResponseWriter, r *http.Request) {
	if s.instanceStore == nil {
		writeError(w, http.StatusServiceUnavailable, "no_store", "instance store not available")
		return
	}

	reader := csv.NewReader(r.Body)
	reader.TrimLeadingSpace = true

	var imported, skipped int
	var errors []string

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Sprintf("CSV parse error: %v", err))
			skipped++
			continue
		}

		if len(record) < 3 {
			errors = append(errors, fmt.Sprintf("row needs at least 3 fields (id, name, dsn), got %d", len(record)))
			skipped++
			continue
		}

		id := strings.TrimSpace(record[0])
		name := strings.TrimSpace(record[1])
		dsn := strings.TrimSpace(record[2])

		if id == "" || dsn == "" {
			errors = append(errors, fmt.Sprintf("skipping row: id and dsn are required (id=%q)", id))
			skipped++
			continue
		}

		enabled := true
		if len(record) > 3 {
			if v := strings.TrimSpace(record[3]); v != "" {
				enabled, _ = strconv.ParseBool(v)
			}
		}

		maxConns := 5
		if len(record) > 4 {
			if v := strings.TrimSpace(record[4]); v != "" {
				if mc, err := strconv.Atoi(v); err == nil && mc > 0 {
					maxConns = mc
				}
			}
		}

		host, port := parseHostPort(dsn)

		rec := storage.InstanceRecord{
			ID:       id,
			Name:     name,
			DSN:      dsn,
			Host:     host,
			Port:     port,
			Enabled:  enabled,
			Source:   "csv_import",
			MaxConns: maxConns,
		}

		if _, err := s.instanceStore.Create(r.Context(), rec); err != nil {
			errors = append(errors, fmt.Sprintf("instance %q: %v", id, err))
			skipped++
			continue
		}

		imported++
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]any{
			"imported": imported,
			"skipped":  skipped,
			"errors":   errors,
		},
	})
}

// handleTestConnection attempts to connect to the DSN provided in the request body
// and runs SELECT version().
func (s *APIServer) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DSN string `json:"dsn"`
	}

	// If DSN is in body, use it; otherwise look up the instance by URL param.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DSN == "" {
		id := chi.URLParam(r, "id")
		if id != "" && s.instanceStore != nil {
			inst, err := s.instanceStore.Get(r.Context(), id)
			if err != nil || inst == nil {
				writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("instance %q not found", id))
				return
			}
			req.DSN = inst.DSN
		} else {
			writeError(w, http.StatusBadRequest, "validation_error", "dsn is required")
			return
		}
	}

	testCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(testCtx, req.DSN)
	if err != nil {
		writeJSON(w, http.StatusOK, Envelope{
			Data: map[string]any{
				"success": false,
				"error":   err.Error(),
			},
		})
		return
	}
	defer conn.Close(context.Background())

	var version string
	if err := conn.QueryRow(testCtx, "SELECT version()").Scan(&version); err != nil {
		writeJSON(w, http.StatusOK, Envelope{
			Data: map[string]any{
				"success": false,
				"error":   fmt.Sprintf("connected but SELECT version() failed: %v", err),
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: map[string]any{
			"success": true,
			"version": version,
		},
	})
}

// parseHostPort extracts host and port from a postgres:// URL DSN.
// Returns empty string and 5432 for unparseable DSNs.
func parseHostPort(dsn string) (string, int) {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return "", 5432
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		return host, 5432
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 5432
	}
	return host, port
}
