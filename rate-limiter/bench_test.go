package ratelimiter_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	ratelimiter "github.com/pin3da/golang-toys/rate-limiter"
	"github.com/pin3da/golang-toys/rate-limiter/fixedwindow"
	"github.com/pin3da/golang-toys/rate-limiter/leakybucket"
	"github.com/pin3da/golang-toys/rate-limiter/slidinglog"
	"github.com/pin3da/golang-toys/rate-limiter/slidingwindow"
	"github.com/pin3da/golang-toys/rate-limiter/tokenbucket"
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
		{"token_bucket", func() ratelimiter.Limiter {
			return tokenbucket.New(1_000_000, 1_000_000)
		}},
		{"leaky_bucket", func() ratelimiter.Limiter {
			return leakybucket.New(1_000_000, 1_000_000)
		}},
	}
}

// scenario describes a benchmark workload. Each scenario runs once per
// limiter factory; sub-benchmarks are named scenario=X/algo=Y so tools
// like benchstat can pivot the results.
type scenario struct {
	name string
	run  func(b *testing.B, l ratelimiter.Limiter)
}

func scenarios() []scenario {
	now := time.Unix(100, 0)
	keys := randomKeys(keyPoolSize)
	return []scenario{
		// single_key: hot path with zero key churn; measures lock + map
		// lookup + counter update.
		{"single_key", func(b *testing.B, l ratelimiter.Limiter) {
			for range b.N {
				l.Allow("k", now)
			}
		}},
		// many_keys: cycles through a random key pool; exposes map-growth
		// and per-key allocation cost.
		{"many_keys", func(b *testing.B, l ratelimiter.Limiter) {
			for i := range b.N {
				l.Allow(keys[i%len(keys)], now)
			}
		}},
		// parallel_single_key: worst case for a single global mutex --
		// every goroutine contends on the same key.
		{"parallel_single_key", func(b *testing.B, l ratelimiter.Limiter) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					l.Allow("k", now)
				}
			})
		}},
		// parallel_many_keys: realistic concurrent load -- many goroutines,
		// many keys, contention comes only from the limiter itself.
		{"parallel_many_keys", func(b *testing.B, l ratelimiter.Limiter) {
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					l.Allow(keys[i%len(keys)], now)
					i++
				}
			})
		}},
	}
}

// BenchmarkAllow runs every scenario against every registered algorithm.
// Sub-benchmark names use key=value labels so benchstat can pivot by
// scenario or algorithm.
func BenchmarkAllow(b *testing.B) {
	for _, s := range scenarios() {
		b.Run("scenario="+s.name, func(b *testing.B) {
			for _, f := range factories() {
				b.Run("algo="+f.name, func(b *testing.B) {
					l := f.make()
					b.ReportAllocs()
					b.ResetTimer()
					s.run(b, l)
				})
			}
		})
	}
}
