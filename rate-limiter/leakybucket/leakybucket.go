// Package leakybucket implements a leaky bucket rate limiter.
//
// Each key owns a queue (conceptually a bucket) that drains at a constant
// rate. Arriving events fill the bucket; events that would overflow capacity
// are rejected. Unlike token bucket, the output rate is smooth: bursts are
// queued rather than passed through.
package leakybucket

import (
	"sync"
	"time"
)

// Limiter is a leaky bucket rate limiter keyed by an opaque string.
//
// Safe for concurrent use by multiple goroutines.
type Limiter struct {
	capacity float64
	leakRate float64

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	level    float64
	lastLeak time.Time
}

// New returns a Limiter with the given bucket capacity (maximum queued
// events) and leak rate in events per second. Both must be positive.
func New(capacity float64, leakPerSecond float64) *Limiter {
	return &Limiter{
		capacity: capacity,
		leakRate: leakPerSecond,
		buckets:  make(map[string]*bucket),
	}
}

// Allow reports whether one event for key is permitted at now.
//
// The bucket is first drained by leakRate * elapsed (floored at zero);
// if the resulting level plus one would exceed capacity the event is
// rejected, otherwise the level is incremented by one.
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lastLeak: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastLeak).Seconds()
		if elapsed > 0 {
			b.level -= elapsed * l.leakRate
			if b.level < 0 {
				b.level = 0
			}
			b.lastLeak = now
		}
	}

	if b.level+1 > l.capacity {
		return false
	}
	b.level++
	return true
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "leaky_bucket" }
