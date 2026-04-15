package latency

import (
	"testing"
	"time"
)

func TestNaiveWindowPercentile(t *testing.T) {
	nw := NewNaiveWindowPercentile(100 * time.Millisecond)
	baseTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	for i, l := range latencies {
		nw.Record(Observation{
			Timestamp: baseTime.Add(time.Duration(i) * time.Millisecond),
			Latency:   l,
		})
	}

	now := baseTime.Add(10 * time.Millisecond)

	if got := nw.Percentile(0.5, now); got != 30*time.Millisecond {
		t.Errorf("expected 30ms, got %v", got)
	}

	if got := nw.Percentile(0.9, now); got != 40*time.Millisecond {
		t.Errorf("expected 40ms, got %v", got)
	}

	expiredNow := now.Add(200 * time.Millisecond)
	if got := nw.Percentile(0.5, expiredNow); got != 0 {
		t.Errorf("expected 0ms after expiration, got %v", got)
	}
}

func BenchmarkWindowPercentile(b *testing.B) {
	// Placeholder for benchmarking implementations
}
