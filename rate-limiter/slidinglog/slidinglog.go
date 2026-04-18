// Package slidinglog implements a sliding window log rate limiter.
//
// For each key the limiter stores the timestamps of recent events. On every
// request, timestamps older than the window are evicted and the event is
// admitted only if the remaining log size is below the limit. Exact but
// O(events) memory per key.
package slidinglog

import "time"

// Limiter is a sliding window log rate limiter keyed by an opaque string.
type Limiter struct {
	limit  int
	window time.Duration
}

// New returns a Limiter that allows at most limit events within any window
// of the given duration ending at now.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window}
}

// Allow reports whether one event for key is permitted at now.
func (l *Limiter) Allow(key string, now time.Time) bool {
	panic("not implemented")
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "sliding_log" }
