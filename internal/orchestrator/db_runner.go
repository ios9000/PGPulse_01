package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/config"
)

const (
	dbPoolMaxConns       = 2
	dbPoolTTLCycles      = 3
	defaultMaxConcurrent = 5
)

// DBRunner manages per-database connection pools and dispatches DBCollectors.
// It discovers databases on each cycle, creates per-DB pools on demand, and
// evicts stale pools after dbPoolTTLCycles missed discovery cycles.
type DBRunner struct {
	instanceID  string
	baseDSN     string
	cfg         config.InstanceConfig
	primaryPool *pgxpool.Pool // borrowed from Runner — used only for discovery query
	collectors  []collector.DBCollector

	mu           sync.Mutex
	pools        map[string]*pgxpool.Pool // key: dbname
	poolLastSeen map[string]int           // key: dbname, value: cycles since last seen

	semaphore   chan struct{}
	logger      *slog.Logger
	metricStore collector.MetricStore
	evalFn      AlertEvaluator
}

// NewDBRunner creates a DBRunner for the given instance.
func NewDBRunner(
	instanceID, baseDSN string,
	cfg config.InstanceConfig,
	primaryPool *pgxpool.Pool,
	collectors []collector.DBCollector,
	store collector.MetricStore,
	eval AlertEvaluator,
	logger *slog.Logger,
) *DBRunner {
	maxConc := cfg.MaxConcurrentDBs
	if maxConc <= 0 {
		maxConc = defaultMaxConcurrent
	}
	return &DBRunner{
		instanceID:   instanceID,
		baseDSN:      baseDSN,
		cfg:          cfg,
		primaryPool:  primaryPool,
		collectors:   collectors,
		pools:        make(map[string]*pgxpool.Pool),
		poolLastSeen: make(map[string]int),
		semaphore:    make(chan struct{}, maxConc),
		logger:       logger,
		metricStore:  store,
		evalFn:       eval,
	}
}

// Run executes one per-DB collection cycle. Called by the instanceRunner.
func (r *DBRunner) Run(ctx context.Context, ic collector.InstanceContext) {
	databases, err := r.discoverDatabases(ctx)
	if err != nil {
		r.logger.Warn("db discovery failed", "instance", r.instanceID, "err", err)
		return
	}

	r.evictStalePools(databases)

	discovered := int64(len(databases))
	var collected, errTimeout, errPerm, errOther int64

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, dbName := range databases {
		pool, poolErr := r.getOrCreatePool(ctx, dbName)
		if poolErr != nil {
			r.logger.Warn("failed to create db pool", "db", dbName, "err", poolErr)
			mu.Lock()
			errOther++
			mu.Unlock()
			continue
		}

		for _, col := range r.collectors {
			wg.Add(1)
			r.semaphore <- struct{}{} // acquire slot
			go func(db string, c collector.DBCollector, p *pgxpool.Pool) {
				defer wg.Done()
				defer func() { <-r.semaphore }() // release slot

				points, cerr := c.CollectDB(ctx, p, db, ic)
				if cerr != nil {
					r.logger.Warn("db collector error",
						"instance", r.instanceID, "db", db,
						"collector", c.Name(), "err", cerr)
					mu.Lock()
					switch classifyDBError(cerr) {
					case "timeout":
						errTimeout++
					case "permission_denied":
						errPerm++
					default:
						errOther++
					}
					mu.Unlock()
					// Even on error, partial points may have been returned.
				}
				if len(points) > 0 {
					// Set InstanceID on all points before writing.
					for i := range points {
						points[i].InstanceID = r.instanceID
					}
					if werr := r.metricStore.Write(ctx, points); werr != nil {
						r.logger.Warn("metric write error", "err", werr)
					}
				}
				if cerr == nil {
					mu.Lock()
					collected++
					mu.Unlock()
				}
			}(dbName, col, pool)
		}
	}
	wg.Wait()

	// Emit internal telemetry.
	now := time.Now()
	telemetry := []collector.MetricPoint{
		{InstanceID: r.instanceID, Metric: "pgpulse.agent.db.discovered", Value: float64(discovered), Timestamp: now},
		{InstanceID: r.instanceID, Metric: "pgpulse.agent.db.collected", Value: float64(collected), Timestamp: now},
		{InstanceID: r.instanceID, Metric: "pgpulse.agent.db.errors",
			Value: float64(errTimeout), Labels: map[string]string{"reason": "timeout"}, Timestamp: now},
		{InstanceID: r.instanceID, Metric: "pgpulse.agent.db.errors",
			Value: float64(errPerm), Labels: map[string]string{"reason": "permission_denied"}, Timestamp: now},
		{InstanceID: r.instanceID, Metric: "pgpulse.agent.db.errors",
			Value: float64(errOther), Labels: map[string]string{"reason": "other"}, Timestamp: now},
	}
	_ = r.metricStore.Write(ctx, telemetry)
}

// discoverDatabases queries pg_database and applies include/exclude filters.
func (r *DBRunner) discoverDatabases(ctx context.Context) ([]string, error) {
	rows, err := r.primaryPool.Query(ctx,
		`SELECT datname FROM pg_database
		 WHERE NOT datistemplate AND datallowconn
		 ORDER BY datname`)
	if err != nil {
		return nil, fmt.Errorf("query pg_database: %w", err)
	}
	defer rows.Close()

	var all []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return nil, fmt.Errorf("scan datname: %w", scanErr)
		}
		all = append(all, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return r.applyFilters(all), nil
}

// applyFilters applies include/exclude glob patterns from config.
func (r *DBRunner) applyFilters(databases []string) []string {
	var result []string
	for _, db := range databases {
		if len(r.cfg.IncludeDatabases) > 0 && !matchesAny(db, r.cfg.IncludeDatabases) {
			continue
		}
		if len(r.cfg.ExcludeDatabases) > 0 && matchesAny(db, r.cfg.ExcludeDatabases) {
			continue
		}
		result = append(result, db)
	}
	return result
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}

// evictStalePools increments lastSeen counters and closes pools not seen in dbPoolTTLCycles.
func (r *DBRunner) evictStalePools(seen []string) {
	seenSet := make(map[string]bool, len(seen))
	for _, db := range seen {
		seenSet[db] = true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for db := range r.pools {
		if seenSet[db] {
			r.poolLastSeen[db] = 0
		} else {
			r.poolLastSeen[db]++
			if r.poolLastSeen[db] >= dbPoolTTLCycles {
				r.logger.Info("evicting stale db pool", "db", db)
				r.pools[db].Close()
				delete(r.pools, db)
				delete(r.poolLastSeen, db)
			}
		}
	}
}

// getOrCreatePool returns an existing pool or creates a new one for the given database.
func (r *DBRunner) getOrCreatePool(ctx context.Context, dbName string) (*pgxpool.Pool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p, ok := r.pools[dbName]; ok {
		return p, nil
	}

	dsn := substituteDBName(r.baseDSN, dbName)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn for db %q: %w", dbName, err)
	}
	cfg.MaxConns = dbPoolMaxConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool for db %q: %w", dbName, err)
	}

	r.pools[dbName] = pool
	r.poolLastSeen[dbName] = 0
	return pool, nil
}

// substituteDBName replaces the database name in a DSN string.
// Handles both URL format (postgres://...) and key=value format.
func substituteDBName(dsn, newDB string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err == nil {
			u.Path = "/" + newDB
			return u.String()
		}
	}
	// Handle key=value format.
	parts := strings.Fields(dsn)
	found := false
	for i, p := range parts {
		if strings.HasPrefix(p, "dbname=") {
			parts[i] = "dbname=" + newDB
			found = true
			break
		}
	}
	if !found {
		parts = append(parts, "dbname="+newDB)
	}
	return strings.Join(parts, " ")
}

// Close closes all per-DB pools. Called when the instance runner shuts down.
func (r *DBRunner) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for db, p := range r.pools {
		p.Close()
		delete(r.pools, db)
	}
}

// classifyDBError categorizes a database error for telemetry reporting.
func classifyDBError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "canceling statement") {
		return "timeout"
	}
	if strings.Contains(msg, "permission denied") {
		return "permission_denied"
	}
	return "other"
}
