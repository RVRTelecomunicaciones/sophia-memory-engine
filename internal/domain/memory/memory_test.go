package memory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
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

func fixedClock() *shared.FixedClock {
	return shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
}

func TestNewMemoryRecord_Episodic_RequiresValidFrom(t *testing.T) {
	_, err := memory.NewMemoryRecord(
		shared.MemoryTypeEpisodic,
		"something happened",
		validScope(),
		validProvenance(),
		fixedClock(),
	)

	if err == nil {
		t.Fatal("expected validation error for episodic without ValidFrom")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("expected ErrValidation, got: %v", err)
	}
}

func TestNewMemoryRecord_Episodic_Valid(t *testing.T) {
	clk := fixedClock()
	now := clk.Now()

	m, err := memory.NewMemoryRecord(
		shared.MemoryTypeEpisodic,
		"something happened",
		validScope(),
		validProvenance(),
		clk,
		memory.WithValidFrom(now),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Type != shared.MemoryTypeEpisodic {
		t.Errorf("expected episodic, got %s", m.Type)
	}
	if m.Content != "something happened" {
		t.Errorf("unexpected content: %s", m.Content)
	}
	if m.Status != shared.MemoryStatusActive {
		t.Errorf("expected active status, got %s", m.Status)
	}
	if m.FTSLanguage != "spanish" {
		t.Errorf("expected spanish FTS language, got %s", m.FTSLanguage)
	}
	if m.Temporal.ValidFrom == nil {
		t.Fatal("expected ValidFrom to be set")
	}
	if !m.Temporal.ValidFrom.Equal(now) {
		t.Errorf("expected ValidFrom %v, got %v", now, *m.Temporal.ValidFrom)
	}
	if m.Temporal.Freshness != shared.FreshnessLevelFresh {
		t.Errorf("expected fresh, got %s", m.Temporal.Freshness)
	}
	if len(m.Tags) != 0 {
		t.Errorf("expected empty tags, got %v", m.Tags)
	}
	if m.ID.IsZero() {
		t.Error("expected non-zero ID")
	}
}

func TestNewMemoryRecord_Semantic_ValidFromOptional(t *testing.T) {
	m, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"Go uses goroutines for concurrency",
		validScope(),
		validProvenance(),
		fixedClock(),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Temporal.ValidFrom != nil {
		t.Error("expected ValidFrom to be nil for semantic without explicit option")
	}
	if m.Type != shared.MemoryTypeSemantic {
		t.Errorf("expected semantic, got %s", m.Type)
	}
}

func TestNewMemoryRecord_EmptyContent_Fails(t *testing.T) {
	_, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"",
		validScope(),
		validProvenance(),
		fixedClock(),
	)

	if err == nil {
		t.Fatal("expected validation error for empty content")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("expected ErrValidation, got: %v", err)
	}
}

func TestMemoryRecord_Archive(t *testing.T) {
	clk := fixedClock()
	m, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"some knowledge",
		validScope(),
		validProvenance(),
		clk,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.Archive("admin", "outdated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Status != shared.MemoryStatusArchived {
		t.Errorf("expected archived, got %s", m.Status)
	}
	if m.ArchivedBy == nil || *m.ArchivedBy != "admin" {
		t.Error("expected ArchivedBy to be 'admin'")
	}
	if m.ArchiveReason == nil || *m.ArchiveReason != "outdated" {
		t.Error("expected ArchiveReason to be 'outdated'")
	}
}

func TestMemoryRecord_Archive_AlreadyArchived(t *testing.T) {
	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"some knowledge",
		validScope(),
		validProvenance(),
		fixedClock(),
	)

	_ = m.Archive("admin", "outdated")
	err := m.Archive("admin", "again")

	if !errors.Is(err, shared.ErrAlreadyArchived) {
		t.Fatalf("expected ErrAlreadyArchived, got: %v", err)
	}
}

func TestMemoryRecord_Archive_Purged(t *testing.T) {
	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"some knowledge",
		validScope(),
		validProvenance(),
		fixedClock(),
	)

	m.MarkPurged()
	err := m.Archive("admin", "reason")

	if !errors.Is(err, shared.ErrPurged) {
		t.Fatalf("expected ErrPurged, got: %v", err)
	}
}

func TestMemoryRecord_MarkPurged_WipesContent(t *testing.T) {
	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic,
		"sensitive data",
		validScope(),
		validProvenance(),
		fixedClock(),
		memory.WithSummary("a summary"),
		memory.WithTags([]string{"secret", "sensitive"}),
	)

	m.MarkPurged()

	if m.Status != shared.MemoryStatusPurged {
		t.Errorf("expected purged, got %s", m.Status)
	}
	if m.Content != "" {
		t.Errorf("expected empty content, got %q", m.Content)
	}
	if m.Summary != nil {
		t.Errorf("expected nil summary, got %v", m.Summary)
	}
	if len(m.Tags) != 0 {
		t.Errorf("expected empty tags, got %v", m.Tags)
	}
}
