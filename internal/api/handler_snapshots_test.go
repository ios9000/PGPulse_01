package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ios9000/PGPulse_01/internal/config"
	"github.com/ios9000/PGPulse_01/internal/statements"
)

// mockPGSSStore implements statements.SnapshotStore for testing.
type mockPGSSStore struct {
	snapshots       []statements.Snapshot
	snapshotsByID   map[int64]*statements.Snapshot
	entries         map[int64][]statements.SnapshotEntry
	entryCount      map[int64]int
	queryEntries    []statements.SnapshotEntry
	querySnapshots  []statements.Snapshot
	listTotal       int
	latestSnapshots []statements.Snapshot
	writeErr        error
	getErr          error
	listErr         error
	latestErr       error
	entriesErr      error
	queryEntriesErr error
}

func (m *mockPGSSStore) WriteSnapshot(_ context.Context, _ statements.Snapshot, _ []statements.SnapshotEntry) (int64, error) {
	return 0, m.writeErr
}

func (m *mockPGSSStore) GetSnapshot(_ context.Context, id int64) (*statements.Snapshot, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if snap, ok := m.snapshotsByID[id]; ok {
		return snap, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockPGSSStore) GetSnapshotEntries(_ context.Context, snapshotID int64, _, _ int) ([]statements.SnapshotEntry, int, error) {
	if m.entriesErr != nil {
		return nil, 0, m.entriesErr
	}
	entries := m.entries[snapshotID]
	count := m.entryCount[snapshotID]
	return entries, count, nil
}

func (m *mockPGSSStore) ListSnapshots(_ context.Context, _ string, _ statements.SnapshotListOptions) ([]statements.Snapshot, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	return m.snapshots, m.listTotal, nil
}

func (m *mockPGSSStore) GetLatestSnapshots(_ context.Context, _ string, _ int) ([]statements.Snapshot, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return m.latestSnapshots, nil
}

func (m *mockPGSSStore) GetEntriesForQuery(_ context.Context, _ string, _ int64, _, _ time.Time) ([]statements.SnapshotEntry, []statements.Snapshot, error) {
	if m.queryEntriesErr != nil {
		return nil, nil, m.queryEntriesErr
	}
	return m.queryEntries, m.querySnapshots, nil
}

func (m *mockPGSSStore) CleanOld(_ context.Context, _ time.Time) error {
	return nil
}

func newPGSSTestServer(t *testing.T, store statements.SnapshotStore) *APIServer {
	t.Helper()
	instances := []config.InstanceConfig{
		{ID: "test-1", Name: "Test Instance", DSN: "postgres://localhost/test"},
	}
	srv := newTestServer(t, &mockStore{}, &mockPinger{}, instances)
	if store != nil {
		srv.SetPGSSStore(store, config.StatementSnapshotsConfig{TopN: 20})
	}
	return srv
}

func TestHandleListSnapshots(t *testing.T) {
	now := time.Now()
	store := &mockPGSSStore{
		snapshots: []statements.Snapshot{
			{ID: 1, InstanceID: "test-1", CapturedAt: now, TotalStatements: 10, TotalCalls: 100},
			{ID: 2, InstanceID: "test-1", CapturedAt: now.Add(-time.Hour), TotalStatements: 8, TotalCalls: 80},
		},
		listTotal: 2,
	}
	srv := newPGSSTestServer(t, store)

	r := chi.NewRouter()
	r.Get("/instances/{id}/snapshots", srv.handleListSnapshots)

	req := httptest.NewRequest(http.MethodGet, "/instances/test-1/snapshots", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["snapshots"]; !ok {
		t.Error("response missing 'snapshots' key")
	}
	if _, ok := resp["total"]; !ok {
		t.Error("response missing 'total' key")
	}
}

func TestHandleGetSnapshotWithEntries(t *testing.T) {
	now := time.Now()
	store := &mockPGSSStore{
		snapshotsByID: map[int64]*statements.Snapshot{
			1: {ID: 1, InstanceID: "test-1", CapturedAt: now, TotalStatements: 2},
		},
		entries: map[int64][]statements.SnapshotEntry{
			1: {
				{SnapshotID: 1, QueryID: 100, Query: "SELECT 1", Calls: 10},
				{SnapshotID: 1, QueryID: 200, Query: "SELECT 2", Calls: 20},
			},
		},
		entryCount: map[int64]int{1: 2},
	}
	srv := newPGSSTestServer(t, store)

	r := chi.NewRouter()
	r.Get("/instances/{id}/snapshots/{snapId}", srv.handleGetSnapshot)

	req := httptest.NewRequest(http.MethodGet, "/instances/test-1/snapshots/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["snapshot"]; !ok {
		t.Error("response missing 'snapshot' key")
	}
	if _, ok := resp["entries"]; !ok {
		t.Error("response missing 'entries' key")
	}
	if _, ok := resp["total_entries"]; !ok {
		t.Error("response missing 'total_entries' key")
	}
}

func TestHandleLatestDiff_NotEnoughSnapshots(t *testing.T) {
	store := &mockPGSSStore{
		latestSnapshots: []statements.Snapshot{
			{ID: 1, InstanceID: "test-1", CapturedAt: time.Now()},
		},
	}
	srv := newPGSSTestServer(t, store)

	r := chi.NewRouter()
	r.Get("/instances/{id}/snapshots/latest-diff", srv.handleLatestDiff)

	req := httptest.NewRequest(http.MethodGet, "/instances/test-1/snapshots/latest-diff", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLatestDiff_Success(t *testing.T) {
	now := time.Now()
	store := &mockPGSSStore{
		latestSnapshots: []statements.Snapshot{
			{ID: 2, InstanceID: "test-1", CapturedAt: now},
			{ID: 1, InstanceID: "test-1", CapturedAt: now.Add(-time.Hour)},
		},
		entries: map[int64][]statements.SnapshotEntry{
			1: {{SnapshotID: 1, QueryID: 100, Query: "SELECT 1", Calls: 10, TotalExecTime: 100}},
			2: {{SnapshotID: 2, QueryID: 100, Query: "SELECT 1", Calls: 20, TotalExecTime: 200}},
		},
		entryCount: map[int64]int{1: 1, 2: 1},
	}
	srv := newPGSSTestServer(t, store)

	r := chi.NewRouter()
	r.Get("/instances/{id}/snapshots/latest-diff", srv.handleLatestDiff)

	req := httptest.NewRequest(http.MethodGet, "/instances/test-1/snapshots/latest-diff", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var diff statements.DiffResult
	if err := json.Unmarshal(w.Body.Bytes(), &diff); err != nil {
		t.Fatal(err)
	}
	if diff.TotalCallsDelta != 10 {
		t.Errorf("expected calls delta 10, got %d", diff.TotalCallsDelta)
	}
}

func TestHandleManualCapture_NilCapturer(t *testing.T) {
	store := &mockPGSSStore{}
	srv := newPGSSTestServer(t, store)
	// pgssCapturer is nil by default

	r := chi.NewRouter()
	r.Post("/instances/{id}/snapshots/capture", srv.handleManualSnapshotCapture)

	req := httptest.NewRequest(http.MethodPost, "/instances/test-1/snapshots/capture", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePGSSStore_Nil(t *testing.T) {
	// pgssStore is nil — should return 404.
	srv := newPGSSTestServer(t, nil)

	r := chi.NewRouter()
	r.Get("/instances/{id}/snapshots", srv.handleListSnapshots)
	r.Get("/instances/{id}/snapshots/latest-diff", srv.handleLatestDiff)
	r.Get("/instances/{id}/snapshots/diff", srv.handleSnapshotDiff)
	r.Get("/instances/{id}/snapshots/{snapId}", srv.handleGetSnapshot)
	r.Get("/instances/{id}/query-insights/{queryid}", srv.handleQueryInsights)
	r.Get("/instances/{id}/workload-report", srv.handleWorkloadReport)

	tests := []struct {
		name string
		path string
	}{
		{"list", "/instances/test-1/snapshots"},
		{"latest-diff", "/instances/test-1/snapshots/latest-diff"},
		{"diff", "/instances/test-1/snapshots/diff?from=1&to=2"},
		{"get", "/instances/test-1/snapshots/1"},
		{"insights", "/instances/test-1/query-insights/123"},
		{"report", "/instances/test-1/workload-report"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
