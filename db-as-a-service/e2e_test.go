//go:build e2e

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestServer_StatsAndAutoCompaction is an end-to-end test that verifies:
//  1. The stats endpoint reports SSTable count accurately after writes.
//  2. The compactor reduces SSTable count when run against the same manager.
//
// Run with: go test -tags e2e ./...
func TestServer_StatsAndAutoCompaction(t *testing.T) {
	// Use a flush threshold of 1 so every PUT produces an SSTable, letting us
	// reach multiple SSTables with just a handful of HTTP requests.
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	if err := m.createWithThreshold("mydb", 1); err != nil {
		t.Fatalf("createWithThreshold: %v", err)
	}

	ts := httptest.NewServer(NewServer(m))
	t.Cleanup(ts.Close)

	// Write three keys — each flush produces one SSTable (threshold=1).
	for _, kv := range [][2]string{{"a", "1"}, {"b", "2"}, {"c", "3"}} {
		resp := putJSON(t, ts, "/databases/mydb/keys/"+kv[0], map[string]string{"value": kv[1]})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT key %q = %d, want 200", kv[0], resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Stats must reflect the SSTables produced by the writes.
	sstableCount := func() int {
		resp := get(t, ts, "/databases/mydb/stats")
		var body map[string]any
		decodeJSON(t, resp, &body)
		return int(body["sstable_count"].(float64))
	}

	before := sstableCount()
	if before < 2 {
		t.Fatalf("expected >= 2 SSTables after writes, stats reported %d", before)
	}

	// Run the compactor for long enough that it fires at least once.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go NewCompactor(m, 10*time.Millisecond).Run(ctx)
	<-ctx.Done()

	// Stats must now show fewer SSTables.
	if after := sstableCount(); after >= before {
		t.Errorf("SSTableCount after compaction = %d, want < %d", after, before)
	}
}
