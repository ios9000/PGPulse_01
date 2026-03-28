package forecast

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/ios9000/PGPulse_01/internal/config"
)

// PGThresholdQuerier queries PostgreSQL for autovacuum/autoanalyze settings.
type PGThresholdQuerier struct {
	connProv InstanceConnProvider
	cfg      config.MaintenanceForecastConfig
	logger   *slog.Logger
}

// NewPGThresholdQuerier creates a threshold querier.
func NewPGThresholdQuerier(connProv InstanceConnProvider, cfg config.MaintenanceForecastConfig, logger *slog.Logger) *PGThresholdQuerier {
	return &PGThresholdQuerier{connProv: connProv, cfg: cfg, logger: logger}
}

// SetConnProvider sets the connection provider after construction.
func (q *PGThresholdQuerier) SetConnProvider(cp InstanceConnProvider) {
	q.connProv = cp
}

// GetTableThresholds queries pg_settings and pg_class.reloptions for all user tables.
func (q *PGThresholdQuerier) GetTableThresholds(ctx context.Context, instanceID string) ([]TableThresholds, error) {
	// Step 0: Discover databases.
	conn, err := q.connProv.ConnFor(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("forecast: threshold connect: %w", err)
	}

	// Step 1: Global defaults.
	globalVacThreshold := q.cfg.VacuumThresholdFallback
	globalVacScale := q.cfg.VacuumScaleFactorFallback
	globalAnThreshold := q.cfg.AnalyzeThresholdFallback
	globalAnScale := q.cfg.AnalyzeScaleFactorFallback
	globalAVEnabled := true

	settingsRows, err := conn.Query(ctx, `
		SELECT name, setting FROM pg_settings
		WHERE name IN (
			'autovacuum_vacuum_threshold',
			'autovacuum_vacuum_scale_factor',
			'autovacuum_analyze_threshold',
			'autovacuum_analyze_scale_factor',
			'autovacuum'
		)`)
	if err == nil {
		defer settingsRows.Close()
		for settingsRows.Next() {
			var name, setting string
			if err := settingsRows.Scan(&name, &setting); err != nil {
				continue
			}
			switch name {
			case "autovacuum_vacuum_threshold":
				if v, e := strconv.Atoi(setting); e == nil {
					globalVacThreshold = v
				}
			case "autovacuum_vacuum_scale_factor":
				if v, e := strconv.ParseFloat(setting, 64); e == nil {
					globalVacScale = v
				}
			case "autovacuum_analyze_threshold":
				if v, e := strconv.Atoi(setting); e == nil {
					globalAnThreshold = v
				}
			case "autovacuum_analyze_scale_factor":
				if v, e := strconv.ParseFloat(setting, 64); e == nil {
					globalAnScale = v
				}
			case "autovacuum":
				globalAVEnabled = setting == "on"
			}
		}
	}

	// Discover databases.
	dbRows, err := conn.Query(ctx, `
		SELECT datname FROM pg_database
		WHERE datallowconn AND NOT datistemplate AND datname != 'template0'`)
	if err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("forecast: discover databases: %w", err)
	}
	var databases []string
	for dbRows.Next() {
		var db string
		if err := dbRows.Scan(&db); err == nil {
			databases = append(databases, db)
		}
	}
	dbRows.Close()
	_ = conn.Close(ctx)

	// Step 2: Per-database table data.
	var allThresholds []TableThresholds
	for _, db := range databases {
		thresholds, err := q.queryDatabaseTables(ctx, instanceID, db,
			globalVacThreshold, globalVacScale, globalAnThreshold, globalAnScale, globalAVEnabled)
		if err != nil {
			q.logger.Warn("forecast: threshold query failed for database",
				"instance", instanceID, "database", db, "err", err)
			continue
		}
		allThresholds = append(allThresholds, thresholds...)
	}

	return allThresholds, nil
}

// queryDatabaseTables connects to a specific database and queries table metadata.
func (q *PGThresholdQuerier) queryDatabaseTables(ctx context.Context, instanceID, database string,
	defVacThreshold int, defVacScale float64, defAnThreshold int, defAnScale float64, defAVEnabled bool) ([]TableThresholds, error) {

	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := q.connProv.ConnForDB(queryCtx, instanceID, database)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", database, err)
	}
	defer func() { _ = conn.Close(queryCtx) }()

	// Transaction-scoped session settings (D710: SET LOCAL only).
	tx, err := conn.Begin(queryCtx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(queryCtx) //nolint:errcheck

	if _, err := tx.Exec(queryCtx, "SET LOCAL statement_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("set statement_timeout: %w", err)
	}
	if _, err := tx.Exec(queryCtx, "SET LOCAL lock_timeout = '2s'"); err != nil {
		return nil, fmt.Errorf("set lock_timeout: %w", err)
	}
	if _, err := tx.Exec(queryCtx, "SET LOCAL application_name = 'pgpulse_forecast'"); err != nil {
		return nil, fmt.Errorf("set application_name: %w", err)
	}

	rows, err := tx.Query(queryCtx, `
		SELECT
			n.nspname AS schema,
			c.relname AS table_name,
			current_database() AS database,
			c.reltuples::bigint AS reltuples,
			c.reloptions
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'm')
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')`)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var thresholds []TableThresholds
	for rows.Next() {
		var schema, table, db string
		var relTuples int64
		var reloptions []string

		if err := rows.Scan(&schema, &table, &db, &relTuples, &reloptions); err != nil {
			q.logger.Debug("forecast: scan table row", "err", err)
			continue
		}

		// Parse reloptions for per-table overrides.
		overrides := parseReloptions(reloptions)
		avEnabled := defAVEnabled
		vacThreshold := defVacThreshold
		vacScale := defVacScale
		anThreshold := defAnThreshold
		anScale := defAnScale

		if v, ok := overrides["autovacuum_enabled"]; ok {
			avEnabled = v == "true" || v == "on"
		}
		if v, ok := overrides["autovacuum_vacuum_threshold"]; ok {
			if n, e := strconv.Atoi(v); e == nil {
				vacThreshold = n
			}
		}
		if v, ok := overrides["autovacuum_vacuum_scale_factor"]; ok {
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				vacScale = f
			}
		}
		if v, ok := overrides["autovacuum_analyze_threshold"]; ok {
			if n, e := strconv.Atoi(v); e == nil {
				anThreshold = n
			}
		}
		if v, ok := overrides["autovacuum_analyze_scale_factor"]; ok {
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				anScale = f
			}
		}

		if relTuples < 0 {
			relTuples = 0
		}

		thresholds = append(thresholds, TableThresholds{
			Database:              db,
			Schema:                schema,
			Table:                 table,
			RelTuples:             relTuples,
			AutovacuumEnabled:     avEnabled,
			VacuumThreshold:       vacThreshold,
			VacuumScaleFactor:     vacScale,
			AnalyzeThreshold:      anThreshold,
			AnalyzeScaleFactor:    anScale,
			EffectiveVacuumLimit:  float64(vacThreshold) + vacScale*float64(relTuples),
			EffectiveAnalyzeLimit: float64(anThreshold) + anScale*float64(relTuples),
		})
	}

	return thresholds, rows.Err()
}

// parseReloptions extracts autovacuum settings from the reloptions text array.
func parseReloptions(opts []string) map[string]string {
	result := make(map[string]string)
	for _, opt := range opts {
		parts := strings.SplitN(opt, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "autovacuum") {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
