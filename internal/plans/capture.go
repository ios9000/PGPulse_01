package plans

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TriggerType identifies what caused a plan capture.
type TriggerType string

const (
	TriggerDuration  TriggerType = "duration_threshold"
	TriggerManual    TriggerType = "manual"
	TriggerScheduled TriggerType = "scheduled_topn"
	TriggerHashDiff  TriggerType = "hash_diff_signal"
)

// CaptureConfig configures the plan capture collector.
type CaptureConfig struct {
	Enabled               bool
	DurationThresholdMs   int64
	DedupWindowSeconds    int
	ScheduledTopNCount    int
	ScheduledTopNInterval time.Duration
	MaxPlanBytes          int
	RetentionDays         int
}

// PlanCapture represents a captured query execution plan.
type PlanCapture struct {
	ID               int64
	InstanceID       string
	DatabaseName     string
	QueryFingerprint string // md5 of normalized query
	PlanHash         string // sha256 of plan bytes
	PlanText         string
	TriggerType      TriggerType
	DurationMs       int64
	QueryText        string
	Truncated        bool
	Metadata         map[string]any
	CapturedAt       time.Time
}

// CaptureStore persists captured plans.
type CaptureStore interface {
	SavePlan(ctx context.Context, p PlanCapture) error
	LatestPlanHash(ctx context.Context, instanceID, fingerprint string) (string, error)
}

type dedupCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
	window  time.Duration
}

func (c *dedupCache) seen(instanceID, fingerprint string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := instanceID + ":" + fingerprint
	if t, ok := c.entries[key]; ok && time.Since(t) < c.window {
		return true
	}
	c.entries[key] = time.Now()
	return false
}

// Collector captures query execution plans from monitored instances.
type Collector struct {
	config     CaptureConfig
	store      CaptureStore
	dedup      *dedupCache
	lastTopN   map[string]time.Time
	lastTopNMu sync.Mutex
}

// NewCollector creates a plan capture collector.
func NewCollector(cfg CaptureConfig, store CaptureStore) *Collector {
	return &Collector{
		config: cfg,
		store:  store,
		dedup: &dedupCache{
			entries: make(map[string]time.Time),
			window:  time.Duration(cfg.DedupWindowSeconds) * time.Second,
		},
		lastTopN: make(map[string]time.Time),
	}
}

// Name returns the collector name.
func (c *Collector) Name() string { return "plan_capture" }

// Collect runs plan capture for a single instance. Called from the orchestrator.
// instanceID is passed separately because InstanceContext doesn't carry it.
func (c *Collector) Collect(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
	if !c.config.Enabled {
		return nil
	}

	if err := c.collectDurationThreshold(ctx, pool, instanceID); err != nil {
		return fmt.Errorf("duration threshold capture: %w", err)
	}

	c.lastTopNMu.Lock()
	shouldRunTopN := time.Since(c.lastTopN[instanceID]) >= c.config.ScheduledTopNInterval
	c.lastTopNMu.Unlock()

	if shouldRunTopN {
		if err := c.collectScheduledTopN(ctx, pool, instanceID); err != nil {
			return fmt.Errorf("scheduled topN capture: %w", err)
		}
		c.lastTopNMu.Lock()
		c.lastTopN[instanceID] = time.Now()
		c.lastTopNMu.Unlock()
	}

	return nil
}

// CaptureManual runs EXPLAIN on a user-provided query and stores the result.
func (c *Collector) CaptureManual(ctx context.Context, pool *pgxpool.Pool, instanceID, database, query string) (*PlanCapture, error) {
	plan, err := c.runExplain(ctx, pool, query)
	if err != nil {
		return nil, fmt.Errorf("explain: %w", err)
	}
	plan.InstanceID = instanceID
	plan.DatabaseName = database
	plan.QueryFingerprint = Fingerprint(query)
	plan.TriggerType = TriggerManual
	plan.QueryText = truncateStr(query, 4096)
	plan.CapturedAt = time.Now()
	if err := c.checkAndSave(ctx, plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func (c *Collector) collectDurationThreshold(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
	rows, err := pool.Query(ctx, `
		SELECT datname, query,
		       EXTRACT(EPOCH FROM (now() - query_start)) * 1000 AS duration_ms
		FROM pg_stat_activity
		WHERE state = 'active'
		  AND query NOT LIKE '%pg_stat_activity%'
		  AND query NOT LIKE '%pgpulse%'
		  AND now() - query_start > ($1::bigint * interval '1 millisecond')
		  AND pid != pg_backend_pid()
	`, c.config.DurationThresholdMs)
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		datname    string
		query      string
		durationMs float64
	}
	var hits []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.datname, &r.query, &r.durationMs); err != nil {
			continue
		}
		hits = append(hits, r)
	}

	for _, h := range hits {
		fp := Fingerprint(h.query)
		if c.dedup.seen(instanceID, fp) {
			continue
		}
		if IsDDL(h.query) {
			continue
		}
		plan, err := c.runExplain(ctx, pool, h.query)
		if err != nil {
			continue
		}
		plan.InstanceID = instanceID
		plan.DatabaseName = h.datname
		plan.QueryFingerprint = fp
		plan.TriggerType = TriggerDuration
		plan.DurationMs = int64(h.durationMs)
		plan.QueryText = truncateStr(h.query, 4096)
		plan.CapturedAt = time.Now()
		_ = c.checkAndSave(ctx, plan)
	}
	return nil
}

func (c *Collector) collectScheduledTopN(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
	rows, err := pool.Query(ctx, `
		SELECT query, d.datname
		FROM pg_stat_statements s
		JOIN pg_database d ON d.oid = s.dbid
		WHERE query NOT LIKE '%pgpulse%'
		  AND length(query) > 10
		  AND query NOT LIKE '<%%'
		ORDER BY s.total_exec_time DESC
		LIMIT $1
	`, c.config.ScheduledTopNCount)
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		query   string
		datname string
	}
	var hits []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.query, &r.datname); err != nil {
			continue
		}
		hits = append(hits, r)
	}

	for i, h := range hits {
		if IsDDLStrict(h.query) {
			continue
		}
		plan, err := c.runExplain(ctx, pool, h.query)
		if err != nil {
			continue
		}
		plan.InstanceID = instanceID
		plan.DatabaseName = h.datname
		plan.QueryFingerprint = Fingerprint(h.query)
		plan.TriggerType = TriggerScheduled
		plan.QueryText = truncateStr(h.query, 4096)
		plan.Metadata = map[string]any{"rank": i + 1}
		plan.CapturedAt = time.Now()
		_ = c.checkAndSave(ctx, plan)
	}
	return nil
}

// runExplain runs EXPLAIN (FORMAT JSON) on the given query.
// NOTE: The query comes from pg_stat_activity or pg_stat_statements on
// the monitored server, NOT from user input. The fmt.Sprintf is intentional.
func (c *Collector) runExplain(ctx context.Context, pool *pgxpool.Pool, query string) (PlanCapture, error) {
	var planBytes []byte
	err := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+query).Scan(&planBytes) //nolint:gosec // query from pg_stat_activity, not user input
	if err != nil {
		return PlanCapture{}, err
	}

	truncated := false
	if len(planBytes) > c.config.MaxPlanBytes {
		planBytes = planBytes[:c.config.MaxPlanBytes]
		truncated = true
	}

	h := sha256.Sum256(planBytes)
	return PlanCapture{
		PlanText:  string(planBytes),
		PlanHash:  fmt.Sprintf("%x", h),
		Truncated: truncated,
	}, nil
}

func (c *Collector) checkAndSave(ctx context.Context, plan PlanCapture) error {
	prevHash, _ := c.store.LatestPlanHash(ctx, plan.InstanceID, plan.QueryFingerprint)
	if prevHash != "" && prevHash != plan.PlanHash {
		regression := plan
		regression.TriggerType = TriggerHashDiff
		regression.Metadata = map[string]any{"prev_hash": prevHash, "new_hash": plan.PlanHash}
		_ = c.store.SavePlan(ctx, regression)
	}
	return c.store.SavePlan(ctx, plan)
}

// Fingerprint returns the md5 hex digest of a normalized query.
func Fingerprint(query string) string {
	h := md5.Sum([]byte(NormalizeQuery(query))) //nolint:gosec // md5 used for fingerprinting, not security
	return fmt.Sprintf("%x", h)
}

// NormalizeQuery lowercases and collapses whitespace.
func NormalizeQuery(q string) string {
	return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}

// IsDDL returns true if the query starts with a DDL keyword.
func IsDDL(q string) bool {
	upper := strings.ToUpper(strings.TrimSpace(q))
	for _, kw := range []string{"DROP ", "CREATE ", "TRUNCATE ", "ALTER ", "REINDEX "} {
		if strings.HasPrefix(upper, kw) {
			return true
		}
	}
	return false
}

// IsDDLStrict returns true for DDL and DML write statements.
func IsDDLStrict(q string) bool {
	upper := strings.ToUpper(strings.TrimSpace(q))
	for _, kw := range []string{"DROP ", "CREATE ", "TRUNCATE ", "ALTER ", "REINDEX ",
		"INSERT ", "UPDATE ", "DELETE "} {
		if strings.HasPrefix(upper, kw) {
			return true
		}
	}
	return false
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
