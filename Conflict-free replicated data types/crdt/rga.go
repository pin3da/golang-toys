// Package crdt implements a Replicated Growable Array (RGA), a sequence CRDT
// that preserves character order across concurrent edits by multiple peers.
package crdt

import "time"

// less reports whether a sorts strictly before b in the total NodeID order:
// smaller Timestamp first, ties broken by lexicographic ClientID.
func less(a, b NodeID) bool {
	if a.Timestamp != b.Timestamp {
		return a.Timestamp < b.Timestamp
	}
	return a.ClientID < b.ClientID
}

// NodeID uniquely identifies a Node in the RGA. The pair (Timestamp, ClientID)
// is globally unique and provides a total order: higher Timestamp wins, ties
// are broken by lexicographic ClientID.
type NodeID struct {
	Timestamp int64
	ClientID  string
}

// Node is a single character in the RGA document. A node is never removed once
// inserted; deletion is represented by setting Tombstone to true.
type Node struct {
	ID        NodeID
	Char      rune
	Tombstone bool
}

// element is the internal doubly linked list cell wrapping a Node.
type element struct {
	node       Node
	prev, next *element
}

// RGA is a replicated sequence of runes. It is not safe for concurrent use;
// callers must serialize access externally.
type RGA struct {
	// root is a sentinel head whose ID is the zero NodeID. Every visible
	// element is reachable by walking root.next.
	root *element
	// index maps every known NodeID (including tombstoned ones) to its
	// element for O(1) lookup during insert and delete.
	index map[NodeID]*element
	// lastIssued is the timestamp of the most recent locally minted NodeID.
	// It guards against two rapid Insert calls producing the same nanosecond
	// on platforms where time.Now().UnixNano() has coarse resolution: the
	// next local timestamp is max(time.Now().UnixNano(), lastIssued+1).
	// Remote timestamps do not advance it; this is a uniqueness guard, not
	// a causal clock.
	lastIssued int64
}

// NewRGA returns an empty RGA with only the sentinel head element.
func NewRGA() *RGA {
	root := &element{}
	return &RGA{
		root:  root,
		index: map[NodeID]*element{{}: root},
	}
}

// Insert appends a new character locally after the node identified by prevID
// and returns the freshly minted NodeID. Use the zero NodeID to insert at the
// beginning of the document. Panics if prevID is non-zero and unknown.
//
// The new NodeID's timestamp is time.Now().UnixNano(), bumped to lastIssued+1
// if the wall clock has not advanced since the previous local insert.
func (r *RGA) Insert(prevID NodeID, char rune, clientID string) NodeID {
	ts := time.Now().UnixNano()
	if ts <= r.lastIssued {
		ts = r.lastIssued + 1
	}
	r.lastIssued = ts
	id := NodeID{Timestamp: ts, ClientID: clientID}
	r.insertAfter(prevID, Node{ID: id, Char: char})
	return id
}

// RemoteInsert applies an insert observed from a peer. The node's ID must not
// already exist in the RGA; prevID must be the zero NodeID or refer to a known
// node.
func (r *RGA) RemoteInsert(prevID NodeID, node Node) {
	if _, ok := r.index[node.ID]; ok {
		return
	}
	r.insertAfter(prevID, node)
}

// insertAfter splices node into the list immediately after prevID, then walks
// forward past any element whose NodeID sorts above node.ID. This places the
// node according to the RGA tie-breaking rule: among siblings sharing prevID,
// the one with the highest NodeID appears first.
//
// Panics if prevID is unknown.
func (r *RGA) insertAfter(prevID NodeID, node Node) {
	prev, ok := r.index[prevID]
	if !ok {
		panic("crdt: insertAfter with unknown prevID")
	}
	left := prev
	for left.next != nil && less(node.ID, left.next.node.ID) {
		left = left.next
	}
	right := left.next
	e := &element{node: node, prev: left, next: right}
	left.next = e
	if right != nil {
		right.prev = e
	}
	r.index[node.ID] = e
}

// Delete tombstones the node identified by id. Deleting an unknown id or a
// node that is already tombstoned is a no-op so that the operation is
// idempotent across replicas.
func (r *RGA) Delete(id NodeID) {
	e, ok := r.index[id]
	if !ok || e == r.root {
		return
	}
	e.node.Tombstone = true
}

// Values returns the visible runes in document order, skipping tombstoned
// nodes. The returned slice is a fresh copy; callers may modify it freely.
func (r *RGA) Values() []rune {
	out := make([]rune, 0, len(r.index)-1)
	for e := r.root.next; e != nil; e = e.next {
		if e.node.Tombstone {
			continue
		}
		out = append(out, e.node.Char)
	}
	return out
}
