// Package slidingwindow implements a sliding window counter rate limiter.
//
// The limiter keeps two fixed-window counters per key -- the current window
// and the immediately previous one -- and estimates the count in the
// sliding interval (now-window, now] by weighting the previous counter by
// the fraction of the previous window that still overlaps the interval:
//
//	estimate = prev * (1 - elapsed/window) + curr
//
// where elapsed = now - currentWindowStart. The approximation assumes
// requests are distributed uniformly within the previous window; in
// practice the error is small and bounded, and memory is O(1) per key
// versus O(limit) for a sliding log.
//
// See: https://blog.cloudflare.com/counting-things-a-lot-of-different-things/
package slidingwindow

import (
	"sync"
	"time"
)

// Limiter is a sliding window counter rate limiter keyed by an opaque string.
//
// Safe for concurrent use by multiple goroutines.
type Limiter struct {
	limit  int
	window time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	currStart time.Time
	currCount int
	prevCount int
}

// New returns a Limiter that allows approximately limit events per sliding
// window of the given duration. Both limit and window must be positive.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether one event for key is permitted at now.
//
// The decision uses the weighted-estimate formula described in the package
// doc; on admit the current window's counter is incremented.
func (l *Limiter) Allow(key string, now time.Time) bool {
	start := now.Truncate(l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{currStart: start}
		l.buckets[key] = b
	}
	l.advance(b, start)

	elapsed := now.Sub(b.currStart)
	prevWeight := 1 - float64(elapsed)/float64(l.window)
	estimate := float64(b.prevCount)*prevWeight + float64(b.currCount)
	if estimate >= float64(l.limit) {
		return false
	}
	b.currCount++
	return true
}

// advance rolls the bucket's counters forward so that currStart == start.
// If start is exactly one window ahead, the current counter becomes the
// previous. A larger gap means both counters are stale and are cleared.
func (l *Limiter) advance(b *bucket, start time.Time) {
	if b.currStart.Equal(start) {
		return
	}
	if b.currStart.Add(l.window).Equal(start) {
		b.prevCount = b.currCount
	} else {
		b.prevCount = 0
	}
	b.currCount = 0
	b.currStart = start
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "sliding_window" }
