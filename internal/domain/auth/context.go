package auth

import "context"

// ctxKey is an unexported type for context keys in this package.
// Using a private struct prevents collision with any other context keys.
type ctxKey struct{}

// AuthContext carries the authentication payload injected by the API-key middleware.
// All fields are plain string copies — no pointer aliasing.
type AuthContext struct {
	// TenantID is the tenant that owns this key (may be empty).
	TenantID string
	// ProjectID is the project the key is scoped to.
	ProjectID string
	// KeyID is the ULID identifier of the API key (from api_keys.id).
	KeyID string
}

// NewContext returns a new context derived from ctx with ac stored under the
// package-private key.
func NewContext(ctx context.Context, ac AuthContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, ac)
}

// FromContext extracts the AuthContext from ctx.
// The second return value is false if ctx carries no auth payload.
func FromContext(ctx context.Context) (AuthContext, bool) {
	ac, ok := ctx.Value(ctxKey{}).(AuthContext)
	return ac, ok
}
