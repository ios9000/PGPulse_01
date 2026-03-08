package patroni

import (
	"context"
	"errors"
	"testing"
)

type mockPatroniProvider struct {
	clusterState *ClusterState
	clusterErr   error
	history      []SwitchoverEvent
	historyErr   error
	version      string
	versionErr   error
	called       bool
}

func (m *mockPatroniProvider) GetClusterState(_ context.Context) (*ClusterState, error) {
	m.called = true
	return m.clusterState, m.clusterErr
}

func (m *mockPatroniProvider) GetHistory(_ context.Context) ([]SwitchoverEvent, error) {
	m.called = true
	return m.history, m.historyErr
}

func (m *mockPatroniProvider) GetVersion(_ context.Context) (string, error) {
	m.called = true
	return m.version, m.versionErr
}

func TestFallbackProvider_PrimarySucceeds(t *testing.T) {
	primary := &mockPatroniProvider{
		clusterState: &ClusterState{ClusterName: "main", Members: []ClusterMember{{Name: "pg1"}}},
		history:      []SwitchoverEvent{{Reason: "planned"}},
		version:      "3.0.4",
	}
	secondary := &mockPatroniProvider{}

	fb := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	state, err := fb.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.ClusterName != "main" {
		t.Errorf("expected cluster name 'main', got %q", state.ClusterName)
	}
	if secondary.called {
		t.Error("secondary should not have been called")
	}

	secondary.called = false
	history, err := fb.GetHistory(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 event, got %d", len(history))
	}
	if secondary.called {
		t.Error("secondary should not have been called")
	}

	secondary.called = false
	version, err := fb.GetVersion(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if version != "3.0.4" {
		t.Errorf("expected version '3.0.4', got %q", version)
	}
	if secondary.called {
		t.Error("secondary should not have been called")
	}
}

func TestFallbackProvider_PrimaryFails_SecondarySucceeds(t *testing.T) {
	primaryErr := errors.New("primary down")
	primary := &mockPatroniProvider{
		clusterErr: primaryErr,
		historyErr: primaryErr,
		versionErr: primaryErr,
	}
	secondary := &mockPatroniProvider{
		clusterState: &ClusterState{ClusterName: "fallback", Members: []ClusterMember{{Name: "pg2"}}},
		history:      []SwitchoverEvent{{Reason: "failover"}},
		version:      "3.0.3",
	}

	fb := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	state, err := fb.GetClusterState(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.ClusterName != "fallback" {
		t.Errorf("expected cluster name 'fallback', got %q", state.ClusterName)
	}
	if !secondary.called {
		t.Error("secondary should have been called")
	}

	secondary.called = false
	history, err := fb.GetHistory(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(history) != 1 || history[0].Reason != "failover" {
		t.Errorf("unexpected history: %+v", history)
	}

	secondary.called = false
	version, err := fb.GetVersion(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if version != "3.0.3" {
		t.Errorf("expected version '3.0.3', got %q", version)
	}
}

func TestFallbackProvider_BothFail(t *testing.T) {
	primaryErr := errors.New("primary down")
	secondaryErr := errors.New("secondary down")
	primary := &mockPatroniProvider{
		clusterErr: primaryErr,
		historyErr: primaryErr,
		versionErr: primaryErr,
	}
	secondary := &mockPatroniProvider{
		clusterErr: secondaryErr,
		historyErr: secondaryErr,
		versionErr: secondaryErr,
	}

	fb := NewFallbackProvider(primary, secondary)
	ctx := context.Background()

	_, err := fb.GetClusterState(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, secondaryErr) {
		t.Errorf("expected secondary error, got %v", err)
	}

	_, err = fb.GetHistory(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, secondaryErr) {
		t.Errorf("expected secondary error, got %v", err)
	}

	_, err = fb.GetVersion(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, secondaryErr) {
		t.Errorf("expected secondary error, got %v", err)
	}
}
