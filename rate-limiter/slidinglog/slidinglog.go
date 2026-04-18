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

// bucket is a FIFO of in-window timestamps. Live entries are ts[head:],
// ordered oldest-first. Storage grows on demand via append (capped at
// limit) and is compacted when the unused prefix exceeds half the backing
// array, keeping per-key memory proportional to actual in-window load
// rather than the configured limit.
type bucket struct {
	ts   []time.Time
	head int
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
		b = &bucket{}
		l.buckets[key] = b
	}

	// Evict timestamps at or before the cutoff. Entries are appended in
	// monotonic order, so the oldest is always at head.
	for b.head < len(b.ts) && !b.ts[b.head].After(cutoff) {
		b.head++
	}

	// Compact when the unused prefix has grown past half the backing array
	// so the log cannot leak unbounded memory under sustained churn.
	if b.head > 0 && b.head >= len(b.ts)/2 {
		n := copy(b.ts, b.ts[b.head:])
		b.ts = b.ts[:n]
		b.head = 0
	}

	if len(b.ts)-b.head >= l.limit {
		return false
	}
	b.ts = append(b.ts, now)
	return true
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "sliding_log" }
