package auth_test

import (
	"context"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/stretchr/testify/assert"
)

func TestContext_RoundTrip(t *testing.T) {
	ac := auth.AuthContext{
		TenantID:  "tenant-1",
		ProjectID: "project-1",
		KeyID:     "01JTEST00000000000000000AA",
	}

	ctx := auth.NewContext(context.Background(), ac)
	got, ok := auth.FromContext(ctx)

	assert.True(t, ok, "expected auth context to be present")
	assert.Equal(t, ac.TenantID, got.TenantID)
	assert.Equal(t, ac.ProjectID, got.ProjectID)
	assert.Equal(t, ac.KeyID, got.KeyID)
}

func TestContext_Missing_ReturnsFalse(t *testing.T) {
	_, ok := auth.FromContext(context.Background())
	assert.False(t, ok, "expected no auth context in a bare context")
}

func TestContext_NoPointerAliasing(t *testing.T) {
	// Ensure that modifying the returned value does not affect future lookups.
	original := auth.AuthContext{TenantID: "a", ProjectID: "b", KeyID: "c"}
	ctx := auth.NewContext(context.Background(), original)

	first, _ := auth.FromContext(ctx)
	first.TenantID = "mutated"

	second, _ := auth.FromContext(ctx)
	assert.Equal(t, "a", second.TenantID, "mutation of returned copy must not affect stored value")
}
