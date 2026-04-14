package outbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// DomainEvent represents a domain event emitted by the memory engine.
type DomainEvent struct {
	ID            string
	Type          shared.EventType
	AggregateID   shared.RecordID
	AggregateType string
	Scope         shared.Scope
	Payload       any
	OccurredAt    time.Time
}

// EventHandler is a function that processes a domain event.
type EventHandler func(ctx context.Context, event DomainEvent) error

// EventPublisher dispatches domain events to registered handlers or external systems.
type EventPublisher interface {
	Publish(ctx context.Context, event DomainEvent) error
}
