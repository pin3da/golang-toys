package latency

import (
	"time"
)

// bin tracks observations within a fixed latency range.
// It uses a queue of timestamps to support sliding window eviction.
type bin struct {
	lower      time.Duration // inclusive
	upper      time.Duration // exclusive
	timestamps []time.Time   // queue of observation timestamps
}

// count returns the number of non-expired observations in this bin.
func (b *bin) count(cutoff time.Time) int {
	// Find first non-expired timestamp using binary search
	idx := 0
	for idx < len(b.timestamps) && !b.timestamps[idx].After(cutoff) {
		idx++
	}
	// Remove expired timestamps from front
	b.timestamps = b.timestamps[idx:]
	return len(b.timestamps)
}

// add appends a timestamp to the bin.
func (b *bin) add(ts time.Time) {
	b.timestamps = append(b.timestamps, ts)
}

// HistogramWindowPercentile implements WindowPercentile using a fixed-bin histogram.
// Latency values are partitioned into fixed-width bins. Each bin maintains a queue
// of timestamps for sliding window eviction.
type HistogramWindowPercentile struct {
	window     time.Duration
	bins       []bin
	binWidth   time.Duration
	maxLatency time.Duration
}

// NewHistogramWindowPercentile creates a new HistogramWindowPercentile.
// The binWidth determines the granularity of the histogram.
// The maxLatency determines the upper bound of the histogram range.
// Observations with latency >= maxLatency are clamped to the last bin.
func NewHistogramWindowPercentile(window, binWidth, maxLatency time.Duration) *HistogramWindowPercentile {
	numBins := int(maxLatency / binWidth)
	if maxLatency%binWidth != 0 {
		numBins++
	}

	bins := make([]bin, numBins)
	for i := range bins {
		bins[i] = bin{
			lower: time.Duration(i) * binWidth,
			upper: time.Duration(i+1) * binWidth,
		}
	}

	return &HistogramWindowPercentile{
		window:     window,
		bins:       bins,
		binWidth:   binWidth,
		maxLatency: maxLatency,
	}
}

func (hwp *HistogramWindowPercentile) Record(obs Observation) {
	idx := int(obs.Latency / hwp.binWidth)
	if idx >= len(hwp.bins) {
		idx = len(hwp.bins) - 1
	}
	hwp.bins[idx].add(obs.Timestamp)
}

func (hwp *HistogramWindowPercentile) Percentile(p float64, now time.Time) time.Duration {
	cutoff := now.Add(-hwp.window)

	// Count total non-expired observations and evict expired ones
	total := 0
	for i := range hwp.bins {
		total += hwp.bins[i].count(cutoff)
	}

	if total == 0 {
		return 0
	}

	// Find the bin containing the p-th percentile
	targetRank := int(float64(total-1) * p)
	accumulated := 0

	for i := range hwp.bins {
		count := len(hwp.bins[i].timestamps)
		if accumulated+count > targetRank {
			// The percentile falls within this bin
			// Return the lower bound as an approximation
			return hwp.bins[i].lower
		}
		accumulated += count
	}

	// Should not reach here, but return upper bound of last bin as fallback
	return hwp.bins[len(hwp.bins)-1].lower
}
