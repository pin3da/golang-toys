package microcassandra

import (
	"path/filepath"
	"testing"
)

// sstablePath returns a unique file path inside t's temp directory.
func sstablePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// TestSSTable_FlushAndGet verifies the round-trip: flush entries then retrieve
// them via get.
func TestSSTable_FlushAndGet(t *testing.T) {
	tests := []struct {
		name    string
		entries []entry
		get     string
		want    string
		found   bool
	}{
		{
			name:    "key present",
			entries: []entry{{"apple", "a"}, {"banana", "b"}, {"cherry", "c"}},
			get:     "banana",
			want:    "b",
			found:   true,
		},
		{
			name:    "first key",
			entries: []entry{{"apple", "a"}, {"banana", "b"}},
			get:     "apple",
			want:    "a",
			found:   true,
		},
		{
			name:    "last key",
			entries: []entry{{"apple", "a"}, {"banana", "b"}},
			get:     "banana",
			want:    "b",
			found:   true,
		},
		{
			name:    "key absent before all entries",
			entries: []entry{{"banana", "b"}, {"cherry", "c"}},
			get:     "apple",
			want:    "",
			found:   false,
		},
		{
			name:    "key absent between two entries (early exit)",
			entries: []entry{{"apple", "a"}, {"cherry", "c"}},
			get:     "banana",
			want:    "",
			found:   false,
		},
		{
			name:    "key absent after all entries",
			entries: []entry{{"apple", "a"}, {"banana", "b"}},
			get:     "cherry",
			want:    "",
			found:   false,
		},
		{
			name:    "empty SSTable",
			entries: nil,
			get:     "anything",
			want:    "",
			found:   false,
		},
		{
			name:    "single entry found",
			entries: []entry{{"only", "val"}},
			get:     "only",
			want:    "val",
			found:   true,
		},
		{
			name:    "empty string value",
			entries: []entry{{"k", ""}},
			get:     "k",
			want:    "",
			found:   true,
		},
		{
			name:    "value with spaces",
			entries: []entry{{"k", "hello world"}},
			get:     "k",
			want:    "hello world",
			found:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := sstablePath(t, "sst.db")

			if err := flush(path, tt.entries); err != nil {
				t.Fatalf("flush() error = %v", err)
			}

			sst, err := openSSTable(path)
			if err != nil {
				t.Fatalf("openSSTable() error = %v", err)
			}

			got, ok, err := sst.get(tt.get)
			if err != nil {
				t.Fatalf("get(%q) unexpected error = %v", tt.get, err)
			}
			if ok != tt.found || got != tt.want {
				t.Errorf("get(%q) = (%q, %v), want (%q, %v)", tt.get, got, ok, tt.want, tt.found)
			}
		})
	}
}

// TestSSTable_Entries verifies that entries returns all rows in sorted order.
func TestSSTable_Entries(t *testing.T) {
	tests := []struct {
		name    string
		entries []entry
		want    []entry
	}{
		{
			name:    "multiple entries in order",
			entries: []entry{{"a", "1"}, {"b", "2"}, {"c", "3"}},
			want:    []entry{{"a", "1"}, {"b", "2"}, {"c", "3"}},
		},
		{
			name:    "single entry",
			entries: []entry{{"only", "v"}},
			want:    []entry{{"only", "v"}},
		},
		{
			name:    "empty SSTable returns empty slice",
			entries: nil,
			want:    []entry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := sstablePath(t, "sst.db")

			if err := flush(path, tt.entries); err != nil {
				t.Fatalf("flush() error = %v", err)
			}

			sst, err := openSSTable(path)
			if err != nil {
				t.Fatalf("openSSTable() error = %v", err)
			}

			got, err := sst.entries()
			if err != nil {
				t.Fatalf("entries() unexpected error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("entries() len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for i, e := range got {
				if e != tt.want[i] {
					t.Errorf("entries()[%d] = %v, want %v", i, e, tt.want[i])
				}
			}
		})
	}
}

// TestSSTable_OpenNonExistent verifies that openSSTable returns an error for a
// missing file.
func TestSSTable_OpenNonExistent(t *testing.T) {
	_, err := openSSTable("/nonexistent/path/sst.db")
	if err == nil {
		t.Error("openSSTable on missing file should return an error, got nil")
	}
}

// TestFlush_PathAlreadyExists verifies that flush returns an error when the
// target path already exists rather than silently overwriting it.
func TestFlush_PathAlreadyExists(t *testing.T) {
	path := sstablePath(t, "sst.db")

	if err := flush(path, []entry{{"a", "1"}}); err != nil {
		t.Fatalf("first flush() error = %v", err)
	}
	if err := flush(path, []entry{{"b", "2"}}); err == nil {
		t.Error("second flush() to existing path should return an error, got nil")
	}
}
