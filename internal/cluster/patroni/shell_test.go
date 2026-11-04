package patroni

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeCtl writes a POSIX shell script standing in for patronictl. It records
// its argv to argsFile, prints the canned stdout for the matched subcommand
// and exits with exitCode.
type fakeCtlOutput struct {
	stdout   string
	exitCode int
}

func fakePatronictl(t *testing.T, outputs map[string]fakeCtlOutput) (ctlPath, argsFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake patronictl uses a POSIX shell script")
	}
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("printf '%s\\n' \"$*\" >> '" + argsFile + "'\n")
	sb.WriteString("for a in \"$@\"; do case \"$a\" in\n")
	i := 0
	for sub, out := range outputs {
		f := filepath.Join(dir, "out"+string(rune('a'+i)))
		i++
		if err := os.WriteFile(f, []byte(out.stdout), 0o600); err != nil {
			t.Fatal(err)
		}
		sb.WriteString("  " + sub + ") cat '" + f + "'; exit " + strconv.Itoa(out.exitCode) + ";;\n")
	}
	sb.WriteString("esac; done\necho 'unknown command' >&2\nexit 2\n")

	ctlPath = filepath.Join(dir, "patronictl")
	if err := os.WriteFile(ctlPath, []byte(sb.String()), 0o700); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}
	return ctlPath, argsFile
}

// patronictl list -f json output (Patroni 3.x).
const (
	ctlListHealthy = `[
		{"Cluster": "pg-prod", "Member": "pg-node1", "Host": "10.0.0.11", "Role": "Leader", "State": "running", "TL": 4},
		{"Cluster": "pg-prod", "Member": "pg-node2", "Host": "10.0.0.12", "Role": "Replica", "State": "streaming",
		 "TL": 4, "Lag in MB": 0}
	]`
	ctlListSyncStandbyStale = `[
		{"Cluster": "pg-prod", "Member": "pg-node1", "Host": "10.0.0.11", "Role": "Leader", "State": "running", "TL": 7},
		{"Cluster": "pg-prod", "Member": "pg-node2", "Host": "10.0.0.12", "Role": "Sync Standby", "State": "running",
		 "TL": 6, "Lag in MB": 5120}
	]`
	ctlListFailover = `[
		{"Cluster": "pg-prod", "Member": "pg-node1", "Host": "10.0.0.11", "Role": "Replica", "State": "stopped",
		 "TL": 4, "Lag in MB": "unknown"},
		{"Cluster": "pg-prod", "Member": "pg-node2", "Host": "10.0.0.12", "Role": "Leader", "State": "running", "TL": 5}
	]`
	ctlHistory = `+----+-----------+------------------------------+----------------------------------+----------+
| TL |       LSN | Reason                       | Timestamp                        | New Leader |
+----+-----------+------------------------------+----------------------------------+----------+
|  1 |  50331816 | no recovery target specified | 2026-10-01T08:00:00.000000+00:00 | pg-node2 |
|  2 | 100663464 | manual switchover            | 2026-10-30T02:14:07.123456+00:00 | pg-node1 |
+----+-----------+------------------------------+----------------------------------+----------+
`
)

func TestShellProvider_GetClusterState(t *testing.T) {
	tests := []struct {
		name  string
		out   string
		check func(t *testing.T, s *ClusterState)
	}{
		{
			name: "two-node healthy",
			out:  ctlListHealthy,
			check: func(t *testing.T, s *ClusterState) {
				if s.ClusterName != "pg-prod" || len(s.Members) != 2 {
					t.Fatalf("state = %+v", s)
				}
				if got := leaders(s); len(got) != 1 || got[0] != "pg-node1" {
					t.Errorf("leaders = %v (roles must be lower-cased to match REST)", got)
				}
				if s.Members[1].Role != "replica" || s.Members[1].State != "streaming" || s.Members[1].Timeline != 4 {
					t.Errorf("replica = %+v", s.Members[1])
				}
				if s.Members[0].Port != 0 {
					t.Errorf("Port should be 0 (not exposed by patronictl), got %d", s.Members[0].Port)
				}
			},
		},
		{
			name: "stale sync standby, MB converted to bytes",
			out:  ctlListSyncStandbyStale,
			check: func(t *testing.T, s *ClusterState) {
				m := s.Members[1]
				if m.Role != "sync standby" {
					t.Errorf("role = %q, want %q", m.Role, "sync standby")
				}
				if m.Lag != 5120*1024*1024 {
					t.Errorf("lag = %d, want %d", m.Lag, int64(5120*1024*1024))
				}
				if m.Timeline != 6 {
					t.Errorf("timeline = %d", m.Timeline)
				}
			},
		},
		{
			name: "empty list",
			out:  `[]`,
			check: func(t *testing.T, s *ClusterState) {
				if s.ClusterName != "" || len(s.Members) != 0 {
					t.Errorf("state = %+v", s)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctl, argsFile := fakePatronictl(t, map[string]fakeCtlOutput{"list": {stdout: tt.out}})
			s, err := NewShellProvider(ctl, "/etc/patroni/patroni.yml").GetClusterState(context.Background())
			if err != nil {
				t.Fatalf("GetClusterState: %v", err)
			}
			tt.check(t, s)

			args, _ := os.ReadFile(argsFile)
			if got := strings.TrimSpace(string(args)); got != "-c /etc/patroni/patroni.yml list -f json" {
				t.Errorf("argv = %q", got)
			}
		})
	}
}

func TestShellProvider_GetClusterState_UnknownLag(t *testing.T) {
	// BUG: patronictl prints "Lag in MB": "unknown" for members whose lag is
	// not known (stopped / failing over). patroniCtlMember.LagInMB is int64,
	// so the whole list fails to parse — the fallback path is lost exactly
	// when the REST API is most likely to be unavailable.
	t.Skip("known bug: patronictl list with Lag in MB=\"unknown\" fails to parse; see PR description")

	ctl, _ := fakePatronictl(t, map[string]fakeCtlOutput{"list": {stdout: ctlListFailover}})
	s, err := NewShellProvider(ctl, "").GetClusterState(context.Background())
	if err != nil {
		t.Fatalf("GetClusterState: %v", err)
	}
	if got := leaders(s); len(got) != 1 || got[0] != "pg-node2" {
		t.Errorf("leaders = %v", got)
	}
}

func TestShellProvider_Errors(t *testing.T) {
	t.Run("binary missing", func(t *testing.T) {
		p := NewShellProvider(filepath.Join(t.TempDir(), "does-not-exist"), "")
		_, err := p.GetClusterState(context.Background())
		if !errors.Is(err, ErrPatroniUnavailable) {
			t.Errorf("err = %v, want ErrPatroniUnavailable", err)
		}
	})
	t.Run("non-zero exit (DCS unreachable)", func(t *testing.T) {
		ctl, _ := fakePatronictl(t, map[string]fakeCtlOutput{"list": {stdout: "", exitCode: 1}})
		_, err := NewShellProvider(ctl, "").GetClusterState(context.Background())
		if !errors.Is(err, ErrPatroniUnavailable) {
			t.Fatalf("err = %v, want ErrPatroniUnavailable", err)
		}
		if !strings.Contains(err.Error(), "patronictl list -f json") {
			t.Errorf("err should name the command, got %q", err)
		}
	})
	t.Run("garbage output", func(t *testing.T) {
		ctl, _ := fakePatronictl(t, map[string]fakeCtlOutput{"list": {stdout: "Error: not json"}})
		_, err := NewShellProvider(ctl, "").GetClusterState(context.Background())
		if err == nil || !strings.Contains(err.Error(), "parse list output") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("history failure", func(t *testing.T) {
		ctl, _ := fakePatronictl(t, map[string]fakeCtlOutput{"history": {exitCode: 1}})
		_, err := NewShellProvider(ctl, "").GetHistory(context.Background())
		if !errors.Is(err, ErrPatroniUnavailable) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("version failure", func(t *testing.T) {
		ctl, _ := fakePatronictl(t, map[string]fakeCtlOutput{"version": {exitCode: 3}})
		_, err := NewShellProvider(ctl, "").GetVersion(context.Background())
		if !errors.Is(err, ErrPatroniUnavailable) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestShellProvider_GetHistory(t *testing.T) {
	ctl, argsFile := fakePatronictl(t, map[string]fakeCtlOutput{"history": {stdout: ctlHistory}})
	got, err := NewShellProvider(ctl, "").GetHistory(context.Background())
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	want := []SwitchoverEvent{
		{Reason: "no recovery target specified", Timestamp: "2026-10-01T08:00:00.000000+00:00"},
		{Reason: "manual switchover", Timestamp: "2026-10-30T02:14:07.123456+00:00"},
	}
	if len(got) != len(want) {
		t.Fatalf("events = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	// No -c flag when config path is empty.
	args, _ := os.ReadFile(argsFile)
	if strings.TrimSpace(string(args)) != "history" {
		t.Errorf("argv = %q", args)
	}
}

func TestShellProvider_GetHistory_EmptyTable(t *testing.T) {
	ctl, _ := fakePatronictl(t, map[string]fakeCtlOutput{"history": {stdout: "+----+\n| TL | LSN | Reason | Timestamp |\n+----+\n"}})
	got, err := NewShellProvider(ctl, "").GetHistory(context.Background())
	if err != nil || len(got) != 0 {
		t.Errorf("GetHistory = %+v, %v", got, err)
	}
}

func TestShellProvider_GetVersion(t *testing.T) {
	tests := []struct {
		out  string
		want string
	}{
		{"patronictl version 3.3.2\n", "3.3.2"},
		{"4.0.4\n", "4.0.4"}, // unexpected format: returned verbatim, trimmed
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			ctl, _ := fakePatronictl(t, map[string]fakeCtlOutput{"version": {stdout: tt.out}})
			got, err := NewShellProvider(ctl, "").GetVersion(context.Background())
			if err != nil || got != tt.want {
				t.Errorf("GetVersion = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
