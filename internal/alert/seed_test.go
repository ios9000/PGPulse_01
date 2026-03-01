package alert

import (
	"context"
	"testing"
)

func TestSeedBuiltinRules(t *testing.T) {
	rs := &mockRuleStore{}
	err := SeedBuiltinRules(context.Background(), rs, discardLogger())
	if err != nil {
		t.Fatalf("SeedBuiltinRules: %v", err)
	}

	expected := len(BuiltinRules())
	if len(rs.rules) != expected {
		t.Errorf("seeded %d rules, want %d", len(rs.rules), expected)
	}
}

func TestSeedBuiltinRules_Idempotent(t *testing.T) {
	rs := &mockRuleStore{}
	ctx := context.Background()
	logger := discardLogger()

	// Seed twice
	if err := SeedBuiltinRules(ctx, rs, logger); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := SeedBuiltinRules(ctx, rs, logger); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	// Should still have same count (UpsertBuiltin updates existing, doesn't duplicate)
	expected := len(BuiltinRules())
	if len(rs.rules) != expected {
		t.Errorf("after double seed: %d rules, want %d", len(rs.rules), expected)
	}
}
