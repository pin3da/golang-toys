package main

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	collectormv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

// IngestHandler handles POST /v1/metrics.
//
// Expects Content-Type: application/x-protobuf with an OTLP
// ExportMetricsServiceRequest body. Extracts all Sum (counter) data points and
// records the latest value per series in store. Non-counter metric types are
// silently ignored.
//
// Returns 200 on success, 400 if the body cannot be decoded.
func IngestHandler(store *WindowedStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}

		var req collectormv1.ExportMetricsServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			http.Error(w, "cannot decode protobuf", http.StatusBadRequest)
			return
		}

		now := time.Now()
		for _, rm := range req.ResourceMetrics {
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					sum, ok := m.Data.(*metricsv1.Metric_Sum)
					if !ok {
						continue
					}
					for _, dp := range sum.Sum.DataPoints {
						key := SeriesKey{
							Name:       m.Name,
							Attributes: fingerprintAttrs(dp.Attributes),
						}
						store.Set(key, dataPointValue(dp), now)
					}
				}
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

// windowedQueryResponse is the JSON shape returned by QueryHandler.
type windowedQueryResponse struct {
	Buckets []bucketResponse `json:"buckets"`
	Total   []seriesResponse `json:"total"`
}

// bucketResponse is one time window within the windowed query response.
type bucketResponse struct {
	Start  time.Time        `json:"start"`
	Series []seriesResponse `json:"series"`
}

// seriesResponse holds a single metric series name, attributes, and value.
type seriesResponse struct {
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
	Value      float64           `json:"value"`
}

// QueryHandler handles GET /metrics.
//
// The optional ?windows=N query parameter controls how many recent time windows
// to include (must be a positive integer). When omitted, all stored windows are
// returned. The response includes per-bucket breakdowns and a rolled-up total.
//
// Returns 200 with JSON on success, 400 if ?windows is present but invalid.
func QueryHandler(store *WindowedStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := maxWindows
		if s := r.URL.Query().Get("windows"); s != "" {
			v, err := strconv.Atoi(s)
			if err != nil || v <= 0 {
				http.Error(w, "windows must be a positive integer", http.StatusBadRequest)
				return
			}
			n = v
		}

		wins, total := store.GetWindows(n, time.Now())

		resp := windowedQueryResponse{
			Buckets: make([]bucketResponse, 0, len(wins)),
			Total:   seriesToResponse(total),
		}
		for _, win := range wins {
			resp.Buckets = append(resp.Buckets, bucketResponse{
				Start:  win.Start,
				Series: seriesToResponse(win.Series),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// seriesToResponse converts a series map to a stable, sorted slice of
// seriesResponse for deterministic JSON output.
func seriesToResponse(series map[SeriesKey]float64) []seriesResponse {
	result := make([]seriesResponse, 0, len(series))
	for key, value := range series {
		result = append(result, seriesResponse{
			Name:       key.Name,
			Attributes: parseFingerprint(key.Attributes),
			Value:      value,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Attributes[""] < result[j].Attributes[""]
	})
	return result
}

// fingerprintAttrs converts a slice of OTLP KeyValue pairs into a stable,
// sorted "k=v,k=v" string suitable for use as a map key.
func fingerprintAttrs(kvs []*commonv1.KeyValue) string {
	pairs := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		pairs = append(pairs, kv.Key+"="+kv.Value.GetStringValue())
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// parseFingerprint is the inverse of fingerprintAttrs. It splits a "k=v,k=v"
// fingerprint back into a map. An empty fingerprint returns an empty map.
func parseFingerprint(fp string) map[string]string {
	attrs := make(map[string]string)
	if fp == "" {
		return attrs
	}
	for pair := range strings.SplitSeq(fp, ",") {
		k, v, _ := strings.Cut(pair, "=")
		attrs[k] = v
	}
	return attrs
}

// dataPointValue extracts the numeric value from a NumberDataPoint,
// normalising AsInt to float64.
func dataPointValue(dp *metricsv1.NumberDataPoint) float64 {
	switch v := dp.Value.(type) {
	case *metricsv1.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricsv1.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	default:
		return 0
	}
}
