package forecast

import (
	"testing"
)

func TestParseReloptions_Empty(t *testing.T) {
	result := parseReloptions(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestParseReloptions_WithOverrides(t *testing.T) {
	opts := []string{
		"autovacuum_vacuum_threshold=100",
		"autovacuum_vacuum_scale_factor=0.05",
		"fillfactor=80",
		"autovacuum_enabled=false",
	}
	result := parseReloptions(opts)

	if result["autovacuum_vacuum_threshold"] != "100" {
		t.Errorf("expected threshold=100, got %q", result["autovacuum_vacuum_threshold"])
	}
	if result["autovacuum_vacuum_scale_factor"] != "0.05" {
		t.Errorf("expected scale=0.05, got %q", result["autovacuum_vacuum_scale_factor"])
	}
	if result["autovacuum_enabled"] != "false" {
		t.Errorf("expected enabled=false, got %q", result["autovacuum_enabled"])
	}
	if _, ok := result["fillfactor"]; ok {
		t.Error("fillfactor should not be in autovacuum overrides")
	}
}

func TestParseReloptions_NoEquals(t *testing.T) {
	opts := []string{"autovacuum_something"}
	result := parseReloptions(opts)
	if len(result) != 0 {
		t.Errorf("expected empty map for malformed option, got %v", result)
	}
}
