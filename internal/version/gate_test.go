package version

import "testing"

func pg(major int) PGVersion { return PGVersion{Major: major, Num: major * 10000} }

func TestAtLeast(t *testing.T) {
	tests := []struct {
		v            PGVersion
		major, minor int
		want         bool
	}{
		{PGVersion{Major: 13, Minor: 9}, 14, 0, false},
		{PGVersion{Major: 14, Minor: 0}, 14, 0, true},
		{PGVersion{Major: 14, Minor: 5}, 14, 6, false},
		{PGVersion{Major: 14, Minor: 6}, 14, 6, true},
		{PGVersion{Major: 15, Minor: 0}, 14, 99, true},
		{PGVersion{Major: 19, Minor: 0}, 18, 0, true},
		{PGVersion{Major: 17, Minor: 0}, 18, 0, false},
	}
	for _, tt := range tests {
		if got := tt.v.AtLeast(tt.major, tt.minor); got != tt.want {
			t.Errorf("%d.%d AtLeast(%d,%d) = %v, want %v", tt.v.Major, tt.v.Minor, tt.major, tt.minor, got, tt.want)
		}
	}
}

func TestString(t *testing.T) {
	if got := (PGVersion{Major: 16, Minor: 4}).String(); got != "16.4" {
		t.Errorf("String() = %q, want 16.4", got)
	}
}

func TestVersionRangeContains(t *testing.T) {
	tests := []struct {
		name string
		r    VersionRange
		v    PGVersion
		want bool
	}{
		{"below min", VersionRange{MinMajor: 14}, pg(13), false},
		{"at min", VersionRange{MinMajor: 14}, pg(14), true},
		{"open upper bound 18", VersionRange{MinMajor: 14}, pg(18), true},
		{"open upper bound 19", VersionRange{MinMajor: 14}, pg(19), true},
		{"closed: at max", VersionRange{MinMajor: 14, MaxMajor: 16, MaxMinor: 99}, pg(16), true},
		{"closed: above max", VersionRange{MinMajor: 14, MaxMajor: 16, MaxMinor: 99}, pg(17), false},
		{"max minor inclusive", VersionRange{MinMajor: 14, MaxMajor: 14, MaxMinor: 5},
			PGVersion{Major: 14, Minor: 5}, true},
		{"max minor exceeded", VersionRange{MinMajor: 14, MaxMajor: 14, MaxMinor: 5},
			PGVersion{Major: 14, Minor: 6}, false},
		{"min minor not reached", VersionRange{MinMajor: 14, MinMinor: 3},
			PGVersion{Major: 14, Minor: 2}, false},
		// MaxMinor: 0 with a MaxMajor set caps the range at exactly X.0.
		{"max minor zero", VersionRange{MinMajor: 14, MaxMajor: 14}, PGVersion{Major: 14, Minor: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.Contains(tt.v); got != tt.want {
				t.Errorf("%+v.Contains(%d.%d) = %v, want %v", tt.r, tt.v.Major, tt.v.Minor, got, tt.want)
			}
		})
	}
}

// Gate shapes below mirror the gates used by collectors so the selection
// boundaries for every supported major (and the unsupported 13 / future 19)
// are pinned.

// Shape of collector.checkpointGate: 14–16 | 17+ (explicit MaxMajor 99).
var checkpointShape = Gate{Name: "checkpoint", Variants: []SQLVariant{
	{Range: VersionRange{MinMajor: 14, MaxMajor: 16, MaxMinor: 99}, SQL: "bgwriter"},
	{Range: VersionRange{MinMajor: 17, MaxMajor: 99, MaxMinor: 99}, SQL: "checkpointer"},
}}

// Shape of collector.replicationSlotsGate: 14 | 15 | 16+.
var slotsShape = Gate{Name: "replication_slots", Variants: []SQLVariant{
	{Range: VersionRange{MinMajor: 14, MaxMajor: 14, MaxMinor: 99}, SQL: "v14"},
	{Range: VersionRange{MinMajor: 15, MaxMajor: 15, MaxMinor: 99}, SQL: "v15"},
	{Range: VersionRange{MinMajor: 16, MaxMajor: 99, MaxMinor: 99}, SQL: "v16"},
}}

// Shape of collector.backupStateGate: 14 only.
var backupShape = Gate{Name: "backup_state", Variants: []SQLVariant{
	{Range: VersionRange{MinMajor: 14, MaxMajor: 14, MaxMinor: 99}, SQL: "pg_is_in_backup"},
}}

// Shape of statements.pgssGate: 13+ open-ended first, then 9–12.
var pgssShape = Gate{Name: "pgss", Variants: []SQLVariant{
	{Range: VersionRange{MinMajor: 13}, SQL: "exec_time"},
	{Range: VersionRange{MinMajor: 9, MaxMajor: 12, MaxMinor: 99}, SQL: "total_time"},
}}

func TestGateSelectBoundaries(t *testing.T) {
	type want struct {
		sql string
		ok  bool
	}
	none := want{"", false}
	tests := []struct {
		gate Gate
		byV  map[int]want
	}{
		{checkpointShape, map[int]want{
			13: none, 14: {"bgwriter", true}, 16: {"bgwriter", true},
			17: {"checkpointer", true}, 18: {"checkpointer", true}, 19: {"checkpointer", true},
		}},
		{slotsShape, map[int]want{
			13: none, 14: {"v14", true}, 15: {"v15", true}, 16: {"v16", true},
			17: {"v16", true}, 18: {"v16", true}, 19: {"v16", true},
		}},
		{backupShape, map[int]want{
			13: none, 14: {"pg_is_in_backup", true}, 15: none, 16: none, 17: none, 18: none, 19: none,
		}},
		{pgssShape, map[int]want{
			12: {"total_time", true}, 13: {"exec_time", true}, 14: {"exec_time", true},
			16: {"exec_time", true}, 17: {"exec_time", true}, 18: {"exec_time", true}, 19: {"exec_time", true},
		}},
	}
	for _, tt := range tests {
		for major, w := range tt.byV {
			sql, ok := tt.gate.Select(pg(major))
			if sql != w.sql || ok != w.ok {
				t.Errorf("%s.Select(%d) = (%q, %v), want (%q, %v)", tt.gate.Name, major, sql, ok, w.sql, w.ok)
			}
		}
	}
}

func TestGateSelectFirstMatchWins(t *testing.T) {
	g := Gate{Name: "overlap", Variants: []SQLVariant{
		{Range: VersionRange{MinMajor: 14}, SQL: "first"},
		{Range: VersionRange{MinMajor: 16}, SQL: "second"},
	}}
	for _, major := range []int{14, 16, 18, 19} {
		if sql, _ := g.Select(pg(major)); sql != "first" {
			t.Errorf("Select(%d) = %q, want first", major, sql)
		}
	}
}

func TestGateSelectEmpty(t *testing.T) {
	if sql, ok := (Gate{Name: "empty"}).Select(pg(16)); ok || sql != "" {
		t.Errorf("empty gate Select = (%q, %v), want (\"\", false)", sql, ok)
	}
}

// A real server reports its minor version, e.g. 16.4. Selection must not
// depend on it for open/99-capped ranges.
func TestGateSelectIgnoresMinorWithinMajor(t *testing.T) {
	for _, minor := range []int{0, 1, 4, 99} {
		v := PGVersion{Major: 16, Minor: minor}
		if sql, ok := slotsShape.Select(v); !ok || sql != "v16" {
			t.Errorf("Select(16.%d) = (%q, %v), want v16", minor, sql, ok)
		}
		v = PGVersion{Major: 14, Minor: minor}
		if sql, ok := backupShape.Select(v); !ok || sql != "pg_is_in_backup" {
			t.Errorf("backup Select(14.%d) = (%q, %v)", minor, sql, ok)
		}
	}
}
