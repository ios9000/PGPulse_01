// Command conformance executes every PGPulse collector's SQL — in the variant
// the collector's own version gating selects — against real PostgreSQL servers
// and writes a collector × version matrix.
//
// Usage:
//
//	go run ./scripts/conformance \
//	    -target pg14=postgres://pgpulse_monitor:conformance@127.0.0.1:5414/postgres?sslmode=disable \
//	    -target pg15=... \
//	    -json docs/conformance/results.json -md docs/conformance/README.md
//
// Normally invoked through scripts/conformance/run.sh, which provisions the servers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"
)

type targetFlags []Target

func (t *targetFlags) String() string { return fmt.Sprint(len(*t)) }

func (t *targetFlags) Set(s string) error {
	label, dsn, ok := strings.Cut(s, "=")
	if !ok || label == "" || dsn == "" {
		return fmt.Errorf("want label=postgres://..., got %q", s)
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return fmt.Errorf("target %s: DSN must be a postgres:// URL", label)
	}
	db := *u
	db.Path = "/conformance"
	*t = append(*t, Target{Label: label, DSN: dsn, DBDSN: db.String()})
	return nil
}

// Report is the JSON document written by the harness.
type Report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Route       string         `json:"route"`
	Commit      string         `json:"commit,omitempty"`
	Servers     []ServerResult `json:"servers"`
}

func main() {
	var targets targetFlags
	flag.Var(&targets, "target", "label=DSN of a server to check (repeatable); DSN must connect as the pg_monitor-only role")
	jsonOut := flag.String("json", "", "write raw results as JSON to this path")
	mdOut := flag.String("md", "", "write the Markdown matrix report to this path")
	route := flag.String("route", "unspecified", "how the servers were provisioned (recorded in the report)")
	commit := flag.String("commit", "", "git commit the harness ran against (recorded in the report)")
	wait := flag.Duration("wait", 90*time.Second, "how long to retry the initial connection to each server")
	strict := flag.Bool("strict", false, "exit 1 if any collector is not ok")
	flag.Parse()

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "conformance: at least one -target is required")
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	h := &runner{wait: *wait, logger: logger}
	ctx := context.Background()

	rep := Report{GeneratedAt: time.Now().UTC().Truncate(time.Second), Route: *route, Commit: *commit}
	for _, t := range targets {
		logger.Info("checking server", "target", t.Label)
		res := h.run(ctx, t)
		if res.ConnectError != "" {
			logger.Error("server not executed", "target", t.Label, "error", res.ConnectError)
		}
		rep.Servers = append(rep.Servers, res)
	}

	if *jsonOut != "" {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*jsonOut, append(b, '\n'), 0o644); err != nil {
			fatal(err)
		}
	}
	md := RenderMarkdown(rep)
	if *mdOut != "" {
		if err := os.WriteFile(*mdOut, []byte(md), 0o644); err != nil {
			fatal(err)
		}
	} else {
		fmt.Print(md)
	}

	s := Summarize(rep)
	logger.Info("conformance finished", "servers", len(rep.Servers), "unreachable", s.Unreachable,
		"ok", s.OK, "warn", s.Warn, "fail", s.Fail)
	if s.Unreachable > 0 {
		os.Exit(3)
	}
	if *strict && (s.Warn > 0 || s.Fail > 0) {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "conformance:", err)
	os.Exit(1)
}
