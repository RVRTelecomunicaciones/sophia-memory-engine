package inbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/projectprofile"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// GenerateProfileCmd carries the data needed to generate a project profile.
type GenerateProfileCmd struct {
	ProjectID string
	Scope     shared.Scope
}

// GetProfileQuery defines the filters for retrieving an existing project profile.
type GetProfileQuery struct {
	ProjectID string
	Scope     shared.Scope
}

// ProjectProfileService defines the inbound port for project profile operations.
type ProjectProfileService interface {
	Generate(ctx context.Context, cmd GenerateProfileCmd) (*projectprofile.ProjectProfile, error)
	Get(ctx context.Context, query GetProfileQuery) (*projectprofile.ProjectProfile, error)
}
