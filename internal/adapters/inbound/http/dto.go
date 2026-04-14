package http

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type scopeDTO struct {
	TenantID    *string `json:"tenant_id,omitempty"`
	ProjectID   string  `json:"project_id"`
	RepoID      *string `json:"repo_id,omitempty"`
	AgentID     *string `json:"agent_id,omitempty"`
	SessionID   *string `json:"session_id,omitempty"`
	Environment *string `json:"environment,omitempty"`
}

type provenanceDTO struct {
	Source    string  `json:"source"`
	SourceURI *string `json:"source_uri,omitempty"`
	Method   string  `json:"method"`
	ParentID *string `json:"parent_id,omitempty"`
}

func parseScopeDTO(dto scopeDTO) (shared.Scope, error) {
	var opts []shared.ScopeOption
	if dto.TenantID != nil {
		opts = append(opts, shared.WithTenantID(*dto.TenantID))
	}
	if dto.RepoID != nil {
		opts = append(opts, shared.WithRepoID(*dto.RepoID))
	}
	if dto.AgentID != nil {
		opts = append(opts, shared.WithAgentID(*dto.AgentID))
	}
	if dto.SessionID != nil {
		opts = append(opts, shared.WithSessionID(*dto.SessionID))
	}
	if dto.Environment != nil {
		opts = append(opts, shared.WithEnvironment(*dto.Environment))
	}
	return shared.NewScope(dto.ProjectID, opts...)
}

func parseProvenanceDTO(dto provenanceDTO) (shared.Provenance, error) {
	var parentID *shared.RecordID
	if dto.ParentID != nil {
		id, err := shared.RecordIDFromString(*dto.ParentID)
		if err != nil {
			return shared.Provenance{}, err
		}
		parentID = &id
	}

	prov, err := shared.NewProvenance(dto.Source, shared.IngestMethod(dto.Method), parentID)
	if err != nil {
		return shared.Provenance{}, err
	}

	if dto.SourceURI != nil {
		prov = prov.WithSourceURI(*dto.SourceURI)
	}

	return prov, nil
}

func parseTime(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
