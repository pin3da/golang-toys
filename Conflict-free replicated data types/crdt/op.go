package crdt

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
