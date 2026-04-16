package main

import (
	"log"

	"github.com/google/uuid"

	"crdt-practice/crdt"
	"crdt-practice/ui"
)

func main() {
	clientID := uuid.NewString()
	rga := crdt.NewRGA()

	broadcast := func(crdt.Op) {} // TODO: wire to server.Broadcast
	if err := ui.Start(rga, clientID, broadcast); err != nil {
		log.Fatal(err)
	}
}
