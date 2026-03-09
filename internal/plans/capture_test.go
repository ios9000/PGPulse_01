package plans

import (
	"testing"
	"time"
)

func TestFingerprint_Stable(t *testing.T) {
	// Same query -> same fingerprint
	fp1 := Fingerprint("SELECT * FROM foo")
	fp2 := Fingerprint("SELECT * FROM foo")
	if fp1 != fp2 {
		t.Errorf("same query gave different fingerprints: %q vs %q", fp1, fp2)
	}
	// Whitespace variation -> same fingerprint
	fp3 := Fingerprint("SELECT *   FROM   foo")
	if fp1 != fp3 {
		t.Errorf("whitespace-normalized query gave different fingerprint: %q vs %q", fp1, fp3)
	}
	// Case insensitive
	fp4 := Fingerprint("select * from FOO")
	if fp1 != fp4 {
		t.Errorf("case-different query gave different fingerprint: %q vs %q", fp1, fp4)
	}
}

func TestFingerprint_Different(t *testing.T) {
	fp1 := Fingerprint("SELECT * FROM foo")
	fp2 := Fingerprint("SELECT * FROM bar")
	if fp1 == fp2 {
		t.Error("different queries gave same fingerprint")
	}
}

func TestFingerprint_NonEmpty(t *testing.T) {
	fp := Fingerprint("SELECT 1")
	if fp == "" {
		t.Error("fingerprint should not be empty")
	}
	// MD5 hex digest is always 32 chars
	if len(fp) != 32 {
		t.Errorf("fingerprint length = %d, want 32", len(fp))
	}
}

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  SELECT   *   FROM   foo  ", "select * from foo"},
		{"SELECT\t1", "select 1"},
		{"  \n  SELECT  \n  1  \n  ", "select 1"},
		{"ALREADY lowercase", "already lowercase"},
	}
	for _, tc := range tests {
		got := NormalizeQuery(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeQuery(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestIsDDL(t *testing.T) {
	tests := []struct {
		q    string
		want bool
	}{
		{"DROP TABLE foo", true},
		{"CREATE INDEX idx ON t(c)", true},
		{"TRUNCATE TABLE foo", true},
		{"ALTER TABLE foo ADD col int", true},
		{"REINDEX TABLE foo", true},
		{"SELECT 1", false},
		{"INSERT INTO foo VALUES(1)", false},
		{"UPDATE foo SET x=1", false},
		{"DELETE FROM foo", false},
		{"  DROP TABLE foo", true},   // leading whitespace
		{"  create index i on t(c)", true}, // lowercase with whitespace
	}
	for _, tc := range tests {
		if got := IsDDL(tc.q); got != tc.want {
			t.Errorf("IsDDL(%q) = %v, want %v", tc.q, got, tc.want)
		}
	}
}

func TestIsDDLStrict(t *testing.T) {
	tests := []struct {
		q    string
		want bool
	}{
		{"INSERT INTO foo VALUES(1)", true},
		{"UPDATE foo SET x=1", true},
		{"DELETE FROM foo", true},
		{"DROP TABLE foo", true},
		{"CREATE INDEX idx ON t(c)", true},
		{"TRUNCATE TABLE foo", true},
		{"ALTER TABLE foo ADD col int", true},
		{"REINDEX TABLE foo", true},
		{"SELECT 1", false},
		{"  insert into foo values(1)", true}, // leading whitespace + lowercase
	}
	for _, tc := range tests {
		if got := IsDDLStrict(tc.q); got != tc.want {
			t.Errorf("IsDDLStrict(%q) = %v, want %v", tc.q, got, tc.want)
		}
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello"},
		{"exact", 5, "exact"},
		{"", 5, ""},
		{"abc", 0, ""},
	}
	for _, tc := range tests {
		got := truncateStr(tc.s, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
		}
	}
}

func TestDedupCache_BlocksRecent(t *testing.T) {
	c := &dedupCache{
		entries: make(map[string]time.Time),
		window:  5 * time.Second,
	}
	// First call: not seen
	if c.seen("inst1", "fp1") {
		t.Error("first call should return false")
	}
	// Second call within window: seen
	if !c.seen("inst1", "fp1") {
		t.Error("second call within window should return true")
	}
}

func TestDedupCache_DifferentKeys(t *testing.T) {
	c := &dedupCache{
		entries: make(map[string]time.Time),
		window:  5 * time.Second,
	}
	c.seen("inst1", "fp1")
	// Different fingerprint: not seen
	if c.seen("inst1", "fp2") {
		t.Error("different fingerprint should return false")
	}
	// Different instance: not seen
	if c.seen("inst2", "fp1") {
		t.Error("different instance should return false")
	}
}

func TestDedupCache_ExpiredEntry(t *testing.T) {
	c := &dedupCache{
		entries: make(map[string]time.Time),
		window:  1 * time.Millisecond,
	}
	c.seen("inst1", "fp1")
	// Wait for expiry
	time.Sleep(5 * time.Millisecond)
	// Should be treated as new
	if c.seen("inst1", "fp1") {
		t.Error("expired entry should return false")
	}
}
