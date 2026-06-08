package consolidation

import (
	"context"
	"time"
)

// EventSubscriber subscribes a handler to a remote orchestator event stream.
// Transport (SSE / webhook / message bus) is chosen at wiring time in
// cmd/workers/main.go; see V4.1 §16 M2 for the transport decision
// (deferred from M-KNOW-PRE-0).
//
// Cancellation of ctx stops the subscription; concrete implementations
// MUST exit cleanly when ctx is done.
type EventSubscriber interface {
	Subscribe(ctx context.Context, eventType string, handler EventHandler) error
}

// EventHandler processes a single received event. A non-nil error triggers
// the subscriber's transport-specific retry/backoff policy (defined by the
// M2 implementation); PRE-0's stub handler always returns nil.
type EventHandler func(ctx context.Context, payload PhaseArchivedReceived) error

// PhaseArchivedReceived is the worker-side mirror of the orchestator's
// inbound.PhaseArchivedPayload. The shape MUST stay byte-compatible with
// the orchestator-side JSON wire format. Field names match the orchestator
// struct exactly; the two repos are independent Go modules so this
// duplication is intentional (cf. explore.md:108-114).
type PhaseArchivedReceived struct {
	ChangeID   string    `json:"change_id"`
	ChangeName string    `json:"change_name"`
	PhaseType  string    `json:"phase_type"`
	ArchivedAt time.Time `json:"archived_at"`
}

// PhaseArchivedEventType is the string the subscriber filters on. Kept as a
// local literal (not imported from orchestator) because the two Go modules are
// independent — explicit decoupling per explore.md.
const PhaseArchivedEventType = "phase.archived"
