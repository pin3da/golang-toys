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

// otlpRequest serializes an ExportMetricsServiceRequest containing the given metrics.
func otlpRequest(t *testing.T, metrics ...*metricsv1.Metric) []byte {
	t.Helper()
	req := &collectormv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{{
			ScopeMetrics: []*metricsv1.ScopeMetrics{{
				Metrics: metrics,
			}},
		}},
	}
	b, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return b
}

// buildKVs converts alternating key, value strings to OTLP KeyValue pairs.
func buildKVs(attrs []string) []*commonv1.KeyValue {
	kvs := make([]*commonv1.KeyValue, 0, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		kvs = append(kvs, &commonv1.KeyValue{
			Key:   attrs[i],
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: attrs[i+1]}},
		})
	}
	return kvs
}

// buildCounter builds a Sum metric with an AsDouble data point.
func buildCounter(name string, value float64, attrs ...string) *metricsv1.Metric {
	return &metricsv1.Metric{
		Name: name,
		Data: &metricsv1.Metric_Sum{
			Sum: &metricsv1.Sum{
				DataPoints: []*metricsv1.NumberDataPoint{{
					Attributes: buildKVs(attrs),
					Value:      &metricsv1.NumberDataPoint_AsDouble{AsDouble: value},
				}},
			},
		},
	}
}

// buildCounterInt builds a Sum metric with an AsInt data point.
func buildCounterInt(name string, value int64, attrs ...string) *metricsv1.Metric {
	return &metricsv1.Metric{
		Name: name,
		Data: &metricsv1.Metric_Sum{
			Sum: &metricsv1.Sum{
				DataPoints: []*metricsv1.NumberDataPoint{{
					Attributes: buildKVs(attrs),
					Value:      &metricsv1.NumberDataPoint_AsInt{AsInt: value},
				}},
			},
		},
	}
}

// buildGauge builds a Gauge metric with an AsDouble data point.
func buildGauge(name string, value float64, attrs ...string) *metricsv1.Metric {
	return &metricsv1.Metric{
		Name: name,
		Data: &metricsv1.Metric_Gauge{
			Gauge: &metricsv1.Gauge{
				DataPoints: []*metricsv1.NumberDataPoint{{
					Attributes: buildKVs(attrs),
					Value:      &metricsv1.NumberDataPoint_AsDouble{AsDouble: value},
				}},
			},
		},
	}
}

// otlpCounter builds a minimal ExportMetricsServiceRequest containing a single
// counter data point. attrs is a list of alternating key, value strings.
func otlpCounter(t *testing.T, name string, value float64, attrs ...string) []byte {
	t.Helper()
	return otlpRequest(t, buildCounter(name, value, attrs...))
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

func TestIngestHandler_EmptyBody(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(nil))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	_, total := store.GetWindows(1, time.Now())
	if len(total) != 0 {
		t.Errorf("store has %d series after empty body, want 0", len(total))
	}
}

func TestIngestHandler_MultipleDataPoints(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	for _, tc := range []struct {
		name  string
		value float64
		attrs []string
	}{
		{"method=GET", 10, []string{"method", "GET"}},
		{"method=POST", 5, []string{"method", "POST"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := otlpCounter(t, "http.requests", tc.value, tc.attrs...)
			r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/x-protobuf")
			w := httptest.NewRecorder()
			IngestHandler(store).ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
		})
	}

	_, total := store.GetWindows(1, time.Now())
	if len(total) != 2 {
		t.Errorf("total has %d series, want 2", len(total))
	}
	if v := total[SeriesKey{Name: "http.requests", Attributes: "method=GET"}]; v != 10 {
		t.Errorf("GET value = %v, want 10", v)
	}
	if v := total[SeriesKey{Name: "http.requests", Attributes: "method=POST"}]; v != 5 {
		t.Errorf("POST value = %v, want 5", v)
	}
}

func TestIngestHandler_NonCounterIgnored(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	body := otlpRequest(t, buildGauge("cpu.usage", 0.75, "host", "a"))
	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	_, total := store.GetWindows(1, time.Now())
	if len(total) != 0 {
		t.Errorf("store has %d series after gauge ingest, want 0", len(total))
	}
}

func TestIngestHandler_IntegerDataPoint(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	body := otlpRequest(t, buildCounterInt("http.requests", 99, "method", "GET"))
	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	windows, _ := store.GetWindows(1, time.Now())
	if len(windows) != 1 {
		t.Fatalf("store has %d windows, want 1", len(windows))
	}
	key := SeriesKey{Name: "http.requests", Attributes: "method=GET"}
	if v := windows[0].Series[key]; v != 99 {
		t.Errorf("series value = %v, want 99", v)
	}
}

func TestIngestHandler_MultipleMetricsInOneRequest(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	body := otlpRequest(t,
		buildCounter("http.requests", 10, "method", "GET"),
		buildCounter("db.queries", 5),
	)
	r := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()

	IngestHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
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
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
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

func TestQueryHandler_MultiBucket(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	now := time.Now()
	// Use truncated past minutes so neither write is at the current epoch boundary.
	t1 := now.Truncate(time.Minute).Add(-2 * time.Minute)
	t2 := now.Truncate(time.Minute).Add(-time.Minute)

	key := SeriesKey{Name: "reqs", Attributes: ""}
	store.Set(key, 10, t1)
	store.Set(key, 20, t2)

	// Request more windows than populated so empty windows don't affect the count.
	r := httptest.NewRequest(http.MethodGet, "/metrics?windows=5", nil)
	w := httptest.NewRecorder()
	QueryHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got testWindowedResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(got.Buckets) != 2 {
		t.Fatalf("got %d buckets, want 2", len(got.Buckets))
	}
	// Buckets are returned oldest-first.
	if s := got.Buckets[0].Series; len(s) != 1 || s[0].Value != 10 {
		t.Errorf("bucket[0].Series = %v, want [{value=10}]", s)
	}
	if s := got.Buckets[1].Series; len(s) != 1 || s[0].Value != 20 {
		t.Errorf("bucket[1].Series = %v, want [{value=20}]", s)
	}
	// Total sums values across both buckets.
	if len(got.Total) != 1 || got.Total[0].Value != 30 {
		t.Errorf("total = %v, want [{value=30}]", got.Total)
	}
}

func TestQueryHandler_DefaultReturnsAllWindows(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	now := time.Now()
	t1 := now.Truncate(time.Minute).Add(-2 * time.Minute)
	t2 := now.Truncate(time.Minute).Add(-time.Minute)

	key := SeriesKey{Name: "reqs", Attributes: ""}
	store.Set(key, 10, t1)
	store.Set(key, 20, t2)

	r := httptest.NewRequest(http.MethodGet, "/metrics", nil) // no ?windows param
	w := httptest.NewRecorder()
	QueryHandler(store).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got testWindowedResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(got.Buckets) != 2 {
		t.Errorf("got %d buckets, want 2", len(got.Buckets))
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
