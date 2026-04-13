# micro-otel

A minimal OpenTelemetry metrics backend that accepts OTLP/HTTP protobuf payloads
and aggregates counter values in memory.

## Endpoints

| Method | Path        | Description                                  |
| ------ | ----------- | -------------------------------------------- |
| POST   | /v1/metrics | Ingest OTLP metrics (application/x-protobuf) |
| GET    | /metrics    | Query all aggregated series as JSON          |

## Run

In one terminal, start the server:

```
go run .
```

In a second terminal, run the example client:

```
go run ./cmd/client/main.go
```

The client sends a batch of counter data points to `POST /v1/metrics`, then
fetches and prints the aggregated series from `GET /metrics`.

---

## Plan

### Main exercise: OTLP counter backend

**Milestone 1 — Skeleton**
Define all public types, method signatures, and route wiring. Method bodies
panic with "not implemented". One test covers the `Store` contract.

**Milestone 2 — Ingest**
`POST /v1/metrics` parses the protobuf body, extracts `Sum` data points
(counters), and records the latest value per series in the `Store`.

A series is uniquely identified by `(metric_name, attributes)`. Attributes are
fingerprinted as a sorted `key=value,...` string so they can serve as a map key.

**Milestone 3 — Query**
`GET /metrics` returns all series as JSON:

```json
[
  {
    "name": "http.server.request.duration",
    "attributes": { "http.method": "GET", "http.status_code": "200" },
    "value": 10482
  }
]
```

**Milestone 4 — Wire up + example client**
`cmd/client/main.go` generates sample metrics, posts them as an
`ExportMetricsServiceRequest` to the running server, then hits `GET /metrics`
and prints the result.

---

### Sub-exercise: time-windowed ring buffer

Replace the flat `Store` with per-minute (configurable) buckets. Goals:

- **Ring buffer**: fixed-size array of `N` buckets; new arrivals write to the
  current bucket without allocating.
- **Bucket rotation**: on each ingest, compute the current bucket index from
  `now / windowSize`. If the index has advanced since the last write, zero out
  stale buckets before writing.
- **Partial-window handling**: the current (open) bucket is incomplete. Decide
  whether to expose it as-is or exclude it from aggregated queries.
- **Query**: `GET /metrics?windows=5` returns per-bucket values for the last 5
  windows plus a rolled-up total.
