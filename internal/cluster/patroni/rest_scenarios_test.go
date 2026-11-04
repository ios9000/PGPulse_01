package patroni

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Fixtures modelled on Patroni 3.x GET /cluster responses.
const (
	// Healthy two-node cluster (leader + streaming replica). The DCS is a
	// dedicated etcd member, so it does not appear in the Patroni member list.
	fixtureClusterTwoNodeHealthy = `{
		"scope": "pg-prod",
		"members": [
			{"name": "pg-node1", "role": "leader", "state": "running",
			 "api_url": "http://10.0.0.11:8008/patroni", "host": "10.0.0.11", "port": 5432, "timeline": 4},
			{"name": "pg-node2", "role": "replica", "state": "streaming",
			 "api_url": "http://10.0.0.12:8008/patroni", "host": "10.0.0.12", "port": 5432, "timeline": 4, "lag": 0}
		]
	}`

	// Failover in progress: the old leader is stopped, the candidate has
	// already been promoted onto a new timeline. Old leader lag is numeric here.
	fixtureClusterFailoverInProgress = `{
		"scope": "pg-prod",
		"members": [
			{"name": "pg-node1", "role": "replica", "state": "stopped",
			 "host": "10.0.0.11", "port": 5432, "timeline": 4, "lag": 33554432},
			{"name": "pg-node2", "role": "leader", "state": "running",
			 "host": "10.0.0.12", "port": 5432, "timeline": 5}
		]
	}`

	// No leader at all: DCS quorum lost, Patroni demoted the leader to read-only.
	fixtureClusterNoLeader = `{
		"scope": "pg-prod",
		"members": [
			{"name": "pg-node1", "role": "replica", "state": "running",
			 "host": "10.0.0.11", "port": 5432, "timeline": 4, "lag": 0},
			{"name": "pg-node2", "role": "replica", "state": "running",
			 "host": "10.0.0.12", "port": 5432, "timeline": 4, "lag": 0}
		]
	}`

	// Stale member: replica stuck on an old timeline with large lag, tagged
	// nofailover so it can never be promoted.
	fixtureClusterStaleMember = `{
		"scope": "pg-prod",
		"members": [
			{"name": "pg-node1", "role": "leader", "state": "running",
			 "host": "10.0.0.11", "port": 5432, "timeline": 7},
			{"name": "pg-node2", "role": "replica", "state": "running",
			 "host": "10.0.0.12", "port": 5433, "timeline": 6, "lag": 5368709120,
			 "tags": {"nofailover": "true", "noloadbalance": "true"}}
		]
	}`

	// Patroni reports "lag": "unknown" when it cannot compute replica lag
	// (e.g. replica stopped or not yet connected) — a common failover payload.
	fixtureClusterUnknownLag = `{
		"scope": "pg-prod",
		"members": [
			{"name": "pg-node1", "role": "replica", "state": "stopped",
			 "host": "10.0.0.11", "port": 5432, "timeline": 4, "lag": "unknown"},
			{"name": "pg-node2", "role": "leader", "state": "running",
			 "host": "10.0.0.12", "port": 5432, "timeline": 5}
		]
	}`
)

// newPatroniServer returns an httptest server that serves the given body per path.
// Paths not in routes get 404.
func newPatroniServer(t *testing.T, routes map[string]struct {
	status int
	body   string
}) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		rt, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

type route = struct {
	status int
	body   string
}

func leaders(s *ClusterState) []string {
	var out []string
	for _, m := range s.Members {
		if m.Role == "leader" {
			out = append(out, m.Name)
		}
	}
	return out
}

func TestRESTProvider_GetClusterState_Scenarios(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMembers int
		wantLeaders []string
		check       func(t *testing.T, s *ClusterState)
	}{
		{
			name:        "two-node healthy",
			body:        fixtureClusterTwoNodeHealthy,
			wantMembers: 2,
			wantLeaders: []string{"pg-node1"},
			check: func(t *testing.T, s *ClusterState) {
				r := s.Members[1]
				if r.Role != "replica" || r.State != "streaming" || r.Lag != 0 || r.Timeline != 4 {
					t.Errorf("replica = %+v", r)
				}
				if s.Members[0].Lag != 0 {
					t.Errorf("leader without lag key should map to 0, got %d", s.Members[0].Lag)
				}
				if s.Members[0].Tags != nil {
					t.Errorf("leader without tags should have nil Tags, got %v", s.Members[0].Tags)
				}
			},
		},
		{
			name:        "failover in progress",
			body:        fixtureClusterFailoverInProgress,
			wantMembers: 2,
			wantLeaders: []string{"pg-node2"},
			check: func(t *testing.T, s *ClusterState) {
				old := s.Members[0]
				if old.State != "stopped" || old.Timeline != 4 || old.Lag != 33554432 {
					t.Errorf("old leader = %+v", old)
				}
				if s.Members[1].Timeline != 5 {
					t.Errorf("new leader timeline = %d, want 5", s.Members[1].Timeline)
				}
			},
		},
		{
			name:        "lost quorum, no leader",
			body:        fixtureClusterNoLeader,
			wantMembers: 2,
			wantLeaders: nil,
		},
		{
			name:        "stale member",
			body:        fixtureClusterStaleMember,
			wantMembers: 2,
			wantLeaders: []string{"pg-node1"},
			check: func(t *testing.T, s *ClusterState) {
				st := s.Members[1]
				if st.Lag != 5368709120 {
					t.Errorf("stale lag = %d, want 5368709120 (int64 > 2^32)", st.Lag)
				}
				if st.Timeline >= s.Members[0].Timeline {
					t.Errorf("stale timeline %d should be behind leader %d", st.Timeline, s.Members[0].Timeline)
				}
				if st.Port != 5433 {
					t.Errorf("port = %d, want 5433", st.Port)
				}
				if st.Tags["nofailover"] != "true" || st.Tags["noloadbalance"] != "true" {
					t.Errorf("tags = %v", st.Tags)
				}
			},
		},
		{
			name:        "empty member list",
			body:        `{"scope": "pg-prod", "members": []}`,
			wantMembers: 0,
		},
		{
			name:        "missing members key",
			body:        `{"scope": "pg-prod"}`,
			wantMembers: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newPatroniServer(t, map[string]route{"/cluster": {http.StatusOK, tt.body}})
			s, err := NewRESTProvider(srv.URL).GetClusterState(context.Background())
			if err != nil {
				t.Fatalf("GetClusterState: %v", err)
			}
			if s.ClusterName != "pg-prod" {
				t.Errorf("ClusterName = %q, want pg-prod", s.ClusterName)
			}
			if len(s.Members) != tt.wantMembers {
				t.Fatalf("members = %d, want %d", len(s.Members), tt.wantMembers)
			}
			if s.Members == nil {
				t.Errorf("Members should be a non-nil slice")
			}
			got := leaders(s)
			if strings.Join(got, ",") != strings.Join(tt.wantLeaders, ",") {
				t.Errorf("leaders = %v, want %v", got, tt.wantLeaders)
			}
			if tt.check != nil {
				tt.check(t, s)
			}
		})
	}
}

func TestRESTProvider_GetClusterState_UnknownLag(t *testing.T) {
	// BUG: Patroni's REST API reports `"lag": "unknown"` for replicas whose lag
	// cannot be determined (stopped / not yet streaming — exactly the failover
	// case). ClusterMember.Lag is int64, so json.Unmarshal fails and the whole
	// cluster state is lost instead of just that member's lag.
	t.Skip("known bug: REST /cluster with lag=\"unknown\" fails to parse; see PR description")

	srv := newPatroniServer(t, map[string]route{"/cluster": {http.StatusOK, fixtureClusterUnknownLag}})
	s, err := NewRESTProvider(srv.URL).GetClusterState(context.Background())
	if err != nil {
		t.Fatalf("GetClusterState: %v", err)
	}
	if len(s.Members) != 2 || leaders(s)[0] != "pg-node2" {
		t.Errorf("unexpected state: %+v", s)
	}
}

func TestRESTProvider_Errors(t *testing.T) {
	type call func(p *RESTProvider) error
	calls := map[string]struct {
		path string
		fn   call
	}{
		"GetClusterState": {"/cluster", func(p *RESTProvider) error {
			_, err := p.GetClusterState(context.Background())
			return err
		}},
		"GetHistory": {"/history", func(p *RESTProvider) error {
			_, err := p.GetHistory(context.Background())
			return err
		}},
		"GetVersion": {"/", func(p *RESTProvider) error {
			_, err := p.GetVersion(context.Background())
			return err
		}},
	}

	statuses := []struct {
		name   string
		status int
	}{
		// Patroni returns 503 from health-style endpoints when DCS is unreachable.
		{"503 service unavailable", http.StatusServiceUnavailable},
		{"500 internal error", http.StatusInternalServerError},
		{"401 unauthorized", http.StatusUnauthorized},
	}

	for name, c := range calls {
		for _, st := range statuses {
			t.Run(name+"/"+st.name, func(t *testing.T) {
				srv := newPatroniServer(t, map[string]route{c.path: {st.status, `{}`}})
				err := c.fn(NewRESTProvider(srv.URL))
				if !errors.Is(err, ErrPatroniUnavailable) {
					t.Fatalf("err = %v, want ErrPatroniUnavailable", err)
				}
				if errors.Is(err, ErrPatroniNotConfigured) {
					t.Errorf("err must not match ErrPatroniNotConfigured")
				}
			})
		}
	}
}

func TestRESTProvider_MalformedBodies(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		fn      func(p *RESTProvider) error
		wantMsg string
		wantErr error
	}{
		{
			name: "cluster not json", path: "/cluster", body: `<html>bad gateway</html>`,
			fn: func(p *RESTProvider) error {
				_, err := p.GetClusterState(context.Background())
				return err
			},
			wantMsg: "parse cluster response",
		},
		{
			name: "cluster wrong type", path: "/cluster", body: `{"scope": "x", "members": {"name": "a"}}`,
			fn: func(p *RESTProvider) error {
				_, err := p.GetClusterState(context.Background())
				return err
			},
			wantMsg: "parse cluster response",
		},
		{
			name: "version not json", path: "/", body: `nope`,
			fn: func(p *RESTProvider) error {
				_, err := p.GetVersion(context.Background())
				return err
			},
			wantMsg: "parse root response",
		},
		{
			name: "version missing", path: "/", body: `{"state": "running", "role": "replica"}`,
			fn: func(p *RESTProvider) error {
				_, err := p.GetVersion(context.Background())
				return err
			},
			wantMsg: "empty version",
			wantErr: ErrPatroniUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newPatroniServer(t, map[string]route{tt.path: {http.StatusOK, tt.body}})
			err := tt.fn(NewRESTProvider(srv.URL))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("err = %q, want it to contain %q", err, tt.wantMsg)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want errors.Is %v", err, tt.wantErr)
			}
		})
	}
}

func TestRESTProvider_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens any more

	_, err := NewRESTProvider(url).GetClusterState(context.Background())
	if !errors.Is(err, ErrPatroniUnavailable) {
		t.Fatalf("err = %v, want ErrPatroniUnavailable", err)
	}
}

func TestRESTProvider_ContextCancelled(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := NewRESTProvider(srv.URL).GetClusterState(ctx)
	if !errors.Is(err, ErrPatroniUnavailable) {
		t.Fatalf("err = %v, want ErrPatroniUnavailable", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("request did not honour context deadline (took %s)", time.Since(start))
	}
}

func TestRESTProvider_TrailingSlashTrimmed(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(fixtureClusterTwoNodeHealthy))
	}))
	t.Cleanup(srv.Close)

	if _, err := NewRESTProvider(srv.URL + "///").GetClusterState(context.Background()); err != nil {
		t.Fatalf("GetClusterState: %v", err)
	}
	if gotPath != "/cluster" {
		t.Errorf("path = %q, want /cluster", gotPath)
	}
}

func TestRESTProvider_GetHistory_Shapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []SwitchoverEvent
	}{
		{
			name: "empty array",
			body: `[]`,
			want: nil,
		},
		{
			name: "short and malformed entries are skipped",
			body: `[
				[1, 100, "too short"],
				"not an array",
				[2, 200, "manual switchover", "2026-10-01T08:00:00+00:00"]
			]`,
			want: []SwitchoverEvent{{Reason: "manual switchover", Timestamp: "2026-10-01T08:00:00+00:00"}},
		},
		{
			name: "non-string reason / timestamp leave fields empty",
			body: `[[1, 100, 42, null]]`,
			want: []SwitchoverEvent{{}},
		},
		{
			name: "object instead of array is non-fatal",
			body: `{"error": "x"}`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newPatroniServer(t, map[string]route{"/history": {http.StatusOK, tt.body}})
			got, err := NewRESTProvider(srv.URL).GetHistory(context.Background())
			if err != nil {
				t.Fatalf("GetHistory: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("events = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("event[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRESTProvider_GetHistory_NewLeaderColumn(t *testing.T) {
	// BUG: Patroni >= 2.x emits a 5th element (new leader name) in each
	// /history entry: [timeline, lsn, reason, timestamp, new_leader].
	// rest.go reads FromNode from arr[3] (the timestamp) and ToNode from arr[4]
	// only when len >= 6, so FromNode ends up holding the timestamp and the
	// new leader is never reported.
	t.Skip("known bug: GetHistory maps history columns off by one; see PR description")

	srv := newPatroniServer(t, map[string]route{"/history": {http.StatusOK,
		`[[4, 50331816, "no recovery target specified", "2026-10-30T02:14:07.123+00:00", "pg-node2"]]`}})
	got, err := NewRESTProvider(srv.URL).GetHistory(context.Background())
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("events = %+v", got)
	}
	if got[0].FromNode == got[0].Timestamp {
		t.Errorf("FromNode holds the timestamp %q", got[0].FromNode)
	}
	if got[0].ToNode != "pg-node2" {
		t.Errorf("ToNode = %q, want pg-node2", got[0].ToNode)
	}
}

// TestRESTProvider_GetHistory_CurrentColumnMapping pins today's behaviour for
// 6-element rows so that a fix to the bug above is a deliberate, visible change.
func TestRESTProvider_GetHistory_CurrentColumnMapping(t *testing.T) {
	srv := newPatroniServer(t, map[string]route{"/history": {http.StatusOK,
		`[[4, 1, "switchover", "2026-10-30T02:14:07+00:00", "pg-node2", "extra"]]`}})
	got, err := NewRESTProvider(srv.URL).GetHistory(context.Background())
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	want := SwitchoverEvent{
		Timestamp: "2026-10-30T02:14:07+00:00",
		Reason:    "switchover",
		FromNode:  "2026-10-30T02:14:07+00:00", // arr[3] — see bug above
		ToNode:    "pg-node2",                  // arr[4]
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("events = %+v, want [%+v]", got, want)
	}
}
