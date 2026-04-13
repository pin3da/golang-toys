package main

import (
	"maps"
	"sync"
)

// SeriesKey uniquely identifies a metric time series.
// Attributes is a sorted "k=v,k=v" fingerprint derived from the OTLP attribute map.
type SeriesKey struct {
	Name       string
	Attributes string
}

// Store holds the latest counter value for each series.
// Safe for concurrent use by multiple goroutines.
type Store struct {
	mu     sync.RWMutex
	series map[SeriesKey]float64
}

// NewStore returns an empty Store ready for use.
func NewStore() *Store {
	return &Store{series: make(map[SeriesKey]float64)}
}

// Set records the latest value for the given series, overwriting any previous value.
func (s *Store) Set(key SeriesKey, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.series[key] = value
}

// GetAll returns a snapshot of all series and their latest values.
// The returned map is a copy; callers may read it freely without holding any lock.
func (s *Store) GetAll() map[SeriesKey]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := make(map[SeriesKey]float64, len(s.series))
	maps.Copy(snapshot, s.series)
	return snapshot
}
