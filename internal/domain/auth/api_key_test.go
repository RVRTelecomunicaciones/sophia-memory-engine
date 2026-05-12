package auth_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPIKey_ValidInputs(t *testing.T) {
	createdAt := time.Now()
	key, err := auth.NewAPIKey(
		"01JTEST00000000000000000AA",
		"a91fcf692498eb1732105be597142d4653a19821b1fde29e3cbf306b6edc7ad5",
		"sophia-dev",
		"sophia-dev",
		"test key",
		auth.StatusActive,
		nil,
		createdAt,
	)
	require.NoError(t, err)
	assert.Equal(t, "01JTEST00000000000000000AA", key.ID)
	assert.Equal(t, "sophia-dev", key.ProjectID)
	assert.Equal(t, auth.StatusActive, key.Status)
	assert.True(t, key.IsActive())
}

func TestNewAPIKey_EmptyKeyHash_ReturnsError(t *testing.T) {
	_, err := auth.NewAPIKey("01JTEST", "", "tenant", "project", "", auth.StatusActive, nil, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key_hash")
}

func TestNewAPIKey_EmptyProjectID_ReturnsError(t *testing.T) {
	_, err := auth.NewAPIKey("01JTEST", "abc123", "tenant", "", "", auth.StatusActive, nil, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project_id")
}

func TestNewAPIKey_EmptyID_ReturnsError(t *testing.T) {
	_, err := auth.NewAPIKey("", "abc123", "tenant", "project", "", auth.StatusActive, nil, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
}

func TestAPIKey_IsActive_Revoked(t *testing.T) {
	key, err := auth.NewAPIKey("01JTEST", "abc123", "", "project", "", auth.StatusRevoked, nil, time.Now())
	require.NoError(t, err)
	assert.False(t, key.IsActive())
}

func TestAPIKey_IsActive_Active(t *testing.T) {
	key, err := auth.NewAPIKey("01JTEST", "abc123", "", "project", "", auth.StatusActive, nil, time.Now())
	require.NoError(t, err)
	assert.True(t, key.IsActive())
}
