// Package tokenbucket implements a token bucket rate limiter.
//
// Each key owns a bucket of capacity tokens that refills at rate tokens per
// second. An event consumes one token; when the bucket is empty the event is
// rejected. Token buckets allow short bursts up to capacity while enforcing
// the long-run average rate.
package tokenbucket

import "time"

// Limiter is a token bucket rate limiter keyed by an opaque string.
type Limiter struct {
	capacity float64
	rate     float64
}

// New returns a Limiter with the given bucket capacity and refill rate in
// tokens per second. Both must be positive.
func New(capacity float64, ratePerSecond float64) *Limiter {
	return &Limiter{capacity: capacity, rate: ratePerSecond}
}

// Allow reports whether one event for key is permitted at now.
func (l *Limiter) Allow(key string, now time.Time) bool {
	panic("not implemented")
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "token_bucket" }
