package main

import (
	"flag"

	"crdt-practice/crdt"
	"crdt-practice/server"
	"crdt-practice/ui"
)

func main() {
	bind := flag.String("bind", "tcp://*:8080", "")
	peer := flag.String("peer", "", "")
	flag.Parse()

	rga := crdt.NewRGA()

	_ = bind
	_ = peer
	_ = rga

	server.Start(*bind, *peer, rga)
	ui.Start(rga)
}
