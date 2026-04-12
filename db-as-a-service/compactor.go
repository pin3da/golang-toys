package main

import (
	"context"
	"log"
	"time"
)

// DefaultCompactionInterval is the compaction period used in production.
const DefaultCompactionInterval = 30 * time.Second

// Compactor runs periodic compaction on all databases in a [DBManager].
// Create one with [NewCompactor] and launch [Compactor.Run] in a goroutine.
type Compactor struct {
	manager  *DBManager
	interval time.Duration
}

// NewCompactor returns a Compactor that compacts all databases in manager
// every interval.
func NewCompactor(manager *DBManager, interval time.Duration) *Compactor {
	return &Compactor{manager: manager, interval: interval}
}

// Run starts the compaction loop, ticking every c.interval. It blocks until
// ctx is cancelled, making it suitable for use with a goroutine and a
// signal-derived context for graceful shutdown.
func (c *Compactor) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.compactAll()
		case <-ctx.Done():
			return
		}
	}
}

// compactAll calls Compact on every open database. Errors are logged but
// do not stop compaction of the remaining databases.
func (c *Compactor) compactAll() {
	for _, name := range c.manager.List() {
		c.compactOne(name)
	}
}

// compactOne compacts a single named database, logging any error.
func (c *Compactor) compactOne(name string) {
	db, err := c.manager.Get(name)
	if err != nil {
		// DB was deleted between List and Get; nothing to do.
		return
	}
	if err := db.Compact(); err != nil {
		log.Printf("compactor: compact %q: %v", name, err)
	}
}
