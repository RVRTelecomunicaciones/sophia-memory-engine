package outbound

import "context"

// TransactionManager abstracts transactional boundaries for use case orchestration.
type TransactionManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}
