package main_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	collectormv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"

	. "otel"
)

// otlpCounter builds a minimal ExportMetricsServiceRequest containing a single
// counter data point. attrs is a list of alternating key, value strings.
func otlpCounter(t *testing.T, name string, value float64, attrs ...string) []byte {
	t.Helper()

	kvs := make([]*commonv1.KeyValue, 0, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		kvs = append(kvs, &commonv1.KeyValue{
			Key:   attrs[i],
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: attrs[i+1]}},
		})
	}

	req := &collectormv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{{
			ScopeMetrics: []*metricsv1.ScopeMetrics{{
				Metrics: []*metricsv1.Metric{{
					Name: name,
					Data: &metricsv1.Metric_Sum{
						Sum: &metricsv1.Sum{
							DataPoints: []*metricsv1.NumberDataPoint{{
								Attributes: kvs,
								Value:      &metricsv1.NumberDataPoint_AsDouble{AsDouble: value},
							}},
						},
					},
				}},
			}},
		}},
	}

	b, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return b
}

// response mirrors the JSON shape returned by QueryHandler.
type response struct {
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
	Value      float64           `json:"value"`
}

func TestIngestHandler_ValidCounter(t *testing.T) {
	store := NewStore()
	body := otlpCounter(t, "http.requests", 42, "method", "GET", "status", "200")

	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	series := store.GetAll()
	if len(series) != 1 {
		t.Fatalf("store has %d series, want 1", len(series))
	}
	for _, v := range series {
		if v != 42 {
			t.Errorf("value = %v, want 42", v)
		}
	}
}

func TestIngestHandler_InvalidBody(t *testing.T) {
	store := NewStore()

	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader([]byte("not protobuf")))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestIngestHandler_MultipleDataPoints(t *testing.T) {
	store := NewStore()

	// Two separate requests, different attribute sets.
	for _, tc := range []struct {
		value float64
		attrs []string
	}{
		{10, []string{"method", "GET"}},
		{5, []string{"method", "POST"}},
	} {
		body := otlpCounter(t, "http.requests", tc.value, tc.attrs...)
		r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/x-protobuf")
		w := httptest.NewRecorder()
		IngestHandler(store).ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	}

	if got := store.GetAll(); len(got) != 2 {
		t.Errorf("store has %d series, want 2", len(got))
	}
}

func TestQueryHandler_Empty(t *testing.T) {
	store := NewStore()

	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	QueryHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got []response
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d items, want 0", len(got))
	}
}

func TestQueryHandler_ReturnsSeries(t *testing.T) {
	store := NewStore()
	store.Set(SeriesKey{Name: "http.requests", Attributes: "method=GET,status=200"}, 42)

	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	QueryHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got []response
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	want := []response{
		{
			Name:       "http.requests",
			Attributes: map[string]string{"method": "GET", "status": "200"},
			Value:      42,
		},
	}
	if diff := cmp.Diff(want, got, cmpopts.SortSlices(func(a, b response) bool {
		return a.Name < b.Name
	})); diff != "" {
		t.Errorf("response mismatch (-want +got):\n%s", diff)
	}
}
