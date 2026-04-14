package inbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// RequestPurgeCmd carries the data needed to request a hard purge.
type RequestPurgeCmd struct {
	TargetID    shared.RecordID
	Reason      shared.PurgeReason
	RequestedBy string
	AuditNote   string
}

// ExecutePurgeCmd carries the identifier of a pending purge to execute.
type ExecutePurgeCmd struct {
	PurgeID shared.RecordID
}

// PurgeService defines the inbound port for security purge operations.
type PurgeService interface {
	Request(ctx context.Context, cmd RequestPurgeCmd) (*purge.PurgeRecord, error)
	Execute(ctx context.Context, cmd ExecutePurgeCmd) (*purge.PurgeRecord, error)
}
