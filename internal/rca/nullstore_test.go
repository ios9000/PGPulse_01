package rca

import (
	"context"
	"testing"
	"time"
)

func TestNullIncidentStore_Create(t *testing.T) {
	t.Parallel()
	s := NewNullIncidentStore()
	id, err := s.Create(context.Background(), &Incident{InstanceID: "inst-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Errorf("expected id 0, got %d", id)
	}
}

func TestNullIncidentStore_Get(t *testing.T) {
	t.Parallel()
	s := NewNullIncidentStore()
	inc, err := s.Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc != nil {
		t.Error("expected nil incident")
	}
}

func TestNullIncidentStore_ListByInstance(t *testing.T) {
	t.Parallel()
	s := NewNullIncidentStore()
	incidents, total, err := s.ListByInstance(context.Background(), "inst-1", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(incidents) != 0 {
		t.Errorf("expected empty list, got %d", len(incidents))
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestNullIncidentStore_ListAll(t *testing.T) {
	t.Parallel()
	s := NewNullIncidentStore()
	incidents, total, err := s.ListAll(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(incidents) != 0 {
		t.Errorf("expected empty list, got %d", len(incidents))
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestNullIncidentStore_Cleanup(t *testing.T) {
	t.Parallel()
	s := NewNullIncidentStore()
	deleted, err := s.Cleanup(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}
