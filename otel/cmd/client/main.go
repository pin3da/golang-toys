// client is an example OTLP metrics producer. It sends several rounds of
// counter data to the micro-otel server, sleeping between rounds so that data
// lands in different time windows when the server is started with a short
// -window flag (e.g. -window 5s). After all rounds it queries GET /metrics and
// prints the per-window breakdown and the rolled-up total.
//
// Run the server with a matching window to see multiple buckets:
//
//	./micro-otel -window 5s
//	./client -sleep 6s
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	collectormv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

// metricsResponse mirrors the JSON shape returned by GET /metrics.
type metricsResponse struct {
	Buckets []bucketResponse `json:"buckets"`
	Total   []seriesResponse `json:"total"`
}

// bucketResponse is one time window within the response.
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

const serverAddr = "http://localhost:4318"

// roundData defines the counter values sent in a single round.
type roundData struct {
	label  string
	points []point
}

type point struct {
	method string
	status string
	value  float64
}

// rounds is the sequence of traffic snapshots the client sends.
// Values represent cumulative request counts at that moment in time.
var rounds = []roundData{
	{
		label: "quiet period",
		points: []point{
			{"GET", "200", 512},
			{"POST", "200", 84},
			{"GET", "500", 3},
		},
	},
	{
		label: "midday peak",
		points: []point{
			{"GET", "200", 4800},
			{"POST", "200", 730},
			{"GET", "500", 17},
			{"POST", "500", 5},
		},
	},
	{
		label: "afternoon traffic",
		points: []point{
			{"GET", "200", 11200},
			{"POST", "200", 1650},
			{"GET", "500", 24},
			{"POST", "500", 9},
			{"DELETE", "204", 41},
		},
	},
}

func main() {
	sleep := flag.Duration("sleep", 5*time.Second, "time to sleep between rounds (should exceed the server -window)")
	flag.Parse()

	for i, r := range rounds {
		fmt.Printf("--- round %d/%d: %s ---\n", i+1, len(rounds), r.label)
		req := buildRequest(r)
		printRequest(req)
		if err := send(req); err != nil {
			log.Fatalf("send: %v", err)
		}
		if i < len(rounds)-1 {
			fmt.Printf("sleeping %s ...\n\n", *sleep)
			time.Sleep(*sleep)
		}
	}

	fmt.Println()
	if err := query(len(rounds)); err != nil {
		log.Fatalf("query: %v", err)
	}
}

// buildRequest constructs an ExportMetricsServiceRequest from a roundData snapshot.
func buildRequest(r roundData) *collectormv1.ExportMetricsServiceRequest {
	dps := make([]*metricsv1.NumberDataPoint, 0, len(r.points))
	for _, p := range r.points {
		dps = append(dps, &metricsv1.NumberDataPoint{
			Attributes: []*commonv1.KeyValue{
				strAttr("http.method", p.method),
				strAttr("http.status_code", p.status),
			},
			Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: p.value},
		})
	}

	return &collectormv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{{
			ScopeMetrics: []*metricsv1.ScopeMetrics{{
				Metrics: []*metricsv1.Metric{{
					Name: "http.server.request.count",
					Data: &metricsv1.Metric_Sum{
						Sum: &metricsv1.Sum{
							IsMonotonic:            true,
							AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
							DataPoints:             dps,
						},
					},
				}},
			}},
		}},
	}
}

// printRequest logs the data points about to be sent so callers can visually
// confirm what each round contributes.
func printRequest(req *collectormv1.ExportMetricsServiceRequest) {
	type entry struct{ label string; value float64 }
	var entries []entry

	for _, rm := range req.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				sum, ok := m.Data.(*metricsv1.Metric_Sum)
				if !ok {
					continue
				}
				for _, dp := range sum.Sum.DataPoints {
					attrs := attrsToMap(dp.Attributes)
					label := fmt.Sprintf("  %-30s %v", m.Name, fmtAttrs(attrs))
					entries = append(entries, entry{label, dpValue(dp)})
				}
			}
		}
	}

	for _, e := range entries {
		fmt.Printf("%s = %g\n", e.label, e.value)
	}
}

// send marshals req as protobuf and posts it to POST /v1/metrics.
func send(req *collectormv1.ExportMetricsServiceRequest) error {
	b, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	resp, err := http.Post(serverAddr+"/v1/metrics", "application/x-protobuf", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
	}

	fmt.Println("ingest: ok")
	return nil
}

// query fetches GET /metrics and prints per-window buckets and the rolled-up total.
func query(n int) error {
	url := fmt.Sprintf("%s/metrics?windows=%d", serverAddr, n)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	var result metricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	fmt.Printf("=== query results (%d window(s) returned) ===\n", len(result.Buckets))
	for i, b := range result.Buckets {
		fmt.Printf("\n[window %d] start=%s\n", i+1, b.Start.Format(time.RFC3339))
		for _, s := range b.Series {
			fmt.Printf("  %-30s %s = %g\n", s.Name, fmtAttrs(s.Attributes), s.Value)
		}
	}

	fmt.Println("\n--- total (sum across all windows) ---")
	for _, s := range result.Total {
		fmt.Printf("  %-30s %s = %g\n", s.Name, fmtAttrs(s.Attributes), s.Value)
	}
	return nil
}

// fmtAttrs formats an attribute map as a stable key=value string.
func fmtAttrs(attrs map[string]string) string {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(attrs[k])
	}
	b.WriteByte('}')
	return b.String()
}

// attrsToMap converts a slice of OTLP KeyValue pairs to a plain map.
func attrsToMap(kvs []*commonv1.KeyValue) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		m[kv.Key] = kv.Value.GetStringValue()
	}
	return m
}

// dpValue extracts the numeric value from a NumberDataPoint.
func dpValue(dp *metricsv1.NumberDataPoint) float64 {
	switch v := dp.Value.(type) {
	case *metricsv1.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricsv1.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	default:
		return 0
	}
}

// strAttr is a convenience constructor for a string-valued OTLP KeyValue.
func strAttr(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   key,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}},
	}
}
