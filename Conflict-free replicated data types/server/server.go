// Package server carries RGA ops between peers over ZeroMQ PUB/SUB. Each
// peer binds a PUB socket for its own outgoing ops and connects a SUB socket
// to a peer's bind address to receive theirs.
//
// The wire format is defined by [crdt.Op.MarshalBinary]; one encoded op per
// ZMQ message.
package server

import (
	"fmt"
	"log"

	"github.com/pebbe/zmq4"

	"crdt-practice/crdt"
)

// Start binds a PUB socket on bindAddr and, if peerAddr is non-empty,
// connects a SUB socket to it. A dedicated goroutine reads encoded ops from
// the SUB socket, decodes them, and hands each off to onRemote. onRemote is
// expected to be non-blocking (e.g. the caller posts the op into a UI event
// loop).
//
// The returned broadcast function encodes op and sends it on the PUB socket.
// It is safe to call only from the goroutine that invoked Start, since the
// PUB socket is not thread-safe.
//
// Callers own no cleanup responsibility: sockets live for the lifetime of
// the process.
func Start(bindAddr, peerAddr string, onRemote func(crdt.Op)) (func(crdt.Op), error) {
	pub, err := zmq4.NewSocket(zmq4.PUB)
	if err != nil {
		return nil, fmt.Errorf("server: new PUB: %w", err)
	}
	if err := pub.Bind(bindAddr); err != nil {
		return nil, fmt.Errorf("server: bind %q: %w", bindAddr, err)
	}

	if peerAddr != "" {
		sub, err := zmq4.NewSocket(zmq4.SUB)
		if err != nil {
			return nil, fmt.Errorf("server: new SUB: %w", err)
		}
		if err := sub.SetSubscribe(""); err != nil {
			return nil, fmt.Errorf("server: subscribe: %w", err)
		}
		if err := sub.Connect(peerAddr); err != nil {
			return nil, fmt.Errorf("server: connect %q: %w", peerAddr, err)
		}
		go recvLoop(sub, onRemote)
	}

	broadcast := func(op crdt.Op) {
		buf, err := op.MarshalBinary()
		if err != nil {
			log.Printf("server: marshal op: %v", err)
			return
		}
		if _, err := pub.SendBytes(buf, 0); err != nil {
			log.Printf("server: send: %v", err)
		}
	}
	return broadcast, nil
}

// recvLoop drains sub and delivers decoded ops to onRemote until the socket
// errors out. Malformed messages are logged and dropped.
func recvLoop(sub *zmq4.Socket, onRemote func(crdt.Op)) {
	for {
		buf, err := sub.RecvBytes(0)
		if err != nil {
			log.Printf("server: recv: %v", err)
			return
		}
		var op crdt.Op
		if err := op.UnmarshalBinary(buf); err != nil {
			log.Printf("server: decode: %v", err)
			continue
		}
		onRemote(op)
	}
}
