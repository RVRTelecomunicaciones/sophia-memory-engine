package shared_test

import (
	"errors"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvidenceRef_MemoryRef(t *testing.T) {
	id := shared.NewRecordID()
	ref, err := shared.NewEvidenceRef(shared.EvidenceTypeMemoryRef, &id, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, shared.EvidenceTypeMemoryRef, ref.Type)
	require.NotNil(t, ref.RecordID)
	assert.True(t, id.Equal(*ref.RecordID))
}

func TestNewEvidenceRef_MemoryRef_RequiresRecordID(t *testing.T) {
	_, err := shared.NewEvidenceRef(shared.EvidenceTypeMemoryRef, nil, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewEvidenceRef_ExternalLink_RequiresURI(t *testing.T) {
	t.Run("nil uri", func(t *testing.T) {
		_, err := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, nil, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("empty uri", func(t *testing.T) {
		empty := ""
		_, err := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, &empty, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})
}

func TestNewEvidenceRef_ExternalLink_Valid(t *testing.T) {
	uri := "https://example.com/doc"
	ref, err := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, &uri, nil)

	require.NoError(t, err)
	assert.Equal(t, shared.EvidenceTypeExternalLink, ref.Type)
	require.NotNil(t, ref.URI)
	assert.Equal(t, "https://example.com/doc", *ref.URI)
}

func TestNewEvidenceRef_InlineExcerpt_RequiresExcerpt(t *testing.T) {
	t.Run("nil excerpt", func(t *testing.T) {
		_, err := shared.NewEvidenceRef(shared.EvidenceTypeInlineExcerpt, nil, nil, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})

	t.Run("empty excerpt", func(t *testing.T) {
		empty := ""
		_, err := shared.NewEvidenceRef(shared.EvidenceTypeInlineExcerpt, nil, nil, &empty)
		require.Error(t, err)
		assert.True(t, errors.Is(err, shared.ErrValidation))
	})
}
