package ratelimiter_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	ratelimiter "github.com/pin3da/golang-toys/rate-limiter"
	"github.com/pin3da/golang-toys/rate-limiter/fixedwindow"
	"github.com/pin3da/golang-toys/rate-limiter/slidinglog"
	"github.com/pin3da/golang-toys/rate-limiter/slidingwindow"
)

// randomKeys returns n pseudo-random hex strings drawn from a fixed seed so
// results are reproducible across runs. Generation is intentionally done
// outside benchmark timing; callers must invoke it before b.ResetTimer.
func randomKeys(n int) []string {
	r := rand.New(rand.NewPCG(0xC0FFEE, 0xFEEDFACE))
	keys := make([]string, n)
	for i := range keys {
		keys[i] = strconv.FormatUint(r.Uint64(), 16)
	}
	return keys
}

const keyPoolSize = 100_000

// limiterFactory builds a fresh Limiter for each benchmark run so state from
// one sub-benchmark cannot leak into another.
type limiterFactory struct {
	name string
	make func() ratelimiter.Limiter
}

func factories() []limiterFactory {
	return []limiterFactory{
		{"fixed_window", func() ratelimiter.Limiter {
			return fixedwindow.New(1_000_000, time.Second)
		}},
		{"sliding_log", func() ratelimiter.Limiter {
			return slidinglog.New(1_000_000, time.Second)
		}},
		{"sliding_window", func() ratelimiter.Limiter {
			return slidingwindow.New(1_000_000, time.Second)
		}},
	}
}

// BenchmarkAllowSingleKey measures hot-path cost with zero key churn: every
// call hits the same bucket, so it exposes per-call overhead (lock + map
// lookup + counter update) without map-growth noise.
func BenchmarkAllowSingleKey(b *testing.B) {
	now := time.Unix(100, 0)
	for _, f := range factories() {
		b.Run(f.name, func(b *testing.B) {
			l := f.make()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				l.Allow("k", now)
			}
		})
	}
}

// BenchmarkAllowManyKeys measures behavior under key churn: calls cycle
// through a pre-generated pool of random keys, so the map grows and the
// access pattern reflects realistic hash distribution rather than the
// dense/sequential layout of strconv.Itoa(i).
func BenchmarkAllowManyKeys(b *testing.B) {
	now := time.Unix(100, 0)
	keys := randomKeys(keyPoolSize)
	for _, f := range factories() {
		b.Run(f.name, func(b *testing.B) {
			l := f.make()
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				l.Allow(keys[i%len(keys)], now)
			}
		})
	}
}

// BenchmarkAllowParallelSingleKey measures contention: many goroutines
// hammer the same key, exercising the mutex. Worst case for a single
// global lock.
func BenchmarkAllowParallelSingleKey(b *testing.B) {
	now := time.Unix(100, 0)
	for _, f := range factories() {
		b.Run(f.name, func(b *testing.B) {
			l := f.make()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l.Allow("k", now)
				}
			})
		})
	}
}

// BenchmarkAllowParallelManyKeys measures realistic concurrent load: many
// goroutines each cycle through the shared random key pool. Each goroutine
// uses a local counter so there is no synchronization on key selection;
// contention comes only from the limiter itself.
func BenchmarkAllowParallelManyKeys(b *testing.B) {
	now := time.Unix(100, 0)
	keys := randomKeys(keyPoolSize)
	for _, f := range factories() {
		b.Run(f.name, func(b *testing.B) {
			l := f.make()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					l.Allow(keys[i%len(keys)], now)
					i++
				}
			})
		})
	}
}
