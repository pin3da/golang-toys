package crdt

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// OpAction names the two mutations that can travel over the wire.
type OpAction string

const (
	OpInsert OpAction = "INSERT"
	OpDelete OpAction = "DELETE"
)

// Op is the minimal wire representation of a local mutation, shipped to peers
// and replayed via [RGA.RemoteInsert] or [RGA.Delete].
//
// For OpInsert, PrevID and Node are both meaningful. For OpDelete, only
// Node.ID is inspected.
type Op struct {
	Action OpAction
	PrevID NodeID
	Node   Node
}

// Wire-format tags. Stable across versions; never reuse a value.
const (
	tagInsert byte = 0x01
	tagDelete byte = 0x02
)

// ErrMalformedOp is returned by [Op.UnmarshalBinary] when the buffer does not
// contain a valid encoded op.
var ErrMalformedOp = errors.New("crdt: malformed op")

// MarshalBinary encodes o using the custom big-endian wire format documented
// in package crdt's doc. Returns an error if a ClientID exceeds 255 bytes.
func (o Op) MarshalBinary() ([]byte, error) {
	switch o.Action {
	case OpInsert:
		if err := checkClientID(o.PrevID.ClientID); err != nil {
			return nil, err
		}
		if err := checkClientID(o.Node.ID.ClientID); err != nil {
			return nil, err
		}
		buf := make([]byte, 0, 1+nodeIDSize(o.PrevID)+nodeIDSize(o.Node.ID)+4)
		buf = append(buf, tagInsert)
		buf = appendNodeID(buf, o.PrevID)
		buf = appendNodeID(buf, o.Node.ID)
		buf = binary.BigEndian.AppendUint32(buf, uint32(o.Node.Char))
		return buf, nil
	case OpDelete:
		if err := checkClientID(o.Node.ID.ClientID); err != nil {
			return nil, err
		}
		buf := make([]byte, 0, 1+nodeIDSize(o.Node.ID))
		buf = append(buf, tagDelete)
		buf = appendNodeID(buf, o.Node.ID)
		return buf, nil
	}
	return nil, fmt.Errorf("crdt: unknown op action %q", o.Action)
}

// UnmarshalBinary decodes data into o. Returns [ErrMalformedOp] for any
// truncation, unknown tag, or trailing garbage.
func (o *Op) UnmarshalBinary(data []byte) error {
	if len(data) < 1 {
		return ErrMalformedOp
	}
	switch data[0] {
	case tagInsert:
		rest := data[1:]
		prev, n, err := readNodeID(rest)
		if err != nil {
			return err
		}
		rest = rest[n:]
		nodeID, n, err := readNodeID(rest)
		if err != nil {
			return err
		}
		rest = rest[n:]
		if len(rest) != 4 {
			return ErrMalformedOp
		}
		char := rune(int32(binary.BigEndian.Uint32(rest)))
		*o = Op{
			Action: OpInsert,
			PrevID: prev,
			Node:   Node{ID: nodeID, Char: char},
		}
		return nil
	case tagDelete:
		id, n, err := readNodeID(data[1:])
		if err != nil {
			return err
		}
		if n != len(data)-1 {
			return ErrMalformedOp
		}
		*o = Op{Action: OpDelete, Node: Node{ID: id}}
		return nil
	}
	return ErrMalformedOp
}

func checkClientID(s string) error {
	if len(s) > 255 {
		return fmt.Errorf("crdt: ClientID length %d exceeds 255", len(s))
	}
	return nil
}

func nodeIDSize(id NodeID) int { return 8 + 1 + len(id.ClientID) }

func appendNodeID(buf []byte, id NodeID) []byte {
	buf = binary.BigEndian.AppendUint64(buf, uint64(id.Timestamp))
	buf = append(buf, byte(len(id.ClientID)))
	buf = append(buf, id.ClientID...)
	return buf
}

func readNodeID(data []byte) (NodeID, int, error) {
	if len(data) < 9 {
		return NodeID{}, 0, ErrMalformedOp
	}
	ts := int64(binary.BigEndian.Uint64(data[:8]))
	clen := int(data[8])
	if len(data) < 9+clen {
		return NodeID{}, 0, ErrMalformedOp
	}
	return NodeID{Timestamp: ts, ClientID: string(data[9 : 9+clen])}, 9 + clen, nil
}
