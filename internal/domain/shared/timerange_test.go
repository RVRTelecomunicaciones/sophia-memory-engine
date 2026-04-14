package shared_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTimeRange_Valid(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	tr, err := shared.NewTimeRange(from, to)

	require.NoError(t, err)
	assert.Equal(t, from, tr.From)
	assert.Equal(t, to, tr.To)

	mid := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	assert.True(t, tr.Contains(mid), "mid-point must be contained")
	assert.True(t, tr.Contains(from), "from boundary must be contained")
	assert.True(t, tr.Contains(to), "to boundary must be contained")
	assert.False(t, tr.Contains(from.Add(-time.Second)), "before range must not be contained")
}

func TestNewTimeRange_FromAfterTo(t *testing.T) {
	from := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := shared.NewTimeRange(from, to)

	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewTimeRange_Equal(t *testing.T) {
	point := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	tr, err := shared.NewTimeRange(point, point)

	require.NoError(t, err, "from == to must be valid")
	assert.True(t, tr.Contains(point), "the single point must be contained")
}
