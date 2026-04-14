package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/projectprofile"
)

// ProjectProfileRepository persists and retrieves generated project profiles.
type ProjectProfileRepository interface {
	Save(ctx context.Context, profile *projectprofile.ProjectProfile) error
	FindLatest(ctx context.Context, projectID string) (*projectprofile.ProjectProfile, error)
}
