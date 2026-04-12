package microcassandra

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	// defaultThreshold is the number of rows that trigger a memtable flush.
	defaultThreshold = 128
	// sstableGlob matches all SSTable files managed by DB.
	sstableGlob = "sstable-*.sst"
)

// DB orchestrates a [Memtable] and an ordered list of on-disk [SSTable] files.
// Writes land in the memtable; when the memtable reaches its row threshold the
// DB flushes it to a new SSTable automatically. Reads check the memtable first,
// then walk SSTables from newest to oldest, returning the first matching value.
//
// DB is safe for concurrent use by multiple goroutines.
type DB struct {
	mu        sync.Mutex
	dir       string
	memtable  *Memtable
	sstables  []*SSTable // oldest at index 0, newest at the end
	nextSeq   int        // monotonically increasing counter for SSTable file names
	threshold int
}

// Open opens (or creates) the storage directory at dir and returns a ready DB.
// Existing SSTable files in dir are discovered and loaded in creation order.
//
// Preconditions: dir is either an existing directory or a path where one can be
// created. The caller must call [DB.Close] when done to flush any pending writes.
// Returns an error if dir cannot be created or SSTable files cannot be opened.
func Open(dir string) (*DB, error) {
	return OpenWithThreshold(dir, defaultThreshold)
}

// OpenWithThreshold is like [Open] but uses a custom flush threshold instead of
// [defaultThreshold]. Intended for testing scenarios that need fine-grained
// control over when flushes occur.
func OpenWithThreshold(dir string, threshold int) (*DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("open: create dir %s: %w", dir, err)
	}
	db := &DB{
		dir:       dir,
		memtable:  newMemtable(threshold),
		threshold: threshold,
	}
	if err := db.loadExistingSSTables(); err != nil {
		return nil, err
	}
	return db, nil
}

// Dir returns the storage directory this DB was opened with.
func (db *DB) Dir() string {
	return db.dir
}

// Put writes key->value into the memtable. If the memtable reaches the flush
// threshold after this write, it is automatically flushed to a new SSTable and
// reset.
//
// Not safe to call after [DB.Close].
// Returns an error only if an automatic flush fails.
func (db *DB) Put(key, value string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.memtable.put(key, value) {
		return db.flushMemtableLocked()
	}
	return nil
}

// Get returns the value for key and whether it was found. The memtable is
// checked first; if not found there, SSTables are scanned newest-to-oldest and
// the first match is returned.
//
// Not safe to call after [DB.Close].
// Returns an error if an SSTable cannot be read.
func (db *DB) Get(key string) (string, bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if v, ok := db.memtable.get(key); ok {
		return v, true, nil
	}
	for i := len(db.sstables) - 1; i >= 0; i-- {
		v, ok, err := db.sstables[i].get(key)
		if err != nil {
			return "", false, fmt.Errorf("get %q: %w", key, err)
		}
		if ok {
			return v, true, nil
		}
	}
	return "", false, nil
}

// Compact merges all SSTables into a single new SSTable, keeping the most
// recent value per key. The old SSTable files are deleted after the new one is
// durably written.
//
// Compact is a no-op when there are zero or one SSTables.
// Not safe to call after [DB.Close].
// Returns an error if any SSTable cannot be read or the merged file cannot be
// written.
func (db *DB) Compact() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if len(db.sstables) <= 1 {
		return nil
	}

	merged, err := mergeEntries(db.sstables)
	if err != nil {
		return fmt.Errorf("compact: %w", err)
	}

	newPath := db.sstablePath(db.nextSeq)
	if err := flush(newPath, merged); err != nil {
		return fmt.Errorf("compact: write merged sstable: %w", err)
	}

	// Collect old paths before replacing the slice.
	oldPaths := make([]string, len(db.sstables))
	for i, sst := range db.sstables {
		oldPaths[i] = sst.path
	}

	newSST, err := openSSTable(newPath)
	if err != nil {
		return fmt.Errorf("compact: open merged sstable: %w", err)
	}
	db.sstables = []*SSTable{newSST}
	db.nextSeq++

	if err := removeFiles(oldPaths); err != nil {
		// Non-fatal: the merged SSTable is durable; stale files are just wasted space.
		return fmt.Errorf("compact: remove old sstables: %w", err)
	}
	return nil
}

// Close flushes any non-empty memtable to disk and releases resources.
// After Close returns the DB must not be used.
//
// Returns an error if the flush fails.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.memtable.size() == 0 {
		return nil
	}
	return db.flushMemtableLocked()
}

// flushMemtableLocked writes the current memtable to a new SSTable file, appends
// the new SSTable to db.sstables, and resets the memtable. The caller must hold
// db.mu.
func (db *DB) flushMemtableLocked() error {
	path := db.sstablePath(db.nextSeq)
	if err := flush(path, db.memtable.sorted()); err != nil {
		return fmt.Errorf("flush memtable: %w", err)
	}
	sst, err := openSSTable(path)
	if err != nil {
		return fmt.Errorf("flush memtable: open new sstable: %w", err)
	}
	db.sstables = append(db.sstables, sst)
	db.nextSeq++
	db.memtable.reset()
	return nil
}

// sstablePath returns the absolute path for the SSTable with the given sequence
// number.
func (db *DB) sstablePath(seq int) string {
	return filepath.Join(db.dir, fmt.Sprintf("sstable-%08d.sst", seq))
}

// loadExistingSSTables discovers and opens all SSTable files in db.dir in
// creation order (lexicographic on the zero-padded sequence-number filename).
// The caller must hold db.mu or call this before the DB is shared.
func (db *DB) loadExistingSSTables() error {
	matches, err := filepath.Glob(filepath.Join(db.dir, sstableGlob))
	if err != nil {
		return fmt.Errorf("open: glob %s: %w", db.dir, err)
	}
	sort.Strings(matches) // lexicographic == creation order due to zero-padded names
	for _, path := range matches {
		sst, err := openSSTable(path)
		if err != nil {
			return fmt.Errorf("open: load sstable %s: %w", path, err)
		}
		db.sstables = append(db.sstables, sst)
	}
	db.nextSeq = len(db.sstables) + 1
	return nil
}

// removeFiles deletes each path in paths. All deletions are attempted even if
// an earlier one fails; the first error is returned.
func removeFiles(paths []string) error {
	var first error
	for _, p := range paths {
		if err := os.Remove(p); err != nil && first == nil {
			first = err
		}
	}
	return first
}
