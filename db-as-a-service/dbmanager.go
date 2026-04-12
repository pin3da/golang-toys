package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	microcassandra "github.com/pin3da/golang-toys/micro-cassandra"
)

// Sentinel errors returned by [DBManager] methods. Use [errors.Is] to inspect.
var (
	ErrNotFound    = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

// DBManager is a concurrent-safe registry of named [microcassandra.DB] instances.
// Each database is stored in its own subdirectory under a shared root data dir.
//
// Safe for concurrent use by multiple goroutines.
type DBManager struct {
	mu      sync.Mutex
	rootDir string
	dbs     map[string]*microcassandra.DB
}

// NewDBManager returns a DBManager rooted at rootDir.
// rootDir is created if it does not exist.
//
// Returns an error if rootDir cannot be created.
func NewDBManager(rootDir string) (*DBManager, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("new db manager: create root dir %s: %w", rootDir, err)
	}
	return &DBManager{
		rootDir: rootDir,
		dbs:     make(map[string]*microcassandra.DB),
	}, nil
}

// Create opens (or creates) the named database.
// Returns an error if a database with that name already exists or the
// underlying storage cannot be initialised.
func (m *DBManager) Create(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.dbs[name]; exists {
		return errAlreadyExists(name)
	}
	db, err := microcassandra.Open(m.dbDir(name))
	if err != nil {
		return fmt.Errorf("create database %q: %w", name, err)
	}
	m.dbs[name] = db
	return nil
}

// Get returns the open DB for name.
// Returns an error if the database does not exist.
func (m *DBManager) Get(name string) (*microcassandra.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	db, exists := m.dbs[name]
	if !exists {
		return nil, errNotFound(name)
	}
	return db, nil
}

// List returns the names of all open databases in sorted order.
func (m *DBManager) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.dbs))
	for name := range m.dbs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Delete closes and removes the named database and its storage directory.
// Returns an error if the database does not exist or removal fails.
func (m *DBManager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	db, exists := m.dbs[name]
	if !exists {
		return errNotFound(name)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("delete database %q: close: %w", name, err)
	}
	delete(m.dbs, name)
	return removeDir(m.dbDir(name))
}

// CloseAll closes every open database. Intended for graceful shutdown.
// All databases are attempted; the first error is returned.
func (m *DBManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var first error
	for name, db := range m.dbs {
		if err := db.Close(); err != nil && first == nil {
			first = fmt.Errorf("close database %q: %w", name, err)
		}
	}
	return first
}

// dbDir returns the storage directory for the named database.
func (m *DBManager) dbDir(name string) string {
	return filepath.Join(m.rootDir, name)
}

// errNotFound returns a wrapped [ErrNotFound] for a database name.
func errNotFound(name string) error {
	return fmt.Errorf("database %q: %w", name, ErrNotFound)
}

// errAlreadyExists returns a wrapped [ErrAlreadyExists] for a database name.
func errAlreadyExists(name string) error {
	return fmt.Errorf("database %q: %w", name, ErrAlreadyExists)
}

// removeDir removes dir and all its contents.
// Wraps os.RemoveAll with a descriptive error.
func removeDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove dir %s: %w", dir, err)
	}
	return nil
}
