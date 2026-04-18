// Package tokenbucket implements a token bucket rate limiter.
//
// Each key owns a bucket of capacity tokens that refills at rate tokens per
// second. An event consumes one token; when the bucket is empty the event is
// rejected. Token buckets allow short bursts up to capacity while enforcing
// the long-run average rate.
package tokenbucket

import (
	"sync"
	"time"
)

// Limiter is a token bucket rate limiter keyed by an opaque string.
//
// Safe for concurrent use by multiple goroutines.
type Limiter struct {
	capacity float64
	rate     float64

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// New returns a Limiter with the given bucket capacity and refill rate in
// tokens per second. Both must be positive.
func New(capacity float64, ratePerSecond float64) *Limiter {
	return &Limiter{
		capacity: capacity,
		rate:     ratePerSecond,
		buckets:  make(map[string]*bucket),
	}
}

// Allow reports whether one event for key is permitted at now.
//
// On first sight of a key the bucket starts full. Each call refills the
// bucket based on elapsed time since its last refill (capped at capacity)
// and then consumes one token if at least one is available.
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, lastRefill: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastRefill).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * l.rate
			if b.tokens > l.capacity {
				b.tokens = l.capacity
			}
			b.lastRefill = now
		}
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "token_bucket" }
