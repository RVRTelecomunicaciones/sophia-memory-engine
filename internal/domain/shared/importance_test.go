package shared_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewImportanceScore_Valid(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	factors := []shared.ImportanceFactor{
		{Name: "recency", Weight: 0.4, Value: 0.9},
		{Name: "access_count", Weight: 0.6, Value: 0.3},
	}

	is, err := shared.NewImportanceScore(0.54, now, factors)

	require.NoError(t, err)
	assert.Equal(t, 0.54, is.Score)
	assert.Equal(t, now, is.ComputedAt)
	assert.Len(t, is.Factors, 2)
}

func TestNewImportanceScore_OutOfRange(t *testing.T) {
	now := time.Now()

	t.Run("above 1.0", func(t *testing.T) {
		_, err := shared.NewImportanceScore(1.1, now, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("below 0.0", func(t *testing.T) {
		_, err := shared.NewImportanceScore(-0.01, now, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})
}

func TestDefaultImportanceScore(t *testing.T) {
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	is := shared.DefaultImportanceScore(now)

	assert.Equal(t, 0.5, is.Score)
	assert.Equal(t, now, is.ComputedAt)
	require.Len(t, is.Factors, 1)
	assert.Equal(t, "default", is.Factors[0].Name)
	assert.Equal(t, 1.0, is.Factors[0].Weight)
	assert.Equal(t, 0.5, is.Factors[0].Value)
}
