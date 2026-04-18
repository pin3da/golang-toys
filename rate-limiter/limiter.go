// Package ratelimiter defines a common interface for rate limiting algorithms
// so that different implementations can be benchmarked for memory and time
// efficiency under identical workloads.
package ratelimiter

import "time"

// Limiter decides whether an event identified by key is permitted at time now.
//
// Implementations must be safe for concurrent use by multiple goroutines.
// The now parameter is injected to keep implementations deterministic and
// testable; production callers should pass time.Now().
type Limiter interface {
	// Allow reports whether one event for key is permitted at now.
	// It returns true if the event is accepted (and any internal counters
	// are updated accordingly), or false if the event would exceed the
	// configured rate.
	Allow(key string, now time.Time) bool

	// Name returns a short human-readable identifier for the algorithm,
	// used in benchmark output and logs.
	Name() string
}
