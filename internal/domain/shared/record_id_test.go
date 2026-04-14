package shared_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRecordID_GeneratesValidULID(t *testing.T) {
	id := shared.NewRecordID()

	assert.Len(t, id.String(), 26, "ULID string representation must be 26 characters")
	assert.True(t, id.IsValid(), "newly generated RecordID must be valid")
}

func TestNewRecordID_IsUnique(t *testing.T) {
	id1 := shared.NewRecordID()
	id2 := shared.NewRecordID()

	assert.False(t, id1.Equal(id2), "two generated RecordIDs must be different")
}

func TestRecordIDFromString_Valid(t *testing.T) {
	original := shared.NewRecordID()
	parsed, err := shared.RecordIDFromString(original.String())

	require.NoError(t, err)
	assert.True(t, original.Equal(parsed), "roundtrip parse must produce equal RecordID")
}

func TestRecordIDFromString_Invalid(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		_, err := shared.RecordIDFromString("")
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("garbage input", func(t *testing.T) {
		_, err := shared.RecordIDFromString("not-a-ulid-at-all")
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})
}

func TestRecordID_IsTimeSortable(t *testing.T) {
	id1 := shared.NewRecordID()
	time.Sleep(2 * time.Millisecond)
	id2 := shared.NewRecordID()

	assert.True(t, id1.String() < id2.String(), "sequential RecordIDs must sort lexicographically by time")
}

func TestRecordID_ZeroValue_IsInvalid(t *testing.T) {
	var id shared.RecordID

	assert.False(t, id.IsValid(), "zero-value RecordID must not be valid")
	assert.True(t, id.IsZero(), "zero-value RecordID must report IsZero")
}
