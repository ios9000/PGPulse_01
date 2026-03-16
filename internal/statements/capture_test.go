package statements

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestNullStoreInterface verifies that NullSnapshotStore satisfies the SnapshotStore interface.
func TestNullStoreInterface(t *testing.T) {
	var _ SnapshotStore = &NullSnapshotStore{}
}

// TestPGSnapshotStoreInterface verifies that PGSnapshotStore satisfies the SnapshotStore interface.
func TestPGSnapshotStoreInterface(t *testing.T) {
	var _ SnapshotStore = &PGSnapshotStore{}
}

// mockInstanceLister implements InstanceLister for tests.
type mockInstanceLister struct {
	ids []string
	err error
}

func (m *mockInstanceLister) ListInstanceIDs(_ context.Context) ([]string, error) {
	return m.ids, m.err
}

// mockConnProvider implements ConnProvider for tests.
type mockConnProvider struct {
	pools map[string]*pgxpool.Pool
	err   error
}

func (m *mockConnProvider) PoolForInstance(id string) (*pgxpool.Pool, error) {
	if m.err != nil {
		return nil, m.err
	}
	p, ok := m.pools[id]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return p, nil
}

func TestNewSnapshotCapturer(t *testing.T) {
	store := &NullSnapshotStore{}
	lister := &mockInstanceLister{ids: []string{"inst1"}}
	connProv := &mockConnProvider{}
	logger := slog.Default()

	capturer := NewSnapshotCapturer(store, connProv, lister, 5*time.Minute, 7, true, logger)
	if capturer == nil {
		t.Fatal("expected non-nil capturer")
	}
	if capturer.interval != 5*time.Minute {
		t.Errorf("interval: got %v, want 5m", capturer.interval)
	}
	if capturer.retention != 7*24*time.Hour {
		t.Errorf("retention: got %v, want 168h", capturer.retention)
	}
	if !capturer.onStartup {
		t.Error("onStartup should be true")
	}
}

func TestSnapshotCapturerStartStop(t *testing.T) {
	store := &NullSnapshotStore{}
	lister := &mockInstanceLister{ids: nil}
	connProv := &mockConnProvider{}
	logger := slog.Default()

	capturer := NewSnapshotCapturer(store, connProv, lister, 1*time.Hour, 7, false, logger)

	ctx := context.Background()
	capturer.Start(ctx)

	// Give goroutine time to start.
	time.Sleep(10 * time.Millisecond)

	capturer.Stop()
	// Should not panic on double stop.
	capturer.Stop()
}
