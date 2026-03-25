package playbook

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

// mockStore is a minimal mock for testing the resolver.
type mockStore struct {
	NullPlaybookStore
	bindings map[string]map[string]*Playbook // bindingType -> value -> playbook
}

func (m *mockStore) FindByTriggerBinding(_ context.Context, bindingType, bindingValue string) (*Playbook, error) {
	if byType, ok := m.bindings[bindingType]; ok {
		if pb, ok := byType[bindingValue]; ok {
			return pb, nil
		}
	}
	return nil, nil
}

func TestResolver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	hookPB := &Playbook{ID: 1, Name: "Hook Match"}
	rootPB := &Playbook{ID: 2, Name: "Root Cause Match"}
	metricPB := &Playbook{ID: 3, Name: "Metric Match"}
	adviserPB := &Playbook{ID: 4, Name: "Adviser Match"}

	store := &mockStore{
		bindings: map[string]map[string]*Playbook{
			"hooks":         {"remediation.checkpoint_completion_target": hookPB},
			"root_causes":   {"root_cause.checkpoint_storm": rootPB},
			"metrics":       {"pg.checkpoint.write_time_ms": metricPB},
			"adviser_rules": {"rem_checkpoint_warn": adviserPB},
		},
	}

	resolver := NewResolver(store, logger)
	ctx := context.Background()

	t.Run("hook has highest priority", func(t *testing.T) {
		pb, reason, err := resolver.Resolve(ctx, ResolverContext{
			HookID:       "remediation.checkpoint_completion_target",
			RootCauseKey: "root_cause.checkpoint_storm",
			MetricKey:    "pg.checkpoint.write_time_ms",
			AdviserRule:  "rem_checkpoint_warn",
		})
		if err != nil {
			t.Fatal(err)
		}
		if pb == nil || pb.ID != 1 {
			t.Fatalf("expected hook playbook (ID=1), got %v", pb)
		}
		if reason != "rca_hook" {
			t.Errorf("expected reason rca_hook, got %s", reason)
		}
	})

	t.Run("root cause when no hook", func(t *testing.T) {
		pb, reason, err := resolver.Resolve(ctx, ResolverContext{
			RootCauseKey: "root_cause.checkpoint_storm",
			MetricKey:    "pg.checkpoint.write_time_ms",
		})
		if err != nil {
			t.Fatal(err)
		}
		if pb == nil || pb.ID != 2 {
			t.Fatalf("expected root cause playbook (ID=2), got %v", pb)
		}
		if reason != "root_cause" {
			t.Errorf("expected reason root_cause, got %s", reason)
		}
	})

	t.Run("metric when no hook or root cause", func(t *testing.T) {
		pb, reason, err := resolver.Resolve(ctx, ResolverContext{
			MetricKey: "pg.checkpoint.write_time_ms",
		})
		if err != nil {
			t.Fatal(err)
		}
		if pb == nil || pb.ID != 3 {
			t.Fatalf("expected metric playbook (ID=3), got %v", pb)
		}
		if reason != "metric" {
			t.Errorf("expected reason metric, got %s", reason)
		}
	})

	t.Run("no match returns nil", func(t *testing.T) {
		pb, reason, err := resolver.Resolve(ctx, ResolverContext{
			HookID: "nonexistent.hook",
		})
		if err != nil {
			t.Fatal(err)
		}
		if pb != nil {
			t.Errorf("expected nil playbook, got %v", pb)
		}
		if reason != "" {
			t.Errorf("expected empty reason, got %s", reason)
		}
	})
}
