package authsvc_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/authsvc"
	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ----------------------------------------------------------------

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

const validKey = "memdev_a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef"

func activeKey() *auth.APIKey {
	k, _ := auth.NewAPIKey(
		"01JTEST00000000000000000AA",
		hashOf(validKey),
		"sophia-dev",
		"sophia-dev",
		"test key",
		auth.StatusActive,
		nil,
		time.Now(),
	)
	return k
}

func revokedKey() *auth.APIKey {
	k, _ := auth.NewAPIKey(
		"01JTEST00000000000000000BB",
		hashOf(validKey),
		"sophia-dev",
		"sophia-dev",
		"revoked key",
		auth.StatusRevoked,
		nil,
		time.Now(),
	)
	return k
}

// --- mock repo --------------------------------------------------------------

type mockRepo struct {
	mu           sync.Mutex
	findResult   *auth.APIKey
	findErr      error
	touchCalled  int
	touchWg      sync.WaitGroup // call wg.Done() on each TouchLastUsed invocation
	touchErr     error
}

func (m *mockRepo) FindByHash(_ context.Context, _ string) (*auth.APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.findResult, m.findErr
}

func (m *mockRepo) TouchLastUsed(_ context.Context, _ string, _ time.Time) error {
	m.mu.Lock()
	m.touchCalled++
	m.mu.Unlock()
	m.touchWg.Done()
	return m.touchErr
}

func (m *mockRepo) getTouchCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.touchCalled
}

// --- tests ------------------------------------------------------------------

func TestAuthenticate_HappyPath(t *testing.T) {
	repo := &mockRepo{findResult: activeKey()}
	repo.touchWg.Add(1)
	clock := shared.NewFixedClock(time.Now())
	svc := authsvc.NewService(repo, clock)

	ac, err := svc.Authenticate(context.Background(), validKey)
	require.NoError(t, err)
	assert.Equal(t, "sophia-dev", ac.TenantID)
	assert.Equal(t, "sophia-dev", ac.ProjectID)
	assert.Equal(t, "01JTEST00000000000000000AA", ac.KeyID)

	// TouchLastUsed must be called asynchronously.
	done := make(chan struct{})
	go func() {
		repo.touchWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TouchLastUsed was not called within 2 seconds")
	}
	assert.Equal(t, 1, repo.getTouchCalled())
}

func TestAuthenticate_EmptyKey_ReturnsInvalidKey(t *testing.T) {
	svc := authsvc.NewService(&mockRepo{}, shared.NewFixedClock(time.Now()))
	_, err := svc.Authenticate(context.Background(), "")
	assert.ErrorIs(t, err, auth.ErrInvalidKey)
}

func TestAuthenticate_TooShortKey_ReturnsInvalidKey(t *testing.T) {
	svc := authsvc.NewService(&mockRepo{}, shared.NewFixedClock(time.Now()))
	_, err := svc.Authenticate(context.Background(), "shortkey")
	assert.ErrorIs(t, err, auth.ErrInvalidKey)
}

func TestAuthenticate_Exactly31Chars_TooShort(t *testing.T) {
	svc := authsvc.NewService(&mockRepo{}, shared.NewFixedClock(time.Now()))
	// 31 characters — just below the 32-char minimum.
	_, err := svc.Authenticate(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.ErrorIs(t, err, auth.ErrInvalidKey)
}

func TestAuthenticate_Exactly32Chars_PassesLengthCheck(t *testing.T) {
	// 32 chars passes length gate; should hit repo and get ErrNotFound → ErrUnauthorized.
	repo := &mockRepo{findErr: auth.ErrNotFound}
	svc := authsvc.NewService(repo, shared.NewFixedClock(time.Now()))
	_, err := svc.Authenticate(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestAuthenticate_NotFound_ReturnsUnauthorized(t *testing.T) {
	repo := &mockRepo{findErr: auth.ErrNotFound}
	svc := authsvc.NewService(repo, shared.NewFixedClock(time.Now()))
	_, err := svc.Authenticate(context.Background(), validKey)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestAuthenticate_Revoked_ReturnsUnauthorized(t *testing.T) {
	repo := &mockRepo{findResult: revokedKey()}
	svc := authsvc.NewService(repo, shared.NewFixedClock(time.Now()))
	_, err := svc.Authenticate(context.Background(), validKey)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestAuthenticate_RepoError_ReturnsUnauthorized(t *testing.T) {
	repo := &mockRepo{findErr: errors.New("connection lost")}
	svc := authsvc.NewService(repo, shared.NewFixedClock(time.Now()))
	_, err := svc.Authenticate(context.Background(), validKey)
	assert.ErrorIs(t, err, auth.ErrUnauthorized)
}

func TestAuthenticate_TouchLastUsed_NotCalledOnFailure(t *testing.T) {
	// TouchLastUsed must NOT be called when authentication fails.
	repo := &mockRepo{findErr: auth.ErrNotFound}
	svc := authsvc.NewService(repo, shared.NewFixedClock(time.Now()))
	_, _ = svc.Authenticate(context.Background(), validKey)

	// Give goroutines a moment in case there is a bug.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, repo.getTouchCalled(), "TouchLastUsed must not be called on auth failure")
}
