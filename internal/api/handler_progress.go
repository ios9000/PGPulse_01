package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ProgressOperation describes a single in-progress maintenance operation.
type ProgressOperation struct {
	OperationType   string                 `json:"operation_type"`
	PID             int                    `json:"pid"`
	Datname         string                 `json:"datname"`
	Relname         *string                `json:"relname"`
	Phase           string                 `json:"phase"`
	ProgressPct     *float64               `json:"progress_pct"`
	DurationSeconds float64                `json:"duration_seconds"`
	Details         map[string]interface{} `json:"details"`
}

// ProgressResponse wraps the list of active progress operations.
type ProgressResponse struct {
	Operations []ProgressOperation `json:"operations"`
}

// handleProgress returns in-progress maintenance operations for a monitored instance.
func (s *APIServer) handleProgress(w http.ResponseWriter, r *http.Request) {
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
		s.logger.ErrorContext(r.Context(), "failed to get connection for progress",
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

	pgVersionNum, err := detectPGVersion(r, conn)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to detect PG version",
			"instance_id", instanceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to detect PostgreSQL version")
		return
	}

	ops := []ProgressOperation{}

	// VACUUM progress — always available (PG >= 9.6, minimum is PG 14).
	if vacOps, err := s.queryVacuumProgress(r, conn); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to query vacuum progress",
			"instance_id", instanceID, "error", err)
	} else {
		ops = append(ops, vacOps...)
	}

	// CLUSTER / VACUUM FULL progress — PG >= 12.
	if pgVersionNum >= 120000 {
		if clusterOps, err := s.queryClusterProgress(r, conn); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query cluster progress",
				"instance_id", instanceID, "error", err)
		} else {
			ops = append(ops, clusterOps...)
		}

		if indexOps, err := s.queryCreateIndexProgress(r, conn); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query create index progress",
				"instance_id", instanceID, "error", err)
		} else {
			ops = append(ops, indexOps...)
		}
	}

	// ANALYZE and BASEBACKUP progress — PG >= 13.
	if pgVersionNum >= 130000 {
		if analyzeOps, err := s.queryAnalyzeProgress(r, conn); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query analyze progress",
				"instance_id", instanceID, "error", err)
		} else {
			ops = append(ops, analyzeOps...)
		}

		if bbOps, err := s.queryBasebackupProgress(r, conn); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query basebackup progress",
				"instance_id", instanceID, "error", err)
		} else {
			ops = append(ops, bbOps...)
		}
	}

	// COPY progress — PG >= 14.
	if pgVersionNum >= 140000 {
		if copyOps, err := s.queryCopyProgress(r, conn); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to query copy progress",
				"instance_id", instanceID, "error", err)
		} else {
			ops = append(ops, copyOps...)
		}
	}

	writeJSON(w, http.StatusOK, Envelope{
		Data: ProgressResponse{Operations: ops},
	})
}

func (s *APIServer) queryVacuumProgress(r *http.Request, conn *pgx.Conn) ([]ProgressOperation, error) {
	rows, err := conn.Query(r.Context(), `SELECT
    v.pid,
    sa.datname,
    v.relid::regclass::text AS relname,
    v.phase,
    v.heap_blks_total,
    v.heap_blks_scanned,
    v.heap_blks_vacuumed,
    v.index_vacuum_count,
    v.max_dead_tuples,
    v.num_dead_tuples,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN v.heap_blks_total > 0
         THEN (v.heap_blks_scanned::float8 / v.heap_blks_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_vacuum v
JOIN pg_stat_activity sa ON sa.pid = v.pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []ProgressOperation
	for rows.Next() {
		var op ProgressOperation
		var relname *string
		var heapTotal, heapScanned, heapVacuumed, indexVacCount, maxDead, numDead int64
		var duration *float64

		if err := rows.Scan(
			&op.PID, &op.Datname, &relname, &op.Phase,
			&heapTotal, &heapScanned, &heapVacuumed,
			&indexVacCount, &maxDead, &numDead,
			&duration, &op.ProgressPct,
		); err != nil {
			return nil, fmt.Errorf("scan vacuum row: %w", err)
		}

		op.OperationType = "vacuum"
		op.Relname = relname
		if duration != nil {
			op.DurationSeconds = *duration
		}
		op.Details = map[string]interface{}{
			"heap_blks_total":    heapTotal,
			"heap_blks_scanned":  heapScanned,
			"heap_blks_vacuumed": heapVacuumed,
			"index_vacuum_count": indexVacCount,
			"max_dead_tuples":    maxDead,
			"num_dead_tuples":    numDead,
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *APIServer) queryClusterProgress(r *http.Request, conn *pgx.Conn) ([]ProgressOperation, error) {
	rows, err := conn.Query(r.Context(), `SELECT
    c.pid,
    sa.datname,
    c.relid::regclass::text AS relname,
    c.command_desc AS phase,
    c.heap_tuples_scanned,
    c.heap_tuples_written,
    c.heap_blks_total,
    c.heap_blks_scanned,
    c.index_rebuild_count,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN c.heap_blks_total > 0
         THEN (c.heap_blks_scanned::float8 / c.heap_blks_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_cluster c
JOIN pg_stat_activity sa ON sa.pid = c.pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []ProgressOperation
	for rows.Next() {
		var op ProgressOperation
		var relname *string
		var tupScanned, tupWritten, blksTotal, blksScanned, idxRebuild int64
		var duration *float64

		if err := rows.Scan(
			&op.PID, &op.Datname, &relname, &op.Phase,
			&tupScanned, &tupWritten, &blksTotal, &blksScanned, &idxRebuild,
			&duration, &op.ProgressPct,
		); err != nil {
			return nil, fmt.Errorf("scan cluster row: %w", err)
		}

		op.OperationType = "cluster"
		op.Relname = relname
		if duration != nil {
			op.DurationSeconds = *duration
		}
		op.Details = map[string]interface{}{
			"heap_tuples_scanned": tupScanned,
			"heap_tuples_written": tupWritten,
			"heap_blks_total":     blksTotal,
			"heap_blks_scanned":   blksScanned,
			"index_rebuild_count": idxRebuild,
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *APIServer) queryCreateIndexProgress(r *http.Request, conn *pgx.Conn) ([]ProgressOperation, error) {
	rows, err := conn.Query(r.Context(), `SELECT
    ci.pid,
    sa.datname,
    ci.relid::regclass::text AS relname,
    ci.index_relid::regclass::text AS index_name,
    ci.command,
    ci.phase,
    ci.tuples_total,
    ci.tuples_done,
    ci.partitions_total,
    ci.partitions_done,
    ci.blocks_total,
    ci.blocks_done,
    ci.lockers_total,
    ci.lockers_done,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN ci.blocks_total > 0
         THEN (ci.blocks_done::float8 / ci.blocks_total) * 100
         WHEN ci.tuples_total > 0
         THEN (ci.tuples_done::float8 / ci.tuples_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_create_index ci
JOIN pg_stat_activity sa ON sa.pid = ci.pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []ProgressOperation
	for rows.Next() {
		var op ProgressOperation
		var relname, indexName *string
		var command string
		var tupTotal, tupDone, partTotal, partDone, blksTotal, blksDone, lockTotal, lockDone int64
		var duration *float64

		if err := rows.Scan(
			&op.PID, &op.Datname, &relname, &indexName,
			&command, &op.Phase,
			&tupTotal, &tupDone, &partTotal, &partDone,
			&blksTotal, &blksDone, &lockTotal, &lockDone,
			&duration, &op.ProgressPct,
		); err != nil {
			return nil, fmt.Errorf("scan create index row: %w", err)
		}

		op.OperationType = "create_index"
		op.Relname = relname
		if duration != nil {
			op.DurationSeconds = *duration
		}
		op.Details = map[string]interface{}{
			"index_name":       indexName,
			"command":          command,
			"tuples_total":     tupTotal,
			"tuples_done":      tupDone,
			"partitions_total": partTotal,
			"partitions_done":  partDone,
			"blocks_total":     blksTotal,
			"blocks_done":      blksDone,
			"lockers_total":    lockTotal,
			"lockers_done":     lockDone,
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *APIServer) queryAnalyzeProgress(r *http.Request, conn *pgx.Conn) ([]ProgressOperation, error) {
	rows, err := conn.Query(r.Context(), `SELECT
    a.pid,
    sa.datname,
    a.relid::regclass::text AS relname,
    a.phase,
    a.sample_blks_total,
    a.sample_blks_scanned,
    a.ext_stats_total,
    a.ext_stats_computed,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN a.sample_blks_total > 0
         THEN (a.sample_blks_scanned::float8 / a.sample_blks_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_analyze a
JOIN pg_stat_activity sa ON sa.pid = a.pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []ProgressOperation
	for rows.Next() {
		var op ProgressOperation
		var relname *string
		var sampleTotal, sampleScanned, extTotal, extComputed int64
		var duration *float64

		if err := rows.Scan(
			&op.PID, &op.Datname, &relname, &op.Phase,
			&sampleTotal, &sampleScanned, &extTotal, &extComputed,
			&duration, &op.ProgressPct,
		); err != nil {
			return nil, fmt.Errorf("scan analyze row: %w", err)
		}

		op.OperationType = "analyze"
		op.Relname = relname
		if duration != nil {
			op.DurationSeconds = *duration
		}
		op.Details = map[string]interface{}{
			"sample_blks_total":   sampleTotal,
			"sample_blks_scanned": sampleScanned,
			"ext_stats_total":     extTotal,
			"ext_stats_computed":  extComputed,
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *APIServer) queryBasebackupProgress(r *http.Request, conn *pgx.Conn) ([]ProgressOperation, error) {
	rows, err := conn.Query(r.Context(), `SELECT
    b.pid,
    sa.datname,
    b.phase,
    b.backup_total,
    b.backup_streamed,
    b.tablespaces_total,
    b.tablespaces_streamed,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN b.backup_total > 0
         THEN (b.backup_streamed::float8 / b.backup_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_basebackup b
JOIN pg_stat_activity sa ON sa.pid = b.pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []ProgressOperation
	for rows.Next() {
		var op ProgressOperation
		var backupTotal, backupStreamed, tsTotal, tsStreamed int64
		var duration *float64

		if err := rows.Scan(
			&op.PID, &op.Datname, &op.Phase,
			&backupTotal, &backupStreamed, &tsTotal, &tsStreamed,
			&duration, &op.ProgressPct,
		); err != nil {
			return nil, fmt.Errorf("scan basebackup row: %w", err)
		}

		op.OperationType = "basebackup"
		op.Relname = nil
		if duration != nil {
			op.DurationSeconds = *duration
		}
		op.Details = map[string]interface{}{
			"backup_total":         backupTotal,
			"backup_streamed":      backupStreamed,
			"tablespaces_total":    tsTotal,
			"tablespaces_streamed": tsStreamed,
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *APIServer) queryCopyProgress(r *http.Request, conn *pgx.Conn) ([]ProgressOperation, error) {
	rows, err := conn.Query(r.Context(), `SELECT
    cp.pid,
    sa.datname,
    cp.relid::regclass::text AS relname,
    cp.command,
    cp.type AS copy_type,
    cp.bytes_total,
    cp.bytes_processed,
    cp.tuples_processed,
    cp.tuples_excluded,
    EXTRACT(EPOCH FROM (now() - sa.xact_start))::float8 AS duration_seconds,
    CASE WHEN cp.bytes_total > 0
         THEN (cp.bytes_processed::float8 / cp.bytes_total) * 100
         ELSE NULL END AS progress_pct
FROM pg_stat_progress_copy cp
JOIN pg_stat_activity sa ON sa.pid = cp.pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []ProgressOperation
	for rows.Next() {
		var op ProgressOperation
		var relname *string
		var command, copyType string
		var bytesTotal, bytesProcessed, tupProcessed, tupExcluded int64
		var duration *float64

		if err := rows.Scan(
			&op.PID, &op.Datname, &relname,
			&command, &copyType,
			&bytesTotal, &bytesProcessed, &tupProcessed, &tupExcluded,
			&duration, &op.ProgressPct,
		); err != nil {
			return nil, fmt.Errorf("scan copy row: %w", err)
		}

		op.OperationType = "copy"
		op.Relname = relname
		op.Phase = command // COPY doesn't have a phase column; use command as phase.
		if duration != nil {
			op.DurationSeconds = *duration
		}
		op.Details = map[string]interface{}{
			"command":          command,
			"copy_type":        copyType,
			"bytes_total":      bytesTotal,
			"bytes_processed":  bytesProcessed,
			"tuples_processed": tupProcessed,
			"tuples_excluded":  tupExcluded,
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}
