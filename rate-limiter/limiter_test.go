package ratelimiter_test

import (
	"sync"
	"testing"
	"time"

	ratelimiter "github.com/pin3da/golang-toys/rate-limiter"
	"github.com/pin3da/golang-toys/rate-limiter/fixedwindow"
	"github.com/pin3da/golang-toys/rate-limiter/leakybucket"
	"github.com/pin3da/golang-toys/rate-limiter/slidinglog"
	"github.com/pin3da/golang-toys/rate-limiter/slidingwindow"
	"github.com/pin3da/golang-toys/rate-limiter/tokenbucket"
)

// limiterCases lists one factory per algorithm, each configured so that only
// capacity (not refill/leak/window rollover) decides admissions within a
// single instant: bursty arrivals at the same now must see the full budget
// and no more.
func limiterCases() []struct {
	name string
	make func(capacity int) ratelimiter.Limiter
} {
	return []struct {
		name string
		make func(capacity int) ratelimiter.Limiter
	}{
		{"fixed_window", func(c int) ratelimiter.Limiter { return fixedwindow.New(c, time.Hour) }},
		{"sliding_log", func(c int) ratelimiter.Limiter { return slidinglog.New(c, time.Hour) }},
		{"sliding_window", func(c int) ratelimiter.Limiter { return slidingwindow.New(c, time.Hour) }},
		{"token_bucket", func(c int) ratelimiter.Limiter { return tokenbucket.New(float64(c), 0.0001) }},
		{"leaky_bucket", func(c int) ratelimiter.Limiter { return leakybucket.New(float64(c), 0.0001) }},
	}
}

// TestLimitersSatisfyInterface ensures every implementation is usable through
// the common Limiter interface. It does not exercise Allow; per-algorithm
// behavior tests live alongside each implementation.
func TestLimitersSatisfyInterface(t *testing.T) {
	for _, tc := range limiterCases() {
		l := tc.make(10)
		if l.Name() == "" {
			t.Errorf("%s: Name() returned empty string", tc.name)
		}
	}
}

// TestConcurrentAllowDoesNotExceedCapacity asserts that under concurrent
// access, no implementation admits more than capacity events at a single
// instant. This exercises the mutex discipline shared by all algorithms.
func TestConcurrentAllowDoesNotExceedCapacity(t *testing.T) {
	const capacity = 100
	const goroutines = 500
	now := time.Unix(100, 0)

	for _, tc := range limiterCases() {
		t.Run(tc.name, func(t *testing.T) {
			l := tc.make(capacity)
			var wg sync.WaitGroup
			var mu sync.Mutex
			allowed := 0
			for range goroutines {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if l.Allow("k", now) {
						mu.Lock()
						allowed++
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			if allowed != capacity {
				t.Fatalf("allowed = %d, want %d", allowed, capacity)
			}
		})
	}
}
