# p-Percentile Window Latency

Different implementations for calculating the p-th percentile latency over a sliding time window.

## Goal

Implementing and benchmarking different algorithms for real-time latency monitoring:

1. **Accuracy**: How close is the result to the true percentile?
2. **Performance**: What is the overhead of `Record()` and `Percentile()` calls?
3. **Memory**: How much space is required to track the window `W`?

## Interface

All implementations satisfy the `WindowPercentile` interface. We also define an `Observation` struct to represent a single data point.

```go
// Observation represents a single latency measurement at a specific time.
type Observation struct {
	Timestamp time.Time
	Latency   time.Duration
}

type WindowPercentile interface {
    // Record adds a new latency observation to the window.
    Record(obs Observation)

    // Percentile returns the p-th percentile (0.0 < p < 1.0) of latencies
    // currently within the window relative to the provided now time.
    // It is expected that now is monotonically increasing across calls.
    Percentile(p float64, now time.Time) time.Duration
}
```

## Candidate Algorithms

- [x] **Naive Sorted Slice**: Copy and sort on every query. Baseline.
- [ ] **Order Statistic Tree**: Logarithmic insertion and percentile lookup.
- [ ] **Fixed-Bin Histogram**: Fast $O(1)$ constant-space approximation.
- [ ] **T-Digest / GK Array**: Advanced streaming approximations.
- [ ] DDSketch: From https://arxiv.org/pdf/1908.10693

## Benchmarking

Run benchmarks to compare implementation performance:

```bash
go test -bench .
```
