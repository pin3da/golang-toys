package main_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// testSeriesResponse mirrors the JSON shape of a single series entry.
type testSeriesResponse struct {
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
	Value      float64           `json:"value"`
}

// testWindowedResponse mirrors the JSON shape returned by QueryHandler.
type testWindowedResponse struct {
	Buckets []testBucketResponse `json:"buckets"`
	Total   []testSeriesResponse `json:"total"`
}

// testBucketResponse mirrors a single bucket in the windowed response.
type testBucketResponse struct {
	Start  time.Time            `json:"start"`
	Series []testSeriesResponse `json:"series"`
}

func TestIngestHandler_ValidCounter(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	body := otlpCounter(t, "http.requests", 42, "method", "GET", "status", "200")

	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// The write lands in the current open window; one window is returned.
	windows, _ := store.GetWindows(1, time.Now())
	if len(windows) != 1 {
		t.Fatalf("store has %d windows, want 1", len(windows))
	}
	key := SeriesKey{Name: "http.requests", Attributes: "method=GET,status=200"}
	if v := windows[0].Series[key]; v != 42 {
		t.Errorf("series value = %v, want 42", v)
	}
}

func TestIngestHandler_InvalidBody(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader([]byte("not protobuf")))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestIngestHandler_MultipleDataPoints(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

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

	_, total := store.GetWindows(1, time.Now())
	if len(total) != 2 {
		t.Errorf("total has %d series, want 2", len(total))
	}
}

func TestQueryHandler_Empty(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	QueryHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got testWindowedResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Buckets) != 0 {
		t.Errorf("got %d buckets, want 0", len(got.Buckets))
	}
	if len(got.Total) != 0 {
		t.Errorf("got %d total entries, want 0", len(got.Total))
	}
}

func TestQueryHandler_ReturnsSeries(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	store.Set(SeriesKey{Name: "http.requests", Attributes: "method=GET,status=200"}, 42, time.Now())

	r := httptest.NewRequest(http.MethodGet, "/metrics?windows=1", nil)
	w := httptest.NewRecorder()

	QueryHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got testWindowedResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantTotal := []testSeriesResponse{
		{
			Name:       "http.requests",
			Attributes: map[string]string{"method": "GET", "status": "200"},
			Value:      42,
		},
	}
	if diff := cmp.Diff(wantTotal, got.Total, cmpopts.SortSlices(func(a, b testSeriesResponse) bool {
		return a.Name < b.Name
	})); diff != "" {
		t.Errorf("total mismatch (-want +got):\n%s", diff)
	}
}

func TestQueryHandler_InvalidWindowsParam(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	for _, bad := range []string{"abc", "0", "-1"} {
		r := httptest.NewRequest(http.MethodGet, "/metrics?windows="+bad, nil)
		w := httptest.NewRecorder()
		QueryHandler(store).ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("?windows=%s: status = %d, want 400", bad, w.Code)
		}
	}
}
