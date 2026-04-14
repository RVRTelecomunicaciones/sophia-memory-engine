package decision_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

func validScope() shared.Scope {
	s, _ := shared.NewScope("proj-1")
	return s
}

func validProvenance() shared.Provenance {
	p, _ := shared.NewProvenance("agent:coder-1", shared.IngestMethodDirect, nil)
	return p
}

func validConfidence() shared.Confidence {
	c, _ := shared.NewConfidence(0.85, shared.ConfidenceSourceAgent)
	return c
}

func validEvidence() []shared.EvidenceRef {
	excerpt := "benchmarks show 2x improvement"
	e, _ := shared.NewEvidenceRef(shared.EvidenceTypeInlineExcerpt, nil, nil, &excerpt)
	return []shared.EvidenceRef{e}
}

func fixedClock() *shared.FixedClock {
	return shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
}

func validDecision(t *testing.T) *decision.Decision {
	t.Helper()
	d, err := decision.NewDecision(
		"arch/cache-strategy",
		"Use Redis for caching",
		"We need a distributed cache layer",
		"Redis provides pub/sub and TTL natively",
		validEvidence(),
		validScope(),
		validProvenance(),
		validConfidence(),
		fixedClock(),
		1,
	)
	if err != nil {
		t.Fatalf("unexpected error creating valid decision: %v", err)
	}
	return d
}

func TestNewDecision_Valid(t *testing.T) {
	d := validDecision(t)

	if d.DecisionKey != "arch/cache-strategy" {
		t.Errorf("expected key 'arch/cache-strategy', got %q", d.DecisionKey)
	}
	if d.Title != "Use Redis for caching" {
		t.Errorf("unexpected title: %q", d.Title)
	}
	if d.Status != shared.DecisionStatusActive {
		t.Errorf("expected active status, got %s", d.Status)
	}
	if d.FTSLanguage != "spanish" {
		t.Errorf("expected spanish FTS language, got %s", d.FTSLanguage)
	}
	if d.Version != 1 {
		t.Errorf("expected version 1, got %d", d.Version)
	}
	if d.Temporal.Freshness != shared.FreshnessLevelFresh {
		t.Errorf("expected fresh, got %s", d.Temporal.Freshness)
	}
	if d.ID.IsZero() {
		t.Error("expected non-zero ID")
	}
	if d.SupersededBy != nil {
		t.Error("expected nil SupersededBy")
	}
}

func TestNewDecision_RequiresEvidence(t *testing.T) {
	_, err := decision.NewDecision(
		"arch/test",
		"Title",
		"Description",
		"Rationale",
		[]shared.EvidenceRef{},
		validScope(),
		validProvenance(),
		validConfidence(),
		fixedClock(),
		1,
	)

	if err == nil {
		t.Fatal("expected validation error for empty evidence")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("expected ErrValidation, got: %v", err)
	}
}

func TestDecision_Supersede(t *testing.T) {
	d := validDecision(t)
	newID := shared.NewRecordID()

	err := d.Supersede(newID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != shared.DecisionStatusSuperseded {
		t.Errorf("expected superseded, got %s", d.Status)
	}
	if d.SupersededBy == nil {
		t.Fatal("expected SupersededBy to be set")
	}
	if !d.SupersededBy.Equal(newID) {
		t.Errorf("expected SupersededBy %s, got %s", newID, d.SupersededBy)
	}
}

func TestDecision_Supersede_NotActive(t *testing.T) {
	d := validDecision(t)
	_ = d.Archive()

	err := d.Supersede(shared.NewRecordID())
	if !errors.Is(err, shared.ErrNotActive) {
		t.Fatalf("expected ErrNotActive, got: %v", err)
	}
}

func TestDecision_Contradict(t *testing.T) {
	d := validDecision(t)

	err := d.Contradict()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != shared.DecisionStatusContradicted {
		t.Errorf("expected contradicted, got %s", d.Status)
	}
}

func TestDecision_Archive(t *testing.T) {
	d := validDecision(t)

	err := d.Archive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != shared.DecisionStatusArchived {
		t.Errorf("expected archived, got %s", d.Status)
	}
}

func TestDecision_Archive_FromSuperseded(t *testing.T) {
	d := validDecision(t)
	_ = d.Supersede(shared.NewRecordID())

	err := d.Archive()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != shared.DecisionStatusArchived {
		t.Errorf("expected archived, got %s", d.Status)
	}
}

func TestDecision_NoTransitionToDeleted(t *testing.T) {
	// Verify that Decision has no Delete method — decisions never disappear.
	dType := reflect.TypeOf(&decision.Decision{})
	_, hasDelete := dType.MethodByName("Delete")
	if hasDelete {
		t.Error("Decision must NOT have a Delete method — decisions never disappear")
	}

	// Status should remain as-is; no "deleted" status exists.
	d := validDecision(t)
	if d.Status != shared.DecisionStatusActive {
		t.Errorf("expected active, got %s", d.Status)
	}
}

func TestDecision_CannotReactivate(t *testing.T) {
	d := validDecision(t)
	_ = d.Archive()

	// Trying to archive again from archived state should fail.
	err := d.Archive()
	if !errors.Is(err, shared.ErrNotActive) {
		t.Fatalf("expected ErrNotActive when archiving already archived decision, got: %v", err)
	}
}
