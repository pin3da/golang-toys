package latency

import (
	"math/rand"
	"testing"
	"time"
)

func TestWindowPercentile(t *testing.T) {
	tests := []struct {
		name string
		impl WindowPercentile
	}{
		{"Naive", NewNaiveWindowPercentile(100 * time.Millisecond)},
		{"Treap", NewTreapWindowPercentile(100 * time.Millisecond)},
		{"Histogram", NewHistogramWindowPercentile(100*time.Millisecond, time.Millisecond, 100*time.Millisecond)},
	}

	baseTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, l := range latencies {
				tt.impl.Record(Observation{
					Timestamp: baseTime.Add(time.Duration(i) * time.Millisecond),
					Latency:   l,
				})
			}

			now := baseTime.Add(10 * time.Millisecond)

			if got := tt.impl.Percentile(0.5, now); got != 30*time.Millisecond {
				t.Errorf("Percentile(0.5) = %v, want 30ms", got)
			}

			if got := tt.impl.Percentile(0.9, now); got != 40*time.Millisecond {
				t.Errorf("Percentile(0.9) = %v, want 40ms", got)
			}

			expiredNow := now.Add(200 * time.Millisecond)
			if got := tt.impl.Percentile(0.5, expiredNow); got != 0 {
				t.Errorf("Percentile(0.5) after expiry = %v, want 0ms", got)
			}
		})
	}
}

func TestWindowPercentileDuplicates(t *testing.T) {
	tests := []struct {
		name string
		impl WindowPercentile
	}{
		{"Naive", NewNaiveWindowPercentile(100 * time.Millisecond)},
		{"Treap", NewTreapWindowPercentile(100 * time.Millisecond)},
		{"Histogram", NewHistogramWindowPercentile(100*time.Millisecond, time.Millisecond, 100*time.Millisecond)},
	}

	baseTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		20 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, l := range latencies {
				tt.impl.Record(Observation{
					Timestamp: baseTime.Add(time.Duration(i) * time.Millisecond),
					Latency:   l,
				})
			}

			now := baseTime.Add(10 * time.Millisecond)

			if got := tt.impl.Percentile(0.5, now); got != 20*time.Millisecond {
				t.Errorf("Percentile(0.5) = %v, want 20ms", got)
			}

			if got := tt.impl.Percentile(0.9, now); got != 20*time.Millisecond {
				t.Errorf("Percentile(0.9) = %v, want 20ms", got)
			}

			if got := tt.impl.Percentile(0.99, now); got != 20*time.Millisecond {
				t.Errorf("Percentile(0.99) = %v, want 20ms", got)
			}
		})
	}
}

func BenchmarkWindowPercentile(b *testing.B) {
	benchCases := []struct {
		name  string
		total int
	}{
		{"100", 100},
		{"1000", 1000},
		{"10000", 10000},
	}

	implementations := []struct {
		name string
		new  func() WindowPercentile
	}{
		{"Naive", func() WindowPercentile { return NewNaiveWindowPercentile(time.Minute) }},
		{"Treap", func() WindowPercentile { return NewTreapWindowPercentile(time.Minute) }},
		{"Histogram", func() WindowPercentile {
			return NewHistogramWindowPercentile(time.Minute, time.Microsecond, 2*time.Millisecond)
		}},
	}

	percentiles := []float64{0.5, 0.9, 0.95, 0.99, 0.999}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			for _, impl := range implementations {
				b.Run(impl.name, func(b *testing.B) {
					rng := rand.New(rand.NewSource(42))

					type op struct {
						record bool
						p      float64
						obs    Observation
					}
					ops := make([]op, bc.total)
					baseTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
					for i := 0; i < bc.total; i++ {
						if rng.Float64() < 0.7 {
							ops[i] = op{
								record: true,
								obs: Observation{
									Timestamp: baseTime.Add(time.Duration(i) * time.Millisecond),
									Latency:   time.Duration(rng.Intn(1000)) * time.Microsecond,
								},
							}
						} else {
							ops[i] = op{
								record: false,
								p:      percentiles[rng.Intn(len(percentiles))],
							}
						}
					}

					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						wp := impl.new()
						for j := 0; j < bc.total; j++ {
							if ops[j].record {
								wp.Record(ops[j].obs)
							} else {
								wp.Percentile(ops[j].p, ops[j].obs.Timestamp)
							}
						}
					}
				})
			}
		})
	}
}
