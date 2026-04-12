package main

import (
	"context"
	"testing"
	"time"
)

// seedSSTables writes n entries into the named database using a flush
// threshold of 1, producing n SSTables. The database must already exist
// in m with that threshold (via createWithThreshold).
func seedSSTables(t *testing.T, m *DBManager, dbName string, n int) {
	t.Helper()
	db, err := m.Get(dbName)
	if err != nil {
		t.Fatalf("Get %q: %v", dbName, err)
	}
	for i := range n {
		key := string(rune('a' + i))
		if err := db.Put(key, "v"); err != nil {
			t.Fatalf("Put %q: %v", key, err)
		}
	}
}

func TestCompactor_CompactAllNoOp(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	if err := m.Create("db"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Must not panic or error on a DB with 0 SSTables.
	c := NewCompactor(m, time.Second)
	c.compactAll()
}

func TestCompactor_CompactAllReducesSSTables(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	if err := m.createWithThreshold("db", 1); err != nil {
		t.Fatalf("createWithThreshold: %v", err)
	}
	seedSSTables(t, m, "db", 3)

	db, _ := m.Get("db")
	before := db.Stats().SSTableCount
	if before < 2 {
		t.Fatalf("expected >= 2 SSTables before compaction, got %d", before)
	}

	c := NewCompactor(m, time.Second)
	c.compactAll()

	if after := db.Stats().SSTableCount; after >= before {
		t.Errorf("SSTableCount = %d after compactAll, want < %d", after, before)
	}
}

func TestCompactor_RunStopsOnContextCancel(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	if err := m.Create("db"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewCompactor(m, 10*time.Millisecond).Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Run did not stop within 1s after context cancellation")
	}
}

func TestCompactor_RunCompactsOnTick(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}
	if err := m.createWithThreshold("db", 1); err != nil {
		t.Fatalf("createWithThreshold: %v", err)
	}
	seedSSTables(t, m, "db", 3)

	db, _ := m.Get("db")
	before := db.Stats().SSTableCount
	if before < 2 {
		t.Fatalf("expected >= 2 SSTables before Run, got %d", before)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go NewCompactor(m, 10*time.Millisecond).Run(ctx)

	<-ctx.Done()
	if after := db.Stats().SSTableCount; after >= before {
		t.Errorf("SSTableCount = %d after Run, expected compaction to have fired", after)
	}
}
