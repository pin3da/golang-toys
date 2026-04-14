# micro-otel

`micro-otel` is a tiny OTLP/HTTP metrics receiver for learning and experiments.
It accepts OpenTelemetry protobuf payloads, keeps recent values in memory using
a fixed-size ring buffer, and exposes a simple JSON query endpoint.

## What this project is for

- Validate OTLP metric ingestion locally without running a full collector stack.
- Inspect recent metric windows and per-series totals quickly.
- Experiment with time-windowed aggregation behavior in a small codebase.

It is intentionally minimal and optimized for clarity, not production usage.

## How it works

The server divides time into fixed-width windows (`-window`, default `1m`). Each
window maps to one slot in a ring buffer of `-buckets` slots (default `10`).

When the current window advances:

1. Slots that fall outside the retention horizon are evicted.
2. New writes go to the slot for the current epoch.
3. Queries read up to the most recent N windows and compute a cross-window total.

This guarantees bounded memory: at most `buckets` windows are retained.

## Quick start

### 1) Run the server

```bash
go run . -window 5s -buckets 10
```

Server listens on `:4318`.

### 2) Run the example client (second terminal)

```bash
go run ./cmd/client -sleep 6s
```

The client sends three rounds of cumulative counter values, sleeping between
rounds so each round lands in a different window.

### 3) Query the stored windows

```bash
curl "http://localhost:4318/metrics?windows=3"
```

## API

### `POST /v1/metrics`

Ingests OTLP protobuf metrics.

- `Content-Type`: `application/x-protobuf`
- Body: `ExportMetricsServiceRequest`
- Status:
  - `200 OK` on success
  - `400 Bad Request` if the body cannot be read or decoded

Notes:

- Only `Metric_Sum` datapoints are ingested.
- Non-sum metric types are ignored.
- Values are stored as the latest datapoint value per `(metric name + attributes)`
  within each window.

### `GET /metrics`

Returns JSON with per-window series and a rolled-up total.

Query parameter:

- `windows` (optional): positive integer limiting how many recent windows are
  returned. If omitted, all retained windows are considered.

Status:

- `200 OK` on success
- `400 Bad Request` if `windows` is not a positive integer

Response shape:

```json
{
  "buckets": [
    {
      "start": "2026-04-14T10:00:00Z",
      "series": [
        {
          "name": "http.server.request.count",
          "attributes": {
            "http.method": "GET",
            "http.status_code": "200"
          },
          "value": 1024
        }
      ]
    }
  ],
  "total": [
    {
      "name": "http.server.request.count",
      "attributes": {
        "http.method": "GET",
        "http.status_code": "200"
      },
      "value": 1024
    }
  ]
}
```

Semantics:

- `buckets`: oldest-first, non-empty windows only.
- `total`: sum per series across all returned buckets.

## Configuration

### Server flags

| Flag | Default | Description |
| --- | --- | --- |
| `-window` | `1m` | Duration of each time window |
| `-buckets` | `10` | Number of windows retained in memory |

### Client flags

| Flag | Default | Description |
| --- | --- | --- |
| `-sleep` | `5s` | Delay between rounds; set greater than server `-window` to force separate windows |

## Development

Run tests:

```bash
go test ./...
```

Build binaries:

```bash
go build -o micro-otel .
go build -o client ./cmd/client
```

## Current limitations

- In-memory only (no persistence across restarts).
- Single process, no distributed coordination.
- Focused on basic counter-style `Sum` ingestion.
- No authentication, authorization, or rate limiting.

## Repository layout

- `main.go`: server entrypoint and HTTP route wiring
- `handler.go`: OTLP ingest + JSON query handlers
- `store.go`: windowed ring-buffer storage implementation
- `cmd/client/main.go`: example OTLP producer and query printer
