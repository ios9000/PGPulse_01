package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

func makePoint(instanceID, metric string, value float64, ts time.Time, labels map[string]string) collector.MetricPoint {
	return collector.MetricPoint{
		InstanceID: instanceID,
		Metric:     metric,
		Value:      value,
		Timestamp:  ts,
		Labels:     labels,
	}
}

func TestMemoryStore_WriteAndQuery(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	base := time.Now()
	for i := range 10 {
		_ = ms.Write(ctx, []collector.MetricPoint{
			makePoint("inst1", "metric.a", float64(i), base.Add(time.Duration(i)*time.Second), nil),
		})
	}

	results, err := ms.Query(ctx, collector.MetricQuery{InstanceID: "inst1", Metric: "metric.a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 10 {
		t.Fatalf("got %d results, want 10", len(results))
	}
	for i := 1; i < len(results); i++ {
		if results[i].Timestamp.Before(results[i-1].Timestamp) {
			t.Fatal("results not sorted by timestamp")
		}
	}
}

func TestMemoryStore_QueryByInstance(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	now := time.Now()
	_ = ms.Write(ctx, []collector.MetricPoint{
		makePoint("inst1", "m", 1, now, nil),
		makePoint("inst2", "m", 2, now, nil),
		makePoint("inst1", "m", 3, now.Add(time.Second), nil),
	})

	results, _ := ms.Query(ctx, collector.MetricQuery{InstanceID: "inst1"})
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
	for _, r := range results {
		if r.InstanceID != "inst1" {
			t.Errorf("got instance %q, want inst1", r.InstanceID)
		}
	}
}

func TestMemoryStore_QueryByMetric(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	now := time.Now()
	_ = ms.Write(ctx, []collector.MetricPoint{
		makePoint("inst1", "cpu", 10, now, nil),
		makePoint("inst1", "mem", 20, now, nil),
		makePoint("inst1", "cpu", 30, now.Add(time.Second), nil),
	})

	results, _ := ms.Query(ctx, collector.MetricQuery{Metric: "cpu"})
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
}

func TestMemoryStore_QueryByLabels(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	now := time.Now()
	_ = ms.Write(ctx, []collector.MetricPoint{
		makePoint("inst1", "m", 1, now, map[string]string{"db": "prod"}),
		makePoint("inst1", "m", 2, now, map[string]string{"db": "test"}),
		makePoint("inst1", "m", 3, now.Add(time.Second), map[string]string{"db": "prod"}),
	})

	results, _ := ms.Query(ctx, collector.MetricQuery{Labels: map[string]string{"db": "prod"}})
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
}

func TestMemoryStore_QueryTimeRange(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 10 {
		_ = ms.Write(ctx, []collector.MetricPoint{
			makePoint("inst1", "m", float64(i), base.Add(time.Duration(i)*time.Minute), nil),
		})
	}

	results, _ := ms.Query(ctx, collector.MetricQuery{
		Start: base.Add(3 * time.Minute),
		End:   base.Add(6 * time.Minute),
	})
	if len(results) != 4 { // minutes 3,4,5,6
		t.Fatalf("got %d, want 4", len(results))
	}
}

func TestMemoryStore_QueryLimit(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	base := time.Now()
	for i := range 100 {
		_ = ms.Write(ctx, []collector.MetricPoint{
			makePoint("inst1", "m", float64(i), base.Add(time.Duration(i)*time.Second), nil),
		})
	}

	results, _ := ms.Query(ctx, collector.MetricQuery{Limit: 10})
	if len(results) != 10 {
		t.Fatalf("got %d, want 10", len(results))
	}
	// Should be the LAST 10 (most recent)
	if results[0].Value != 90 {
		t.Errorf("first value = %v, want 90", results[0].Value)
	}
	if results[9].Value != 99 {
		t.Errorf("last value = %v, want 99", results[9].Value)
	}
}

func TestMemoryStore_Expiry(t *testing.T) {
	ms := NewMemoryStore(100 * time.Millisecond)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	_ = ms.Write(ctx, []collector.MetricPoint{
		makePoint("inst1", "m", 1, time.Now(), nil),
	})

	results, _ := ms.Query(ctx, collector.MetricQuery{})
	if len(results) != 1 {
		t.Fatalf("before expiry: got %d, want 1", len(results))
	}

	time.Sleep(200 * time.Millisecond)
	ms.expire() // trigger manually

	results, _ = ms.Query(ctx, collector.MetricQuery{})
	if len(results) != 0 {
		t.Fatalf("after expiry: got %d, want 0", len(results))
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer func() { _ = ms.Close() }()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 100 {
				_ = ms.Write(ctx, []collector.MetricPoint{
					makePoint("inst1", "m", float64(n*100+j), time.Now(), nil),
				})
			}
		}(i)
	}
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_, _ = ms.Query(ctx, collector.MetricQuery{})
			}
		}()
	}
	wg.Wait()
}

func TestMemoryStore_Close(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	_ = ms.Close()
	_ = ms.Close() // double close should not panic

	// Write after close should still work (Close only stops the expiry goroutine)
	err := ms.Write(context.Background(), []collector.MetricPoint{
		makePoint("inst1", "m", 1, time.Now(), nil),
	})
	if err != nil {
		t.Fatalf("write after close: %v", err)
	}
}
