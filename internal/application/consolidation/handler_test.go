package consolidation_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Handle_LogsReceiptAndReturnsNil(t *testing.T) {
	// Setup: FakeSubscriber + Handler with a captured slog.Logger and FakeClock.
	fixedNow := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	clock := shared.NewFixedClock(fixedNow)

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	sub := consolidation.NewFakeSubscriber()
	handler := consolidation.NewHandler(log, clock)

	ctx := context.Background()

	// Subscribe the handler.
	err := sub.Subscribe(ctx, consolidation.PhaseArchivedEventType, handler.Handle)
	require.NoError(t, err)

	// Emit a synthetic event.
	archivedAt := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "change-001",
		ChangeName: "my-feature",
		PhaseType:  "archive",
		ArchivedAt: archivedAt,
	}
	err = sub.Emit(ctx, consolidation.PhaseArchivedEventType, payload)
	require.NoError(t, err, "handler must return nil")

	// Assert log output contains expected fields.
	logOutput := buf.String()
	assert.Contains(t, logOutput, "phase.archived received", "log must contain the event message")
	assert.Contains(t, logOutput, "change-001", "log must contain change_id")
	assert.Contains(t, logOutput, "my-feature", "log must contain change_name")
	// The archived_at and received_at appear as time strings in JSON output.
	assert.Contains(t, logOutput, "archived_at", "log must contain archived_at key")
	assert.Contains(t, logOutput, "received_at", "log must contain received_at key")
}

func TestHandler_Negative_EmitWithNoHandlerSubscribed(t *testing.T) {
	// Negative case: Emit with no handler subscribed -> FakeSubscriber returns nil.
	sub := consolidation.NewFakeSubscriber()
	ctx := context.Background()

	payload := consolidation.PhaseArchivedReceived{
		ChangeID:   "ghost-change",
		ChangeName: "ghost",
		PhaseType:  "archive",
		ArchivedAt: time.Now(),
	}

	// No Subscribe call — emit should return nil gracefully.
	err := sub.Emit(ctx, consolidation.PhaseArchivedEventType, payload)
	require.NoError(t, err, "Emit with no handler must return nil")
}
