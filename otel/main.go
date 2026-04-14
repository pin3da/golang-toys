package main

import (
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	window := flag.Duration("window", time.Minute, "width of each time window")
	buckets := flag.Int("buckets", 10, "number of ring-buffer buckets to retain")
	flag.Parse()

	store := NewWindowedStore(*buckets, *window)

	mux := http.NewServeMux()
	mux.Handle("POST /v1/metrics", IngestHandler(store))
	mux.Handle("GET /metrics", QueryHandler(store))

	log.Printf("listening on :4318 (window=%s, buckets=%d)", *window, *buckets)
	log.Fatal(http.ListenAndServe(":4318", mux))
}
