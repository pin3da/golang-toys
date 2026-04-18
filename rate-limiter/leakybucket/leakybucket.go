// Package leakybucket implements a leaky bucket rate limiter.
//
// Each key owns a queue (conceptually a bucket) that drains at a constant
// rate. Arriving events fill the bucket; events that would overflow capacity
// are rejected. Unlike token bucket, the output rate is smooth: bursts are
// queued rather than passed through.
package leakybucket

import "time"

// Limiter is a leaky bucket rate limiter keyed by an opaque string.
type Limiter struct {
	capacity float64
	leakRate float64
}

// New returns a Limiter with the given bucket capacity (maximum queued
// events) and leak rate in events per second.
func New(capacity float64, leakPerSecond float64) *Limiter {
	return &Limiter{capacity: capacity, leakRate: leakPerSecond}
}

// Allow reports whether one event for key is permitted at now.
func (l *Limiter) Allow(key string, now time.Time) bool {
	panic("not implemented")
}

// Name returns the algorithm identifier.
func (l *Limiter) Name() string { return "leaky_bucket" }
