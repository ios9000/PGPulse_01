package patroni

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestNewProvider_Selection(t *testing.T) {
	tests := []struct {
		name string
		cfg  PatroniConfig
		want string // "noop", "rest", "shell", "fallback"
		// for shell: expected resolved ctl path
		wantCtl string
	}{
		{name: "nothing configured", cfg: PatroniConfig{}, want: "noop"},
		{name: "rest only", cfg: PatroniConfig{PatroniURL: "http://10.0.0.11:8008"}, want: "rest"},
		{name: "ctl only", cfg: PatroniConfig{PatroniCtlPath: "/opt/patroni/bin/patronictl"},
			want: "shell", wantCtl: "/opt/patroni/bin/patronictl"},
		{name: "config only defaults ctl path", cfg: PatroniConfig{PatroniConfig: "/etc/patroni/patroni.yml"},
			want: "shell", wantCtl: "/usr/bin/patronictl"},
		{name: "rest and ctl", cfg: PatroniConfig{
			PatroniURL: "http://10.0.0.11:8008", PatroniCtlPath: "/usr/local/bin/patronictl"}, want: "fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider(tt.cfg)
			switch v := p.(type) {
			case *NoOpProvider:
				if tt.want != "noop" {
					t.Fatalf("got NoOpProvider, want %s", tt.want)
				}
			case *RESTProvider:
				if tt.want != "rest" {
					t.Fatalf("got RESTProvider, want %s", tt.want)
				}
				if v.baseURL != tt.cfg.PatroniURL {
					t.Errorf("baseURL = %q", v.baseURL)
				}
			case *ShellProvider:
				if tt.want != "shell" {
					t.Fatalf("got ShellProvider, want %s", tt.want)
				}
				if v.ctlPath != tt.wantCtl || v.configPath != tt.cfg.PatroniConfig {
					t.Errorf("shell = %+v", v)
				}
			case *FallbackProvider:
				if tt.want != "fallback" {
					t.Fatalf("got FallbackProvider, want %s", tt.want)
				}
				if _, ok := v.primary.(*RESTProvider); !ok {
					t.Errorf("primary = %T, want *RESTProvider", v.primary)
				}
				if _, ok := v.secondary.(*ShellProvider); !ok {
					t.Errorf("secondary = %T, want *ShellProvider", v.secondary)
				}
			default:
				t.Fatalf("unexpected provider type %T", p)
			}
		})
	}
}

func TestNoOpProvider_ReturnsNotConfigured(t *testing.T) {
	p := &NoOpProvider{}
	ctx := context.Background()

	s, err := p.GetClusterState(ctx)
	if s != nil || !errors.Is(err, ErrPatroniNotConfigured) {
		t.Errorf("GetClusterState = %v, %v", s, err)
	}
	h, err := p.GetHistory(ctx)
	if h != nil || !errors.Is(err, ErrPatroniNotConfigured) {
		t.Errorf("GetHistory = %v, %v", h, err)
	}
	v, err := p.GetVersion(ctx)
	if v != "" || !errors.Is(err, ErrPatroniNotConfigured) {
		t.Errorf("GetVersion = %q, %v", v, err)
	}
	if errors.Is(err, ErrPatroniUnavailable) {
		t.Errorf("NotConfigured must be distinguishable from Unavailable")
	}
}

// Lost DCS quorum on the local node: REST returns 503, the provider chain must
// fall back to the secondary (e.g. patronictl talking to a different node).
func TestFallbackProvider_RESTDownFallsBackToSecondary(t *testing.T) {
	srv := newPatroniServer(t, map[string]route{
		"/cluster": {http.StatusServiceUnavailable, `{}`},
		"/history": {http.StatusServiceUnavailable, `{}`},
		"/":        {http.StatusServiceUnavailable, `{}`},
	})
	secondary := &mockPatroniProvider{
		clusterState: &ClusterState{ClusterName: "pg-prod", Members: []ClusterMember{{Name: "pg-node2", Role: "leader"}}},
		history:      []SwitchoverEvent{{Reason: "failover"}},
		version:      "3.3.2",
	}
	fp := NewFallbackProvider(NewRESTProvider(srv.URL), secondary)
	ctx := context.Background()

	s, err := fp.GetClusterState(ctx)
	if err != nil || s.Members[0].Name != "pg-node2" {
		t.Errorf("GetClusterState = %+v, %v", s, err)
	}
	h, err := fp.GetHistory(ctx)
	if err != nil || len(h) != 1 {
		t.Errorf("GetHistory = %+v, %v", h, err)
	}
	v, err := fp.GetVersion(ctx)
	if err != nil || v != "3.3.2" {
		t.Errorf("GetVersion = %q, %v", v, err)
	}
}

func TestFallbackProvider_SecondaryErrorIsReturned(t *testing.T) {
	primaryErr := errors.New("primary")
	fp := NewFallbackProvider(
		&mockPatroniProvider{clusterErr: primaryErr},
		&NoOpProvider{},
	)
	_, err := fp.GetClusterState(context.Background())
	if !errors.Is(err, ErrPatroniNotConfigured) {
		t.Errorf("err = %v, want the secondary's error", err)
	}
	if errors.Is(err, primaryErr) {
		t.Errorf("primary error should not leak when secondary is tried")
	}
}
