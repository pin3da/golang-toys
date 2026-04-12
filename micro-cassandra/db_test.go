package microcassandra_test

import (
	"testing"

	microcassandra "github.com/pin3da/golang-toys/micro-cassandra"
)

func openDB(t *testing.T) *microcassandra.DB {
	t.Helper()
	db, err := microcassandra.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return db
}

func mustPut(t *testing.T, db *microcassandra.DB, key, value string) {
	t.Helper()
	if err := db.Put(key, value); err != nil {
		t.Fatalf("Put(%q, %q) error = %v", key, value, err)
	}
}

func mustGet(t *testing.T, db *microcassandra.DB, key, wantValue string, wantFound bool) {
	t.Helper()
	got, found, err := db.Get(key)
	if err != nil {
		t.Fatalf("Get(%q) error = %v", key, err)
	}
	if found != wantFound || got != wantValue {
		t.Errorf("Get(%q) = (%q, %v), want (%q, %v)", key, got, found, wantValue, wantFound)
	}
}

func TestDB_PutGet(t *testing.T) {
	tests := []struct {
		name      string
		puts      [][2]string
		getKey    string
		wantValue string
		wantFound bool
	}{
		{
			name:      "key present",
			puts:      [][2]string{{"a", "1"}, {"b", "2"}},
			getKey:    "a",
			wantValue: "1",
			wantFound: true,
		},
		{
			name:      "key absent",
			puts:      [][2]string{{"a", "1"}},
			getKey:    "z",
			wantValue: "",
			wantFound: false,
		},
		{
			name:      "overwrite returns latest value",
			puts:      [][2]string{{"a", "old"}, {"a", "new"}},
			getKey:    "a",
			wantValue: "new",
			wantFound: true,
		},
		{
			name:      "empty db",
			puts:      nil,
			getKey:    "x",
			wantValue: "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openDB(t)
			for _, kv := range tt.puts {
				mustPut(t, db, kv[0], kv[1])
			}
			mustGet(t, db, tt.getKey, tt.wantValue, tt.wantFound)
		})
	}
}

func TestDB_FlushTrigger(t *testing.T) {
	db, err := microcassandra.OpenWithThreshold(t.TempDir(), 2)
	if err != nil {
		t.Fatalf("OpenWithThreshold() error = %v", err)
	}

	mustPut(t, db, "a", "1")
	mustPut(t, db, "b", "2") // triggers flush
	mustPut(t, db, "c", "3")

	mustGet(t, db, "a", "1", true)
	mustGet(t, db, "b", "2", true)
	mustGet(t, db, "c", "3", true)
}

func TestDB_NewestValueWinsAcrossTables(t *testing.T) {
	db, err := microcassandra.OpenWithThreshold(t.TempDir(), 2)
	if err != nil {
		t.Fatalf("OpenWithThreshold() error = %v", err)
	}

	mustPut(t, db, "k", "old")
	mustPut(t, db, "x", "x") // triggers flush -> "k":"old" goes to SSTable
	mustPut(t, db, "k", "new")

	mustGet(t, db, "k", "new", true)
}

func TestDB_Compact(t *testing.T) {
	db, err := microcassandra.OpenWithThreshold(t.TempDir(), 2)
	if err != nil {
		t.Fatalf("OpenWithThreshold() error = %v", err)
	}

	mustPut(t, db, "k", "first")
	mustPut(t, db, "a", "a") // flush -> SSTable 1: {"a":"a","k":"first"}
	mustPut(t, db, "k", "second")
	mustPut(t, db, "b", "b") // flush -> SSTable 2: {"b":"b","k":"second"}

	if err := db.Compact(); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	mustGet(t, db, "k", "second", true)
	mustGet(t, db, "a", "a", true)
	mustGet(t, db, "b", "b", true)
	mustGet(t, db, "missing", "", false)
}

func TestDB_CompactNoOp(t *testing.T) {
	db := openDB(t)

	if err := db.Compact(); err != nil {
		t.Fatalf("Compact() on empty DB error = %v", err)
	}

	mustPut(t, db, "a", "1")
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db2, err := microcassandra.Open(db.Dir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db2.Compact(); err != nil {
		t.Fatalf("Compact() on single-SSTable DB error = %v", err)
	}
}

func TestDB_Close_FlushesMemtable(t *testing.T) {
	dir := t.TempDir()
	db, err := microcassandra.Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	mustPut(t, db, "persist", "yes")

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db2, err := microcassandra.Open(dir)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	mustGet(t, db2, "persist", "yes", true)
}

func TestDB_Stats(t *testing.T) {
	tests := []struct {
		name         string
		threshold    int
		puts         [][2]string
		wantSSTables int
		wantMemtable int
	}{
		{
			name:         "empty db",
			threshold:    128,
			puts:         nil,
			wantSSTables: 0,
			wantMemtable: 0,
		},
		{
			name:         "entries below flush threshold stay in memtable",
			threshold:    128,
			puts:         [][2]string{{"a", "1"}, {"b", "2"}},
			wantSSTables: 0,
			wantMemtable: 2,
		},
		{
			name:         "flush produces one sstable",
			threshold:    2,
			puts:         [][2]string{{"a", "1"}, {"b", "2"}},
			wantSSTables: 1,
			wantMemtable: 0,
		},
		{
			name:         "two flushes produce two sstables",
			threshold:    2,
			puts:         [][2]string{{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}},
			wantSSTables: 2,
			wantMemtable: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := microcassandra.OpenWithThreshold(t.TempDir(), tt.threshold)
			if err != nil {
				t.Fatalf("OpenWithThreshold() error = %v", err)
			}
			for _, kv := range tt.puts {
				mustPut(t, db, kv[0], kv[1])
			}

			got := db.Stats()
			if got.SSTableCount != tt.wantSSTables {
				t.Errorf("Stats().SSTableCount = %d, want %d", got.SSTableCount, tt.wantSSTables)
			}
			if got.MemtableSize != tt.wantMemtable {
				t.Errorf("Stats().MemtableSize = %d, want %d", got.MemtableSize, tt.wantMemtable)
			}
		})
	}
}

func TestDB_Reopen(t *testing.T) {
	dir := t.TempDir()

	db, err := microcassandra.OpenWithThreshold(dir, 2)
	if err != nil {
		t.Fatalf("OpenWithThreshold() error = %v", err)
	}
	mustPut(t, db, "a", "1")
	mustPut(t, db, "b", "2") // triggers flush
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db2, err := microcassandra.Open(dir)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	mustGet(t, db2, "a", "1", true)
	mustGet(t, db2, "b", "2", true)
}
