package ratelimiter_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	ratelimiter "github.com/pin3da/golang-toys/rate-limiter"
	"github.com/pin3da/golang-toys/rate-limiter/fixedwindow"
	"github.com/pin3da/golang-toys/rate-limiter/leakybucket"
	"github.com/pin3da/golang-toys/rate-limiter/slidinglog"
	"github.com/pin3da/golang-toys/rate-limiter/slidingwindow"
	"github.com/pin3da/golang-toys/rate-limiter/tokenbucket"
)

// TestApproximationAccuracy computes how closely the approximate rate limiters
// match the exact sliding window log limiter over a long, bursty workload.
// Run with "go test -v -run TestApproximationAccuracy" to see the full report.
func TestApproximationAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long accuracy test in short mode")
	}

	const (
		capacity = 50
		window   = time.Second
		// 1,000,000 windows will generate ~240M events and take roughly 40-60s
		duration = 1000000 * time.Second
	)

	// Generate a deterministic but bursty sequence of simulated events.
	// We want enough scale to confidently measure the error bounds.
	r := rand.New(rand.NewSource(42))
	var events []time.Time
	now := time.Unix(0, 0)
	endTime := now.Add(duration)

	for now.Before(endTime) {
		if r.Float64() < 0.01 {
			// 1% chance: Long idle gap (up to 1 full window)
			now = now.Add(time.Duration(r.Float64() * float64(window)))
		} else if r.Float64() < 0.05 {
			// 5% chance: Rapid burst of closely spaced events
			burstSize := r.Intn(100) + 1
			for i := 0; i < burstSize; i++ {
				events = append(events, now)
				now = now.Add(time.Microsecond)
			}
		} else {
			// Typical spacing: events arrive slightly faster than the limit
			// on average to enforce frequent contention.
			events = append(events, now)
			now = now.Add(time.Duration(r.ExpFloat64() * float64(window/time.Duration(capacity*2))))
		}
	}

	tests := []struct {
		name string
		make func() ratelimiter.Limiter
	}{
		{"fixed_window", func() ratelimiter.Limiter { return fixedwindow.New(capacity, window) }},
		{"sliding_window", func() ratelimiter.Limiter { return slidingwindow.New(capacity, window) }},
		{"token_bucket", func() ratelimiter.Limiter {
			return tokenbucket.New(float64(capacity), float64(capacity)/window.Seconds())
		}},
		{"leaky_bucket", func() ratelimiter.Limiter {
			return leakybucket.New(float64(capacity), float64(capacity)/window.Seconds())
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exact := slidinglog.New(capacity, window)
			approx := tt.make()

			var exactAllowed, approxAllowed int
			var falsePositives, falseNegatives int

			for _, ev := range events {
				e := exact.Allow("k", ev)
				a := approx.Allow("k", ev)

				if e {
					exactAllowed++
				}
				if a {
					approxAllowed++
				}

				if a && !e {
					falsePositives++ // approx allowed but exact did not
				} else if !a && e {
					falseNegatives++ // exact allowed but approx did not
				}
			}

			totalEvents := len(events)
			fpRate := 100.0 * float64(falsePositives) / float64(totalEvents)
			fnRate := 100.0 * float64(falseNegatives) / float64(totalEvents)
			agreement := 100.0 * float64(totalEvents-(falsePositives+falseNegatives)) / float64(totalEvents)

			summary := fmt.Sprintf("\n"+
				"=== Accuracy Report for %s ===\n"+
				"Total Events:     %d\n"+
				"Exact Allowed:    %d\n"+
				"Approx Allowed:   %d\n"+
				"False Positives:  %d (%.2f%% of all events)\n"+
				"False Negatives:  %d (%.2f%% of all events)\n"+
				"Agreement:        %.2f%%\n",
				tt.name, totalEvents, exactAllowed, approxAllowed,
				falsePositives, fpRate, falseNegatives, fnRate, agreement)

			t.Log(summary)

			// Fast sanity check: algorithms should be somewhat comparable to the exact version.
			if agreement < 75.0 {
				t.Errorf("%s agreement %.2f%% is below minimum acceptable accuracy of 75.0%%", tt.name, agreement)
			}
		})
	}
}

// TestLimiterCharacteristics exposes the varying behavioral tradeoffs
// of the different rate limit algorithms under specific scenarios.
func TestLimiterCharacteristics(t *testing.T) {
	const (
		capacity = 50
		window   = time.Second
	)

	tests := []struct {
		name string
		make func() ratelimiter.Limiter
	}{
		{"sliding_log", func() ratelimiter.Limiter { return slidinglog.New(capacity, window) }},
		{"fixed_window", func() ratelimiter.Limiter { return fixedwindow.New(capacity, window) }},
		{"sliding_window", func() ratelimiter.Limiter { return slidingwindow.New(capacity, window) }},
		{"token_bucket", func() ratelimiter.Limiter {
			return tokenbucket.New(float64(capacity), float64(capacity)/window.Seconds())
		}},
		{"leaky_bucket", func() ratelimiter.Limiter {
			return leakybucket.New(float64(capacity), float64(capacity)/window.Seconds())
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Strict Window Adherence
			limiter := tt.make()

			r := rand.New(rand.NewSource(42))
			var events []time.Time
			now := time.Unix(0, 0)
			endTime := now.Add(10 * time.Second)

			for now.Before(endTime) {
				if r.Float64() < 0.01 {
					now = now.Add(time.Duration(r.Float64() * float64(window)))
				} else if r.Float64() < 0.05 {
					burstSize := r.Intn(200) + 1 // bursts larger than capacity
					for i := 0; i < burstSize; i++ {
						events = append(events, now)
						now = now.Add(time.Microsecond)
					}
				} else {
					events = append(events, now)
					now = now.Add(time.Duration(r.ExpFloat64() * float64(window/time.Duration(capacity*2))))
				}
			}

			var allowed []time.Time
			for _, ev := range events {
				if limiter.Allow("k", ev) {
					allowed = append(allowed, ev)
				}
			}

			maxInStrictWindow := 0
			for i := 0; i < len(allowed); i++ {
				count := 0
				cutoff := allowed[i].Add(-window)
				// Look backward into the 1s window (cutoff, allowed[i]]
				for j := i; j >= 0 && allowed[j].After(cutoff); j-- {
					count++
				}
				if count > maxInStrictWindow {
					maxInStrictWindow = count
				}
			}

			// 2. Recovery Time
			limiter2 := tt.make()
			now2 := time.Unix(1000000, 0)

			// Drain capacity completely
			for i := 0; i < capacity*2; i++ {
				limiter2.Allow("k2", now2)
			}

			// Advance time incrementally until 1 request is permitted
			recoveryStart := now2
			for {
				now2 = now2.Add(time.Millisecond)
				if limiter2.Allow("k2", now2) {
					break
				}
			}
			recoveryMs := now2.Sub(recoveryStart).Milliseconds()

			// 3. Long-term Throughput
			limiter3 := tt.make()
			now3 := time.Unix(2000000, 0)
			totalAllowed := 0

			// Sustain 200 req/sec (4x overload) for 60 seconds
			step := time.Second / 200
			for i := 0; i < 200*60; i++ {
				if limiter3.Allow("k3", now3) {
					totalAllowed++
				}
				now3 = now3.Add(step)
			}

			t.Logf("\n=== %s Characteristics ==="+
				"\nMax allowed in any strict 1s window (Limit %d): %d"+
				"\nTime back to first allowed req after full burst: %d ms"+
				"\nTotal allowed over 60s at 200 req/s (Target %d): %d\n",
				tt.name, capacity, maxInStrictWindow, recoveryMs, capacity*60, totalAllowed)
		})
	}
}
