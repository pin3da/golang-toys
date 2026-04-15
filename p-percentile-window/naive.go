package latency

import (
	"slices"
	"time"
)

// NaiveWindowPercentile implements WindowPercentile by keeping all observations
// in a slice and sorting them on every Percentile() call.
type NaiveWindowPercentile struct {
	window       time.Duration
	observations []Observation
}

// NewNaiveWindowPercentile creates a new NaiveWindowPercentile with the given window size.
func NewNaiveWindowPercentile(window time.Duration) *NaiveWindowPercentile {
	return &NaiveWindowPercentile{
		window: window,
	}
}

func (nwp *NaiveWindowPercentile) Record(obs Observation) {
	nwp.observations = append(nwp.observations, obs)
}

func (nwp *NaiveWindowPercentile) Percentile(p float64, now time.Time) time.Duration {
	cutoff := now.Add(-nwp.window)

	// Prune expired observations in-place
	nwp.observations = slices.DeleteFunc(nwp.observations, func(o Observation) bool {
		return !o.Timestamp.After(cutoff)
	})

	if len(nwp.observations) == 0 {
		return 0
	}

	// Extract latencies for sorting
	latencies := make([]time.Duration, len(nwp.observations))
	for i, o := range nwp.observations {
		latencies[i] = o.Latency
	}
	slices.Sort(latencies)

	index := int(float64(len(latencies)-1) * p)
	return latencies[index]
}
