package events

import (
	"context"
	"log/slog"
	"sync"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// EventHandler processes a domain event.
type EventHandler func(ctx context.Context, event outbound.DomainEvent) error

// InProcessEventPublisher is a synchronous in-process event bus.
// Handlers are invoked sequentially. On handler error, the error is logged
// and processing continues (at-most-once delivery).
type InProcessEventPublisher struct {
	mu       sync.RWMutex
	handlers map[shared.EventType][]EventHandler
}

// Compile-time check: InProcessEventPublisher implements outbound.EventPublisher.
var _ outbound.EventPublisher = (*InProcessEventPublisher)(nil)

// NewInProcessEventPublisher creates a new in-process event publisher.
func NewInProcessEventPublisher() *InProcessEventPublisher {
	return &InProcessEventPublisher{
		handlers: make(map[shared.EventType][]EventHandler),
	}
}

// Subscribe registers a handler for the given event type.
func (p *InProcessEventPublisher) Subscribe(eventType shared.EventType, handler EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

// Publish dispatches the event to all registered handlers for its type.
// On handler error, the error is logged and processing continues.
// Returns nil always (at-most-once semantics).
func (p *InProcessEventPublisher) Publish(ctx context.Context, event outbound.DomainEvent) error {
	p.mu.RLock()
	handlers := p.handlers[event.Type]
	p.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, event); err != nil {
			slog.ErrorContext(ctx, "event handler failed",
				"event_type", string(event.Type),
				"event_id", event.ID,
				"aggregate_id", event.AggregateID.String(),
				"error", err,
			)
		}
	}

	return nil
}
