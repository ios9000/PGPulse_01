package alert

import "context"

// Notifier delivers alert notifications via a specific channel.
type Notifier interface {
	Name() string
	Send(ctx context.Context, event AlertEvent) error
}

// NotifierRegistry maps channel names to notifier implementations.
type NotifierRegistry struct {
	notifiers map[string]Notifier
}

// NewNotifierRegistry creates an empty registry.
func NewNotifierRegistry() *NotifierRegistry {
	return &NotifierRegistry{notifiers: make(map[string]Notifier)}
}

// Register adds a notifier to the registry, keyed by its Name().
func (r *NotifierRegistry) Register(n Notifier) {
	r.notifiers[n.Name()] = n
}

// Get returns the notifier for the given channel name, or nil if not found.
func (r *NotifierRegistry) Get(name string) Notifier {
	return r.notifiers[name]
}

// Names returns all registered channel names.
func (r *NotifierRegistry) Names() []string {
	names := make([]string, 0, len(r.notifiers))
	for name := range r.notifiers {
		names = append(names, name)
	}
	return names
}
