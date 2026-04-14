package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInProcessEventPublisher_PublishAndSubscribe(t *testing.T) {
	publisher := events.NewInProcessEventPublisher()

	var received events.DomainEvent
	publisher.Subscribe(shared.EventTypeMemoryIngested, func(ctx context.Context, event events.DomainEvent) error {
		received = event
		return nil
	})

	evt := events.DomainEvent{
		ID:            "test-event-001",
		Type:          shared.EventTypeMemoryIngested,
		AggregateID:   shared.NewRecordID(),
		AggregateType: "memory",
		Scope:         shared.Scope{ProjectID: "test-project"},
		Payload:       map[string]string{"key": "value"},
		OccurredAt:    time.Now(),
	}

	err := publisher.Publish(context.Background(), evt)
	require.NoError(t, err)

	assert.Equal(t, evt.ID, received.ID)
	assert.Equal(t, evt.Type, received.Type)
	assert.Equal(t, evt.AggregateID, received.AggregateID)
	assert.Equal(t, evt.AggregateType, received.AggregateType)
	assert.Equal(t, evt.Scope.ProjectID, received.Scope.ProjectID)
}

func TestInProcessEventPublisher_NoSubscribers(t *testing.T) {
	publisher := events.NewInProcessEventPublisher()

	evt := events.DomainEvent{
		ID:            "test-event-002",
		Type:          shared.EventTypeDecisionRecorded,
		AggregateID:   shared.NewRecordID(),
		AggregateType: "decision",
		Scope:         shared.Scope{ProjectID: "test-project"},
		Payload:       nil,
		OccurredAt:    time.Now(),
	}

	err := publisher.Publish(context.Background(), evt)
	require.NoError(t, err)
}

func TestInProcessEventPublisher_HandlerErrorDoesNotStopProcessing(t *testing.T) {
	publisher := events.NewInProcessEventPublisher()

	var callCount int

	// First handler: fails
	publisher.Subscribe(shared.EventTypeMemoryIngested, func(ctx context.Context, event events.DomainEvent) error {
		callCount++
		return errors.New("handler failed")
	})

	// Second handler: succeeds
	publisher.Subscribe(shared.EventTypeMemoryIngested, func(ctx context.Context, event events.DomainEvent) error {
		callCount++
		return nil
	})

	evt := events.DomainEvent{
		ID:            "test-event-003",
		Type:          shared.EventTypeMemoryIngested,
		AggregateID:   shared.NewRecordID(),
		AggregateType: "memory",
		Scope:         shared.Scope{ProjectID: "test-project"},
		OccurredAt:    time.Now(),
	}

	err := publisher.Publish(context.Background(), evt)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "both handlers should have been called despite first one failing")
}

func TestInProcessEventPublisher_MultipleEventTypes(t *testing.T) {
	publisher := events.NewInProcessEventPublisher()

	var memoryCount, decisionCount int

	publisher.Subscribe(shared.EventTypeMemoryIngested, func(ctx context.Context, event events.DomainEvent) error {
		memoryCount++
		return nil
	})

	publisher.Subscribe(shared.EventTypeDecisionRecorded, func(ctx context.Context, event events.DomainEvent) error {
		decisionCount++
		return nil
	})

	memEvt := events.DomainEvent{
		ID:         "mem-001",
		Type:       shared.EventTypeMemoryIngested,
		AggregateID: shared.NewRecordID(),
		Scope:      shared.Scope{ProjectID: "test"},
		OccurredAt: time.Now(),
	}
	decEvt := events.DomainEvent{
		ID:         "dec-001",
		Type:       shared.EventTypeDecisionRecorded,
		AggregateID: shared.NewRecordID(),
		Scope:      shared.Scope{ProjectID: "test"},
		OccurredAt: time.Now(),
	}

	_ = publisher.Publish(context.Background(), memEvt)
	_ = publisher.Publish(context.Background(), decEvt)

	assert.Equal(t, 1, memoryCount)
	assert.Equal(t, 1, decisionCount)
}
