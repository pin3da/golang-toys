package main

import (
	"flag"
	"log"

	"github.com/google/uuid"

	"crdt-practice/crdt"
	"crdt-practice/server"
	"crdt-practice/ui"
)

func main() {
	bind := flag.String("bind", "tcp://*:8080", "ZMQ endpoint to bind the PUB socket on")
	peer := flag.String("peer", "", "ZMQ endpoint of a peer's PUB socket to subscribe to; empty runs offline")
	flag.Parse()

	clientID := uuid.NewString()
	rga := crdt.NewRGA()

	broadcast, err := server.Start(*bind, *peer, ui.PostRemoteOp)
	if err != nil {
		log.Fatal(err)
	}
	if err := ui.Start(rga, clientID, broadcast); err != nil {
		log.Fatal(err)
	}
}
