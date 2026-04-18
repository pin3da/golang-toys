// Package fixedwindow implements a fixed window counter rate limiter.
//
// Time is divided into contiguous windows of fixed width. Each key has a
// counter per active window; when the counter reaches the limit, further
// events in that window are rejected. Simple and memory-cheap, but permits
// bursts of up to 2*limit across a window boundary.
package fixedwindow

import "time"

// Limiter is a fixed-window counter rate limiter keyed by an opaque string.
type Limiter struct {
	limit  int
	window time.Duration
}

// New returns a Limiter that allows at most limit events per window.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window}
}

// Allow reports whether one event for key is permitted at now.
func (l *Limiter) Allow(key string, now time.Time) bool {
	panic("not implemented")
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "fixed_window" }
