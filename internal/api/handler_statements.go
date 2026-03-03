package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// StatementsConfig holds pg_stat_statements configuration and fill info.
type StatementsConfig struct {
	Max                  int      `json:"max"`
	Track                string   `json:"track"`
	IOTiming             bool     `json:"io_timing"`
	CurrentCount         int      `json:"current_count"`
	FillPct              float64  `json:"fill_pct"`
	StatsReset           *string  `json:"stats_reset"`
	StatsResetAgeSeconds *float64 `json:"stats_reset_age_seconds"`
}

// StatementEntry represents a single row from pg_stat_statements.
type StatementEntry struct {
	QueryID         int64   `json:"queryid"`
	QueryText       string  `json:"query_text"`
	DBName          string  `json:"dbname"`
	Username        string  `json:"username"`
	Calls           int64   `json:"calls"`
	TotalExecTimeMs float64 `json:"total_exec_time_ms"`
	MeanExecTimeMs  float64 `json:"mean_exec_time_ms"`
	Rows            int64   `json:"rows"`
	BlkReadTimeMs   float64 `json:"blk_read_time_ms"`
	BlkWriteTimeMs  float64 `json:"blk_write_time_ms"`
	IOTimeMs        float64 `json:"io_time_ms"`
	CPUTimeMs       float64 `json:"cpu_time_ms"`
	SharedBlksHit   int64   `json:"shared_blks_hit"`
	SharedBlksRead  int64   `json:"shared_blks_read"`
	HitRatio        float64 `json:"hit_ratio"`
	PctOfTotalTime  float64 `json:"pct_of_total_time"`
}

// StatementsResponse wraps pgss config and statement entries.
type StatementsResponse struct {
	Config     StatementsConfig `json:"config"`
	Statements []StatementEntry `json:"statements"`
}

// sortColumn returns a safe SQL expression for ORDER BY based on the
// user-provided sort parameter and the PostgreSQL version number.
// Only whitelisted values are accepted; unknown values default to total time.
func sortColumn(sort string, pgVersionNum int) string {
	switch sort {
	case "io_time":
		return "(s.blk_read_time + s.blk_write_time)"
	case "cpu_time":
		if pgVersionNum >= 130000 {
			return "(s.total_exec_time - s.blk_read_time - s.blk_write_time)"
		}
		return "(s.total_time - s.blk_read_time - s.blk_write_time)"
	case "calls":
		return "s.calls"
	case "rows":
		return "s.rows"
	default: // "total_time" or empty
		if pgVersionNum >= 130000 {
			return "s.total_exec_time"
		}
		return "s.total_time"
	}
}

// handleStatements returns top pg_stat_statements entries for a monitored instance.
func (s *APIServer) handleStatements(w http.ResponseWriter, r *http.Request) {
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

	// Parse query params.
	sort := r.URL.Query().Get("sort")
	limit := 25
	if ls := r.URL.Query().Get("limit"); ls != "" {
		v, err := strconv.Atoi(ls)
		if err != nil || v < 1 || v > 100 {
			writeError(w, http.StatusBadRequest, "bad_request",
				"'limit' must be an integer between 1 and 100")
			return
		}
		limit = v
	}

	conn, err := s.connProvider.ConnFor(r.Context(), instanceID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to get connection for statements",
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

	// Check if pg_stat_statements extension is installed.
	var extExists bool
	if err := conn.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&extExists); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to check pg_stat_statements extension",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to check extensions")
		return
	}
	if !extExists {
		writeError(w, http.StatusNotFound, "EXTENSION_NOT_FOUND",
			"pg_stat_statements extension not available")
		return
	}

	// Detect PG version.
	pgVersionNum, err := detectPGVersion(r, conn)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to detect PG version",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to detect PostgreSQL version")
		return
	}

	// Build config response.
	cfg, err := s.queryStatementsConfig(w, r, conn, instanceID, pgVersionNum)
	if err != nil {
		return // error already logged and written
	}

	// Build the main query with version-appropriate time column.
	col := sortColumn(sort, pgVersionNum)

	var query string
	if pgVersionNum >= 130000 {
		query = fmt.Sprintf(`SELECT
    s.queryid,
    LEFT(s.query, 500) AS query_text,
    d.datname,
    r.rolname AS usename,
    s.calls,
    s.total_exec_time AS total_exec_time_ms,
    s.total_exec_time / NULLIF(s.calls, 0) AS mean_exec_time_ms,
    s.rows,
    s.blk_read_time AS blk_read_time_ms,
    s.blk_write_time AS blk_write_time_ms,
    (s.blk_read_time + s.blk_write_time) AS io_time_ms,
    (s.total_exec_time - s.blk_read_time - s.blk_write_time) AS cpu_time_ms,
    s.shared_blks_hit,
    s.shared_blks_read,
    CASE WHEN (s.shared_blks_hit + s.shared_blks_read) > 0
         THEN s.shared_blks_hit::float8 / (s.shared_blks_hit + s.shared_blks_read)
         ELSE 0 END AS hit_ratio,
    s.total_exec_time / NULLIF(sum(s.total_exec_time) OVER (), 0) * 100 AS pct_of_total_time
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
JOIN pg_roles r ON r.oid = s.userid
ORDER BY %s DESC
LIMIT $1`, col)
	} else {
		query = fmt.Sprintf(`SELECT
    s.queryid,
    LEFT(s.query, 500) AS query_text,
    d.datname,
    r.rolname AS usename,
    s.calls,
    s.total_time AS total_exec_time_ms,
    s.total_time / NULLIF(s.calls, 0) AS mean_exec_time_ms,
    s.rows,
    s.blk_read_time AS blk_read_time_ms,
    s.blk_write_time AS blk_write_time_ms,
    (s.blk_read_time + s.blk_write_time) AS io_time_ms,
    (s.total_time - s.blk_read_time - s.blk_write_time) AS cpu_time_ms,
    s.shared_blks_hit,
    s.shared_blks_read,
    CASE WHEN (s.shared_blks_hit + s.shared_blks_read) > 0
         THEN s.shared_blks_hit::float8 / (s.shared_blks_hit + s.shared_blks_read)
         ELSE 0 END AS hit_ratio,
    s.total_time / NULLIF(sum(s.total_time) OVER (), 0) * 100 AS pct_of_total_time
FROM pg_stat_statements s
JOIN pg_database d ON d.oid = s.dbid
JOIN pg_roles r ON r.oid = s.userid
ORDER BY %s DESC
LIMIT $1`, col)
	}

	rows, err := conn.Query(r.Context(), query, limit)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to query pg_stat_statements",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query statements")
		return
	}
	defer rows.Close()

	stmts := []StatementEntry{}
	for rows.Next() {
		var e StatementEntry
		if err := rows.Scan(
			&e.QueryID, &e.QueryText, &e.DBName, &e.Username,
			&e.Calls, &e.TotalExecTimeMs, &e.MeanExecTimeMs, &e.Rows,
			&e.BlkReadTimeMs, &e.BlkWriteTimeMs, &e.IOTimeMs, &e.CPUTimeMs,
			&e.SharedBlksHit, &e.SharedBlksRead, &e.HitRatio, &e.PctOfTotalTime,
		); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to scan statement row",
				"instance_id", instanceID, "error", err)
			continue
		}
		stmts = append(stmts, e)
	}
	if err := rows.Err(); err != nil {
		s.logger.ErrorContext(r.Context(), "statements rows error",
			"instance_id", instanceID, "error", err)
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: StatementsResponse{
			Config:     cfg,
			Statements: stmts,
		},
	})
}

// detectPGVersion queries SHOW server_version_num and parses the integer.
func detectPGVersion(r *http.Request, conn *pgx.Conn) (int, error) {
	var versionNumStr string
	if err := conn.QueryRow(r.Context(), "SHOW server_version_num").Scan(&versionNumStr); err != nil {
		return 0, fmt.Errorf("query server_version_num: %w", err)
	}
	num, err := strconv.Atoi(versionNumStr)
	if err != nil {
		return 0, fmt.Errorf("parse server_version_num %q: %w", versionNumStr, err)
	}
	return num, nil
}

// queryStatementsConfig fetches pgss configuration and fill stats.
// On error it writes the HTTP error response and returns a non-nil error.
func (s *APIServer) queryStatementsConfig(
	w http.ResponseWriter, r *http.Request, conn *pgx.Conn,
	instanceID string, pgVersionNum int,
) (StatementsConfig, error) {
	var cfg StatementsConfig
	var maxStr, trackStr, ioTimingStr string

	err := conn.QueryRow(r.Context(), `SELECT
    (SELECT setting FROM pg_settings WHERE name = 'pg_stat_statements.max') AS max_setting,
    (SELECT setting FROM pg_settings WHERE name = 'pg_stat_statements.track') AS track_setting,
    (SELECT setting FROM pg_settings WHERE name = 'track_io_timing') AS io_timing,
    (SELECT count(*) FROM pg_stat_statements) AS current_count`).Scan(
		&maxStr, &trackStr, &ioTimingStr, &cfg.CurrentCount)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to query pgss config",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to query statements configuration")
		return cfg, err
	}

	cfg.Max, _ = strconv.Atoi(maxStr)
	cfg.Track = trackStr
	cfg.IOTiming = ioTimingStr == "on"
	if cfg.Max > 0 {
		cfg.FillPct = float64(cfg.CurrentCount) / float64(cfg.Max) * 100
	}

	// PG >= 14: query pg_stat_statements_info for stats_reset.
	if pgVersionNum >= 140000 {
		var reset *string
		var resetAge *float64
		err := conn.QueryRow(r.Context(), `SELECT
    stats_reset::text,
    EXTRACT(EPOCH FROM (now() - stats_reset)) AS stats_reset_age_seconds
FROM pg_stat_statements_info`).Scan(&reset, &resetAge)
		if err == nil {
			cfg.StatsReset = reset
			cfg.StatsResetAgeSeconds = resetAge
		} else {
			s.logger.ErrorContext(r.Context(), "failed to query pg_stat_statements_info",
				"instance_id", instanceID, "error", err)
		}
	}

	return cfg, nil
}
