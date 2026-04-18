// Package slidingwindow implements a sliding window counter rate limiter.
//
// The limiter keeps the counter for the current fixed window plus the
// counter for the previous window, and estimates the sliding count by
// weighting the previous window by the fraction that still overlaps the
// sliding interval. Bounded memory and a much tighter approximation than
// fixed window.
package slidingwindow

import "time"

// Limiter is a sliding window counter rate limiter keyed by an opaque string.
type Limiter struct {
	limit  int
	window time.Duration
}

// New returns a Limiter that allows at most limit events per sliding window.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window}
}

// Allow reports whether one event for key is permitted at now.
func (l *Limiter) Allow(key string, now time.Time) bool {
	panic("not implemented")
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "sliding_window" }
