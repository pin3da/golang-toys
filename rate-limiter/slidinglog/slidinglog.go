// Package slidinglog implements a sliding window log rate limiter.
//
// For each key the limiter stores the timestamps of events currently inside
// the window (now-window, now]. On every request, expired timestamps are
// evicted and the event is admitted only if fewer than limit timestamps
// remain. The answer is exact -- unlike fixed or sliding-window counters,
// there is no boundary approximation -- at the cost of O(limit) memory per
// active key.
package slidinglog

import (
	"sync"
	"time"
)

// Limiter is a sliding window log rate limiter keyed by an opaque string.
//
// Safe for concurrent use by multiple goroutines.
type Limiter struct {
	limit  int
	window time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
}

// bucket is a fixed-capacity ring buffer of at most limit timestamps,
// ordered oldest-first from head. Using a ring buffer bounds per-key memory
// to exactly limit entries, independent of churn.
type bucket struct {
	ts   []time.Time
	head int
	size int
}

// New returns a Limiter that allows at most limit events within any window
// of the given duration ending at now. Both limit and window must be
// positive.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether one event for key is permitted at now.
//
// If permitted, now is recorded in the key's log. Timestamps strictly older
// than now-window are evicted on every call, so expired keys' logs shrink
// to empty (the bucket entry itself is retained).
func (l *Limiter) Allow(key string, now time.Time) bool {
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{ts: make([]time.Time, l.limit)}
		l.buckets[key] = b
	}

	// Evict timestamps at or before the cutoff. Entries are appended in
	// monotonic order, so the oldest is always at head.
	for b.size > 0 && !b.ts[b.head].After(cutoff) {
		b.head = (b.head + 1) % l.limit
		b.size--
	}

	if b.size >= l.limit {
		return false
	}
	tail := (b.head + b.size) % l.limit
	b.ts[tail] = now
	b.size++
	return true
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "sliding_log" }
