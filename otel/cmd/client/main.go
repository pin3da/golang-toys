// client is an example OTLP metrics producer. It generates a small set of
// counter data points, posts them to the micro-otel server at localhost:4318,
// and prints the aggregated series returned by GET /metrics.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	collectormv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

const serverAddr = "http://localhost:4318"

func main() {
	if err := send(buildRequest()); err != nil {
		log.Fatalf("send: %v", err)
	}
	if err := query(); err != nil {
		log.Fatalf("query: %v", err)
	}
}

// buildRequest constructs a sample ExportMetricsServiceRequest with two
// http.server.request.count series differing only by HTTP method.
func buildRequest() *collectormv1.ExportMetricsServiceRequest {
	counter := func(method, status string, value float64) *metricsv1.NumberDataPoint {
		return &metricsv1.NumberDataPoint{
			Attributes: []*commonv1.KeyValue{
				strAttr("http.method", method),
				strAttr("http.status_code", status),
			},
			Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: value},
		}
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
							DataPoints: []*metricsv1.NumberDataPoint{
								counter("GET", "200", 1024),
								counter("POST", "200", 312),
								counter("GET", "500", 7),
							},
						},
					},
				}},
			}},
		}},
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

	log.Println("ingest: ok")
	return nil
}

// query fetches GET /metrics and pretty-prints the JSON response.
func query() error {
	resp, err := http.Get(serverAddr + "/metrics")
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	var series []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&series); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	out, err := json.MarshalIndent(series, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	fmt.Println(string(out))
	return nil
}

// strAttr is a convenience constructor for a string-valued OTLP KeyValue.
func strAttr(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   key,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}},
	}
}
