package etcd

import (
	"context"
	"errors"
	"testing"
)

type mockETCDProvider struct {
	members   []ETCDMember
	memberErr error
	health    map[string]bool
	healthErr error
	called    bool
}

func (m *mockETCDProvider) GetMembers(_ context.Context) ([]ETCDMember, error) {
	m.called = true
	return m.members, m.memberErr
}

func (m *mockETCDProvider) GetEndpointHealth(_ context.Context) (map[string]bool, error) {
	m.called = true
	return m.health, m.healthErr
}

func TestFallbackProvider_PrimarySucceeds(t *testing.T) {
	primary := &mockETCDProvider{
		members: []ETCDMember{{Name: "etcd1", ID: "abc"}},
		health:  map[string]bool{"http://10.0.0.1:2379": true},
	}
	secondary := &mockETCDProvider{}

	fb := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	members, err := fb.GetMembers(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 1 || members[0].Name != "etcd1" {
		t.Errorf("unexpected members: %+v", members)
	}
	if secondary.called {
		t.Error("secondary should not have been called")
	}

	secondary.called = false
	health, err := fb.GetEndpointHealth(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !health["http://10.0.0.1:2379"] {
		t.Errorf("expected endpoint healthy, got %v", health)
	}
	if secondary.called {
		t.Error("secondary should not have been called")
	}
}

func TestFallbackProvider_PrimaryFails_SecondarySucceeds(t *testing.T) {
	primaryErr := errors.New("primary down")
	primary := &mockETCDProvider{
		memberErr: primaryErr,
		healthErr: primaryErr,
	}
	secondary := &mockETCDProvider{
		members: []ETCDMember{{Name: "etcd2", ID: "def"}},
		health:  map[string]bool{"http://10.0.0.2:2379": true},
	}

	fb := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	members, err := fb.GetMembers(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 1 || members[0].Name != "etcd2" {
		t.Errorf("unexpected members: %+v", members)
	}
	if !secondary.called {
		t.Error("secondary should have been called")
	}

	secondary.called = false
	health, err := fb.GetEndpointHealth(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !health["http://10.0.0.2:2379"] {
		t.Errorf("expected endpoint healthy, got %v", health)
	}
}

func TestFallbackProvider_BothFail(t *testing.T) {
	primaryErr := errors.New("primary down")
	secondaryErr := errors.New("secondary down")
	primary := &mockETCDProvider{
		memberErr: primaryErr,
		healthErr: primaryErr,
	}
	secondary := &mockETCDProvider{
		memberErr: secondaryErr,
		healthErr: secondaryErr,
	}

	fb := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	_, err := fb.GetMembers(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, secondaryErr) {
		t.Errorf("expected secondary error, got %v", err)
	}

	_, err = fb.GetEndpointHealth(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, secondaryErr) {
		t.Errorf("expected secondary error, got %v", err)
	}
}
