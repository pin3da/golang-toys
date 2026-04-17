package crdt

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNewRGA_EmptyValues(t *testing.T) {
	r := NewRGA()
	if diff := cmp.Diff([]rune{}, r.Values()); diff != "" {
		t.Errorf("NewRGA().Values() mismatch (-want +got):\n%s", diff)
	}
}

func TestInsert_Sequential(t *testing.T) {
	r := NewRGA()
	prev := NodeID{}
	for _, c := range "hello" {
		prev = r.Insert(prev, c, "A")
	}
	if diff := cmp.Diff([]rune("hello"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
}

func TestInsert_AtHead(t *testing.T) {
	r := NewRGA()
	tail := r.Insert(NodeID{}, 'b', "A")
	r.Insert(tail, 'c', "A")
	r.Insert(NodeID{}, 'a', "A")
	if diff := cmp.Diff([]rune("abc"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
}

func TestDelete_Tombstones(t *testing.T) {
	r := NewRGA()
	prev := NodeID{}
	ids := make([]NodeID, 0, 5)
	for _, c := range "hello" {
		prev = r.Insert(prev, c, "A")
		ids = append(ids, prev)
	}
	r.Delete(ids[1]) // delete first 'e'
	r.Delete(ids[1]) // idempotent
	r.Delete(NodeID{Timestamp: 999, ClientID: "ghost"})
	if diff := cmp.Diff([]rune("hllo"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
}

// TestRemoteInsert_ConcurrentSiblings exercises the RGA tie-break rule: two
// peers insert after the same prevID with overlapping timestamps; the higher
// (Timestamp, ClientID) must appear first, regardless of arrival order.
func TestRemoteInsert_ConcurrentSiblings(t *testing.T) {
	build := func(applyOrder []int) []rune {
		r := NewRGA()
		root := NodeID{}
		// Two concurrent inserts after root with the same timestamp;
		// ClientID breaks the tie. "B" > "A", so 'b' must come first.
		ops := []struct {
			prev NodeID
			node Node
		}{
			{root, Node{ID: NodeID{Timestamp: 10, ClientID: "A"}, Char: 'a'}},
			{root, Node{ID: NodeID{Timestamp: 10, ClientID: "B"}, Char: 'b'}},
		}
		for _, i := range applyOrder {
			r.RemoteInsert(ops[i].prev, ops[i].node)
		}
		return r.Values()
	}
	want := []rune("ba")
	if diff := cmp.Diff(want, build([]int{0, 1})); diff != "" {
		t.Errorf("apply A then B mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, build([]int{1, 0})); diff != "" {
		t.Errorf("apply B then A mismatch (-want +got):\n%s", diff)
	}
}

func TestVisibleNodes_SkipsTombstones(t *testing.T) {
	r := NewRGA()
	prev := NodeID{}
	var ids []NodeID
	for _, c := range "abc" {
		prev = r.Insert(prev, c, "A")
		ids = append(ids, prev)
	}
	r.Delete(ids[1]) // tombstone 'b'
	got := r.VisibleNodes()
	if len(got) != 2 || got[0].Char != 'a' || got[1].Char != 'c' {
		t.Fatalf("VisibleNodes = %+v, want [a c]", got)
	}
	if got[0].ID != ids[0] || got[1].ID != ids[2] {
		t.Errorf("VisibleNodes IDs mismatch: got %v, want [%v %v]", []NodeID{got[0].ID, got[1].ID}, ids[0], ids[2])
	}
}

func TestRemoteInsert_Idempotent(t *testing.T) {
	r := NewRGA()
	n := Node{ID: NodeID{Timestamp: 5, ClientID: "X"}, Char: 'x'}
	r.RemoteInsert(NodeID{}, n)
	r.RemoteInsert(NodeID{}, n)
	if diff := cmp.Diff([]rune("x"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
}
