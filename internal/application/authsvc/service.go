package authsvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// minPlaintextLen is the minimum acceptable length for a plaintext API key.
// 32 hex characters = 128-bit minimum, but we expect 64-char (256-bit) secrets.
const minPlaintextLen = 32

// touchTimeout is the maximum time allowed for the fire-and-forget
// TouchLastUsed call. It uses a detached context so it does not inherit
// cancellation from the request context.
const touchTimeout = 5 * time.Second

// Compile-time check that Service implements inbound.AuthService.
var _ inbound.AuthService = (*Service)(nil)

// Service implements API-key authentication.
type Service struct {
	repo  outbound.APIKeyRepository
	clock outbound.Clock
}

// NewService creates a new authentication Service.
func NewService(repo outbound.APIKeyRepository, clock outbound.Clock) *Service {
	return &Service{repo: repo, clock: clock}
}

// Authenticate validates a plaintext API key and returns the AuthContext on success.
//
// Algorithm:
//  1. Reject keys shorter than minPlaintextLen characters (early exit, no DB hit).
//  2. Hash the key with SHA-256 and hex-encode it (lowercase).
//  3. Look up the hash in the repository.
//  4. Reject revoked keys.
//  5. Fire-and-forget: update last_used_at in a detached goroutine.
//  6. Return the AuthContext.
//
// All failure modes (not found, revoked, repo error) surface as auth.ErrUnauthorized
// so callers cannot distinguish between them — this is intentional.
func (s *Service) Authenticate(ctx context.Context, plaintextKey string) (auth.AuthContext, error) {
	// Step 1: length / format guard.
	if len(plaintextKey) < minPlaintextLen {
		return auth.AuthContext{}, fmt.Errorf("%w: key too short (min %d chars)", auth.ErrInvalidKey, minPlaintextLen)
	}

	// Step 2: hash.
	sum := sha256.Sum256([]byte(plaintextKey))
	hashHex := hex.EncodeToString(sum[:])

	// Step 3: repository lookup.
	key, err := s.repo.FindByHash(ctx, hashHex)
	if err != nil {
		// Log key_id prefix only — never the plaintext or full hash.
		slog.Warn("auth: key lookup failed", "hash_prefix", hashHex[:8], "error", err)
		return auth.AuthContext{}, auth.ErrUnauthorized
	}

	// Step 4: status check.
	if !key.IsActive() {
		slog.Warn("auth: revoked key used", "key_id", key.ID)
		return auth.AuthContext{}, auth.ErrUnauthorized
	}

	// Step 5: fire-and-forget last_used_at update.
	keyID := key.ID
	now := s.clock.Now()
	repo := s.repo
	go func() {
		touchCtx, cancel := context.WithTimeout(context.Background(), touchTimeout)
		defer cancel()
		if err := repo.TouchLastUsed(touchCtx, keyID, now); err != nil {
			// Already logged inside TouchLastUsed; do not 401 on a stats write failure.
		}
	}()

	// Step 6: return context payload.
	return auth.AuthContext{
		TenantID:  key.TenantID,
		ProjectID: key.ProjectID,
		KeyID:     key.ID,
	}, nil
}
