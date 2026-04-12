package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer starts a real HTTP test server backed by a temporary data dir.
// It is torn down automatically when t ends.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	ts := httptest.NewServer(NewServer(m))
	t.Cleanup(ts.Close)
	return ts
}

// get issues a GET request and returns the response.
func get(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// putJSON issues a PUT request with a JSON body and returns the response.
func putJSON(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, ts.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	return resp
}

// postEmpty issues a POST request with no body and returns the response.
func postEmpty(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Post(ts.URL+path, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// deleteReq issues a DELETE request and returns the response.
func deleteReq(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

// decodeJSON decodes the response body into v.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// --- health ---

func TestServer_Health(t *testing.T) {
	ts := newTestServer(t)
	resp := get(t, ts, "/health")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
}

// --- database management ---

func TestServer_CreateAndListDB(t *testing.T) {
	ts := newTestServer(t)

	resp := postEmpty(t, ts, "/databases/mydb")
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST /databases/mydb = %d, want 201", resp.StatusCode)
	}
	var created map[string]string
	decodeJSON(t, resp, &created)
	if created["name"] != "mydb" {
		t.Errorf(`body["name"] = %q, want "mydb"`, created["name"])
	}

	resp = get(t, ts, "/databases")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /databases = %d, want 200", resp.StatusCode)
	}
	var listed map[string][]string
	decodeJSON(t, resp, &listed)
	if len(listed["databases"]) != 1 || listed["databases"][0] != "mydb" {
		t.Errorf(`body["databases"] = %v, want ["mydb"]`, listed["databases"])
	}
}

func TestServer_CreateDBDuplicate(t *testing.T) {
	ts := newTestServer(t)
	postEmpty(t, ts, "/databases/mydb")
	resp := postEmpty(t, ts, "/databases/mydb")
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate POST = %d, want 409", resp.StatusCode)
	}
}

func TestServer_DeleteDB(t *testing.T) {
	ts := newTestServer(t)
	postEmpty(t, ts, "/databases/mydb")

	resp := deleteReq(t, ts, "/databases/mydb")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("DELETE /databases/mydb = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["name"] != "mydb" {
		t.Errorf(`body["name"] = %q, want "mydb"`, body["name"])
	}
}

func TestServer_DeleteDBNotFound(t *testing.T) {
	ts := newTestServer(t)
	resp := deleteReq(t, ts, "/databases/missing")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE /databases/missing = %d, want 404", resp.StatusCode)
	}
}

func TestServer_GetDBStats(t *testing.T) {
	ts := newTestServer(t)
	postEmpty(t, ts, "/databases/mydb")

	resp := get(t, ts, "/databases/mydb/stats")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /databases/mydb/stats = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	if body["name"] != "mydb" {
		t.Errorf(`body["name"] = %q, want "mydb"`, body["name"])
	}
	if _, ok := body["sstable_count"]; !ok {
		t.Error(`body missing "sstable_count"`)
	}
}

func TestServer_GetDBStatsNotFound(t *testing.T) {
	ts := newTestServer(t)
	resp := get(t, ts, "/databases/missing/stats")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /databases/missing/stats = %d, want 404", resp.StatusCode)
	}
}

// --- key-value ---

func TestServer_PutAndGetKey(t *testing.T) {
	ts := newTestServer(t)
	postEmpty(t, ts, "/databases/mydb")

	resp := putJSON(t, ts, "/databases/mydb/keys/greeting", map[string]string{"value": "hello"})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("PUT key = %d, want 200", resp.StatusCode)
	}
	var put map[string]string
	decodeJSON(t, resp, &put)
	if put["key"] != "greeting" || put["value"] != "hello" {
		t.Errorf("PUT response = %v, want {key:greeting value:hello}", put)
	}

	resp = get(t, ts, "/databases/mydb/keys/greeting")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET key = %d, want 200", resp.StatusCode)
	}
	var got map[string]string
	decodeJSON(t, resp, &got)
	if got["key"] != "greeting" || got["value"] != "hello" {
		t.Errorf("GET response = %v, want {key:greeting value:hello}", got)
	}
}

func TestServer_GetKeyNotFound(t *testing.T) {
	ts := newTestServer(t)
	postEmpty(t, ts, "/databases/mydb")

	resp := get(t, ts, "/databases/mydb/keys/missing")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET missing key = %d, want 404", resp.StatusCode)
	}
}

func TestServer_PutKeyDBNotFound(t *testing.T) {
	ts := newTestServer(t)
	resp := putJSON(t, ts, "/databases/ghost/keys/k", map[string]string{"value": "v"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PUT to missing db = %d, want 404", resp.StatusCode)
	}
}

func TestServer_GetKeyDBNotFound(t *testing.T) {
	ts := newTestServer(t)
	resp := get(t, ts, "/databases/ghost/keys/k")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET from missing db = %d, want 404", resp.StatusCode)
	}
}

func TestServer_PutKeyBadBody(t *testing.T) {
	ts := newTestServer(t)
	postEmpty(t, ts, "/databases/mydb")

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/databases/mydb/keys/k", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT bad body = %d, want 400", resp.StatusCode)
	}
}
