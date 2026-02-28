package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/config"
)

// InstanceResponse is the JSON representation of a monitored PostgreSQL instance.
type InstanceResponse struct {
	ID          string `json:"id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

func (s *APIServer) handleListInstances(w http.ResponseWriter, r *http.Request) {
	items := make([]InstanceResponse, 0, len(s.instances))
	for _, inst := range s.instances {
		items = append(items, toInstanceResponse(inst))
	}
	writeJSON(w, http.StatusOK, Envelope{
		Data: items,
		Meta: map[string]int{"count": len(items)},
	})
}

func (s *APIServer) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	for _, inst := range s.instances {
		if inst.ID == id {
			writeJSON(w, http.StatusOK, Envelope{Data: toInstanceResponse(inst)})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found",
		fmt.Sprintf("instance '%s' not found", id))
}

// toInstanceResponse maps config.InstanceConfig to the API response shape.
// Host and port are derived from the DSN (postgres://... URL format).
func toInstanceResponse(ic config.InstanceConfig) InstanceResponse {
	host, port := extractHostPort(ic.DSN)
	enabled := ic.Enabled == nil || *ic.Enabled
	return InstanceResponse{
		ID:          ic.ID,
		Host:        host,
		Port:        port,
		Description: ic.Description,
		Enabled:     enabled,
	}
}

// extractHostPort parses a postgres URL DSN and returns host and port.
// Returns empty string and 0 if the DSN cannot be parsed or is not URL format.
func extractHostPort(dsn string) (string, int) {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return "", 0
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
