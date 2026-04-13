package main

import (
	"math"
	"sync"
	"time"
)

// SeriesKey uniquely identifies a metric time series.
// Attributes is a sorted "k=v,k=v" fingerprint derived from the OTLP attribute map.
type SeriesKey struct {
	Name       string
	Attributes string
}

// bucket is a single slot in the WindowedStore ring buffer.
type bucket struct {
	// epoch is the time-window index that last wrote to this slot.
	// math.MinInt64 means the slot has never been written.
	epoch  int64
	series map[SeriesKey]float64
}

// WindowResult holds the counter values recorded during a single time window.
type WindowResult struct {
	// Start is the wall-clock time at the beginning of this window.
	Start time.Time
	// Series holds the latest counter value per series recorded in this window.
	Series map[SeriesKey]float64
}

// WindowedStore holds per-series counter values across a fixed number of
// non-overlapping, equal-width time windows arranged in a ring buffer.
// The buffer will hold information for the last numBuckets*windowDuration, based
// on the more recently written data.
//
// Time is divided into epochs of length windowDuration. Epoch index E covers the
// half-open interval [E*windowDuration, (E+1)*windowDuration). Each epoch maps to a
// ring slot: slot = E % numBuckets.
//
// The current open window (whose epoch equals now's epoch) is included in
// query results as-is; it may contain partial data for the period so far.
//
// Safe for concurrent use by multiple goroutines.
type WindowedStore struct {
	mu             sync.RWMutex
	buckets        []bucket // ring buffer of length numBuckets
	windowDuration time.Duration
	numBuckets     int
	lastEpoch      int64 // epoch index of the most recently written bucket
}

// NewWindowedStore returns an empty WindowedStore with numBuckets ring-buffer
// slots, each spanning windowDuration. Panics if numBuckets < 1 or windowDuration <= 0.
func NewWindowedStore(numBuckets int, windowDuration time.Duration) *WindowedStore {
	if numBuckets < 1 {
		panic("otel: numBuckets must be >= 1")
	}
	if windowDuration <= 0 {
		panic("otel: windowDuration must be > 0")
	}
	buckets := make([]bucket, numBuckets)
	for i := range buckets {
		buckets[i].epoch = math.MinInt64
	}
	return &WindowedStore{
		buckets:        buckets,
		windowDuration: windowDuration,
		numBuckets:     numBuckets,
		lastEpoch:      math.MinInt64,
	}
}

// epoch returns the window index for a given point in time.
func (s *WindowedStore) epoch(t time.Time) int64 {
	return t.UnixNano() / int64(s.windowDuration)
}

// epochStart returns the wall-clock time at the beginning of the given epoch.
func (s *WindowedStore) epochStart(epoch int64) time.Time {
	return time.Unix(0, epoch*int64(s.windowDuration))
}

// slotFor returns the ring-buffer index for the given epoch.
func (s *WindowedStore) slotFor(epoch int64) int {
	if epoch < 0 {
		panic("epoch can't be negavtive")
	}
	return int(epoch % int64(s.numBuckets))
}

// Set records the latest value for the given series in the time window that
// contains `now`.
//
// Storing data might evict data for older time windows if they are too old
// for the WindowedStore. That's data that was recorded more than
// windowDuration*numBuckets from `now`.
//
// Safe for concurrent use by multiple goroutines.
func (s *WindowedStore) Set(key SeriesKey, value float64, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentEpoch := s.epoch(now)

	// Evict stale ring slots when the epoch has advanced.
	if s.lastEpoch != math.MinInt64 && currentEpoch > s.lastEpoch {
		numToEvict := min(currentEpoch-s.lastEpoch, int64(s.numBuckets))
		for i := int64(1); i <= numToEvict; i++ {
			slot := s.slotFor(s.lastEpoch + i)
			s.buckets[slot].epoch = math.MinInt64
			s.buckets[slot].series = nil
		}
	}
	if currentEpoch > s.lastEpoch {
		s.lastEpoch = currentEpoch
	}

	slot := s.slotFor(currentEpoch)
	b := &s.buckets[slot]
	if b.epoch != currentEpoch {
		b.epoch = currentEpoch
		b.series = make(map[SeriesKey]float64)
	}
	b.series[key] = value
}

// GetWindows returns the last (at most) n non-empty time windows, oldest first,
// and an aggregated total that sums each series's value across all returned windows.
//
// The current open window (epoch == now's epoch) is included as the most recent
// entry. Empty windows are omitted.
//
// Safe for concurrent use by multiple goroutines.
func (s *WindowedStore) GetWindows(n int, now time.Time) (windows []WindowResult, total map[SeriesKey]float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total = make(map[SeriesKey]float64)
	currentEpoch := s.epoch(now)

	if n > s.numBuckets {
		n = s.numBuckets
	}

	// Iterate oldest-first: epoch (currentEpoch - n + 1) up to currentEpoch.
	for i := n - 1; i >= 0; i-- {
		e := currentEpoch - int64(i)
		b := &s.buckets[s.slotFor(e)]
		is_stale_data := b.epoch != e
		if is_stale_data || len(b.series) == 0 {
			continue
		}
		win := WindowResult{
			Start:  s.epochStart(e),
			Series: make(map[SeriesKey]float64, len(b.series)),
		}
		for k, v := range b.series {
			win.Series[k] = v
			total[k] += v
		}
		windows = append(windows, win)
	}

	return windows, total
}

// maxInt is exposed so callers can request "all stored windows".
const maxWindows = math.MaxInt
