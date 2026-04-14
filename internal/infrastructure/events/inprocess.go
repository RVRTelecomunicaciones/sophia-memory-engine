package events

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// DomainEvent represents a domain event published through the in-process bus.
// Defined locally to avoid import cycle with ports/outbound.
// Will be reconciled to use ports/outbound type when available.
type DomainEvent struct {
	ID            string
	Type          shared.EventType
	AggregateID   shared.RecordID
	AggregateType string
	Scope         shared.Scope
	Payload       any
	OccurredAt    time.Time
}

// EventHandler processes a domain event.
type EventHandler func(ctx context.Context, event DomainEvent) error

// InProcessEventPublisher is a synchronous in-process event bus.
// Handlers are invoked sequentially. On handler error, the error is logged
// and processing continues (at-most-once delivery).
type InProcessEventPublisher struct {
	mu       sync.RWMutex
	handlers map[shared.EventType][]EventHandler
}

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
func (p *InProcessEventPublisher) Publish(ctx context.Context, event DomainEvent) error {
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
