package shared_test

import (
	"errors"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScope_ProjectIDRequired(t *testing.T) {
	_, err := shared.NewScope("")
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))
}

func TestNewScope_MinimalValid(t *testing.T) {
	s, err := shared.NewScope("proj-1")

	require.NoError(t, err)
	assert.Equal(t, "proj-1", s.ProjectID)
	assert.Nil(t, s.TenantID)
	assert.Nil(t, s.RepoID)
	assert.Nil(t, s.AgentID)
	assert.Nil(t, s.SessionID)
	assert.Nil(t, s.Environment)
}

func TestScope_WithOptionalFields(t *testing.T) {
	s, err := shared.NewScope("proj-1",
		shared.WithTenantID("tenant-1"),
		shared.WithRepoID("repo-1"),
		shared.WithAgentID("agent-1"),
		shared.WithSessionID("session-1"),
		shared.WithEnvironment("production"),
	)

	require.NoError(t, err)
	assert.Equal(t, "proj-1", s.ProjectID)
	require.NotNil(t, s.TenantID)
	assert.Equal(t, "tenant-1", *s.TenantID)
	require.NotNil(t, s.RepoID)
	assert.Equal(t, "repo-1", *s.RepoID)
	require.NotNil(t, s.AgentID)
	assert.Equal(t, "agent-1", *s.AgentID)
	require.NotNil(t, s.SessionID)
	assert.Equal(t, "session-1", *s.SessionID)
	require.NotNil(t, s.Environment)
	assert.Equal(t, "production", *s.Environment)
}

func TestScope_Matches_NilFieldMatchesAll(t *testing.T) {
	filter, _ := shared.NewScope("proj-1")
	record, _ := shared.NewScope("proj-1",
		shared.WithTenantID("tenant-1"),
		shared.WithAgentID("agent-1"),
	)

	assert.True(t, filter.Matches(record), "nil filter fields should match any record value")
}

func TestScope_Matches_ExactFieldRequired(t *testing.T) {
	filter, _ := shared.NewScope("proj-1", shared.WithAgentID("agent-1"))

	t.Run("matching agent", func(t *testing.T) {
		record, _ := shared.NewScope("proj-1", shared.WithAgentID("agent-1"))
		assert.True(t, filter.Matches(record))
	})

	t.Run("different agent", func(t *testing.T) {
		record, _ := shared.NewScope("proj-1", shared.WithAgentID("agent-2"))
		assert.False(t, filter.Matches(record))
	})

	t.Run("missing agent on record", func(t *testing.T) {
		record, _ := shared.NewScope("proj-1")
		assert.False(t, filter.Matches(record))
	})

	t.Run("different project", func(t *testing.T) {
		record, _ := shared.NewScope("proj-2", shared.WithAgentID("agent-1"))
		assert.False(t, filter.Matches(record))
	})
}
