package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/version"
)

// TestRegistryCoversAllConstructors fails when a New*Collector constructor is
// added to internal/collector without being added to instanceCollectors (or
// excludedConstructors / the DB collector path), so the matrix cannot silently
// miss a collector.
func TestRegistryCoversAllConstructors(t *testing.T) {
	declared := exportedConstructors(t, filepath.Join("..", "..", "internal", "collector"))
	used := usedConstructors(t, "harness.go")

	for name := range declared {
		if used[name] {
			continue
		}
		if _, ok := excludedConstructors[name]; ok {
			continue
		}
		t.Errorf("collector constructor %s is not run by the conformance harness; add it to instanceCollectors "+
			"(or excludedConstructors with a reason)", name)
	}
	for name := range excludedConstructors {
		if !declared[name] {
			t.Errorf("excludedConstructors lists %s, which no longer exists", name)
		}
	}
}

func exportedConstructors(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "New") && strings.HasSuffix(fn.Name.Name, "Collector") {
				out[fn.Name.Name] = true
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no collector constructors; wrong directory?")
	}
	return out
}

func usedConstructors(t *testing.T, file string) map[string]bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "collector" {
			out[sel.Sel.Name] = true
		}
		return true
	})
	return out
}

// TestInstanceCollectorNamesUnique guards the matrix row keys.
func TestInstanceCollectorNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range instanceCollectors("x", version.PGVersion{Major: 17}) {
		if seen[c.Name()] {
			t.Errorf("duplicate collector name %q", c.Name())
		}
		seen[c.Name()] = true
	}
	for name := range standbyPathCollectors {
		if !seen[name] {
			t.Errorf("standbyPathCollectors lists unknown collector %q", name)
		}
	}
}

func TestFinishStatus(t *testing.T) {
	var r CollectorResult
	finish(&r, nil, []Statement{{SQL: "a"}}, time.Now())
	if r.Status != StatusOK {
		t.Errorf("status = %s, want ok", r.Status)
	}
	finish(&r, nil, []Statement{{SQL: "a"}, {SQL: "b", Error: "boom"}}, time.Now())
	if r.Status != StatusWarn {
		t.Errorf("status = %s, want warn", r.Status)
	}
	finish(&r, os.ErrNotExist, nil, time.Now())
	if r.Status != StatusFail || r.Error == "" {
		t.Errorf("status = %s err=%q, want fail", r.Status, r.Error)
	}
}

func TestRenderMarkdown(t *testing.T) {
	rep := Report{
		Route: "test",
		Servers: []ServerResult{
			{Label: "pg16", ServerVersion: "16.4", ServerVersionNum: 160004, Detected: "16.0",
				Collectors: []CollectorResult{
					{Name: "a", Path: "primary", Status: StatusOK, Statements: []Statement{{SQL: "SELECT 1", Rows: 1}}},
					{Name: "b", Path: "primary", Status: StatusFail, Error: "b: boom",
						Statements: []Statement{{SQL: "SELECT x", Error: "ERROR: column \"x\" does not exist (SQLSTATE 42703)"}}},
				}},
			{Label: "pg18", ConnectError: "refused"},
		},
	}
	md := RenderMarkdown(rep)
	for _, want := range []string{
		"| `a` | ✅ ok (1) | not executed |",
		"| `b` | ❌ fail | not executed |",
		"catalog / version",
		"**NO** — refused",
		"16.0 (expected 16.4)",
		"SELECT x",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
	if s := Summarize(rep); s.OK != 1 || s.Fail != 1 || s.Unreachable != 1 {
		t.Errorf("summary = %+v", s)
	}
}

func TestCategory(t *testing.T) {
	cases := map[string]string{
		"ERROR: permission denied for function pg_read_file (SQLSTATE 42501)": "privilege",
		"ERROR: column \"blk_read_time\" does not exist (SQLSTATE 42703)":     "catalog / version",
		"ERROR: relation \"pg_stat_io\" does not exist (SQLSTATE 42P01)":      "catalog / version",
		"timeout": "other",
	}
	for in, want := range cases {
		if got := category(in); got != want {
			t.Errorf("category(%q) = %q, want %q", in, got, want)
		}
	}
}
