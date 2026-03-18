package api

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/statements"
)

//go:embed templates/workload_report.html
var reportTemplateFS embed.FS

var reportFuncMap = template.FuncMap{
	"formatInt64": func(n int64) string {
		return formatNumberCommas(n)
	},
	"formatDurationMs": func(ms float64) string {
		if ms < 1 {
			return "< 1ms"
		}
		if ms < 1000 {
			return fmt.Sprintf("%.1f ms", ms)
		}
		secs := ms / 1000
		if secs < 60 {
			return fmt.Sprintf("%.1f s", secs)
		}
		mins := secs / 60
		if mins < 60 {
			return fmt.Sprintf("%.1f min", mins)
		}
		hours := mins / 60
		return fmt.Sprintf("%.1f h", hours)
	},
	"formatTime": func(t time.Time) string {
		return t.Format("Jan 2, 2006 3:04 PM")
	},
	"truncate": func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "..."
	},
	"add": func(a, b int) int {
		return a + b
	},
	"makeSection": func(title string, entries []statements.DiffEntry) map[string]any {
		return map[string]any{
			"Title":   title,
			"Entries": entries,
		}
	},
}

var reportTemplate = template.Must(
	template.New("workload_report.html").Funcs(reportFuncMap).ParseFS(
		reportTemplateFS, "templates/workload_report.html",
	),
)

func formatNumberCommas(n int64) string {
	if n < 0 {
		return "-" + formatNumberCommas(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// htmlReportData wraps WorkloadReport with additional template fields.
type htmlReportData struct {
	statements.WorkloadReport
	GeneratedAt string
}

// handleWorkloadReportHTML renders the workload report as a standalone HTML file.
// GET /instances/{id}/workload-report/html
func (s *APIServer) handleWorkloadReportHTML(w http.ResponseWriter, r *http.Request) {
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
			s.logger.ErrorContext(r.Context(), "failed to list snapshots for HTML report",
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
			s.logger.ErrorContext(r.Context(), "failed to get latest snapshots for HTML report",
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

	fromEntries, _, err := s.pgssStore.GetSnapshotEntries(r.Context(), fromSnap.ID, 0, 0)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get from snapshot entries for HTML report",
			"snapshot_id", fromSnap.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get snapshot entries")
		return
	}
	toEntries, _, err := s.pgssStore.GetSnapshotEntries(r.Context(), toSnap.ID, 0, 0)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get to snapshot entries for HTML report",
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

	data := htmlReportData{
		WorkloadReport: *report,
		GeneratedAt:    time.Now().Format("Jan 2, 2006 3:04:05 PM MST"),
	}

	// Set response headers.
	inline := r.URL.Query().Get("inline") == "true"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !inline {
		filename := fmt.Sprintf("pgpulse-workload-report-%s-%s.html",
			instanceID, time.Now().Format("20060102-150405"))
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	}

	if err := reportTemplate.Execute(w, data); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to render HTML report", "error", err)
	}
}
