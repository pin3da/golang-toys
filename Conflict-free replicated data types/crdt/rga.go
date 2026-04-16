package crdt

type NodeID struct {
	Timestamp int64
	ClientID  string
}

type Node struct {
	ID        NodeID
	Char      rune
	Tombstone bool
}

type RGA struct {
}

func NewRGA() *RGA {
	panic("not implemented")
}

func (r *RGA) Insert(prevID NodeID, char rune, clientID string) NodeID {
	panic("not implemented")
}

func (r *RGA) Delete(id NodeID) {
	panic("not implemented")
}

func (r *RGA) Merge(remoteNodes []Node) {
	panic("not implemented")
}

func (r *RGA) Values() []rune {
	panic("not implemented")
}
