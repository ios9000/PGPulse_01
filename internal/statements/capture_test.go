package statements

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ios9000/PGPulse_01/internal/version"
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

// TestPgssGateSelect verifies the pg_stat_statements block-timing columns chosen
// for PG 16 (pre-1.11 blk_*_time) and PG 17/18 (1.11 shared_/local_blk_*_time).
func TestPgssGateSelect(t *testing.T) {
	v17plus := []string{
		"s.shared_blk_read_time + s.local_blk_read_time AS blk_read_time",
		"s.shared_blk_write_time + s.local_blk_write_time AS blk_write_time",
	}
	tests := []struct {
		major   int
		want    []string
		notWant []string
	}{
		{
			major:   16,
			want:    []string{"s.blk_read_time, s.blk_write_time,", "s.total_exec_time"},
			notWant: []string{"shared_blk_read_time", "local_blk_read_time"},
		},
		{major: 17, want: v17plus, notWant: []string{"s.blk_read_time", "s.blk_write_time"}},
		{major: 18, want: v17plus, notWant: []string{"s.blk_read_time", "s.blk_write_time"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("PG%d", tt.major), func(t *testing.T) {
			sql, ok := pgssGate.Select(version.PGVersion{Major: tt.major, Num: tt.major * 10000})
			if !ok {
				t.Fatalf("expected gate to select a variant for PG %d", tt.major)
			}
			for _, s := range tt.want {
				if !strings.Contains(sql, s) {
					t.Errorf("PG %d variant missing %q", tt.major, s)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(sql, s) {
					t.Errorf("PG %d variant must not contain %q", tt.major, s)
				}
			}
		})
	}
}
