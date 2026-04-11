# micro-cassandra

A stripped-down Cassandra-inspired storage engine in Go, built to practice the core concepts behind Cassandra's LSM-tree write path.

## Goal

Implement the essential read/write storage loop in under one hour (including tests), covering the concepts that make Cassandra distinct.

## Concepts covered

| Concept | Simplified version |
|---|---|
| **Memtable** | In-memory sorted map; accepts writes |
| **SSTable** | Immutable sorted file flushed from the memtable |
| **Flush** | When memtable exceeds a row threshold, write it to disk as an SSTable |
| **Read path** | Check memtable first, then walk SSTables newest-to-oldest |
| **Compaction** | Merge two SSTables into one, keeping the most recent value per key |

## What is intentionally omitted

- Commit log / WAL (no crash recovery)
- Bloom filters
- Replication and consistent hashing
- Column families / wide rows
- Tombstones / deletions (stretch goal)

## Data model

Simple `string → string` key-value store. Keys are sorted lexicographically within each SSTable.

## API (target)

```go
db := microcassandra.Open(dir)   // opens/creates storage directory

db.Put(key, value string) error  // write to memtable; flushes if threshold reached
db.Get(key string) (string, bool) // read: memtable first, then SSTables
db.Compact() error               // merge all SSTables into one
db.Close() error
```

## Implementation plan

1. **`memtable.go`** — sorted in-memory map with a configurable row threshold
2. **`sstable.go`** — write (flush) and read (scan or point lookup) a sorted flat file
3. **`db.go`** — orchestrates the memtable + list of SSTable files, implements `Put`/`Get`/`Compact`
4. **`db_test.go`** — table-driven tests covering write/read, flush trigger, and compaction

## SSTable file format

Each SSTable is a plain text file (one `key\tvalue\n` line per entry, sorted by key). Simple enough to inspect with standard tools; real Cassandra uses a binary format.

SSTable files are artificially capped to a few KB to force the DB to scan multiple files (and then use smarter filtering).

## Stretch goals (if time permits)

- Tombstones: `Put(key, "")` marks deletion; `Get` returns `false` if the latest entry is a tombstone
- A size-tiered compaction strategy: only compact when two SSTables are within the same size tier


