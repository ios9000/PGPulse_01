package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/settings"
)

// handleSettingsHistory returns a list of settings snapshots for an instance.
// GET /instances/{id}/settings/history
func (s *APIServer) handleSettingsHistory(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.snapshotStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings snapshot storage not configured")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	metas, err := s.snapshotStore.ListSnapshots(r.Context(), instanceID, limit)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to list settings snapshots",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list snapshots")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: metas,
		Meta: map[string]int{"count": len(metas)},
	})
}

// handleSettingsDiff compares two settings snapshots and returns the differences.
// GET /instances/{id}/settings/diff?from={id}&to={id}
func (s *APIServer) handleSettingsDiff(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.snapshotStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings snapshot storage not configured")
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_param", "from and to snapshot IDs are required")
		return
	}

	fromID, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_param", "from must be an integer")
		return
	}
	toID, err := strconv.ParseInt(toStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_param", "to must be an integer")
		return
	}

	fromSnap, err := s.snapshotStore.GetSnapshot(r.Context(), fromID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "from snapshot not found")
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to get from snapshot", "id", fromID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot")
		return
	}

	toSnap, err := s.snapshotStore.GetSnapshot(r.Context(), toID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "to snapshot not found")
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to get to snapshot", "id", toID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot")
		return
	}

	// Verify both snapshots belong to the requested instance.
	if fromSnap.InstanceID != instanceID || toSnap.InstanceID != instanceID {
		writeError(w, http.StatusBadRequest, "invalid_param", "snapshots do not belong to this instance")
		return
	}

	diff := settings.DiffSnapshots(fromSnap.Settings, toSnap.Settings)

	writeJSON(w, http.StatusOK, Envelope{
		Data: diff,
		Meta: map[string]any{
			"from_id":         fromID,
			"to_id":           toID,
			"from_captured_at": fromSnap.CapturedAt,
			"to_captured_at":   toSnap.CapturedAt,
		},
	})
}

// handleSettingsLatest returns the latest settings snapshot for an instance.
// GET /instances/{id}/settings/latest
func (s *APIServer) handleSettingsLatest(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.snapshotStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings snapshot storage not configured")
		return
	}

	snap, err := s.snapshotStore.LatestSnapshot(r.Context(), instanceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "no snapshots found for this instance")
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to get latest snapshot",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot")
		return
	}

	writeJSON(w, http.StatusOK, Envelope{Data: snap})
}

// handleSettingsPendingRestart returns settings that require a restart from the latest snapshot.
// GET /instances/{id}/settings/pending-restart
func (s *APIServer) handleSettingsPendingRestart(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.snapshotStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings snapshot storage not configured")
		return
	}

	snap, err := s.snapshotStore.LatestSnapshot(r.Context(), instanceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, Envelope{
				Data: []string{},
				Meta: map[string]int{"count": 0},
			})
			return
		}
		s.logger.ErrorContext(r.Context(), "failed to get latest snapshot",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot")
		return
	}

	var pending []settings.SettingChange
	for name, val := range snap.Settings {
		if val.PendingRestart {
			pending = append(pending, settings.SettingChange{
				Name:     name,
				NewValue: val.Setting,
				Unit:     val.Unit,
				Source:   val.Source,
			})
		}
	}
	if pending == nil {
		pending = []settings.SettingChange{}
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: pending,
		Meta: map[string]int{"count": len(pending)},
	})
}

// handleSettingsManualSnapshot triggers an immediate settings snapshot capture.
// POST /instances/{id}/settings/snapshot
func (s *APIServer) handleSettingsManualSnapshot(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.snapshotStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings snapshot storage not configured")
		return
	}

	if s.connProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "connection provider not available")
		return
	}

	conn, err := s.connProvider.ConnFor(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get connection for settings snapshot",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to connect to instance")
		return
	}
	defer func() { _ = conn.Close(r.Context()) }()

	// Collect settings from pg_settings.
	rows, err := conn.Query(r.Context(), `
		SELECT name, setting, COALESCE(unit, ''), source, pending_restart
		FROM pg_settings
		ORDER BY name
	`)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to query pg_settings",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to query settings")
		return
	}
	defer rows.Close()

	settingsMap := make(map[string]settings.SettingValue)
	for rows.Next() {
		var name, setting, unit, source string
		var pendingRestart bool
		if err := rows.Scan(&name, &setting, &unit, &source, &pendingRestart); err != nil {
			continue
		}
		settingsMap[name] = settings.SettingValue{
			Setting:        setting,
			Unit:           unit,
			Source:         source,
			PendingRestart: pendingRestart,
		}
	}
	if err := rows.Err(); err != nil {
		s.logger.ErrorContext(r.Context(), "error reading pg_settings rows",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to read settings")
		return
	}

	// Get PG version.
	var pgVersion string
	_ = conn.QueryRow(r.Context(), "SHOW server_version").Scan(&pgVersion)

	snap := settings.Snapshot{
		InstanceID:  instanceID,
		CapturedAt:  time.Now(),
		TriggerType: "manual",
		PGVersion:   pgVersion,
		Settings:    settingsMap,
	}

	if err := s.snapshotStore.SaveSnapshot(r.Context(), snap); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to save manual snapshot",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to save snapshot")
		return
	}

	writeJSON(w, http.StatusCreated, Envelope{
		Data: map[string]any{
			"instance_id":    instanceID,
			"settings_count": len(settingsMap),
			"pg_version":     pgVersion,
		},
	})
}
