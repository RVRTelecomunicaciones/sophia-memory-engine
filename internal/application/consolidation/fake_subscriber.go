package consolidation

import (
	"context"
	"sync"
)

// FakeSubscriber is a test fake for EventSubscriber. Subscribe records the
// handler; Emit synchronously drives a synthetic payload into the recorded
// handler. Used by handler_test.go and any future test that needs to exercise
// the consolidation pipeline without a real transport.
type FakeSubscriber struct {
	mu       sync.Mutex
	handlers map[string]EventHandler
}

// NewFakeSubscriber creates a new FakeSubscriber ready to use in tests.
func NewFakeSubscriber() *FakeSubscriber {
	return &FakeSubscriber{handlers: make(map[string]EventHandler)}
}

// Subscribe records the handler for eventType. A second Subscribe for the
// same eventType replaces the previous handler.
func (f *FakeSubscriber) Subscribe(_ context.Context, eventType string, handler EventHandler) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[eventType] = handler
	return nil
}

// Emit drives payload synchronously into the handler registered for eventType.
// Returns the handler's error verbatim, or nil if no handler was registered
// (subscribe-before-emit is the test's responsibility).
func (f *FakeSubscriber) Emit(ctx context.Context, eventType string, payload PhaseArchivedReceived) error {
	f.mu.Lock()
	h, ok := f.handlers[eventType]
	f.mu.Unlock()
	if !ok {
		return nil
	}
	return h(ctx, payload)
}
