package outbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
)

// APIKeyRepository provides read access to the api_keys table.
//
// FindByHash performs a lookup against the sha256 hex column; it never
// receives the plaintext secret. Status filtering (active / revoked) is
// intentionally left to the application layer so the repo stays thin and
// the business rule lives in one place.
//
// TouchLastUsed is best-effort: callers MUST NOT fail the authentication
// request if this call errors. Log and move on.
type APIKeyRepository interface {
	// FindByHash returns the APIKey whose key_hash equals sha256Hex.
	// Returns auth.ErrNotFound if no row matches.
	FindByHash(ctx context.Context, sha256Hex string) (*auth.APIKey, error)

	// TouchLastUsed updates last_used_at for the given keyID to at.
	// Fire-and-forget: the caller may ignore the returned error.
	TouchLastUsed(ctx context.Context, keyID string, at time.Time) error
}
