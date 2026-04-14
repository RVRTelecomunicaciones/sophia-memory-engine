package shared_test

import (
	"errors"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvenance_SourceRequired(t *testing.T) {
	_, err := shared.NewProvenance("", shared.IngestMethodDirect, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewProvenance_DerivedRequiresParent(t *testing.T) {
	_, err := shared.NewProvenance("agent-x", shared.IngestMethodDerived, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewProvenance_DerivedWithParent(t *testing.T) {
	parentID := shared.NewRecordID()
	p, err := shared.NewProvenance("agent-x", shared.IngestMethodDerived, &parentID)

	require.NoError(t, err)
	assert.Equal(t, "agent-x", p.Source)
	assert.Equal(t, shared.IngestMethodDerived, p.Method)
	require.NotNil(t, p.ParentID)
	assert.True(t, parentID.Equal(*p.ParentID))
}

func TestNewProvenance_DirectNoParent(t *testing.T) {
	p, err := shared.NewProvenance("user-input", shared.IngestMethodDirect, nil)

	require.NoError(t, err)
	assert.Equal(t, "user-input", p.Source)
	assert.Equal(t, shared.IngestMethodDirect, p.Method)
	assert.Nil(t, p.ParentID)
	assert.Nil(t, p.SourceURI)
}

func TestNewProvenance_InvalidMethod(t *testing.T) {
	_, err := shared.NewProvenance("source", shared.IngestMethod("invalid"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}
