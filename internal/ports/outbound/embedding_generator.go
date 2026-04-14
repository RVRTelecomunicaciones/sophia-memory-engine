package outbound

import (
	"context"
	"errors"
)

// ErrEmbeddingsNotConfigured indicates that no embedding provider has been configured.
var ErrEmbeddingsNotConfigured = errors.New("embeddings not configured")

// EmbeddingGenerator produces vector embeddings from text input.
type EmbeddingGenerator interface {
	Generate(ctx context.Context, text string) ([]float64, error)
	BatchGenerate(ctx context.Context, texts []string) ([][]float64, error)
}
