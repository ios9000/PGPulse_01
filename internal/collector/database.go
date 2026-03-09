package collector

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const dbStatementTimeout = "60s"

// DatabaseCollector implements DBCollector for per-database analysis.
// Ports analiz_db.php Q2-Q18.
type DatabaseCollector struct{}

// NewDatabaseCollector creates a new DatabaseCollector.
func NewDatabaseCollector() *DatabaseCollector { return &DatabaseCollector{} }

// Name returns the collector's identifier.
func (c *DatabaseCollector) Name() string { return "database" }

// Interval returns the collection interval.
func (c *DatabaseCollector) Interval() time.Duration { return 5 * time.Minute }

// CollectDB collects per-database metrics from the given database.
// Partial success: individual sub-collector failures are non-fatal.
func (c *DatabaseCollector) CollectDB(ctx context.Context, q Queryer, dbName string, _ InstanceContext) ([]MetricPoint, error) {
	// Set statement timeout for this session.
	rows, err := q.Query(ctx, "SET statement_timeout = '"+dbStatementTimeout+"'")
	if err != nil {
		return nil, fmt.Errorf("set timeout: %w", err)
	}
	rows.Close()

	var points []MetricPoint
	var errs []error

	appendPoints := func(fn func(context.Context, Queryer, string) ([]MetricPoint, error)) {
		pts, fnErr := fn(ctx, q, dbName)
		if fnErr != nil {
			errs = append(errs, fnErr)
			return
		}
		points = append(points, pts...)
	}

	appendPoints(collectLargeObjects)
	appendPoints(collectFunctionStats)
	appendPoints(collectSequences)
	appendPoints(collectSchemaSizes)
	appendPoints(collectUnloggedObjects)
	appendPoints(collectTableSizes)
	appendPoints(collectBloat)
	appendPoints(collectCatalogSizes)
	appendPoints(collectTableCacheHit)
	appendPoints(collectAutovacuumOptions)
	appendPoints(collectVacuumNeed)
	appendPoints(collectIndexUsage)
	appendPoints(collectUnusedIndexes)
	appendPoints(collectToastSizes)
	appendPoints(collectPartitions)
	appendPoints(collectLargeObjectSizes)
	appendPoints(collectLogicalReplication)

	if len(errs) > 0 {
		return points, fmt.Errorf("%d sub-collectors failed (first: %v)", len(errs), errs[0])
	}
	return points, nil
}

// dbPoint creates a MetricPoint with db-prefixed metric name and database label.
func dbPoint(metric string, value float64, dbName string, extra map[string]string) MetricPoint {
	labels := map[string]string{"database": dbName}
	for k, v := range extra {
		labels[k] = v
	}
	return MetricPoint{
		Metric:    metricPrefix + "." + metric,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// collectLargeObjects — analiz_db.php Q2: count large objects.
func collectLargeObjects(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const sql = `SELECT COUNT(*) FROM pg_largeobject_metadata`

	row := q.QueryRow(ctx, sql)
	var count int64
	if err := row.Scan(&count); err != nil {
		return nil, fmt.Errorf("large_objects count: %w", err)
	}
	return []MetricPoint{
		dbPoint("db.large_objects.count", float64(count), dbName, nil),
	}, nil
}

// collectFunctionStats — analiz_db.php Q4: per-function call statistics.
func collectFunctionStats(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	// Check if track_functions is enabled.
	var trackFn string
	if err := q.QueryRow(ctx, "SHOW track_functions").Scan(&trackFn); err != nil {
		return nil, fmt.Errorf("show track_functions: %w", err)
	}
	if trackFn == "none" {
		return nil, nil // not enabled, skip silently
	}

	const fnSQL = `
		SELECT schemaname, funcname, calls, total_time, self_time
		FROM pg_stat_user_functions
		ORDER BY total_time DESC
		LIMIT 50`

	rows, err := q.Query(ctx, fnSQL)
	if err != nil {
		return nil, fmt.Errorf("function_stats query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, funcName string
		var calls int64
		var totalTime, selfTime float64
		if err := rows.Scan(&schema, &funcName, &calls, &totalTime, &selfTime); err != nil {
			return nil, fmt.Errorf("function_stats scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "function": funcName}
		points = append(points,
			dbPoint("db.functions.calls", float64(calls), dbName, labels),
			dbPoint("db.functions.total_time_ms", totalTime, dbName, labels),
			dbPoint("db.functions.self_time_ms", selfTime, dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectSequences — analiz_db.php Q5: sequence usage and remaining capacity.
func collectSequences(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const seqSQL = `
		SELECT schemaname, sequencename, last_value, max_value, increment_by,
		       COALESCE(CASE WHEN max_value > 0 AND last_value IS NOT NULL
		            THEN ROUND((last_value::numeric / max_value * 100), 2)
		            ELSE 0
		       END, 0) AS pct_used
		FROM pg_sequences
		ORDER BY pct_used DESC
		LIMIT 50`

	rows, err := q.Query(ctx, seqSQL)
	if err != nil {
		return nil, fmt.Errorf("sequences query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, seqName string
		var lastVal, maxVal, incBy sql.NullInt64
		var pctUsed float64
		if err := rows.Scan(&schema, &seqName, &lastVal, &maxVal, &incBy, &pctUsed); err != nil {
			return nil, fmt.Errorf("sequences scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "sequence": seqName}
		points = append(points,
			dbPoint("db.sequences.pct_used", pctUsed, dbName, labels),
		)
		if lastVal.Valid {
			points = append(points,
				dbPoint("db.sequences.last_value", float64(lastVal.Int64), dbName, labels),
			)
		}
	}
	return points, rows.Err()
}

// collectSchemaSizes — analiz_db.php Q6: size of each schema.
func collectSchemaSizes(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const schemaSQL = `
		SELECT n.nspname AS schema_name,
		       COALESCE(SUM(pg_total_relation_size(c.oid)), 0) AS total_bytes
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND c.relkind IN ('r', 'i', 'm', 'S', 't')
		GROUP BY n.nspname
		ORDER BY total_bytes DESC`

	rows, err := q.Query(ctx, schemaSQL)
	if err != nil {
		return nil, fmt.Errorf("schema_sizes query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema string
		var totalBytes int64
		if err := rows.Scan(&schema, &totalBytes); err != nil {
			return nil, fmt.Errorf("schema_sizes scan: %w", err)
		}
		points = append(points,
			dbPoint("db.schema.size_bytes", float64(totalBytes), dbName,
				map[string]string{"schema": schema}),
		)
	}
	return points, rows.Err()
}

// collectUnloggedObjects — analiz_db.php Q7: unlogged tables and their sizes.
func collectUnloggedObjects(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const unloggedSQL = `
		SELECT n.nspname AS schema_name, c.relname AS table_name,
		       pg_total_relation_size(c.oid) AS total_bytes
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relpersistence = 'u'
		  AND c.relkind = 'r'
		ORDER BY total_bytes DESC`

	rows, err := q.Query(ctx, unloggedSQL)
	if err != nil {
		return nil, fmt.Errorf("unlogged_objects query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table string
		var totalBytes int64
		if err := rows.Scan(&schema, &table, &totalBytes); err != nil {
			return nil, fmt.Errorf("unlogged_objects scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table}
		points = append(points,
			dbPoint("db.unlogged.size_bytes", float64(totalBytes), dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectTableSizes — analiz_db.php Q11: table sizes (data, index, toast, total).
func collectTableSizes(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const tableSizeSQL = `
		SELECT schemaname, relname,
		       pg_table_size(relid) AS table_bytes,
		       pg_indexes_size(relid) AS index_bytes,
		       COALESCE(pg_total_relation_size(relid) - pg_table_size(relid) - pg_indexes_size(relid), 0) AS toast_bytes,
		       pg_total_relation_size(relid) AS total_bytes,
		       n_live_tup
		FROM pg_stat_user_tables
		ORDER BY pg_total_relation_size(relid) DESC
		LIMIT 100`

	rows, err := q.Query(ctx, tableSizeSQL)
	if err != nil {
		return nil, fmt.Errorf("table_sizes query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table string
		var tableBytes, indexBytes, toastBytes, totalBytes, liveTup int64
		if err := rows.Scan(&schema, &table, &tableBytes, &indexBytes, &toastBytes, &totalBytes, &liveTup); err != nil {
			return nil, fmt.Errorf("table_sizes scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table}
		points = append(points,
			dbPoint("db.table.data_bytes", float64(tableBytes), dbName, labels),
			dbPoint("db.table.index_bytes", float64(indexBytes), dbName, labels),
			dbPoint("db.table.toast_bytes", float64(toastBytes), dbName, labels),
			dbPoint("db.table.total_bytes", float64(totalBytes), dbName, labels),
			dbPoint("db.table.live_tuples", float64(liveTup), dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectBloat — analiz_db.php Q12: table and index bloat estimation using pg_stats.
func collectBloat(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const bloatSQL = `
	WITH constants AS (
		SELECT current_setting('block_size')::numeric AS bs, 23 AS hdr, 4 AS ma
	),
	bloat_info AS (
		SELECT
			schemaname, tablename, reltuples, relpages, otta,
			ROUND((CASE WHEN otta=0 THEN 0.0 ELSE sml.relpages::float/otta END)::numeric,1) AS tbloat,
			GREATEST(relpages::bigint - otta, 0) AS wastedpages,
			GREATEST(bs*(relpages::bigint-otta),0) AS wastedbytes,
			iname, ituples, ipages, iotta,
			ROUND((CASE WHEN iotta=0 OR ipages=0 THEN 0.0 ELSE ipages::float/iotta END)::numeric,1) AS ibloat,
			GREATEST(ipages::bigint-iotta,0) AS wastedipages,
			GREATEST(bs*(ipages::bigint-iotta),0) AS wastedibytes
		FROM (
			SELECT
				schemaname, tablename, cc.reltuples, cc.relpages,
				CEIL((cc.reltuples*((datahdr+ma-
					(CASE WHEN datahdr%ma=0 THEN ma ELSE datahdr%ma END))+nullhdr2+4))/(bs-20::float)) AS otta,
				c2.relname AS iname, c2.reltuples AS ituples, c2.relpages AS ipages,
				CEIL((c2.reltuples*(datahdr-12))/(bs-20::float)) AS iotta
			FROM (
				SELECT
					ma, bs, schemaname, tablename,
					(datawidth+(hdr+ma-(CASE WHEN hdr%ma=0 THEN ma ELSE hdr%ma END)))::numeric AS datahdr,
					(maxfracsum*(nullhdr+ma-(CASE WHEN nullhdr%ma=0 THEN ma ELSE nullhdr%ma END))) AS nullhdr2
				FROM (
					SELECT
						schemaname, tablename, hdr, ma, bs,
						SUM((1-null_frac)*avg_width) AS datawidth,
						MAX(null_frac) AS maxfracsum,
						hdr+(1+(COUNT(CASE WHEN null_frac>0 THEN 1 END)*8)/bitlength) AS nullhdr
					FROM pg_stats CROSS JOIN constants
					LEFT JOIN (SELECT 8 AS bitlength) bl ON true
					GROUP BY 1,2,3,4,5
				) AS foo
			) AS rs
			JOIN pg_class cc ON cc.relname=rs.tablename
			JOIN pg_namespace nn ON cc.relnamespace=nn.oid AND nn.nspname=rs.schemaname AND nn.nspname<>'information_schema'
			LEFT JOIN pg_index i ON indrelid=cc.oid
			LEFT JOIN pg_class c2 ON c2.oid=i.indexrelid
		) AS sml
	)
	SELECT schemaname, tablename,
		   wastedbytes, tbloat,
		   iname, wastedibytes, ibloat
	FROM bloat_info
	WHERE tbloat > 1.5 OR ibloat > 1.5
	ORDER BY wastedbytes DESC NULLS LAST
	LIMIT 100`

	rows, err := q.Query(ctx, bloatSQL)
	if err != nil {
		return nil, fmt.Errorf("bloat query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table string
		var wastedBytes int64
		var tbloat float64
		var iname sql.NullString
		var wastedIBytes sql.NullInt64
		var ibloat sql.NullFloat64
		if err := rows.Scan(&schema, &table, &wastedBytes, &tbloat, &iname, &wastedIBytes, &ibloat); err != nil {
			return nil, fmt.Errorf("bloat scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table}
		points = append(points,
			dbPoint("db.bloat.table_wasted_bytes", float64(wastedBytes), dbName, labels),
			dbPoint("db.bloat.table_ratio", tbloat, dbName, labels),
		)
		if iname.Valid {
			idxLabels := map[string]string{"schema": schema, "table": table, "index": iname.String}
			points = append(points,
				dbPoint("db.bloat.index_wasted_bytes", float64(wastedIBytes.Int64), dbName, idxLabels),
				dbPoint("db.bloat.index_ratio", ibloat.Float64, dbName, idxLabels),
			)
		}
	}
	return points, rows.Err()
}

// collectCatalogSizes — analiz_db.php Q13: size of system catalog tables.
func collectCatalogSizes(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const catSQL = `
		SELECT relname,
		       pg_total_relation_size(oid) AS total_bytes
		FROM pg_class
		WHERE relnamespace = 'pg_catalog'::regnamespace
		  AND relkind = 'r'
		ORDER BY total_bytes DESC
		LIMIT 20`

	rows, err := q.Query(ctx, catSQL)
	if err != nil {
		return nil, fmt.Errorf("catalog_sizes query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var relname string
		var totalBytes int64
		if err := rows.Scan(&relname, &totalBytes); err != nil {
			return nil, fmt.Errorf("catalog_sizes scan: %w", err)
		}
		points = append(points,
			dbPoint("db.catalog.size_bytes", float64(totalBytes), dbName,
				map[string]string{"table": relname}),
		)
	}
	return points, rows.Err()
}

// collectTableCacheHit — analiz_db.php Q14: per-table buffer cache hit ratio.
func collectTableCacheHit(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const cacheSQL = `
		SELECT schemaname, relname,
		       heap_blks_read, heap_blks_hit,
		       CASE WHEN heap_blks_read + heap_blks_hit = 0 THEN 0
		            ELSE ROUND(heap_blks_hit::numeric / (heap_blks_read + heap_blks_hit) * 100, 2)
		       END AS hit_pct,
		       idx_blks_read, idx_blks_hit
		FROM pg_statio_user_tables
		WHERE heap_blks_read + heap_blks_hit > 0
		ORDER BY heap_blks_read DESC
		LIMIT 50`

	rows, err := q.Query(ctx, cacheSQL)
	if err != nil {
		return nil, fmt.Errorf("table_cache_hit query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table string
		var heapRead, heapHit int64
		var hitPct float64
		var idxRead, idxHit int64
		if err := rows.Scan(&schema, &table, &heapRead, &heapHit, &hitPct, &idxRead, &idxHit); err != nil {
			return nil, fmt.Errorf("table_cache_hit scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table}
		points = append(points,
			dbPoint("db.table.cache_hit_pct", hitPct, dbName, labels),
			dbPoint("db.table.heap_blks_read", float64(heapRead), dbName, labels),
			dbPoint("db.table.heap_blks_hit", float64(heapHit), dbName, labels),
			dbPoint("db.table.idx_blks_read", float64(idxRead), dbName, labels),
			dbPoint("db.table.idx_blks_hit", float64(idxHit), dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectAutovacuumOptions — analiz_db.php Q15: per-table autovacuum settings.
func collectAutovacuumOptions(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const avSQL = `
		SELECT n.nspname AS schema_name, c.relname AS table_name,
		       COALESCE((SELECT option_value FROM pg_options_to_table(c.reloptions)
		                 WHERE option_name = 'autovacuum_vacuum_threshold'), '-1') AS vac_threshold,
		       COALESCE((SELECT option_value FROM pg_options_to_table(c.reloptions)
		                 WHERE option_name = 'autovacuum_vacuum_scale_factor'), '-1') AS vac_scale,
		       COALESCE((SELECT option_value FROM pg_options_to_table(c.reloptions)
		                 WHERE option_name = 'autovacuum_analyze_threshold'), '-1') AS analyze_threshold,
		       COALESCE((SELECT option_value FROM pg_options_to_table(c.reloptions)
		                 WHERE option_name = 'autovacuum_analyze_scale_factor'), '-1') AS analyze_scale,
		       COALESCE((SELECT option_value FROM pg_options_to_table(c.reloptions)
		                 WHERE option_name = 'autovacuum_enabled'), 'true') AS av_enabled
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r'
		  AND c.reloptions IS NOT NULL
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY n.nspname, c.relname`

	rows, err := q.Query(ctx, avSQL)
	if err != nil {
		return nil, fmt.Errorf("autovacuum_options query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table string
		var vacThreshold, vacScale, analyzeThreshold, analyzeScale, avEnabled string
		if err := rows.Scan(&schema, &table, &vacThreshold, &vacScale, &analyzeThreshold, &analyzeScale, &avEnabled); err != nil {
			return nil, fmt.Errorf("autovacuum_options scan: %w", err)
		}
		labels := map[string]string{
			"schema":             schema,
			"table":              table,
			"vac_threshold":      vacThreshold,
			"vac_scale_factor":   vacScale,
			"analyze_threshold":  analyzeThreshold,
			"analyze_scale":      analyzeScale,
			"autovacuum_enabled": avEnabled,
		}
		enabled := 0.0
		if avEnabled == "true" {
			enabled = 1.0
		}
		points = append(points,
			dbPoint("db.autovacuum.enabled", enabled, dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectVacuumNeed — analiz_db.php Q16: tables needing vacuum.
func collectVacuumNeed(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const vacuumSQL = `
		SELECT schemaname, relname,
		       n_dead_tup, n_live_tup,
		       ROUND(n_dead_tup::numeric / GREATEST(n_live_tup + n_dead_tup, 1) * 100, 2) AS dead_pct,
		       last_autovacuum, last_vacuum, last_autoanalyze, last_analyze,
		       n_mod_since_analyze,
		       EXTRACT(EPOCH FROM (now() - last_autovacuum))::bigint AS autovacuum_age_sec,
		       EXTRACT(EPOCH FROM (now() - last_autoanalyze))::bigint AS autoanalyze_age_sec
		FROM pg_stat_user_tables
		ORDER BY n_dead_tup DESC
		LIMIT 50`

	rows, err := q.Query(ctx, vacuumSQL)
	if err != nil {
		return nil, fmt.Errorf("vacuum_need query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table string
		var deadTup, liveTup int64
		var deadPct float64
		var lastAutoVac, lastVac, lastAutoAnalyze, lastAnalyze sql.NullTime
		var modSinceAnalyze int64
		var autoVacAgeSec, autoAnalyzeAgeSec sql.NullInt64

		if err := rows.Scan(
			&schema, &table, &deadTup, &liveTup, &deadPct,
			&lastAutoVac, &lastVac, &lastAutoAnalyze, &lastAnalyze,
			&modSinceAnalyze, &autoVacAgeSec, &autoAnalyzeAgeSec,
		); err != nil {
			return nil, fmt.Errorf("vacuum_need scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table}
		points = append(points,
			dbPoint("db.vacuum.dead_tuples", float64(deadTup), dbName, labels),
			dbPoint("db.vacuum.live_tuples", float64(liveTup), dbName, labels),
			dbPoint("db.vacuum.dead_pct", deadPct, dbName, labels),
			dbPoint("db.vacuum.mod_since_analyze", float64(modSinceAnalyze), dbName, labels),
		)
		if autoVacAgeSec.Valid {
			points = append(points,
				dbPoint("db.vacuum.autovacuum_age_sec", float64(autoVacAgeSec.Int64), dbName, labels),
			)
		}
		if autoAnalyzeAgeSec.Valid {
			points = append(points,
				dbPoint("db.vacuum.autoanalyze_age_sec", float64(autoAnalyzeAgeSec.Int64), dbName, labels),
			)
		}
	}
	return points, rows.Err()
}

// collectIndexUsage — analiz_db.php Q17: index usage statistics.
func collectIndexUsage(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const idxSQL = `
		SELECT schemaname, relname, indexrelname,
		       idx_scan, idx_tup_read, idx_tup_fetch,
		       pg_relation_size(indexrelid) AS index_bytes
		FROM pg_stat_user_indexes
		ORDER BY idx_scan DESC
		LIMIT 100`

	rows, err := q.Query(ctx, idxSQL)
	if err != nil {
		return nil, fmt.Errorf("index_usage query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table, index string
		var idxScan, idxTupRead, idxTupFetch, indexBytes int64
		if err := rows.Scan(&schema, &table, &index, &idxScan, &idxTupRead, &idxTupFetch, &indexBytes); err != nil {
			return nil, fmt.Errorf("index_usage scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table, "index": index}
		points = append(points,
			dbPoint("db.index.scans", float64(idxScan), dbName, labels),
			dbPoint("db.index.tuples_read", float64(idxTupRead), dbName, labels),
			dbPoint("db.index.tuples_fetched", float64(idxTupFetch), dbName, labels),
			dbPoint("db.index.size_bytes", float64(indexBytes), dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectUnusedIndexes — analiz_db.php Q18: indexes with zero scans.
func collectUnusedIndexes(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const unusedSQL = `
		SELECT s.schemaname, s.relname, s.indexrelname,
		       pg_relation_size(s.indexrelid) AS index_bytes,
		       s.idx_scan
		FROM pg_stat_user_indexes s
		JOIN pg_index i ON s.indexrelid = i.indexrelid
		WHERE s.idx_scan = 0
		  AND NOT i.indisunique
		  AND NOT i.indisprimary
		ORDER BY pg_relation_size(s.indexrelid) DESC
		LIMIT 50`

	rows, err := q.Query(ctx, unusedSQL)
	if err != nil {
		return nil, fmt.Errorf("unused_indexes query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table, index string
		var indexBytes, idxScan int64
		if err := rows.Scan(&schema, &table, &index, &indexBytes, &idxScan); err != nil {
			return nil, fmt.Errorf("unused_indexes scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table, "index": index}
		points = append(points,
			dbPoint("db.index.unused_size_bytes", float64(indexBytes), dbName, labels),
			dbPoint("db.index.unused_scans", float64(idxScan), dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectToastSizes — analiz_db.php Q10: TOAST table sizes.
func collectToastSizes(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const toastSQL = `
		SELECT n.nspname AS schema_name, c.relname AS table_name,
		       pg_total_relation_size(t.oid) AS toast_bytes
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_class t ON t.oid = c.reltoastrelid
		WHERE c.relkind = 'r'
		  AND c.reltoastrelid != 0
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY toast_bytes DESC
		LIMIT 50`

	rows, err := q.Query(ctx, toastSQL)
	if err != nil {
		return nil, fmt.Errorf("toast_sizes query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var schema, table string
		var toastBytes int64
		if err := rows.Scan(&schema, &table, &toastBytes); err != nil {
			return nil, fmt.Errorf("toast_sizes scan: %w", err)
		}
		labels := map[string]string{"schema": schema, "table": table}
		points = append(points,
			dbPoint("db.toast.size_bytes", float64(toastBytes), dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectPartitions — analiz_db.php Q9: partitioned tables and their children.
func collectPartitions(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const partSQL = `
		SELECT nmsp_parent.nspname AS parent_schema,
		       parent.relname AS parent_table,
		       nmsp_child.nspname AS child_schema,
		       child.relname AS child_table,
		       pg_total_relation_size(child.oid) AS child_bytes
		FROM pg_inherits
		JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
		JOIN pg_class child ON pg_inherits.inhrelid = child.oid
		JOIN pg_namespace nmsp_parent ON nmsp_parent.oid = parent.relnamespace
		JOIN pg_namespace nmsp_child ON nmsp_child.oid = child.relnamespace
		ORDER BY parent.relname, child.relname`

	rows, err := q.Query(ctx, partSQL)
	if err != nil {
		return nil, fmt.Errorf("partitions query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var parentSchema, parentTable, childSchema, childTable string
		var childBytes int64
		if err := rows.Scan(&parentSchema, &parentTable, &childSchema, &childTable, &childBytes); err != nil {
			return nil, fmt.Errorf("partitions scan: %w", err)
		}
		labels := map[string]string{
			"parent_schema": parentSchema,
			"parent_table":  parentTable,
			"child_schema":  childSchema,
			"child_table":   childTable,
		}
		points = append(points,
			dbPoint("db.partition.child_bytes", float64(childBytes), dbName, labels),
		)
	}
	return points, rows.Err()
}

// collectLargeObjectSizes — analiz_db.php Q8: total size of large objects.
func collectLargeObjectSizes(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const loSQL = `
		SELECT COALESCE(SUM(pg_largeobject_metadata.oid::text::bigint), 0) AS lo_count,
		       COALESCE((SELECT SUM(length(data)) FROM pg_largeobject), 0) AS lo_total_bytes
		FROM pg_largeobject_metadata`

	row := q.QueryRow(ctx, loSQL)
	var loCount, loTotalBytes int64
	if err := row.Scan(&loCount, &loTotalBytes); err != nil {
		return nil, fmt.Errorf("large_object_sizes scan: %w", err)
	}
	return []MetricPoint{
		dbPoint("db.large_objects.total_count", float64(loCount), dbName, nil),
		dbPoint("db.large_objects.total_bytes", float64(loTotalBytes), dbName, nil),
	}, nil
}

// collectLogicalReplication — PGAM Q41: logical replication sync status.
func collectLogicalReplication(ctx context.Context, q Queryer, dbName string) ([]MetricPoint, error) {
	const lrSQL = `
		SELECT s.subname,
		       r.srrelid::regclass::text AS table_name,
		       r.srsubstate,
		       COALESCE(r.srsublsn::text, '') AS sync_lsn
		FROM pg_subscription_rel r
		JOIN pg_subscription s ON s.oid = r.srsubid
		WHERE r.srsubstate <> 'r'`

	rows, err := q.Query(ctx, lrSQL)
	if err != nil {
		// pg_subscription may not exist (no logical subscriptions, older PG, etc.)
		// Return 0 pending tables — do NOT fail the collection cycle.
		return []MetricPoint{
			dbPoint("db.logical_replication.pending_sync_tables", 0, dbName, nil),
		}, nil
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var subname, tableName, syncState, syncLSN string
		if err := rows.Scan(&subname, &tableName, &syncState, &syncLSN); err != nil {
			return []MetricPoint{
				dbPoint("db.logical_replication.pending_sync_tables", 0, dbName, nil),
			}, nil
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return []MetricPoint{
			dbPoint("db.logical_replication.pending_sync_tables", 0, dbName, nil),
		}, nil
	}

	return []MetricPoint{
		dbPoint("db.logical_replication.pending_sync_tables", float64(count), dbName, nil),
	}, nil
}
