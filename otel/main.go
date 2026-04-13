package main

import (
	"log"
	"net/http"
)

func main() {
	store := NewStore()

	mux := http.NewServeMux()
	mux.Handle("POST /v1/metrics", IngestHandler(store))
	mux.Handle("GET /metrics", QueryHandler(store))

	log.Println("listening on :4318")
	log.Fatal(http.ListenAndServe(":4318", mux))
}
