package storage

import (
	"sort"
	"testing"
)

func TestMigrateFS_ContainsFiles(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	for _, want := range []string{"001_metrics.sql", "002_timescaledb.sql"} {
		if !names[want] {
			t.Errorf("migration %q not found in embedded FS", want)
		}
	}
}

func TestMigrateFS_FilesAreSorted(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)

	for i := range names {
		if names[i] != sorted[i] {
			t.Errorf("files not in sorted order: got %v, want %v", names, sorted)
			break
		}
	}
}

func TestIsConditional_TimescaleDisabled(t *testing.T) {
	if !isConditional("002_timescaledb.sql", MigrateOptions{UseTimescaleDB: false}) {
		t.Error("expected isConditional=true for timescaledb migration when UseTimescaleDB=false")
	}
}

func TestIsConditional_TimescaleEnabled(t *testing.T) {
	if isConditional("002_timescaledb.sql", MigrateOptions{UseTimescaleDB: true}) {
		t.Error("expected isConditional=false for timescaledb migration when UseTimescaleDB=true")
	}
}

func TestIsConditional_RegularMigration(t *testing.T) {
	for _, name := range []string{"001_metrics.sql", "003_future.sql", "anything.sql"} {
		if isConditional(name, MigrateOptions{UseTimescaleDB: false}) {
			t.Errorf("expected isConditional=false for %q", name)
		}
		if isConditional(name, MigrateOptions{UseTimescaleDB: true}) {
			t.Errorf("expected isConditional=false for %q", name)
		}
	}
}
