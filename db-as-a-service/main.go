// Package main is the entry point for the db-as-a-service HTTP server.
// It parses flags, wires up the HTTP routes, and handles graceful shutdown
// on SIGINT/SIGTERM.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "./data", "root directory for database storage")
	flag.Parse()

	manager, err := NewDBManager(*data)
	if err != nil {
		log.Fatalf("init db manager: %v", err)
	}

	srv := &http.Server{
		Addr:    *addr,
		Handler: NewServer(manager),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go NewCompactor(manager, DefaultCompactionInterval).Run(ctx)

	go func() {
		log.Printf("listening on %s, data dir %s", *addr, *data)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// blocks until the context is cancelled.
	<-ctx.Done()

	log.Println("shutting down")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := manager.CloseAll(); err != nil {
		log.Printf("close databases: %v", err)
	}
}
