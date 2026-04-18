# rate-limiter

A playground for rate limiting algorithms in Go. Each algorithm is implemented
behind a common `Limiter` interface so they can be swapped, tested, and
benchmarked against one another for time and memory efficiency.

## Common interface

```go
type Limiter interface {
    Allow(key string, now time.Time) bool
    Name() string
}
```

- `key` identifies the caller / bucket (user ID, IP, API key, ...).
- `now` is injected rather than read from the clock to keep implementations
  deterministic and easy to benchmark.
- Implementations must be safe for concurrent use.

## Algorithms

Each lives in its own package with an identical constructor shape
(`New(...) *Limiter`) and the `Limiter` interface.

| Package         | Algorithm              | Idea                                                                                      | Memory / key |
| --------------- | ---------------------- | ----------------------------------------------------------------------------------------- | ------------ |
| `tokenbucket`   | Token bucket           | Bucket of size `capacity` refills at `rate` tokens/sec; each event consumes one token.     | O(1)         |
| `leakybucket`   | Leaky bucket           | Queue of size `capacity` drains at a constant `leakRate`; overflow is rejected.            | O(1)         |
| `fixedwindow`   | Fixed window counter   | Counter per `window`-sized bucket; resets at boundaries. Cheap, allows boundary bursts.    | O(1)         |
| `slidinglog`    | Sliding window log     | Store timestamps of recent events; evict older than `window`. Exact but costly.            | O(events)    |
| `slidingwindow` | Sliding window counter | Weighted blend of current + previous fixed-window counters. Bounded memory, good accuracy. | O(1)         |

## Layout

```
rate-limiter/
  limiter.go          -- Limiter interface
  limiter_test.go     -- shared interface smoke test
  tokenbucket/
  leakybucket/
  fixedwindow/
  slidinglog/
  slidingwindow/
```

## Running

```sh
go test ./...
go test -bench=. -benchmem ./...
```

## Planned next steps

1. Flesh out each `Allow` implementation (one package at a time).
2. Add per-package behavior tests (burst, steady-state, idle-then-burst).
3. Add a shared benchmark harness in `bench/` that runs identical workloads
   against every `Limiter` and reports ops/sec and bytes/key.
4. Add a concurrent stress test to validate thread-safety claims.
