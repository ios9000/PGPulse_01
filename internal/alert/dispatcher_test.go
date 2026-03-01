package alert

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type mockNotifier struct {
	name     string
	mu       sync.Mutex
	calls    []AlertEvent
	err      error
	sendFunc func(ctx context.Context, event AlertEvent) error
}

func (m *mockNotifier) Name() string { return m.name }

func (m *mockNotifier) Send(ctx context.Context, event AlertEvent) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, event)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, event)
	return m.err
}

func (m *mockNotifier) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}


func newTestDispatcher(registry *NotifierRegistry, defaultChannels []string, cooldownMinutes int) *Dispatcher {
	logger := slog.Default()
	d := NewDispatcher(registry, defaultChannels, cooldownMinutes, logger)
	d.retryDelays = []time.Duration{1 * time.Millisecond, 2 * time.Millisecond}
	return d
}

func TestDispatcher_RoutesToCorrectChannel(t *testing.T) {
	emailMock := &mockNotifier{name: "email"}
	slackMock := &mockNotifier{name: "slack"}

	reg := NewNotifierRegistry()
	reg.Register(emailMock)
	reg.Register(slackMock)

	d := newTestDispatcher(reg, nil, 0)
	d.Start()

	ev := newTestEvent()
	ev.Channels = []string{"email"}
	d.Dispatch(ev)
	d.Stop()

	if emailMock.callCount() != 1 {
		t.Errorf("email notifier calls = %d, want 1", emailMock.callCount())
	}
	if slackMock.callCount() != 0 {
		t.Errorf("slack notifier calls = %d, want 0", slackMock.callCount())
	}
}

func TestDispatcher_DefaultChannels(t *testing.T) {
	emailMock := &mockNotifier{name: "email"}

	reg := NewNotifierRegistry()
	reg.Register(emailMock)

	d := newTestDispatcher(reg, []string{"email"}, 0)
	d.Start()

	ev := newTestEvent()
	ev.Channels = nil // no event-level channels -> use default
	d.Dispatch(ev)
	d.Stop()

	if emailMock.callCount() != 1 {
		t.Errorf("email notifier calls = %d, want 1", emailMock.callCount())
	}
}

func TestDispatcher_CooldownSuppresses(t *testing.T) {
	emailMock := &mockNotifier{name: "email"}

	reg := NewNotifierRegistry()
	reg.Register(emailMock)

	// Cooldown of 60 minutes ensures second event is suppressed.
	d := newTestDispatcher(reg, []string{"email"}, 60)
	d.Start()

	ev := newTestEvent()
	ev.Channels = []string{"email"}
	d.Dispatch(ev)
	d.Dispatch(ev) // same rule+instance+severity -> should be suppressed
	d.Stop()

	if emailMock.callCount() != 1 {
		t.Errorf("email notifier calls = %d, want 1 (second should be suppressed)", emailMock.callCount())
	}
}

func TestDispatcher_CooldownAllowsResolution(t *testing.T) {
	emailMock := &mockNotifier{name: "email"}

	reg := NewNotifierRegistry()
	reg.Register(emailMock)

	d := newTestDispatcher(reg, []string{"email"}, 60)
	d.Start()

	// Fire event.
	ev := newTestEvent()
	ev.Channels = []string{"email"}
	d.Dispatch(ev)

	// Resolution event should bypass cooldown.
	resEv := ev
	resEv.IsResolution = true
	resolvedAt := ev.FiredAt.Add(5 * time.Minute)
	resEv.ResolvedAt = &resolvedAt
	d.Dispatch(resEv)
	d.Stop()

	if emailMock.callCount() != 2 {
		t.Errorf("email notifier calls = %d, want 2 (fire + resolution)", emailMock.callCount())
	}
}

func TestDispatcher_RetryOnFailure(t *testing.T) {
	var mu sync.Mutex
	callNum := 0

	emailMock := &mockNotifier{
		name: "email",
		sendFunc: func(_ context.Context, event AlertEvent) error {
			mu.Lock()
			defer mu.Unlock()
			callNum++
			if callNum == 1 {
				return errors.New("temporary failure")
			}
			return nil
		},
	}

	reg := NewNotifierRegistry()
	reg.Register(emailMock)

	d := newTestDispatcher(reg, nil, 0)
	d.Start()

	ev := newTestEvent()
	ev.Channels = []string{"email"}
	d.Dispatch(ev)
	d.Stop()

	mu.Lock()
	got := callNum
	mu.Unlock()
	if got != 2 {
		t.Errorf("send attempts = %d, want 2 (1 fail + 1 success)", got)
	}
}

func TestDispatcher_GracefulShutdown(t *testing.T) {
	emailMock := &mockNotifier{name: "email"}

	reg := NewNotifierRegistry()
	reg.Register(emailMock)

	d := newTestDispatcher(reg, []string{"email"}, 0)
	d.Start()

	n := 10
	for i := 0; i < n; i++ {
		ev := newTestEvent()
		ev.RuleID = "rule-" + string(rune('a'+i))
		d.Dispatch(ev)
	}

	d.Stop() // should wait for all events to drain

	if emailMock.callCount() != n {
		t.Errorf("email notifier calls = %d, want %d", emailMock.callCount(), n)
	}
}

func TestDispatcher_BufferFull(t *testing.T) {
	reg := NewNotifierRegistry()
	d := newTestDispatcher(reg, nil, 0)
	// Do NOT call Start() — the buffer will fill up.

	for i := 0; i < defaultBufferSize; i++ {
		ev := newTestEvent()
		if !d.Dispatch(ev) {
			t.Fatalf("Dispatch() returned false at event %d, buffer should not be full yet", i)
		}
	}

	// Buffer is now full; next dispatch should return false.
	ev := newTestEvent()
	if d.Dispatch(ev) {
		t.Error("Dispatch() returned true, want false (buffer full)")
	}
}

func TestDispatcher_UnregisteredChannel(t *testing.T) {
	emailMock := &mockNotifier{name: "email"}

	reg := NewNotifierRegistry()
	reg.Register(emailMock)

	d := newTestDispatcher(reg, nil, 0)
	d.Start()

	ev := newTestEvent()
	ev.Channels = []string{"nonexistent"}
	d.Dispatch(ev)
	d.Stop()

	// No panic, no email calls.
	if emailMock.callCount() != 0 {
		t.Errorf("email notifier calls = %d, want 0", emailMock.callCount())
	}
}
