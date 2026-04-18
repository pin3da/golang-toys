// Package fixedwindow implements a fixed window counter rate limiter.
//
// Time is divided into contiguous windows of fixed width aligned to the Unix
// epoch. Each key has a counter for the current window; when the counter
// reaches the limit, further events in that window are rejected. Simple and
// memory-cheap, but permits bursts of up to 2*limit across a window boundary.
package fixedwindow

import (
	"sync"
	"time"
)

// Limiter is a fixed-window counter rate limiter keyed by an opaque string.
//
// Safe for concurrent use by multiple goroutines.
type Limiter struct {
	limit  int
	window time.Duration

	mu      sync.Mutex
	buckets map[string]bucket
}

type bucket struct {
	windowStart time.Time
	count       int
}

// New returns a Limiter that allows at most limit events per window.
// Both limit and window must be positive.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]bucket),
	}
}

// Allow reports whether one event for key is permitted at now.
//
// If permitted, the key's counter for the current window is incremented.
func (l *Limiter) Allow(key string, now time.Time) bool {
	start := now.Truncate(l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.buckets[key]
	if !b.windowStart.Equal(start) {
		b = bucket{windowStart: start}
	}
	if b.count >= l.limit {
		l.buckets[key] = b
		return false
	}
	b.count++
	l.buckets[key] = b
	return true
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "fixed_window" }
