package auth

import (
	"errors"
	"time"
)

// Sentinel errors for the auth domain.
var (
	// ErrNotFound is returned when no API key matches the provided hash.
	ErrNotFound = errors.New("api key not found")
	// ErrRevoked is returned when the API key exists but has been revoked.
	ErrRevoked = errors.New("api key revoked")
	// ErrInvalidKey is returned when the plaintext key fails format validation
	// (e.g. too short) before any DB lookup is attempted.
	ErrInvalidKey = errors.New("invalid api key format")
	// ErrUnauthorized is the opaque error surfaced to callers when authentication
	// fails for any reason other than format validation.
	ErrUnauthorized = errors.New("unauthorized")
)

// StatusActive is the only status that allows authentication.
const StatusActive = "active"

// StatusRevoked means the key has been disabled and must not be used.
const StatusRevoked = "revoked"

// APIKey is the auth aggregate. It holds the hashed secret and its metadata.
// The plaintext secret is NEVER stored; only the SHA-256 hex digest.
type APIKey struct {
	// ID is the ULID key identifier (key_id injected into the request context).
	ID string
	// KeyHash is the lowercase hex SHA-256 digest of the plaintext secret.
	KeyHash string
	// TenantID groups keys that belong to the same tenant (optional).
	TenantID string
	// ProjectID scopes the key to a single project (required).
	ProjectID string
	// Description is a human-readable label for the key.
	Description string
	// Status is either "active" or "revoked".
	Status string
	// LastUsedAt records the last successful authentication (best-effort).
	LastUsedAt *time.Time
	// CreatedAt is the key creation timestamp.
	CreatedAt time.Time
}

// NewAPIKey constructs and validates an APIKey aggregate.
// key_hash and project_id must be non-empty.
func NewAPIKey(id, keyHash, tenantID, projectID, description, status string, lastUsedAt *time.Time, createdAt time.Time) (*APIKey, error) {
	if keyHash == "" {
		return nil, errors.New("auth: key_hash must not be empty")
	}
	if projectID == "" {
		return nil, errors.New("auth: project_id must not be empty")
	}
	if id == "" {
		return nil, errors.New("auth: id must not be empty")
	}
	return &APIKey{
		ID:          id,
		KeyHash:     keyHash,
		TenantID:    tenantID,
		ProjectID:   projectID,
		Description: description,
		Status:      status,
		LastUsedAt:  lastUsedAt,
		CreatedAt:   createdAt,
	}, nil
}

// IsActive reports whether the key is in the active status.
func (k *APIKey) IsActive() bool {
	return k.Status == StatusActive
}
