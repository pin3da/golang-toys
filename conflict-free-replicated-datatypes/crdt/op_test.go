package crdt

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestOpWireFormat_Roundtrip(t *testing.T) {
	tests := []struct {
		name string
		op   Op
	}{
		{
			name: "insert after root",
			op: Op{
				Action: OpInsert,
				PrevID: NodeID{},
				Node:   Node{ID: NodeID{Timestamp: 1, ClientID: "A"}, Char: 'a'},
			},
		},
		{
			name: "insert with long uuid client",
			op: Op{
				Action: OpInsert,
				PrevID: NodeID{Timestamp: 1_700_000_000_000_000_000, ClientID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
				Node:   Node{ID: NodeID{Timestamp: 1_700_000_000_000_000_001, ClientID: "ffffffff-0000-1111-2222-333333333333"}, Char: 'Z'},
			},
		},
		{
			name: "insert non-ascii rune",
			op: Op{
				Action: OpInsert,
				PrevID: NodeID{Timestamp: 42, ClientID: "x"},
				Node:   Node{ID: NodeID{Timestamp: 43, ClientID: "y"}, Char: 'ñ'},
			},
		},
		{
			name: "delete",
			op: Op{
				Action: OpDelete,
				Node:   Node{ID: NodeID{Timestamp: 99, ClientID: "peer"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf, err := tc.op.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary(%+v) error = %v", tc.op, err)
			}
			var got Op
			if err := got.UnmarshalBinary(buf); err != nil {
				t.Fatalf("UnmarshalBinary error = %v", err)
			}
			// Tombstone is not on the wire; compare the transmitted fields.
			want := tc.op
			want.Node.Tombstone = false
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("roundtrip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOpWireFormat_RejectsGarbage(t *testing.T) {
	cases := map[string][]byte{
		"empty":           {},
		"unknown tag":     {0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		"truncated":       {tagInsert, 0, 0, 0, 0, 0, 0, 0, 1},
		"trailing bytes":  append(append([]byte{tagDelete}, encodeNodeIDFor(t, NodeID{Timestamp: 1, ClientID: "a"})...), 0xAA),
		"insert no rune":  append([]byte{tagInsert}, encodeNodeIDFor(t, NodeID{})...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			var op Op
			if err := op.UnmarshalBinary(data); err == nil {
				t.Errorf("UnmarshalBinary(%x) err = nil, want error", data)
			}
		})
	}
}

func TestOpWireFormat_RejectsOversizedClientID(t *testing.T) {
	op := Op{
		Action: OpDelete,
		Node:   Node{ID: NodeID{Timestamp: 1, ClientID: strings.Repeat("x", 256)}},
	}
	if _, err := op.MarshalBinary(); err == nil {
		t.Errorf("MarshalBinary with 256-byte ClientID err = nil, want error")
	}
}

func encodeNodeIDFor(t *testing.T, id NodeID) []byte {
	t.Helper()
	return appendNodeID(nil, id)
}
