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

func TestHandleKey_EnterInsertsNewline(t *testing.T) {
	r, ids := seed(t, "ab")
	var got []crdt.Op
	broadcast := func(op crdt.Op) { got = append(got, op) }

	cursor := handleKey(tcell.KeyEnter, 0, r, "T", broadcast, ids[1])

	if diff := cmp.Diff([]rune("ab\n"), r.Values()); diff != "" {
		t.Errorf("Values mismatch (-want +got):\n%s", diff)
	}
	visible := r.VisibleNodes()
	if visible[2].ID != cursor || visible[2].Char != '\n' {
		t.Errorf("cursor = %v, want new '\\n' id %v", cursor, visible[2].ID)
	}
	if len(got) != 1 || got[0].Action != crdt.OpInsert || got[0].Node.Char != '\n' {
		t.Errorf("broadcast mismatch: got %+v", got)
	}
}

// posOf finds the NodeID of the i-th visible rune (0-indexed). Useful for
// addressing nodes in a multi-line buffer without plumbing ids out of seed.
func posOf(r *crdt.RGA, i int) crdt.NodeID {
	return r.VisibleNodes()[i].ID
}

func TestHandleKey_DownPreservesColumnWhenFits(t *testing.T) {
	// "abc\ndefgh"; cursor after 'b' (line 0, col 2). Down -> line 1, col 2
	// which is after 'e'.
	r, _ := seed(t, "abc\ndefgh")
	broadcast := func(crdt.Op) {}

	start := posOf(r, 1) // 'b'
	want := posOf(r, 5)  // 'e' (index 0:'a' 1:'b' 2:'c' 3:'\n' 4:'d' 5:'e')

	got := handleKey(tcell.KeyDown, 0, r, "T", broadcast, start)
	if got != want {
		t.Errorf("Down cursor = %v, want %v", got, want)
	}
}

func TestHandleKey_DownClampsToShorterLine(t *testing.T) {
	// "abcde\nxy"; cursor after 'd' (line 0, col 4). Down -> line 1 end
	// (after 'y', col 2).
	r, _ := seed(t, "abcde\nxy")
	broadcast := func(crdt.Op) {}

	start := posOf(r, 3) // 'd'
	want := posOf(r, 7)  // 'y' (0:'a'1:'b'2:'c'3:'d'4:'e'5:'\n'6:'x'7:'y')

	got := handleKey(tcell.KeyDown, 0, r, "T", broadcast, start)
	if got != want {
		t.Errorf("Down cursor = %v, want %v", got, want)
	}
}

func TestHandleKey_UpPreservesColumn(t *testing.T) {
	// "abcd\nwxyz"; cursor after 'y' (line 1, col 3). Up -> line 0 col 3
	// which is after 'c'.
	r, _ := seed(t, "abcd\nwxyz")
	broadcast := func(crdt.Op) {}

	start := posOf(r, 7) // 'y'
	want := posOf(r, 2)  // 'c'

	got := handleKey(tcell.KeyUp, 0, r, "T", broadcast, start)
	if got != want {
		t.Errorf("Up cursor = %v, want %v", got, want)
	}
}

func TestHandleKey_UpAtTopStays(t *testing.T) {
	r, ids := seed(t, "abc")
	broadcast := func(crdt.Op) {}

	got := handleKey(tcell.KeyUp, 0, r, "T", broadcast, ids[1])
	if got != ids[1] {
		t.Errorf("Up at top = %v, want unchanged %v", got, ids[1])
	}
}

func TestHandleKey_DownAtBottomStays(t *testing.T) {
	r, _ := seed(t, "ab\ncd")
	broadcast := func(crdt.Op) {}

	start := posOf(r, 4) // 'd'
	got := handleKey(tcell.KeyDown, 0, r, "T", broadcast, start)
	if got != start {
		t.Errorf("Down at bottom = %v, want unchanged %v", got, start)
	}
}

func TestHandleKey_UpToStartOfLine0(t *testing.T) {
	// "ab\ncd"; cursor at start of line 1 (after '\n'). Up with col 0 on
	// line 0 has no node at col <= 0, so cursor must collapse to zero.
	r, _ := seed(t, "ab\ncd")
	broadcast := func(crdt.Op) {}

	start := posOf(r, 2) // '\n'; cursor after '\n' is (line 1, col 0)

	got := handleKey(tcell.KeyUp, 0, r, "T", broadcast, start)
	if got != (crdt.NodeID{}) {
		t.Errorf("Up to col 0 of line 0 = %v, want zero NodeID", got)
	}
}

func TestHandleKey_DownThroughEmptyLine(t *testing.T) {
	// "ab\n\ncd"; cursor after 'b' (line 0, col 2). Down -> line 1 col 0
	// (the first '\n', which anchors the cursor at start of line 1).
	r, _ := seed(t, "ab\n\ncd")
	broadcast := func(crdt.Op) {}

	start := posOf(r, 1) // 'b'
	want := posOf(r, 2)  // first '\n'; anchor here is (line 1, col 0)

	got := handleKey(tcell.KeyDown, 0, r, "T", broadcast, start)
	if got != want {
		t.Errorf("Down onto empty line = %v, want %v", got, want)
	}
}

func TestApplyRemote_InsertThenDelete(t *testing.T) {
	r := crdt.NewRGA()
	id := crdt.NodeID{Timestamp: 1, ClientID: "peer"}
	applyRemote(r, crdt.Op{
		Action: crdt.OpInsert,
		PrevID: crdt.NodeID{},
		Node:   crdt.Node{ID: id, Char: 'z'},
	})
	if diff := cmp.Diff([]rune("z"), r.Values()); diff != "" {
		t.Errorf("after remote insert (-want +got):\n%s", diff)
	}
	applyRemote(r, crdt.Op{Action: crdt.OpDelete, Node: crdt.Node{ID: id}})
	if diff := cmp.Diff([]rune{}, r.Values()); diff != "" {
		t.Errorf("after remote delete (-want +got):\n%s", diff)
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
