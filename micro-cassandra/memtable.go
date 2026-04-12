package microcassandra

import (
	"sort"
)

// entry is a key-value pair stored in the memtable.
type entry struct {
	key   string
	value string
}

// Memtable is an in-memory write buffer. Entries are accumulated here until
// the row count hits the flush threshold, at which point the caller should
// flush the memtable to an SSTable and reset it.
type Memtable struct {
	data      map[string]string
	threshold int
}

// newMemtable creates a new Memtable that should be flushed once it contains
// `threshold` rows.
func newMemtable(threshold int) *Memtable {
	return &Memtable{
		data:      map[string]string{},
		threshold: threshold,
	}
}

// put writes or overwrites a `key`. Returns true if the memtable has reached the
// flush threshold after this write and should be flushed.
// The value is stored even if the threshold was exceeded.
func (m *Memtable) put(key, value string) bool {
	m.data[key] = value
	return len(m.data) >= m.threshold
}

// get returns the value for `key` and whether it was found.
func (m *Memtable) get(key string) (string, bool) {
	v, ok := m.data[key]
	return v, ok
}

// sorted returns all entries sorted lexicographically by key. Used when
// flushing to an SSTable, which requires sorted order.
func (m *Memtable) sorted() []entry {
	entries := make([]entry, 0, len(m.data))
	for k, v := range m.data {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})
	return entries
}

// size returns the number of rows currently in the memtable.
func (m *Memtable) size() int {
	return len(m.data)
}

// reset clears all entries, ready to accept new writes after a flush.
func (m *Memtable) reset() {
	m.data = map[string]string{}
}
