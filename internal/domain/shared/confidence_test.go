package shared_test

import (
	"errors"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfidence_Valid(t *testing.T) {
	c, err := shared.NewConfidence(0.85, shared.ConfidenceSourceHuman)

	require.NoError(t, err)
	assert.Equal(t, 0.85, c.Score)
	assert.Equal(t, shared.ConfidenceSourceHuman, c.Source)
}

func TestNewConfidence_ClampsAboveOne(t *testing.T) {
	_, err := shared.NewConfidence(1.5, shared.ConfidenceSourceAgent)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewConfidence_ClampsBelowZero(t *testing.T) {
	_, err := shared.NewConfidence(-0.1, shared.ConfidenceSourceAgent)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewConfidence_InvalidSource(t *testing.T) {
	_, err := shared.NewConfidence(0.5, shared.ConfidenceSource("invalid"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewConfidence_BoundaryValues(t *testing.T) {
	t.Run("zero is valid", func(t *testing.T) {
		c, err := shared.NewConfidence(0.0, shared.ConfidenceSourceComputed)
		require.NoError(t, err)
		assert.Equal(t, 0.0, c.Score)
	})

	t.Run("one is valid", func(t *testing.T) {
		c, err := shared.NewConfidence(1.0, shared.ConfidenceSourceDerived)
		require.NoError(t, err)
		assert.Equal(t, 1.0, c.Score)
	})
}
