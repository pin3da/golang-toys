package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	store := NewWindowedStore(10, time.Minute)

	mux := http.NewServeMux()
	mux.Handle("POST /v1/metrics", IngestHandler(store))
	mux.Handle("GET /metrics", QueryHandler(store))

	log.Println("listening on :4318")
	log.Fatal(http.ListenAndServe(":4318", mux))
}
