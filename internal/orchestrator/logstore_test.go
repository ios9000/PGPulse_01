package orchestrator

import (
	"context"
	"testing"

	"github.com/ios9000/PGPulse_01/internal/collector"
)

// discardLogger is defined in group_test.go and shared across this package's test files.

func TestLogStore_Write(t *testing.T) {
	store := NewLogStore(discardLogger())
	points := make([]collector.MetricPoint, 10)
	for i := range points {
		points[i] = makePoint("pg.test")
	}
	if err := store.Write(context.Background(), points); err != nil {
		t.Errorf("Write() error = %v", err)
	}
}

func TestLogStore_Write_Empty(t *testing.T) {
	store := NewLogStore(discardLogger())
	if err := store.Write(context.Background(), nil); err != nil {
		t.Errorf("Write(nil) error = %v", err)
	}
}

func TestLogStore_Query(t *testing.T) {
	store := NewLogStore(discardLogger())
	pts, err := store.Query(context.Background(), collector.MetricQuery{})
	if err != nil {
		t.Errorf("Query() error = %v", err)
	}
	if pts != nil {
		t.Errorf("Query() = %v, want nil", pts)
	}
}

func TestLogStore_Close(t *testing.T) {
	store := NewLogStore(discardLogger())
	if err := store.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
