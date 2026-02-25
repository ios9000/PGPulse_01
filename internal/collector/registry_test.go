package collector_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// mockCollector is a test double for the Collector interface.
type mockCollector struct {
	name     string
	points   []collector.MetricPoint
	err      error
	interval time.Duration
}

func (m *mockCollector) Name() string { return m.name }
func (m *mockCollector) Collect(_ context.Context, _ *pgx.Conn) ([]collector.MetricPoint, error) {
	return m.points, m.err
}
func (m *mockCollector) Interval() time.Duration { return m.interval }

func TestRegistry_CollectAll_Success(t *testing.T) {
	pt1 := collector.MetricPoint{Metric: "test.metric.one", Value: 1.0}
	pt2 := collector.MetricPoint{Metric: "test.metric.two", Value: 2.0}
	pt3 := collector.MetricPoint{Metric: "test.metric.three", Value: 3.0}

	c1 := &mockCollector{name: "c1", points: []collector.MetricPoint{pt1, pt2}}
	c2 := &mockCollector{name: "c2", points: []collector.MetricPoint{pt3}}

	reg := collector.NewRegistry()
	reg.Register(c1)
	reg.Register(c2)

	points := reg.CollectAll(context.Background(), nil)
	require.Len(t, points, 3)
	assert.Equal(t, "test.metric.one", points[0].Metric)
	assert.Equal(t, "test.metric.two", points[1].Metric)
	assert.Equal(t, "test.metric.three", points[2].Metric)
}

func TestRegistry_CollectAll_PartialFailure(t *testing.T) {
	passing := &mockCollector{
		name:   "passing",
		points: []collector.MetricPoint{{Metric: "good.metric", Value: 42.0}},
	}
	failing := &mockCollector{
		name: "failing",
		err:  errors.New("simulated collector failure"),
	}

	reg := collector.NewRegistry()
	reg.Register(failing)
	reg.Register(passing)

	points := reg.CollectAll(context.Background(), nil)
	// Only the passing collector's results are returned; failing does not abort the batch
	require.Len(t, points, 1)
	assert.Equal(t, "good.metric", points[0].Metric)
	assert.Equal(t, 42.0, points[0].Value)
}

func TestRegistry_CollectAll_Empty(t *testing.T) {
	reg := collector.NewRegistry()
	// Empty registry must return nil/empty without panic
	points := reg.CollectAll(context.Background(), nil)
	assert.Empty(t, points)
}

func TestRegistry_CollectAll_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// A collector that respects context cancellation should receive a cancelled context.
	// We verify no panic and the registry returns without hanging.
	slow := &mockCollector{
		name:   "slow",
		points: []collector.MetricPoint{{Metric: "slow.metric", Value: 1.0}},
	}

	reg := collector.NewRegistry()
	reg.Register(slow)

	// Should return without hanging even with a cancelled context
	done := make(chan []collector.MetricPoint, 1)
	go func() {
		done <- reg.CollectAll(ctx, nil)
	}()

	select {
	case <-done:
		// passed — no hang
	case <-time.After(3 * time.Second):
		t.Fatal("CollectAll hung with a cancelled context")
	}
}

func TestRegistry_Register_MultipleCollectors(t *testing.T) {
	reg := collector.NewRegistry()

	for i := 0; i < 5; i++ {
		reg.Register(&mockCollector{
			name:   "collector",
			points: []collector.MetricPoint{{Metric: "m", Value: float64(i)}},
		})
	}

	points := reg.CollectAll(context.Background(), nil)
	assert.Len(t, points, 5)
}
