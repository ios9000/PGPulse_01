package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/collector"
	"github.com/ios9000/PGPulse_01/internal/statements"
	"github.com/ios9000/PGPulse_01/internal/version"
)

// Status values recorded per collector run.
const (
	StatusOK   = "ok"   // Collect returned no error and every traced statement succeeded.
	StatusWarn = "warn" // Collect returned nil, but at least one statement failed (error swallowed by the collector).
	StatusFail = "fail" // Collect returned an error.
)

// Statement is one SQL statement a collector executed, as seen by the pgx tracer.
type Statement struct {
	SQL   string   `json:"sql"`
	Args  []string `json:"args,omitempty"`
	Rows  int64    `json:"rows"`
	Error string   `json:"error,omitempty"`
}

// CollectorResult is the outcome of running one collector against one server.
type CollectorResult struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"` // "primary" or "standby" (InstanceContext.IsRecovery the collector was given)
	Status     string      `json:"status"`
	Error      string      `json:"error,omitempty"`
	Points     int         `json:"points"`
	Statements []Statement `json:"statements"`
	DurationMS int64       `json:"duration_ms"`
}

// ServerResult holds everything recorded for one PostgreSQL server.
type ServerResult struct {
	Label            string            `json:"label"`
	ServerVersion    string            `json:"server_version"`
	ServerVersionNum int               `json:"server_version_num"`
	Detected         string            `json:"detected"` // version.PGVersion.String() as the collectors see it
	IsRecovery       bool              `json:"is_recovery"`
	ConnectError     string            `json:"connect_error,omitempty"`
	Collectors       []CollectorResult `json:"collectors"`
}

// recorder is a pgx.QueryTracer that attributes every statement to the
// collector currently being run. Statements issued while current == "" (the
// harness's own setup queries) are not recorded.
type recorder struct {
	mu      sync.Mutex
	current string
	stmts   []Statement
}

type traceKey struct{}

type traceData struct {
	sql  string
	args []string
}

func (r *recorder) begin(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = name
	r.stmts = nil
}

func (r *recorder) end() []Statement {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.stmts
	r.current = ""
	r.stmts = nil
	return out
}

// TraceQueryStart implements pgx.QueryTracer.
func (r *recorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	td := traceData{sql: data.SQL}
	for _, a := range data.Args {
		td.args = append(td.args, truncate(fmt.Sprint(a), 80))
	}
	return context.WithValue(ctx, traceKey{}, td)
}

// TraceQueryEnd implements pgx.QueryTracer.
func (r *recorder) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == "" {
		return
	}
	td, _ := ctx.Value(traceKey{}).(traceData)
	st := Statement{SQL: normalizeSQL(td.sql), Args: td.args, Rows: data.CommandTag.RowsAffected()}
	if data.Err != nil {
		st.Error = data.Err.Error()
	}
	r.stmts = append(r.stmts, st)
}

// normalizeSQL collapses whitespace so statements render on one line.
func normalizeSQL(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// instanceCollectors returns every instance-level collector that issues SQL
// against the monitored connection, built with the same constructors and
// arguments as orchestrator.buildCollectors (internal/orchestrator/runner.go).
// Each collector receives the PGVersion detected from the server, so its own
// gate / version checks select the SQL variant — nothing is re-implemented here.
//
// Deliberately excluded (they never touch the monitored connection):
//   - NewOSCollector:      talks to the pgpulse-agent over HTTP.
//   - NewClusterCollector: talks to Patroni / etcd.
//
// TestRegistryCoversAllConstructors guards this list against drift.
func instanceCollectors(id string, v version.PGVersion) []collector.Collector {
	return []collector.Collector{
		// high
		collector.NewConnectionsCollector(id, v),
		collector.NewCacheCollector(id, v),
		collector.NewWaitEventsCollector(id, v),
		collector.NewLockTreeCollector(id, v),
		collector.NewLongTransactionsCollector(id, v),
		// medium
		collector.NewReplicationStatusCollector(id, v),
		collector.NewReplicationLagCollector(id, v),
		collector.NewReplicationSlotsCollector(id, v),
		collector.NewStatementsConfigCollector(id, v),
		collector.NewStatementsTopCollector(id, v, 0),
		collector.NewCheckpointCollector(id, v),
		collector.NewVacuumProgressCollector(id, v),
		collector.NewClusterProgressCollector(id, v),
		collector.NewAnalyzeProgressCollector(id, v),
		collector.NewCreateIndexProgressCollector(id, v),
		collector.NewBasebackupProgressCollector(id, v),
		collector.NewCopyProgressCollector(id, v),
		collector.NewOSSQLCollector(id, v),
		// low
		collector.NewServerInfoCollector(id, v),
		collector.NewDatabaseSizesCollector(id, v),
		collector.NewSettingsCollector(id, v),
		collector.NewExtensionsCollector(id, v),
		collector.NewTransactionsCollector(id, v),
		collector.NewIOStatsCollector(id, v),
	}
}

// standbyPathCollectors run a second time with InstanceContext.IsRecovery=true
// because they issue different SQL on a replica. The fixture servers are
// primaries, so this checks the standby SQL compiles and executes against the
// version's catalog (it returns no rows on a primary).
var standbyPathCollectors = map[string]bool{
	"replication_status": true,
}

// excludedConstructors lists exported New*Collector functions intentionally not
// run by the harness, with the reason. Used by the drift test.
var excludedConstructors = map[string]string{
	"NewOSCollector":      "no SQL: reads OS metrics from pgpulse-agent over HTTP",
	"NewClusterCollector": "no SQL: reads Patroni / etcd state",
}

// Target is one server to check.
type Target struct {
	Label string // e.g. "pg14"
	DSN   string // admin-free DSN for the pgpulse_monitor role, database "postgres"
	DBDSN string // same, database "conformance" (per-database collector)
}

// runner executes the collectors against a target.
type runner struct {
	wait   time.Duration
	logger *slog.Logger
}

func (h *runner) connect(ctx context.Context, dsn string, rec *recorder) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.Tracer = rec
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	cfg.RuntimeParams["application_name"] = "pgpulse_conformance"

	deadline := time.Now().Add(h.wait)
	for {
		conn, err := pgx.ConnectConfig(ctx, cfg)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(time.Second)
	}
}

func (h *runner) run(ctx context.Context, t Target) ServerResult {
	res := ServerResult{Label: t.Label}
	rec := &recorder{}

	conn, err := h.connect(ctx, t.DSN, rec)
	if err != nil {
		res.ConnectError = err.Error()
		return res
	}
	v, err := version.Detect(ctx, conn)
	if err != nil {
		res.ConnectError = err.Error()
		_ = conn.Close(ctx)
		return res
	}
	res.ServerVersion = v.Full
	res.ServerVersionNum = v.Num
	res.Detected = v.String()
	if err := conn.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&res.IsRecovery); err != nil {
		res.ConnectError = fmt.Sprintf("pg_is_in_recovery: %v", err)
		_ = conn.Close(ctx)
		return res
	}
	// Generate some pg_stat_statements entries for the statements collectors.
	for i := 0; i < 3; i++ {
		_, _ = conn.Exec(ctx, "SELECT count(*) FROM pg_class WHERE relkind = $1", "r")
	}
	_ = conn.Close(ctx)

	ic := collector.InstanceContext{IsRecovery: res.IsRecovery}
	for _, c := range instanceCollectors(t.Label, v) {
		res.Collectors = append(res.Collectors, h.runOne(ctx, t.DSN, c, ic, pathName(ic)))
		if standbyPathCollectors[c.Name()] && !ic.IsRecovery {
			res.Collectors = append(res.Collectors,
				h.runOne(ctx, t.DSN, c, collector.InstanceContext{IsRecovery: true}, "standby"))
		}
	}
	res.Collectors = append(res.Collectors, h.runDatabase(ctx, t.DBDSN, ic))
	res.Collectors = append(res.Collectors, h.runStatementsCapture(ctx, t.DSN))
	return res
}

func pathName(ic collector.InstanceContext) string {
	if ic.IsRecovery {
		return "standby"
	}
	return "primary"
}

func finish(r *CollectorResult, collectErr error, stmts []Statement, start time.Time) {
	r.Statements = stmts
	r.DurationMS = time.Since(start).Milliseconds()
	switch {
	case collectErr != nil:
		r.Status = StatusFail
		r.Error = collectErr.Error()
	default:
		r.Status = StatusOK
		for _, s := range stmts {
			if s.Error != "" {
				r.Status = StatusWarn
				break
			}
		}
	}
}

// runOne runs a single instance collector on a fresh connection so session
// state left by one collector cannot affect the next.
func (h *runner) runOne(ctx context.Context, dsn string, c collector.Collector,
	ic collector.InstanceContext, path string) CollectorResult {
	r := CollectorResult{Name: c.Name(), Path: path}
	rec := &recorder{}
	conn, err := h.connect(ctx, dsn, rec)
	if err != nil {
		r.Status = StatusFail
		r.Error = "connect: " + err.Error()
		return r
	}
	defer func() { _ = conn.Close(ctx) }()

	start := time.Now()
	rec.begin(c.Name())
	// Mirror collector.Registry.CollectAll: 5s per-collector timeout.
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	points, cerr := c.Collect(cctx, conn, ic)
	cancel()
	finish(&r, cerr, rec.end(), start)
	r.Points = len(points)
	return r
}

// runDatabase runs the per-database DBCollector against the seeded database.
func (h *runner) runDatabase(ctx context.Context, dsn string, ic collector.InstanceContext) CollectorResult {
	dc := collector.NewDatabaseCollector()
	r := CollectorResult{Name: dc.Name(), Path: pathName(ic)}
	rec := &recorder{}
	conn, err := h.connect(ctx, dsn, rec)
	if err != nil {
		r.Status = StatusFail
		r.Error = "connect: " + err.Error()
		return r
	}
	defer func() { _ = conn.Close(ctx) }()

	start := time.Now()
	rec.begin(dc.Name())
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	cfg := conn.Config()
	points, cerr := dc.CollectDB(cctx, conn, cfg.Database, ic)
	cancel()
	finish(&r, cerr, rec.end(), start)
	r.Points = len(points)
	return r
}

// runStatementsCapture runs statements.SnapshotCapturer.CaptureInstance, the
// other consumer of a version.Gate (pgssGate) against monitored instances.
func (h *runner) runStatementsCapture(ctx context.Context, dsn string) CollectorResult {
	r := CollectorResult{Name: "statements_snapshot", Path: "primary"}
	rec := &recorder{}
	pcfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		r.Status = StatusFail
		r.Error = err.Error()
		return r
	}
	pcfg.ConnConfig.Tracer = rec
	pcfg.ConnConfig.RuntimeParams["application_name"] = "pgpulse_conformance"
	pcfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		r.Status = StatusFail
		r.Error = err.Error()
		return r
	}
	defer pool.Close()

	capt := statements.NewSnapshotCapturer(nopSnapshotStore{}, poolProvider{pool: pool}, nil,
		time.Hour, 1, false, slog.New(slog.NewTextHandler(io.Discard, nil)))

	start := time.Now()
	rec.begin(r.Name)
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	snap, cerr := capt.CaptureInstance(cctx, "conformance")
	cancel()
	finish(&r, cerr, rec.end(), start)
	if snap != nil {
		r.Points = snap.TotalStatements
	}
	return r
}

type poolProvider struct{ pool *pgxpool.Pool }

func (p poolProvider) PoolForInstance(string) (*pgxpool.Pool, error) { return p.pool, nil }

// nopSnapshotStore discards snapshots; the harness only cares that the SQL ran.
type nopSnapshotStore struct{}

func (nopSnapshotStore) WriteSnapshot(context.Context, statements.Snapshot, []statements.SnapshotEntry) (int64, error) {
	return 1, nil
}
func (nopSnapshotStore) GetSnapshot(context.Context, int64) (*statements.Snapshot, error) {
	return nil, nil
}
func (nopSnapshotStore) GetSnapshotEntries(context.Context, int64, int, int) ([]statements.SnapshotEntry, int, error) {
	return nil, 0, nil
}
func (nopSnapshotStore) ListSnapshots(context.Context, string, statements.SnapshotListOptions) ([]statements.Snapshot, int, error) {
	return nil, 0, nil
}
func (nopSnapshotStore) GetLatestSnapshots(context.Context, string, int) ([]statements.Snapshot, error) {
	return nil, nil
}
func (nopSnapshotStore) GetEntriesForQuery(context.Context, string, int64, time.Time, time.Time) ([]statements.SnapshotEntry, []statements.Snapshot, error) {
	return nil, nil, nil
}
func (nopSnapshotStore) CleanOld(context.Context, time.Time) error { return nil }
