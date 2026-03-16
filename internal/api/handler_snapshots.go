package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/statements"
)

// handleListSnapshots returns a paginated list of PGSS snapshots for an instance.
// GET /instances/{id}/snapshots
func (s *APIServer) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.pgssStore == nil {
		writeError(w, http.StatusNotFound, "not_found", "statement snapshots not enabled")
		return
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	opts := statements.SnapshotListOptions{
		Limit:  limit,
		Offset: offset,
	}

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.From = &t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.To = &t
		}
	}

	snaps, total, err := s.pgssStore.ListSnapshots(r.Context(), instanceID, opts)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to list pgss snapshots",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list snapshots")
		return
	}

	if snaps == nil {
		snaps = []statements.Snapshot{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"snapshots": snaps,
		"total":     total,
	})
}

// handleGetSnapshot returns a single snapshot with its entries.
// GET /instances/{id}/snapshots/{snapId}
func (s *APIServer) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.pgssStore == nil {
		writeError(w, http.StatusNotFound, "not_found", "statement snapshots not enabled")
		return
	}

	snapIDStr := chi.URLParam(r, "snapId")
	snapID, err := strconv.ParseInt(snapIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_param", "snapId must be an integer")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	snap, err := s.pgssStore.GetSnapshot(r.Context(), snapID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get pgss snapshot",
			"id", snapID, "error", err)
		writeError(w, http.StatusNotFound, "not_found", "snapshot not found")
		return
	}

	if snap.InstanceID != instanceID {
		writeError(w, http.StatusNotFound, "not_found", "snapshot not found for this instance")
		return
	}

	entries, totalEntries, err := s.pgssStore.GetSnapshotEntries(r.Context(), snapID, limit, offset)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get pgss snapshot entries",
			"snapshot_id", snapID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot entries")
		return
	}

	if entries == nil {
		entries = []statements.SnapshotEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot":      snap,
		"entries":       entries,
		"total_entries": totalEntries,
	})
}

// handleSnapshotDiff computes the diff between two snapshots.
// GET /instances/{id}/snapshots/diff?from={id}&to={id} or ?from_time=...&to_time=...
func (s *APIServer) handleSnapshotDiff(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.pgssStore == nil {
		writeError(w, http.StatusNotFound, "not_found", "statement snapshots not enabled")
		return
	}

	var fromSnap, toSnap *statements.Snapshot

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	fromTimeStr := r.URL.Query().Get("from_time")
	toTimeStr := r.URL.Query().Get("to_time")

	if fromStr != "" && toStr != "" {
		// By snapshot IDs.
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

		fromSnap, err = s.pgssStore.GetSnapshot(r.Context(), fromID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "from snapshot not found")
			return
		}
		toSnap, err = s.pgssStore.GetSnapshot(r.Context(), toID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "to snapshot not found")
			return
		}
	} else if fromTimeStr != "" && toTimeStr != "" {
		// By time range.
		fromTime, err := time.Parse(time.RFC3339, fromTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "from_time must be RFC3339")
			return
		}
		toTime, err := time.Parse(time.RFC3339, toTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "to_time must be RFC3339")
			return
		}

		snaps, _, err := s.pgssStore.ListSnapshots(r.Context(), instanceID, statements.SnapshotListOptions{
			From:  &fromTime,
			To:    &toTime,
			Limit: 1000,
		})
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to list snapshots for diff",
				"instance_id", instanceID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to list snapshots")
			return
		}
		if len(snaps) < 2 {
			writeError(w, http.StatusNotFound, "not_found", "need at least 2 snapshots in time range for diff")
			return
		}
		// ListSnapshots returns DESC order, so last is oldest, first is newest.
		fromSnap = &snaps[len(snaps)-1]
		toSnap = &snaps[0]
	} else {
		writeError(w, http.StatusBadRequest, "invalid_param",
			"provide from/to snapshot IDs or from_time/to_time")
		return
	}

	if fromSnap.InstanceID != instanceID || toSnap.InstanceID != instanceID {
		writeError(w, http.StatusBadRequest, "invalid_param", "snapshots do not belong to this instance")
		return
	}

	diff := s.computePGSSDiff(w, r, fromSnap, toSnap)
	if diff == nil {
		return // error already written
	}

	writeJSON(w, http.StatusOK, diff)
}

// handleLatestDiff computes the diff between the two most recent snapshots.
// GET /instances/{id}/snapshots/latest-diff
func (s *APIServer) handleLatestDiff(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.pgssStore == nil {
		writeError(w, http.StatusNotFound, "not_found", "statement snapshots not enabled")
		return
	}

	snaps, err := s.pgssStore.GetLatestSnapshots(r.Context(), instanceID, 2)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get latest snapshots",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get latest snapshots")
		return
	}

	if len(snaps) < 2 {
		writeError(w, http.StatusNotFound, "not_found", "need at least 2 snapshots for diff")
		return
	}

	// GetLatestSnapshots returns DESC order: [newest, oldest].
	fromSnap := &snaps[1]
	toSnap := &snaps[0]

	diff := s.computePGSSDiff(w, r, fromSnap, toSnap)
	if diff == nil {
		return // error already written
	}

	writeJSON(w, http.StatusOK, diff)
}

// handleQueryInsights returns time-series data for a single query.
// GET /instances/{id}/query-insights/{queryid}
func (s *APIServer) handleQueryInsights(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.pgssStore == nil {
		writeError(w, http.StatusNotFound, "not_found", "statement snapshots not enabled")
		return
	}

	queryIDStr := chi.URLParam(r, "queryid")
	queryID, err := strconv.ParseInt(queryIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_param", "queryid must be an integer")
		return
	}

	now := time.Now()
	from := now.Add(-24 * time.Hour)
	to := now

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	entries, snapshots, err := s.pgssStore.GetEntriesForQuery(r.Context(), instanceID, queryID, from, to)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get entries for query",
			"instance_id", instanceID, "queryid", queryID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get query data")
		return
	}

	insight := statements.BuildQueryInsight(entries, snapshots)
	if insight == nil {
		writeError(w, http.StatusNotFound, "not_found", "no data found for this query")
		return
	}

	writeJSON(w, http.StatusOK, insight)
}

// handleWorkloadReport generates a workload report from snapshot diffs.
// GET /instances/{id}/workload-report
func (s *APIServer) handleWorkloadReport(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.pgssStore == nil {
		writeError(w, http.StatusNotFound, "not_found", "statement snapshots not enabled")
		return
	}

	var fromSnap, toSnap *statements.Snapshot

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	fromTimeStr := r.URL.Query().Get("from_time")
	toTimeStr := r.URL.Query().Get("to_time")

	if fromStr != "" && toStr != "" {
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

		fromSnap, err = s.pgssStore.GetSnapshot(r.Context(), fromID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "from snapshot not found")
			return
		}
		toSnap, err = s.pgssStore.GetSnapshot(r.Context(), toID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "to snapshot not found")
			return
		}
	} else if fromTimeStr != "" && toTimeStr != "" {
		fromTime, err := time.Parse(time.RFC3339, fromTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "from_time must be RFC3339")
			return
		}
		toTime, err := time.Parse(time.RFC3339, toTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "to_time must be RFC3339")
			return
		}

		snaps, _, err := s.pgssStore.ListSnapshots(r.Context(), instanceID, statements.SnapshotListOptions{
			From:  &fromTime,
			To:    &toTime,
			Limit: 1000,
		})
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to list snapshots for report",
				"instance_id", instanceID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to list snapshots")
			return
		}
		if len(snaps) < 2 {
			writeError(w, http.StatusNotFound, "not_found", "need at least 2 snapshots for report")
			return
		}
		fromSnap = &snaps[len(snaps)-1]
		toSnap = &snaps[0]
	} else {
		// Default: latest 2 snapshots.
		snaps, err := s.pgssStore.GetLatestSnapshots(r.Context(), instanceID, 2)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to get latest snapshots for report",
				"instance_id", instanceID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshots")
			return
		}
		if len(snaps) < 2 {
			writeError(w, http.StatusNotFound, "not_found", "need at least 2 snapshots for report")
			return
		}
		fromSnap = &snaps[1]
		toSnap = &snaps[0]
	}

	if fromSnap.InstanceID != instanceID || toSnap.InstanceID != instanceID {
		writeError(w, http.StatusBadRequest, "invalid_param", "snapshots do not belong to this instance")
		return
	}

	// Load entries for both snapshots.
	fromEntries, _, err := s.pgssStore.GetSnapshotEntries(r.Context(), fromSnap.ID, 0, 0)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get from snapshot entries",
			"snapshot_id", fromSnap.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot entries")
		return
	}
	toEntries, _, err := s.pgssStore.GetSnapshotEntries(r.Context(), toSnap.ID, 0, 0)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get to snapshot entries",
			"snapshot_id", toSnap.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot entries")
		return
	}

	diff := statements.ComputeDiff(*fromSnap, *toSnap, fromEntries, toEntries, statements.DiffOptions{
		SortBy: "total_exec_time",
	})

	topN := s.stmtSnapshotsCfg.TopN
	if v := r.URL.Query().Get("top_n"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			topN = n
		}
	}

	report := statements.GenerateReport(diff, topN)
	report.InstanceID = instanceID

	writeJSON(w, http.StatusOK, report)
}

// handleManualSnapshotCapture triggers an immediate PGSS snapshot capture.
// POST /instances/{id}/snapshots/capture
func (s *APIServer) handleManualSnapshotCapture(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	if !s.instanceExists(instanceID) {
		writeError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("instance '%s' not found", instanceID))
		return
	}

	if s.pgssCapturer == nil {
		writeError(w, http.StatusNotFound, "not_found", "statement snapshots not enabled")
		return
	}

	snap, err := s.pgssCapturer.CaptureInstance(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to capture pgss snapshot",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal",
			fmt.Sprintf("failed to capture snapshot: %s", err))
		return
	}

	writeJSON(w, http.StatusCreated, snap)
}

// computePGSSDiff is a helper that loads entries and computes a diff between two snapshots.
// On error it writes the HTTP error response and returns nil.
func (s *APIServer) computePGSSDiff(w http.ResponseWriter, r *http.Request, fromSnap, toSnap *statements.Snapshot) *statements.DiffResult {
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "total_exec_time"
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Load all entries for both snapshots (limit=0 means all).
	fromEntries, _, err := s.pgssStore.GetSnapshotEntries(r.Context(), fromSnap.ID, 0, 0)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get from snapshot entries",
			"snapshot_id", fromSnap.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot entries")
		return nil
	}

	toEntries, _, err := s.pgssStore.GetSnapshotEntries(r.Context(), toSnap.ID, 0, 0)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get to snapshot entries",
			"snapshot_id", toSnap.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot entries")
		return nil
	}

	diff := statements.ComputeDiff(*fromSnap, *toSnap, fromEntries, toEntries, statements.DiffOptions{
		SortBy: sortBy,
		Limit:  limit,
		Offset: offset,
	})

	return diff
}
