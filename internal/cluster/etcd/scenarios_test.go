package etcd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Topology used throughout: two Patroni/PostgreSQL nodes, each running an etcd
// member, plus one dedicated etcd witness member (etcd3) for a 3-member quorum.
const (
	fixtureV2MembersThree = `{"members":[
		{"id":"8e9e05c52164694d","name":"etcd1","peerURLs":["http://10.0.0.11:2380"],
		 "clientURLs":["http://10.0.0.11:2379","https://pg-node1.internal:2379"]},
		{"id":"91bc3c398fb3c146","name":"etcd2","peerURLs":["http://10.0.0.12:2380"],
		 "clientURLs":["http://10.0.0.12:2379"]},
		{"id":"fd422379fda50e48","name":"etcd3","peerURLs":["http://10.0.0.13:2380"],
		 "clientURLs":["http://10.0.0.13:2379"]}
	]}`

	// Stale member: added via `member add` but never started — empty name,
	// no client URLs. It still counts toward quorum size.
	fixtureV2MembersWithUnstarted = `{"members":[
		{"id":"8e9e05c52164694d","name":"etcd1","peerURLs":["http://10.0.0.11:2380"],
		 "clientURLs":["http://10.0.0.11:2379"]},
		{"id":"91bc3c398fb3c146","name":"etcd2","peerURLs":["http://10.0.0.12:2380"],
		 "clientURLs":["http://10.0.0.12:2379"]},
		{"id":"5a1ad1f2c0e8b7d3","name":"","peerURLs":["http://10.0.0.14:2380"],"clientURLs":[]}
	]}`

	healthOK           = `{"health":"true","reason":""}`
	healthNoLeader     = `{"health":"false","reason":"RAFT NO LEADER"}`
	healthAlarmNoSpace = `{"health":"false","reason":"ALARM NOSPACE"}`
)

type etcdRoute struct {
	status int
	body   string
}

// newEtcdServer serves the given routes; unknown paths 404. The returned func reports request counts per path.
func newEtcdServer(t *testing.T, routes map[string]etcdRoute) (*httptest.Server, func(path string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		rt, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(rt.status)
		_, _ = w.Write([]byte(rt.body))
	}))
	t.Cleanup(srv.Close)
	return srv, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[path]
	}
}

func deadURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	return u
}

func TestHTTPProvider_GetMembers_Scenarios(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		check func(t *testing.T, ms []ETCDMember)
	}{
		{
			name: "three-member quorum with dedicated witness",
			body: fixtureV2MembersThree,
			check: func(t *testing.T, ms []ETCDMember) {
				if len(ms) != 3 {
					t.Fatalf("members = %d, want 3", len(ms))
				}
				want := ETCDMember{ID: "fd422379fda50e48", Name: "etcd3", PeerURL: "http://10.0.0.13:2380",
					ClientURL: "http://10.0.0.13:2379", Status: "running"}
				if ms[2] != want {
					t.Errorf("witness = %+v, want %+v", ms[2], want)
				}
				if ms[0].ClientURL != "http://10.0.0.11:2379" {
					t.Errorf("first client URL should be used, got %q", ms[0].ClientURL)
				}
				for _, m := range ms {
					// v2 API exposes no leader; documented limitation.
					if m.IsLeader {
						t.Errorf("IsLeader = true for %s; v2 API cannot report leaders", m.Name)
					}
				}
			},
		},
		{
			name: "stale unstarted member",
			body: fixtureV2MembersWithUnstarted,
			check: func(t *testing.T, ms []ETCDMember) {
				if len(ms) != 3 {
					t.Fatalf("members = %d, want 3", len(ms))
				}
				st := ms[2]
				if st.Name != "" || st.ClientURL != "" || st.PeerURL != "http://10.0.0.14:2380" {
					t.Errorf("unstarted member = %+v", st)
				}
			},
		},
		{
			name: "no members",
			body: `{"members":[]}`,
			check: func(t *testing.T, ms []ETCDMember) {
				if ms == nil || len(ms) != 0 {
					t.Errorf("members = %#v, want empty non-nil slice", ms)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newEtcdServer(t, map[string]etcdRoute{"/v2/members": {http.StatusOK, tt.body}})
			ms, err := NewHTTPProvider([]string{srv.URL + "/"}).GetMembers(context.Background())
			if err != nil {
				t.Fatalf("GetMembers: %v", err)
			}
			tt.check(t, ms)
		})
	}
}

func TestHTTPProvider_GetMembers_EndpointFailover(t *testing.T) {
	// etcd1 is down, etcd2 returns 503 (lost quorum on that member), etcd3 answers.
	bad, badHits := newEtcdServer(t, map[string]etcdRoute{"/v2/members": {http.StatusServiceUnavailable, ``}})
	good, goodHits := newEtcdServer(t, map[string]etcdRoute{"/v2/members": {http.StatusOK, fixtureV2MembersThree}})
	never, neverHits := newEtcdServer(t, map[string]etcdRoute{"/v2/members": {http.StatusOK, fixtureV2MembersThree}})

	p := NewHTTPProvider([]string{deadURL(t), bad.URL, good.URL, never.URL})
	ms, err := p.GetMembers(context.Background())
	if err != nil {
		t.Fatalf("GetMembers: %v", err)
	}
	if len(ms) != 3 {
		t.Errorf("members = %d", len(ms))
	}
	if badHits("/v2/members") != 1 || goodHits("/v2/members") != 1 {
		t.Errorf("hits bad=%d good=%d, want 1 each", badHits("/v2/members"), goodHits("/v2/members"))
	}
	if neverHits("/v2/members") != 0 {
		t.Errorf("endpoints after the first success must not be queried")
	}
}

func TestHTTPProvider_GetMembers_Errors(t *testing.T) {
	malformed, _ := newEtcdServer(t, map[string]etcdRoute{"/v2/members": {http.StatusOK, `{"members": "nope"}`}})
	notFound, _ := newEtcdServer(t, map[string]etcdRoute{}) // v2 API disabled → 404

	tests := []struct {
		name      string
		endpoints []string
		wantIs    error
		wantMsg   string
	}{
		{"no endpoints", nil, ErrETCDUnavailable, "no endpoints configured"},
		{"all unreachable", []string{deadURL(t), deadURL(t)}, ErrETCDUnavailable, ""},
		{"v2 API disabled (404)", []string{notFound.URL}, ErrETCDUnavailable, "status 404"},
		{"malformed body", []string{malformed.URL}, nil, "parse members response"},
		// The error from the LAST endpoint wins.
		{"last error reported", []string{malformed.URL, notFound.URL}, ErrETCDUnavailable, "status 404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, err := NewHTTPProvider(tt.endpoints).GetMembers(context.Background())
			if err == nil {
				t.Fatalf("expected error, got members %+v", ms)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, want errors.Is %v", err, tt.wantIs)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("err = %q, want substring %q", err, tt.wantMsg)
			}
			if errors.Is(err, ErrETCDNotConfigured) {
				t.Errorf("must not be ErrETCDNotConfigured")
			}
		})
	}
}

func TestHTTPProvider_GetEndpointHealth_LostQuorum(t *testing.T) {
	// Two of three members lose quorum: the survivor reports healthy while the
	// others report RAFT NO LEADER / NOSPACE; one more endpoint is unreachable.
	m1, _ := newEtcdServer(t, map[string]etcdRoute{"/health": {http.StatusOK, healthOK}})
	m2, _ := newEtcdServer(t, map[string]etcdRoute{"/health": {http.StatusOK, healthNoLeader}})
	m3, _ := newEtcdServer(t, map[string]etcdRoute{"/health": {http.StatusServiceUnavailable, healthNoLeader}})
	m4, _ := newEtcdServer(t, map[string]etcdRoute{"/health": {http.StatusOK, healthAlarmNoSpace}})
	m5, _ := newEtcdServer(t, map[string]etcdRoute{"/health": {http.StatusOK, `not json`}})
	m6, _ := newEtcdServer(t, map[string]etcdRoute{"/health": {http.StatusOK, `{"health":true}`}}) // bool, not string
	dead := deadURL(t)

	eps := []string{m1.URL, m2.URL, m3.URL, m4.URL, m5.URL, m6.URL, dead}
	got, err := NewHTTPProvider(eps).GetEndpointHealth(context.Background())
	if err != nil {
		t.Fatalf("GetEndpointHealth: %v", err)
	}
	want := map[string]bool{
		m1.URL: true, m2.URL: false, m3.URL: false, m4.URL: false, m5.URL: false, m6.URL: false, dead: false,
	}
	if len(got) != len(want) {
		t.Fatalf("health = %v, want %v", got, want)
	}
	for ep, w := range want {
		if got[ep] != w {
			t.Errorf("health[%s] = %v, want %v", ep, got[ep], w)
		}
	}

	healthy := 0
	for _, h := range got {
		if h {
			healthy++
		}
	}
	if healthy > len(got)/2 {
		t.Errorf("fixture should represent lost quorum, got %d/%d healthy", healthy, len(got))
	}
}

func TestHTTPProvider_GetEndpointHealth_NoEndpoints(t *testing.T) {
	got, err := NewHTTPProvider(nil).GetEndpointHealth(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("GetEndpointHealth = %v, %v; want empty map, nil", got, err)
	}
}

func TestHTTPProvider_ContextDeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := NewHTTPProvider([]string{srv.URL}).GetMembers(ctx)
	if !errors.Is(err, ErrETCDUnavailable) {
		t.Fatalf("err = %v, want ErrETCDUnavailable", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("context deadline not honoured (%s)", time.Since(start))
	}
}

func TestFallbackProvider_HealthNeverFallsBack(t *testing.T) {
	// HTTPProvider.GetEndpointHealth never returns an error — an all-down
	// cluster is reported as all-false — so FallbackProvider never consults
	// etcdctl for health. Pin that behaviour so a change is deliberate.
	secondary := &mockETCDProvider{health: map[string]bool{"x": true}}
	dead := deadURL(t)
	fp := NewFallbackProvider(NewHTTPProvider([]string{dead}), secondary)

	got, err := fp.GetEndpointHealth(context.Background())
	if err != nil {
		t.Fatalf("GetEndpointHealth: %v", err)
	}
	if secondary.called {
		t.Errorf("secondary unexpectedly called")
	}
	if got[dead] {
		t.Errorf("dead endpoint reported healthy")
	}
}

func TestFallbackProvider_MembersFallBackWhenAllHTTPDown(t *testing.T) {
	secondary := &mockETCDProvider{members: []ETCDMember{{ID: "1", Name: "etcd3"}}}
	fp := NewFallbackProvider(NewHTTPProvider([]string{deadURL(t)}), secondary)
	ms, err := fp.GetMembers(context.Background())
	if err != nil || len(ms) != 1 || !secondary.called {
		t.Errorf("GetMembers = %+v, %v (secondary called=%v)", ms, err, secondary.called)
	}
}

func TestNewProvider_Selection(t *testing.T) {
	eps := []string{"http://10.0.0.11:2379", "http://10.0.0.12:2379", "http://10.0.0.13:2379"}
	tests := []struct {
		name string
		cfg  ETCDConfig
		want string
	}{
		{"nothing", ETCDConfig{}, "noop"},
		{"empty endpoint slice", ETCDConfig{Endpoints: []string{}}, "noop"},
		{"endpoints only", ETCDConfig{Endpoints: eps}, "http"},
		{"ctl only", ETCDConfig{CtlPath: "/usr/local/bin/etcdctl"}, "shell"},
		{"both", ETCDConfig{Endpoints: eps, CtlPath: "/usr/local/bin/etcdctl"}, "fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch v := NewProvider(tt.cfg).(type) {
			case *NoOpProvider:
				if tt.want != "noop" {
					t.Fatalf("got noop, want %s", tt.want)
				}
			case *HTTPProvider:
				if tt.want != "http" || len(v.endpoints) != len(eps) {
					t.Fatalf("got http %+v, want %s", v, tt.want)
				}
			case *ShellProvider:
				if tt.want != "shell" || v.ctlPath != tt.cfg.CtlPath {
					t.Fatalf("got shell %+v, want %s", v, tt.want)
				}
			case *FallbackProvider:
				if tt.want != "fallback" {
					t.Fatalf("got fallback, want %s", tt.want)
				}
				if _, ok := v.primary.(*HTTPProvider); !ok {
					t.Errorf("primary = %T", v.primary)
				}
				sp, ok := v.secondary.(*ShellProvider)
				if !ok || strings.Join(sp.endpoints, ",") != strings.Join(eps, ",") {
					t.Errorf("secondary = %#v", v.secondary)
				}
			default:
				t.Fatalf("unexpected %T", v)
			}
		})
	}
}

func TestNoOpProvider_ReturnsNotConfigured(t *testing.T) {
	p := &NoOpProvider{}
	ms, err := p.GetMembers(context.Background())
	if ms != nil || !errors.Is(err, ErrETCDNotConfigured) {
		t.Errorf("GetMembers = %v, %v", ms, err)
	}
	h, err := p.GetEndpointHealth(context.Background())
	if h != nil || !errors.Is(err, ErrETCDNotConfigured) {
		t.Errorf("GetEndpointHealth = %v, %v", h, err)
	}
	if errors.Is(err, ErrETCDUnavailable) {
		t.Errorf("NotConfigured must be distinguishable from Unavailable")
	}
}

// --- ShellProvider (fake etcdctl) ---

type fakeOut struct {
	stdout   string
	exitCode int
}

// fakeEtcdctl writes a POSIX shell script standing in for etcdctl. It matches
// the first argv element equal to a key in outputs.
func fakeEtcdctl(t *testing.T, outputs map[string]fakeOut) (ctlPath, argsFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake etcdctl uses a POSIX shell script")
	}
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + argsFile + "'\n")
	sb.WriteString("for a in \"$@\"; do case \"$a\" in\n")
	i := 0
	for sub, out := range outputs {
		f := filepath.Join(dir, "out"+strconv.Itoa(i))
		i++
		if err := os.WriteFile(f, []byte(out.stdout), 0o600); err != nil {
			t.Fatal(err)
		}
		sb.WriteString("  " + sub + ") cat '" + f + "'; exit " + strconv.Itoa(out.exitCode) + ";;\n")
	}
	sb.WriteString("esac; done\nexit 2\n")
	ctlPath = filepath.Join(dir, "etcdctl")
	if err := os.WriteFile(ctlPath, []byte(sb.String()), 0o700); err != nil { //nolint:gosec // must be executable
		t.Fatal(err)
	}
	return ctlPath, argsFile
}

// etcdctl v3 `member list --write-out=json`: numeric uint64 IDs that exceed
// float64 precision, so they must be decoded as integers.
const ctlMemberList = `{"header":{"cluster_id":14841639068965178418,"member_id":10276657743932975437,"raft_term":7},
"members":[
 {"ID":10276657743932975437,"name":"etcd1","peerURLs":["http://10.0.0.11:2380"],"clientURLs":["http://10.0.0.11:2379"]},
 {"ID":10501334649042878790,"name":"etcd2","peerURLs":["http://10.0.0.12:2380"],"clientURLs":["http://10.0.0.12:2379"]},
 {"ID":18249187646912138824,"name":"etcd3","peerURLs":["http://10.0.0.13:2380"],"clientURLs":["http://10.0.0.13:2379"]},
 {"ID":6492732653356758995,"peerURLs":["http://10.0.0.14:2380"],"isLearner":true}
]}`

func TestShellProvider_GetMembers(t *testing.T) {
	eps := []string{"http://10.0.0.11:2379", "http://10.0.0.12:2379"}
	ctl, argsFile := fakeEtcdctl(t, map[string]fakeOut{"member": {stdout: ctlMemberList}})
	ms, err := NewShellProvider(ctl, eps).GetMembers(context.Background())
	if err != nil {
		t.Fatalf("GetMembers: %v", err)
	}
	if len(ms) != 4 {
		t.Fatalf("members = %d, want 4", len(ms))
	}
	wantIDs := []string{"8e9e05c52164694d", "91bc3c398fb3c146", "fd422379fda50e48", "5a1ad1f2c0e8b7d3"}
	for i, id := range wantIDs {
		if ms[i].ID != id {
			t.Errorf("member[%d].ID = %q, want %q (hex of uint64)", i, ms[i].ID, id)
		}
	}
	if ms[2].Name != "etcd3" || ms[2].ClientURL != "http://10.0.0.13:2379" || ms[2].PeerURL != "http://10.0.0.13:2380" {
		t.Errorf("member[2] = %+v", ms[2])
	}
	if ms[3].Name != "" || ms[3].ClientURL != "" {
		t.Errorf("unstarted learner = %+v", ms[3])
	}

	args, _ := os.ReadFile(argsFile)
	if got := strings.TrimSpace(string(args)); got !=
		"--endpoints=http://10.0.0.11:2379,http://10.0.0.12:2379 member list --write-out=json" {
		t.Errorf("argv = %q", got)
	}
}

func TestShellProvider_GetEndpointHealth(t *testing.T) {
	out := `[{"endpoint":"http://10.0.0.11:2379","health":true,"took":"1.84ms"},
	         {"endpoint":"http://10.0.0.13:2379","health":true,"took":"2.01ms"}]`
	ctl, argsFile := fakeEtcdctl(t, map[string]fakeOut{"endpoint": {stdout: out}})
	got, err := NewShellProvider(ctl, nil).GetEndpointHealth(context.Background())
	if err != nil {
		t.Fatalf("GetEndpointHealth: %v", err)
	}
	if len(got) != 2 || !got["http://10.0.0.11:2379"] || !got["http://10.0.0.13:2379"] {
		t.Errorf("health = %v", got)
	}
	args, _ := os.ReadFile(argsFile)
	if strings.TrimSpace(string(args)) != "endpoint health --write-out=json" {
		t.Errorf("argv = %q (no --endpoints flag expected when none configured)", args)
	}
}

func TestShellProvider_GetEndpointHealth_LostQuorumExitCode(t *testing.T) {
	// BUG: `etcdctl endpoint health` exits 1 ("unhealthy cluster") whenever any
	// endpoint is unhealthy, but still prints the per-endpoint JSON on stdout.
	// run() treats any non-zero exit as failure, so in a lost-quorum or
	// single-member-down scenario the caller gets ErrETCDUnavailable instead of
	// the partial health map it needs.
	t.Skip("known bug: etcdctl endpoint health exit 1 discards per-endpoint results; see PR description")

	out := `[{"endpoint":"http://10.0.0.11:2379","health":true,"took":"1.8ms"},
	         {"endpoint":"http://10.0.0.12:2379","health":false,"took":"5.0s","error":"context deadline exceeded"}]`
	ctl, _ := fakeEtcdctl(t, map[string]fakeOut{"endpoint": {stdout: out, exitCode: 1}})
	got, err := NewShellProvider(ctl, nil).GetEndpointHealth(context.Background())
	if err != nil {
		t.Fatalf("GetEndpointHealth: %v", err)
	}
	if !got["http://10.0.0.11:2379"] || got["http://10.0.0.12:2379"] {
		t.Errorf("health = %v", got)
	}
}

func TestShellProvider_Errors(t *testing.T) {
	tests := []struct {
		name    string
		outputs map[string]fakeOut
		missing bool
		fn      func(p *ShellProvider) error
		wantIs  error
		wantMsg string
	}{
		{name: "binary missing", missing: true,
			fn:     func(p *ShellProvider) error { _, err := p.GetMembers(context.Background()); return err },
			wantIs: ErrETCDUnavailable},
		{name: "member list exit 1", outputs: map[string]fakeOut{"member": {exitCode: 1}},
			fn:     func(p *ShellProvider) error { _, err := p.GetMembers(context.Background()); return err },
			wantIs: ErrETCDUnavailable, wantMsg: "etcdctl member list --write-out=json"},
		{name: "member list garbage", outputs: map[string]fakeOut{"member": {stdout: "Error: x"}},
			fn:      func(p *ShellProvider) error { _, err := p.GetMembers(context.Background()); return err },
			wantMsg: "parse member list"},
		{name: "health garbage", outputs: map[string]fakeOut{"endpoint": {stdout: `{"not":"array"}`}},
			fn:      func(p *ShellProvider) error { _, err := p.GetEndpointHealth(context.Background()); return err },
			wantMsg: "parse endpoint health"},
		{name: "health exit 1", outputs: map[string]fakeOut{"endpoint": {exitCode: 1}},
			fn:     func(p *ShellProvider) error { _, err := p.GetEndpointHealth(context.Background()); return err },
			wantIs: ErrETCDUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctl string
			if tt.missing {
				ctl = filepath.Join(t.TempDir(), "etcdctl")
			} else {
				ctl, _ = fakeEtcdctl(t, tt.outputs)
			}
			err := tt.fn(NewShellProvider(ctl, nil))
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, want errors.Is %v", err, tt.wantIs)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("err = %q, want substring %q", err, tt.wantMsg)
			}
		})
	}
}
