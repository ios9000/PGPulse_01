package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildLockTree_Empty(t *testing.T) {
	result := BuildLockTree(nil)

	if result.Summary.RootBlockers != 0 {
		t.Errorf("RootBlockers = %d, want 0", result.Summary.RootBlockers)
	}
	if result.Summary.TotalBlocked != 0 {
		t.Errorf("TotalBlocked = %d, want 0", result.Summary.TotalBlocked)
	}
	if result.Summary.MaxDepth != 0 {
		t.Errorf("MaxDepth = %d, want 0", result.Summary.MaxDepth)
	}
	if len(result.Locks) != 0 {
		t.Errorf("Locks length = %d, want 0", len(result.Locks))
	}
}

func TestBuildLockTree_EmptySlice(t *testing.T) {
	result := BuildLockTree([]RawLockEntry{})

	if result.Summary.RootBlockers != 0 {
		t.Errorf("RootBlockers = %d, want 0", result.Summary.RootBlockers)
	}
	if len(result.Locks) != 0 {
		t.Errorf("Locks length = %d, want 0", len(result.Locks))
	}
}

func TestBuildLockTree_NoBlocking(t *testing.T) {
	raw := []RawLockEntry{
		{PID: 100, Usename: "alice", Datname: "db1", State: "active", Query: "SELECT 1"},
		{PID: 200, Usename: "bob", Datname: "db1", State: "idle", Query: "SELECT 2"},
		{PID: 300, Usename: "carol", Datname: "db1", State: "active", Query: "SELECT 3"},
	}

	result := BuildLockTree(raw)

	if result.Summary.RootBlockers != 0 {
		t.Errorf("RootBlockers = %d, want 0", result.Summary.RootBlockers)
	}
	if result.Summary.TotalBlocked != 0 {
		t.Errorf("TotalBlocked = %d, want 0", result.Summary.TotalBlocked)
	}
	if result.Summary.MaxDepth != 0 {
		t.Errorf("MaxDepth = %d, want 0", result.Summary.MaxDepth)
	}
	if len(result.Locks) != 0 {
		t.Errorf("Locks length = %d, want 0 (uninvolved sessions excluded)", len(result.Locks))
	}
}

func TestBuildLockTree_SingleRoot(t *testing.T) {
	raw := []RawLockEntry{
		{PID: 100, Usename: "alice", Datname: "db1", State: "active", Query: "UPDATE t1 SET x=1", BlockingPIDs: nil},
		{PID: 200, Usename: "bob", Datname: "db1", State: "active", Query: "UPDATE t1 SET x=2", BlockingPIDs: []int{100}},
		{PID: 300, Usename: "carol", Datname: "db1", State: "active", Query: "UPDATE t1 SET x=3", BlockingPIDs: []int{100}},
	}

	result := BuildLockTree(raw)

	if result.Summary.RootBlockers != 1 {
		t.Errorf("RootBlockers = %d, want 1", result.Summary.RootBlockers)
	}
	if result.Summary.TotalBlocked != 2 {
		t.Errorf("TotalBlocked = %d, want 2", result.Summary.TotalBlocked)
	}
	if result.Summary.MaxDepth != 1 {
		t.Errorf("MaxDepth = %d, want 1", result.Summary.MaxDepth)
	}
	if len(result.Locks) != 3 {
		t.Fatalf("Locks length = %d, want 3", len(result.Locks))
	}

	// Verify entries are sorted by depth then PID.
	byPID := map[int]LockEntry{}
	for _, e := range result.Locks {
		byPID[e.PID] = e
	}

	root := byPID[100]
	if root.Depth != 0 {
		t.Errorf("PID 100 depth = %d, want 0", root.Depth)
	}
	if !root.IsRoot {
		t.Error("PID 100 should be root")
	}
	if root.BlockingCount != 2 {
		t.Errorf("PID 100 BlockingCount = %d, want 2", root.BlockingCount)
	}

	child200 := byPID[200]
	if child200.Depth != 1 {
		t.Errorf("PID 200 depth = %d, want 1", child200.Depth)
	}
	if child200.IsRoot {
		t.Error("PID 200 should not be root")
	}
	if child200.ParentPID == nil || *child200.ParentPID != 100 {
		t.Errorf("PID 200 ParentPID = %v, want 100", child200.ParentPID)
	}

	child300 := byPID[300]
	if child300.Depth != 1 {
		t.Errorf("PID 300 depth = %d, want 1", child300.Depth)
	}
	if child300.ParentPID == nil || *child300.ParentPID != 100 {
		t.Errorf("PID 300 ParentPID = %v, want 100", child300.ParentPID)
	}
}

func TestBuildLockTree_MultiRoot(t *testing.T) {
	raw := []RawLockEntry{
		{PID: 100, Usename: "alice", Datname: "db1", State: "active", Query: "UPDATE t1"},
		{PID: 200, Usename: "bob", Datname: "db1", State: "active", Query: "UPDATE t1", BlockingPIDs: []int{100}},
		{PID: 300, Usename: "carol", Datname: "db2", State: "active", Query: "UPDATE t2"},
		{PID: 400, Usename: "dave", Datname: "db2", State: "active", Query: "UPDATE t2", BlockingPIDs: []int{300}},
	}

	result := BuildLockTree(raw)

	if result.Summary.RootBlockers != 2 {
		t.Errorf("RootBlockers = %d, want 2", result.Summary.RootBlockers)
	}
	if result.Summary.TotalBlocked != 2 {
		t.Errorf("TotalBlocked = %d, want 2", result.Summary.TotalBlocked)
	}
	if result.Summary.MaxDepth != 1 {
		t.Errorf("MaxDepth = %d, want 1", result.Summary.MaxDepth)
	}
	if len(result.Locks) != 4 {
		t.Fatalf("Locks length = %d, want 4", len(result.Locks))
	}

	// Verify both roots are at depth 0.
	byPID := map[int]LockEntry{}
	for _, e := range result.Locks {
		byPID[e.PID] = e
	}
	if byPID[100].Depth != 0 || !byPID[100].IsRoot {
		t.Error("PID 100 should be root at depth 0")
	}
	if byPID[300].Depth != 0 || !byPID[300].IsRoot {
		t.Error("PID 300 should be root at depth 0")
	}
}

func TestBuildLockTree_DeepChain(t *testing.T) {
	raw := []RawLockEntry{
		{PID: 100, Usename: "alice", Datname: "db1", State: "active", Query: "UPDATE t1"},
		{PID: 200, Usename: "bob", Datname: "db1", State: "active", Query: "UPDATE t1", BlockingPIDs: []int{100}},
		{PID: 300, Usename: "carol", Datname: "db1", State: "active", Query: "UPDATE t1", BlockingPIDs: []int{200}},
	}

	result := BuildLockTree(raw)

	if result.Summary.RootBlockers != 1 {
		t.Errorf("RootBlockers = %d, want 1", result.Summary.RootBlockers)
	}
	if result.Summary.TotalBlocked != 2 {
		t.Errorf("TotalBlocked = %d, want 2", result.Summary.TotalBlocked)
	}
	if result.Summary.MaxDepth != 2 {
		t.Errorf("MaxDepth = %d, want 2", result.Summary.MaxDepth)
	}
	if len(result.Locks) != 3 {
		t.Fatalf("Locks length = %d, want 3", len(result.Locks))
	}

	byPID := map[int]LockEntry{}
	for _, e := range result.Locks {
		byPID[e.PID] = e
	}

	if byPID[100].Depth != 0 {
		t.Errorf("PID 100 depth = %d, want 0", byPID[100].Depth)
	}
	if byPID[200].Depth != 1 {
		t.Errorf("PID 200 depth = %d, want 1", byPID[200].Depth)
	}
	if byPID[300].Depth != 2 {
		t.Errorf("PID 300 depth = %d, want 2", byPID[300].Depth)
	}

	// Verify parent chain.
	if byPID[200].ParentPID == nil || *byPID[200].ParentPID != 100 {
		t.Errorf("PID 200 parent = %v, want 100", byPID[200].ParentPID)
	}
	if byPID[300].ParentPID == nil || *byPID[300].ParentPID != 200 {
		t.Errorf("PID 300 parent = %v, want 200", byPID[300].ParentPID)
	}
}

func TestBuildLockTree_ExcludesUninvolved(t *testing.T) {
	raw := []RawLockEntry{
		{PID: 100, Usename: "alice", Datname: "db1", State: "active", Query: "UPDATE t1"},
		{PID: 200, Usename: "bob", Datname: "db1", State: "active", Query: "UPDATE t1", BlockingPIDs: []int{100}},
		{PID: 300, Usename: "carol", Datname: "db1", State: "idle", Query: "SELECT 1"},
		{PID: 400, Usename: "dave", Datname: "db1", State: "idle", Query: "SELECT 2"},
		{PID: 500, Usename: "eve", Datname: "db1", State: "active", Query: "SELECT 3"},
	}

	result := BuildLockTree(raw)

	if len(result.Locks) != 2 {
		t.Errorf("Locks length = %d, want 2 (only PIDs 100 and 200)", len(result.Locks))
	}

	pids := map[int]bool{}
	for _, e := range result.Locks {
		pids[e.PID] = true
	}
	if !pids[100] {
		t.Error("PID 100 should be in output")
	}
	if !pids[200] {
		t.Error("PID 200 should be in output")
	}
	if pids[300] || pids[400] || pids[500] {
		t.Error("uninvolved PIDs should not be in output")
	}
}

func TestBuildLockTree_DurationAndMetadata(t *testing.T) {
	wet := "Lock"
	we := "transactionid"
	raw := []RawLockEntry{
		{
			PID: 100, Usename: "alice", Datname: "db1", State: "active",
			Query: "UPDATE t1", DurationSeconds: 5.5,
		},
		{
			PID: 200, Usename: "bob", Datname: "db1", State: "active",
			WaitEventType: &wet, WaitEvent: &we,
			Query: "UPDATE t1", DurationSeconds: 3.2, BlockingPIDs: []int{100},
		},
	}

	result := BuildLockTree(raw)

	if len(result.Locks) != 2 {
		t.Fatalf("Locks length = %d, want 2", len(result.Locks))
	}

	byPID := map[int]LockEntry{}
	for _, e := range result.Locks {
		byPID[e.PID] = e
	}

	root := byPID[100]
	if root.DurationSeconds != 5.5 {
		t.Errorf("PID 100 duration = %f, want 5.5", root.DurationSeconds)
	}
	if root.Usename != "alice" {
		t.Errorf("PID 100 usename = %q, want alice", root.Usename)
	}

	child := byPID[200]
	if child.WaitEventType == nil || *child.WaitEventType != "Lock" {
		t.Errorf("PID 200 WaitEventType = %v, want Lock", child.WaitEventType)
	}
	if child.WaitEvent == nil || *child.WaitEvent != "transactionid" {
		t.Errorf("PID 200 WaitEvent = %v, want transactionid", child.WaitEvent)
	}
	if child.BlockedByCount != 1 {
		t.Errorf("PID 200 BlockedByCount = %d, want 1", child.BlockedByCount)
	}
}

func TestHandleLockTree_NoConnProvider(t *testing.T) {
	srv := newTestServer(t, &mockStore{}, nil, testInstance("test-1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/test-1/activity/locks", nil)
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp.Error.Code != "not_available" {
		t.Errorf("expected error code 'not_available', got %q", errResp.Error.Code)
	}
}

func TestHandleLockTree_InstanceNotFound(t *testing.T) {
	srv := newTestServer(t, &mockStore{}, nil, testInstance("test-1"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/nonexistent/activity/locks", nil)
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp.Error.Code != "not_found" {
		t.Errorf("expected error code 'not_found', got %q", errResp.Error.Code)
	}
}
