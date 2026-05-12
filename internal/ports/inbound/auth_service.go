package inbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
)

// AuthService is the inbound port for API-key authentication.
// Implementations live in the application layer (application/authsvc).
type AuthService interface {
	// Authenticate validates plaintextKey and, on success, returns the
	// AuthContext that should be injected into the request context.
	//
	// Errors:
	//   auth.ErrInvalidKey  — key is empty or shorter than 32 hex chars
	//   auth.ErrUnauthorized — key not found, revoked, or repo error
	Authenticate(ctx context.Context, plaintextKey string) (auth.AuthContext, error)
}
