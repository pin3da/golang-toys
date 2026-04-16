package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/google/go-cmp/cmp"

	"crdt-practice/crdt"
)

// seed builds an RGA from s using clientID "T" and returns it alongside the
// NodeIDs of each inserted rune, indexed by position.
func seed(t *testing.T, s string) (*crdt.RGA, []crdt.NodeID) {
	t.Helper()
	r := crdt.NewRGA()
	prev := crdt.NodeID{}
	ids := make([]crdt.NodeID, 0, len(s))
	for _, c := range s {
		prev = r.Insert(prev, c, "T")
		ids = append(ids, prev)
	}
	return r, ids
}

func TestHandleKey_InsertRuneAtStart(t *testing.T) {
	r, _ := seed(t, "bc")
	var got []crdt.Op
	broadcast := func(op crdt.Op) { got = append(got, op) }

	cursor := handleKey(tcell.KeyRune, 'a', r, "T", broadcast, crdt.NodeID{})

	if diff := cmp.Diff([]rune("abc"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
	if cursor == (crdt.NodeID{}) {
		t.Errorf("cursor should point to the new 'a', got zero NodeID")
	}
	if len(got) != 1 || got[0].Action != crdt.OpInsert || got[0].Node.Char != 'a' || got[0].PrevID != (crdt.NodeID{}) {
		t.Errorf("broadcast mismatch: got %+v", got)
	}
}

func TestHandleKey_InsertRuneInMiddle(t *testing.T) {
	r, ids := seed(t, "ac")
	broadcast := func(crdt.Op) {}

	cursor := handleKey(tcell.KeyRune, 'b', r, "T", broadcast, ids[0])

	if diff := cmp.Diff([]rune("abc"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
	visible := r.VisibleNodes()
	if visible[1].ID != cursor {
		t.Errorf("cursor = %v, want new 'b' id %v", cursor, visible[1].ID)
	}
}

func TestHandleKey_BackspaceAtStartNoop(t *testing.T) {
	r, _ := seed(t, "abc")
	calls := 0
	broadcast := func(crdt.Op) { calls++ }

	cursor := handleKey(tcell.KeyBackspace, 0, r, "T", broadcast, crdt.NodeID{})

	if cursor != (crdt.NodeID{}) {
		t.Errorf("cursor = %v, want zero", cursor)
	}
	if diff := cmp.Diff([]rune("abc"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
	if calls != 0 {
		t.Errorf("broadcast called %d times at start, want 0", calls)
	}
}

func TestHandleKey_BackspaceMiddle(t *testing.T) {
	r, ids := seed(t, "abc")
	var got []crdt.Op
	broadcast := func(op crdt.Op) { got = append(got, op) }

	// cursor after 'b'; backspace should delete 'b' and park cursor after 'a'.
	cursor := handleKey(tcell.KeyBackspace2, 0, r, "T", broadcast, ids[1])

	if diff := cmp.Diff([]rune("ac"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
	if cursor != ids[0] {
		t.Errorf("cursor = %v, want ids[0]=%v", cursor, ids[0])
	}
	if len(got) != 1 || got[0].Action != crdt.OpDelete || got[0].Node.ID != ids[1] {
		t.Errorf("broadcast mismatch: got %+v", got)
	}
}

func TestHandleKey_BackspaceFirstVisible(t *testing.T) {
	r, ids := seed(t, "ab")
	broadcast := func(crdt.Op) {}

	// cursor after 'a'; backspace deletes 'a', cursor falls back to start.
	cursor := handleKey(tcell.KeyBackspace, 0, r, "T", broadcast, ids[0])

	if diff := cmp.Diff([]rune("b"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
	if cursor != (crdt.NodeID{}) {
		t.Errorf("cursor = %v, want zero", cursor)
	}
}

func TestHandleKey_ArrowNavigation(t *testing.T) {
	r, ids := seed(t, "ab")
	calls := 0
	broadcast := func(crdt.Op) { calls++ }

	// Right from start -> first visible.
	c := handleKey(tcell.KeyRight, 0, r, "T", broadcast, crdt.NodeID{})
	if c != ids[0] {
		t.Errorf("Right from zero = %v, want %v", c, ids[0])
	}
	// Right again -> second visible.
	c = handleKey(tcell.KeyRight, 0, r, "T", broadcast, c)
	if c != ids[1] {
		t.Errorf("Right from ids[0] = %v, want %v", c, ids[1])
	}
	// Right at end -> unchanged.
	c = handleKey(tcell.KeyRight, 0, r, "T", broadcast, c)
	if c != ids[1] {
		t.Errorf("Right at end = %v, want unchanged %v", c, ids[1])
	}
	// Left -> back to first.
	c = handleKey(tcell.KeyLeft, 0, r, "T", broadcast, c)
	if c != ids[0] {
		t.Errorf("Left from ids[1] = %v, want %v", c, ids[0])
	}
	// Left from first visible -> zero.
	c = handleKey(tcell.KeyLeft, 0, r, "T", broadcast, c)
	if c != (crdt.NodeID{}) {
		t.Errorf("Left from ids[0] = %v, want zero", c)
	}
	// Left at start -> zero.
	c = handleKey(tcell.KeyLeft, 0, r, "T", broadcast, c)
	if c != (crdt.NodeID{}) {
		t.Errorf("Left from zero = %v, want zero", c)
	}

	if calls != 0 {
		t.Errorf("navigation broadcast %d times, want 0", calls)
	}
}

func TestHandleKey_UnknownKeyIsNoop(t *testing.T) {
	r, ids := seed(t, "a")
	calls := 0
	broadcast := func(crdt.Op) { calls++ }

	cursor := handleKey(tcell.KeyF1, 0, r, "T", broadcast, ids[0])

	if cursor != ids[0] {
		t.Errorf("cursor = %v, want unchanged %v", cursor, ids[0])
	}
	if diff := cmp.Diff([]rune("a"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
	if calls != 0 {
		t.Errorf("unknown key broadcast %d times, want 0", calls)
	}
}
