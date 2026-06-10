package consolidation_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

func testLogger(_ *testing.T) *slog.Logger {
	return slog.Default()
}

func testClock(_ *testing.T) shared.Clock {
	return shared.NewFixedClock(time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC))
}
