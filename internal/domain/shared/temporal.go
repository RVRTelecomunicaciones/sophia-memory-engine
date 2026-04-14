package shared

import "time"

// Clock abstracts time access for testability.
type Clock interface {
	Now() time.Time
}

// RealClock returns the actual system time.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// FixedClock returns a fixed time, useful for deterministic tests.
type FixedClock struct {
	FixedTime time.Time
}

// NewFixedClock creates a FixedClock pinned to the given time.
func NewFixedClock(t time.Time) *FixedClock {
	return &FixedClock{FixedTime: t}
}

// Now returns the fixed time.
func (c *FixedClock) Now() time.Time {
	return c.FixedTime
}

// Advance moves the fixed time forward by the given duration.
func (c *FixedClock) Advance(d time.Duration) {
	c.FixedTime = c.FixedTime.Add(d)
}

// TemporalMetadata holds temporal validity and freshness data for a memory.
type TemporalMetadata struct {
	ValidFrom    *time.Time
	ValidUntil   *time.Time
	LastAccessed *time.Time
	Freshness    FreshnessLevel
}

// FreshnessConfig defines the thresholds for freshness computation.
type FreshnessConfig struct {
	AgingThreshold time.Duration
	StaleThreshold time.Duration
}

// DefaultFreshnessConfig returns default freshness thresholds:
// aging after 7 days, stale after 30 days.
func DefaultFreshnessConfig() FreshnessConfig {
	return FreshnessConfig{
		AgingThreshold: 7 * 24 * time.Hour,
		StaleThreshold: 30 * 24 * time.Hour,
	}
}

// ComputeFreshness determines the freshness level based on temporal data.
// Rules:
//  1. If ValidUntil is set and in the past → expired
//  2. If LastAccessed is nil and no expiry → fresh
//  3. age = now - LastAccessed: > StaleThreshold → stale, > AgingThreshold → aging, else → fresh
func (tm TemporalMetadata) ComputeFreshness(cfg FreshnessConfig, clock Clock) FreshnessLevel {
	now := clock.Now()

	// Rule 1: expired if past validity window
	if tm.ValidUntil != nil && now.After(*tm.ValidUntil) {
		return FreshnessLevelExpired
	}

	// Rule 2: timeless (never accessed, no expiry) → fresh
	if tm.LastAccessed == nil {
		return FreshnessLevelFresh
	}

	// Rule 3: compute age from last access
	age := now.Sub(*tm.LastAccessed)
	if age > cfg.StaleThreshold {
		return FreshnessLevelStale
	}
	if age > cfg.AgingThreshold {
		return FreshnessLevelAging
	}
	return FreshnessLevelFresh
}

// IsExpired returns true if the memory has passed its ValidUntil time.
func (tm TemporalMetadata) IsExpired(clock Clock) bool {
	if tm.ValidUntil == nil {
		return false
	}
	return clock.Now().After(*tm.ValidUntil)
}
