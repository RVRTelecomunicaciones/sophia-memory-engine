package embeddings

import (
	"context"
	"errors"
)

// ErrEmbeddingsNotConfigured is returned when no embedding provider is available.
var ErrEmbeddingsNotConfigured = errors.New("embeddings not configured")

// NoopEmbeddingGenerator is a stub implementation of the EmbeddingGenerator port.
// It always returns ErrEmbeddingsNotConfigured. Phase 1 does not depend on embeddings.
type NoopEmbeddingGenerator struct{}

// NewNoopEmbeddingGenerator creates a new noop embedding generator.
func NewNoopEmbeddingGenerator() *NoopEmbeddingGenerator {
	return &NoopEmbeddingGenerator{}
}

// Generate returns ErrEmbeddingsNotConfigured.
func (n *NoopEmbeddingGenerator) Generate(_ context.Context, _ string) ([]float64, error) {
	return nil, ErrEmbeddingsNotConfigured
}

// BatchGenerate returns ErrEmbeddingsNotConfigured.
func (n *NoopEmbeddingGenerator) BatchGenerate(_ context.Context, _ []string) ([][]float64, error) {
	return nil, ErrEmbeddingsNotConfigured
}
