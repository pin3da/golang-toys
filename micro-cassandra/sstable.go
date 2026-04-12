package microcassandra

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// SSTable is an immutable, sorted, on-disk key-value store flushed from a
// [Memtable]. Entries are stored in ascending lexicographic key order using a
// length-prefix binary encoding: each entry is a 4-byte big-endian key length,
// followed by key bytes, followed by a 4-byte big-endian value length, followed
// by value bytes. Keys and values may contain arbitrary bytes.
//
// The 4-byte length prefix limits each key and value to at most 2^32−1 bytes
// (~4 GiB). No enforcement is done at runtime; passing larger strings will
// silently truncate the stored length and produce a corrupt file.
type SSTable struct {
	path string
}

// flush writes entries to a new SSTable file at path using the length-prefix
// binary format. entries must be sorted in ascending lexicographic key order;
// callers typically obtain this via [Memtable.sorted].
//
// Preconditions: entries is sorted by key; path does not already exist.
// Postconditions: the file at path is complete and durable before flush returns.
// Returns an error if the file cannot be created or written.
func flush(path string, entries []entry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, e := range entries {
		if err := writeField(w, e.key); err != nil {
			f.Close()
			return fmt.Errorf("flush: write key %q: %w", e.key, err)
		}
		if err := writeField(w, e.value); err != nil {
			f.Close()
			return fmt.Errorf("flush: write value for key %q: %w", e.key, err)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return fmt.Errorf("flush: finalize %s: %w", path, err)
	}
	return f.Close()
}

// writeField writes a length-prefixed field: 4-byte big-endian length followed
// by the raw bytes of s.
func writeField(w *bufio.Writer, s string) error {
	length := uint32(len(s))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}
	_, err := io.WriteString(w, s)
	return err
}

// openSSTable opens an existing SSTable at path for reading.
//
// Returns an error if the file does not exist or cannot be opened.
func openSSTable(path string) (*SSTable, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return &SSTable{path: path}, nil
}

// get scans the SSTable linearly for key.
//
// Returns (value, true, nil) if found, ("", false, nil) if the key is absent,
// or ("", false, err) if the file cannot be read or parsed.
//
// Note: linear scan is O(n); acceptable for the small SSTable sizes used here.
func (s *SSTable) get(key string) (string, bool, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		k, err := readField(r)
		if err == io.EOF {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("sstable get: read key: %w", err)
		}
		v, err := readField(r)
		if err != nil {
			return "", false, fmt.Errorf("sstable get: read value for key %q: %w", k, err)
		}
		if k == key {
			return v, true, nil
		}
		if k > key {
			// Entries are sorted; no later key can match.
			return "", false, nil
		}
	}
}

// entries returns all key-value pairs in sorted order.
// Intended for compaction: each call opens and reads the file independently,
// so it is safe for concurrent use.
//
// Returns a non-nil error if the file cannot be read or parsed.
func (s *SSTable) entries() ([]entry, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var result []entry
	r := bufio.NewReader(f)
	for {
		k, err := readField(r)
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, fmt.Errorf("sstable entries: read key: %w", err)
		}
		v, err := readField(r)
		if err != nil {
			return nil, fmt.Errorf("sstable entries: read value for key %q: %w", k, err)
		}
		result = append(result, entry{k, v})
	}
}

// readField reads a length-prefixed field written by writeField.
// Returns io.EOF only when the reader is positioned exactly at EOF before
// the length header; any partial read returns a wrapped error instead.
func readField(r *bufio.Reader) (string, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
