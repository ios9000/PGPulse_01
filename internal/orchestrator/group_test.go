package orchestrator

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ios9000/PGPulse_01/internal/alert"
	"github.com/ios9000/PGPulse_01/internal/collector"
)

// discardLogger returns a logger that silently drops all output.
// Shared by all test files in this package.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockCollector implements collector.Collector for testing without a real DB.
type mockCollector struct {
	name     string
	interval time.Duration
	points   []collector.MetricPoint
	err      error
	called   bool
}

func (m *mockCollector) Name() string                  { return m.name }
func (m *mockCollector) Interval() time.Duration       { return m.interval }
func (m *mockCollector) Collect(_ context.Context, _ *pgx.Conn, _ collector.InstanceContext) ([]collector.MetricPoint, error) {
	m.called = true
	return m.points, m.err
}

// mockStore implements collector.MetricStore for testing.
type mockStore struct {
	written [][]collector.MetricPoint
	err     error
}

func (s *mockStore) Write(_ context.Context, points []collector.MetricPoint) error {
	if s.err != nil {
		return s.err
	}
	s.written = append(s.written, points)
	return nil
}
func (s *mockStore) Query(_ context.Context, _ collector.MetricQuery) ([]collector.MetricPoint, error) {
	return nil, nil
}
func (s *mockStore) Close() error { return nil }

// staticICFunc is a test icFunc that returns a fixed InstanceContext without a DB.
func staticICFunc(_ context.Context, _ *pgx.Conn) (collector.InstanceContext, error) {
	return collector.InstanceContext{IsRecovery: false}, nil
}

// makePoint creates a minimal MetricPoint for testing.
func makePoint(metric string) collector.MetricPoint {
	return collector.MetricPoint{InstanceID: "test", Metric: metric, Value: 1}
}

// newTestGroup builds an intervalGroup with injected icFunc and no real DB.
// skipAcquire bypasses pool.Acquire; staticICFunc and mockCollector both ignore the conn.
func newTestGroup(collectors []collector.Collector, store collector.MetricStore) *intervalGroup {
	g := newIntervalGroup("test", 10*time.Second, collectors, nil, store, discardLogger(),
		&NoOpAlertEvaluator{}, &NoOpAlertDispatcher{})
	g.icFunc = staticICFunc
	g.skipAcquire = true
	return g
}

// TestIntervalGroup_Collect_AllSuccess: 3 collectors all return points →
// store.Write called once with all points combined.
func TestIntervalGroup_Collect_AllSuccess(t *testing.T) {
	store := &mockStore{}
	collectors := []collector.Collector{
		&mockCollector{name: "a", points: []collector.MetricPoint{makePoint("pgpulse.a")}},
		&mockCollector{name: "b", points: []collector.MetricPoint{makePoint("pgpulse.b")}},
		&mockCollector{name: "c", points: []collector.MetricPoint{makePoint("pgpulse.c"), makePoint("pgpulse.c2")}},
	}

	g := newTestGroup(collectors, store)
	g.collect(context.Background())

	if len(store.written) != 1 {
		t.Fatalf("Write called %d times, want 1", len(store.written))
	}
	if len(store.written[0]) != 4 {
		t.Errorf("batch size = %d, want 4", len(store.written[0]))
	}
}

// TestIntervalGroup_Collect_PartialFailure: middle collector errors →
// other collectors' points still written.
func TestIntervalGroup_Collect_PartialFailure(t *testing.T) {
	store := &mockStore{}
	collectors := []collector.Collector{
		&mockCollector{name: "a", points: []collector.MetricPoint{makePoint("pgpulse.a")}},
		&mockCollector{name: "b", err: errors.New("db error")},
		&mockCollector{name: "c", points: []collector.MetricPoint{makePoint("pgpulse.c")}},
	}

	g := newTestGroup(collectors, store)
	g.collect(context.Background())

	if len(store.written) != 1 {
		t.Fatalf("Write called %d times, want 1", len(store.written))
	}
	if len(store.written[0]) != 2 {
		t.Errorf("batch size = %d, want 2 (skipped errored collector)", len(store.written[0]))
	}
}

// TestIntervalGroup_Collect_AllFail: all collectors error → no store.Write called.
func TestIntervalGroup_Collect_AllFail(t *testing.T) {
	store := &mockStore{}
	collectors := []collector.Collector{
		&mockCollector{name: "a", err: errors.New("error a")},
		&mockCollector{name: "b", err: errors.New("error b")},
	}

	g := newTestGroup(collectors, store)
	g.collect(context.Background())

	if len(store.written) != 0 {
		t.Errorf("Write called %d times, want 0", len(store.written))
	}
}

// TestIntervalGroup_Collect_NilPoints: collectors return nil, nil → no store.Write called.
func TestIntervalGroup_Collect_NilPoints(t *testing.T) {
	store := &mockStore{}
	collectors := []collector.Collector{
		&mockCollector{name: "a", points: nil, err: nil},
		&mockCollector{name: "b", points: nil, err: nil},
	}

	g := newTestGroup(collectors, store)
	g.collect(context.Background())

	if len(store.written) != 0 {
		t.Errorf("Write called %d times, want 0 (no points produced)", len(store.written))
	}
}

// --- Mock evaluator/dispatcher for alert tests ---

type mockAlertEvaluator struct {
	mu     sync.Mutex
	calls  int
	events []alert.AlertEvent
	err    error
}

func (m *mockAlertEvaluator) Evaluate(_ context.Context, _ []collector.MetricPoint) ([]alert.AlertEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.events, m.err
}

type mockAlertDispatcher struct {
	mu         sync.Mutex
	dispatched []alert.AlertEvent
	full       bool
}

func (m *mockAlertDispatcher) Dispatch(event alert.AlertEvent) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.full {
		return false
	}
	m.dispatched = append(m.dispatched, event)
	return true
}

func newTestGroupWithEvaluator(collectors []collector.Collector, store collector.MetricStore,
	eval AlertEvaluator, disp AlertDispatcher) *intervalGroup {
	g := newIntervalGroup("test", 10*time.Second, collectors, nil, store, discardLogger(), eval, disp)
	g.icFunc = staticICFunc
	g.skipAcquire = true
	return g
}

// TestGroupCollect_WithEvaluator: evaluator returns 1 event → dispatcher receives it.
func TestGroupCollect_WithEvaluator(t *testing.T) {
	store := &mockStore{}
	ev := &mockAlertEvaluator{
		events: []alert.AlertEvent{
			{RuleID: "rule-1", InstanceID: "test", Severity: alert.SeverityWarning},
		},
	}
	disp := &mockAlertDispatcher{}
	cols := []collector.Collector{
		&mockCollector{name: "a", points: []collector.MetricPoint{makePoint("pgpulse.a")}},
	}

	g := newTestGroupWithEvaluator(cols, store, ev, disp)
	g.collect(context.Background())

	ev.mu.Lock()
	calls := ev.calls
	ev.mu.Unlock()
	if calls != 1 {
		t.Errorf("evaluator calls = %d, want 1", calls)
	}

	disp.mu.Lock()
	dispatched := len(disp.dispatched)
	disp.mu.Unlock()
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1", dispatched)
	}
}

// TestGroupCollect_NoOpEvaluator: NoOp evaluator → no events, store still written.
func TestGroupCollect_NoOpEvaluator(t *testing.T) {
	store := &mockStore{}
	cols := []collector.Collector{
		&mockCollector{name: "a", points: []collector.MetricPoint{makePoint("pgpulse.a")}},
	}

	g := newTestGroupWithEvaluator(cols, store, &NoOpAlertEvaluator{}, &NoOpAlertDispatcher{})
	g.collect(context.Background())

	if len(store.written) != 1 {
		t.Fatalf("Write called %d times, want 1", len(store.written))
	}
	if len(store.written[0]) != 1 {
		t.Errorf("batch size = %d, want 1", len(store.written[0]))
	}
}

// TestGroupCollect_EvaluatorError: evaluator errors → store still written, no panic.
func TestGroupCollect_EvaluatorError(t *testing.T) {
	store := &mockStore{}
	ev := &mockAlertEvaluator{err: errors.New("eval error")}
	disp := &mockAlertDispatcher{}
	cols := []collector.Collector{
		&mockCollector{name: "a", points: []collector.MetricPoint{makePoint("pgpulse.a")}},
	}

	g := newTestGroupWithEvaluator(cols, store, ev, disp)
	g.collect(context.Background())

	if len(store.written) != 1 {
		t.Fatalf("Write called %d times, want 1 (store should write despite eval error)", len(store.written))
	}

	disp.mu.Lock()
	dispatched := len(disp.dispatched)
	disp.mu.Unlock()
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0 (no events on eval error)", dispatched)
	}
}

// TestGroupCollect_NoPoints: all collectors return nil → evaluator NOT called.
func TestGroupCollect_NoPoints(t *testing.T) {
	store := &mockStore{}
	ev := &mockAlertEvaluator{}
	cols := []collector.Collector{
		&mockCollector{name: "a", points: nil, err: nil},
	}

	g := newTestGroupWithEvaluator(cols, store, ev, &NoOpAlertDispatcher{})
	g.collect(context.Background())

	ev.mu.Lock()
	calls := ev.calls
	ev.mu.Unlock()
	if calls != 0 {
		t.Errorf("evaluator calls = %d, want 0 (no points to evaluate)", calls)
	}
}
