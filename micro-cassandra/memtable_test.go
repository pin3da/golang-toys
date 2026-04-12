package microcassandra

import (
	"testing"
)

func TestMemtable_GetAfterPut(t *testing.T) {
	tests := []struct {
		name  string
		puts  []entry
		get   string
		want  string
		found bool
	}{
		{
			name:  "key present",
			puts:  []entry{{"a", "1"}, {"b", "2"}},
			get:   "a",
			want:  "1",
			found: true,
		},
		{
			name:  "key absent",
			puts:  []entry{{"a", "1"}},
			get:   "z",
			want:  "",
			found: false,
		},
		{
			name:  "overwrite returns latest value",
			puts:  []entry{{"a", "old"}, {"a", "another"}, {"a", "new"}},
			get:   "a",
			want:  "new",
			found: true,
		},
		{
			name:  "empty memtable",
			puts:  nil,
			get:   "a",
			want:  "",
			found: false,
		},
		{
			name:  "empty string key",
			puts:  []entry{{"", "v"}},
			get:   "",
			want:  "v",
			found: true,
		},
		{
			name:  "empty string value",
			puts:  []entry{{"k", ""}},
			get:   "k",
			want:  "",
			found: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMemtable(100)
			for _, e := range tt.puts {
				m.put(e.key, e.value)
			}
			got, ok := m.get(tt.get)
			if ok != tt.found || got != tt.want {
				t.Errorf("get(%q) = (%q, %v), want (%q, %v)", tt.get, got, ok, tt.want, tt.found)
			}
		})
	}
}

func TestMemtable_FlushThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold int
		writes    []entry
		wantFlush bool // whether the last put should signal flush
	}{
		{
			name:      "exactly at threshold",
			threshold: 3,
			writes:    []entry{{"a", "1"}, {"b", "2"}, {"c", "3"}},
			wantFlush: true,
		},
		{
			name:      "below threshold",
			threshold: 5,
			writes:    []entry{{"a", "1"}, {"b", "2"}},
			wantFlush: false,
		},
		{
			name:      "overwrite does not increase size past threshold early",
			threshold: 2,
			writes:    []entry{{"a", "1"}, {"a", "2"}, {"a", "3"}},
			wantFlush: false, // still only 1 unique key
		},
		{
			name:      "threshold of 1 triggers on first write",
			threshold: 1,
			writes:    []entry{{"a", "1"}},
			wantFlush: true,
		},
		{
			name:      "writes beyond threshold keep signaling flush",
			threshold: 2,
			writes:    []entry{{"a", "1"}, {"b", "2"}, {"c", "3"}},
			wantFlush: true,
		},
		{
			name:      "no capacity",
			threshold: 0,
			writes:    []entry{{"a", "1"}},
			wantFlush: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMemtable(tt.threshold)
			var flush bool
			for _, e := range tt.writes {
				flush = m.put(e.key, e.value)
			}
			if flush != tt.wantFlush {
				t.Errorf("last put flush signal = %v, want %v (size=%d, threshold=%d)",
					flush, tt.wantFlush, m.size(), tt.threshold)
			}
		})
	}
}

func TestMemtable_Sorted(t *testing.T) {
	tests := []struct {
		name string
		puts []entry
		want []entry
	}{
		{
			name: "multiple entries returned in lexicographic order",
			puts: []entry{{"banana", "b"}, {"apple", "a"}, {"cherry", "c"}},
			want: []entry{{"apple", "a"}, {"banana", "b"}, {"cherry", "c"}},
		},
		{
			name: "single entry",
			puts: []entry{{"only", "v"}},
			want: []entry{{"only", "v"}},
		},
		{
			name: "empty memtable returns empty slice",
			puts: nil,
			want: []entry{},
		},
		{
			name: "distinct keys with duplicate values sorted by key",
			puts: []entry{{"z", "same"}, {"a", "same"}},
			want: []entry{{"a", "same"}, {"z", "same"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMemtable(100)
			for _, e := range tt.puts {
				m.put(e.key, e.value)
			}
			got := m.sorted()
			if len(got) != len(tt.want) {
				t.Fatalf("sorted() len = %d, want %d", len(got), len(tt.want))
			}
			for i, e := range got {
				if e != tt.want[i] {
					t.Errorf("sorted()[%d] = %v, want %v", i, e, tt.want[i])
				}
			}
		})
	}
}

func TestMemtable_FlushThreshold_AfterReset(t *testing.T) {
	m := newMemtable(2)
	m.put("a", "1")
	m.put("b", "2") // triggers flush
	m.reset()

	// After reset, the first write should NOT signal flush (threshold=2, size=1).
	flush := m.put("c", "3")
	if flush {
		t.Errorf("put after reset triggered flush with only 1 entry (threshold=2)")
	}
}

func TestMemtable_Sorted_LenMatchesSize(t *testing.T) {
	m := newMemtable(100)
	m.put("x", "1")
	m.put("y", "2")
	m.put("x", "updated") // overwrite — must not inflate sorted()

	if got, want := len(m.sorted()), m.size(); got != want {
		t.Errorf("len(sorted()) = %d, size() = %d; must be equal", got, want)
	}
}

func TestMemtable_Sorted_CommonPrefixes(t *testing.T) {
	m := newMemtable(100)
	for _, e := range []entry{{"ab", "2"}, {"a", "1"}, {"b", "3"}} {
		m.put(e.key, e.value)
	}
	got := m.sorted()
	want := []entry{{"a", "1"}, {"ab", "2"}, {"b", "3"}}
	if len(got) != len(want) {
		t.Fatalf("sorted() len = %d, want %d", len(got), len(want))
	}
	for i, e := range got {
		if e != want[i] {
			t.Errorf("sorted()[%d] = %v, want %v", i, e, want[i])
		}
	}
}

func TestMemtable_Reset(t *testing.T) {
	m := newMemtable(10)
	m.put("a", "1")
	m.put("b", "2")
	m.reset()

	if m.size() != 0 {
		t.Errorf("size after reset = %d, want 0", m.size())
	}
	if _, ok := m.get("a"); ok {
		t.Error("get after reset should return false")
	}

	// Memtable must be fully functional after reset.
	m.put("c", "3")
	if v, ok := m.get("c"); !ok || v != "3" {
		t.Errorf("get after reset+put = (%q, %v), want (\"3\", true)", v, ok)
	}
	if m.size() != 1 {
		t.Errorf("size after reset+put = %d, want 1", m.size())
	}
}
