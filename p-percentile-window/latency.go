package latency

import "time"

// Observation represents a single latency measurement at a specific time.
type Observation struct {
	Timestamp time.Time
	Latency   time.Duration
}

// WindowPercentile tracks latencies and calculates percentiles over a sliding window.
type WindowPercentile interface {
	// Record adds a new latency observation to the window.
	Record(obs Observation)

	// Percentile returns the p-th percentile (0.0 < p < 1.0) of latencies
	// currently within the window relative to the provided now time.
	// It is expected that `now` is monotonically increasing across calls.
	Percentile(p float64, now time.Time) time.Duration
}
