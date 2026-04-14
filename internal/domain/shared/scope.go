package shared

// Scope defines the multi-dimensional isolation boundary for memory records.
type Scope struct {
	TenantID    *string
	ProjectID   string
	RepoID      *string
	AgentID     *string
	SessionID   *string
	Environment *string
}

// ScopeOption configures optional fields on a Scope.
type ScopeOption func(*Scope)

// WithTenantID sets the tenant identifier.
func WithTenantID(id string) ScopeOption {
	return func(s *Scope) { s.TenantID = &id }
}

// WithRepoID sets the repository identifier.
func WithRepoID(id string) ScopeOption {
	return func(s *Scope) { s.RepoID = &id }
}

// WithAgentID sets the agent identifier.
func WithAgentID(id string) ScopeOption {
	return func(s *Scope) { s.AgentID = &id }
}

// WithSessionID sets the session identifier.
func WithSessionID(id string) ScopeOption {
	return func(s *Scope) { s.SessionID = &id }
}

// WithEnvironment sets the environment label.
func WithEnvironment(env string) ScopeOption {
	return func(s *Scope) { s.Environment = &env }
}

// NewScope creates a Scope with a required projectID and optional fields.
func NewScope(projectID string, opts ...ScopeOption) (Scope, error) {
	if projectID == "" {
		return Scope{}, NewValidationError(FieldError{
			Field:   "project_id",
			Message: "required",
			Value:   projectID,
		})
	}

	s := Scope{ProjectID: projectID}
	for _, opt := range opts {
		opt(&s)
	}
	return s, nil
}

// Matches returns true if the receiver (used as a filter) matches the given record.
// Nil filter fields match everything; present fields require exact match.
func (filter Scope) Matches(record Scope) bool {
	if filter.ProjectID != record.ProjectID {
		return false
	}
	if filter.TenantID != nil && (record.TenantID == nil || *filter.TenantID != *record.TenantID) {
		return false
	}
	if filter.RepoID != nil && (record.RepoID == nil || *filter.RepoID != *record.RepoID) {
		return false
	}
	if filter.AgentID != nil && (record.AgentID == nil || *filter.AgentID != *record.AgentID) {
		return false
	}
	if filter.SessionID != nil && (record.SessionID == nil || *filter.SessionID != *record.SessionID) {
		return false
	}
	if filter.Environment != nil && (record.Environment == nil || *filter.Environment != *record.Environment) {
		return false
	}
	return true
}
