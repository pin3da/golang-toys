// Package ui is the tcell-backed terminal front end for the collaborative
// editor. It owns the screen, translates key events into RGA mutations, and
// renders the RGA contents on every repaint.
package ui

import (
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"

	"crdt-practice/crdt"
)

// BroadcastFunc ships a locally produced [crdt.Op] to peers. The UI calls it
// synchronously after applying the op to the local RGA; implementations must
// not block.
type BroadcastFunc func(op crdt.Op)

// remoteOpEvent carries a peer op from a network goroutine into the UI event
// loop, where it is applied to the local RGA. This keeps every RGA mutation
// on the UI goroutine so no locking is required.
type remoteOpEvent struct {
	t  time.Time
	op crdt.Op
}

// When implements [tcell.Event].
func (e *remoteOpEvent) When() time.Time { return e.t }

// activeScreen holds the screen owned by the currently running [Start], or
// nil otherwise. Accessed by [PostRemoteOp] from arbitrary goroutines.
var activeScreen atomic.Pointer[tcell.Screen]

// Start runs the tcell event loop until the user hits Ctrl-C or the screen
// errors out. It takes ownership of stdin/stdout drawing.
//
// rga is the shared document; Start mutates it on local key events and on
// remote ops delivered via [PostRemoteOp]. clientID is stamped into every
// locally minted NodeID. broadcast is invoked after each local mutation;
// pass a no-op to run offline.
//
// Not safe for concurrent use; call once from the main goroutine. Peer
// goroutines must route remote ops through [PostRemoteOp], not touch rga
// directly.
func Start(rga *crdt.RGA, clientID string, broadcast BroadcastFunc) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()

	activeScreen.Store(&screen)
	defer activeScreen.Store(nil)

	// cursor is the NodeID the cursor sits immediately after. The zero
	// NodeID means the cursor is at the very start of the document.
	var cursor crdt.NodeID

	draw(screen, rga, cursor)
	for {
		switch ev := screen.PollEvent().(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *remoteOpEvent:
			applyRemote(rga, ev.op)
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC {
				return nil
			}
			cursor = handleKey(ev.Key(), ev.Rune(), rga, clientID, broadcast, cursor)
		}
		draw(screen, rga, cursor)
	}
}

// PostRemoteOp queues op for application on the UI goroutine. Safe to call
// from any goroutine; a no-op if [Start] has not been called or has already
// returned.
func PostRemoteOp(op crdt.Op) {
	sp := activeScreen.Load()
	if sp == nil {
		return
	}
	(*sp).PostEvent(&remoteOpEvent{t: time.Now(), op: op})
}

// applyRemote replays op against rga. Unknown actions are ignored; the
// underlying [crdt.RGA] methods are already idempotent.
func applyRemote(rga *crdt.RGA, op crdt.Op) {
	switch op.Action {
	case crdt.OpInsert:
		rga.RemoteInsert(op.PrevID, op.Node)
	case crdt.OpDelete:
		rga.Delete(op.Node.ID)
	}
}

// handleKey applies the local mutation implied by (key, r) and returns the
// new cursor anchor. r is only consulted when key == [tcell.KeyRune].
// Unknown keys leave cursor and rga unchanged.
func handleKey(key tcell.Key, r rune, rga *crdt.RGA, clientID string, broadcast BroadcastFunc, cursor crdt.NodeID) crdt.NodeID {
	switch key {
	case tcell.KeyLeft:
		return prevVisible(rga, cursor)
	case tcell.KeyRight:
		return nextVisible(rga, cursor)
	case tcell.KeyUp:
		return moveByLine(rga, cursor, -1)
	case tcell.KeyDown:
		return moveByLine(rga, cursor, +1)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if cursor == (crdt.NodeID{}) {
			return cursor
		}
		deleted := cursor
		newCursor := prevVisible(rga, cursor)
		rga.Delete(deleted)
		broadcast(crdt.Op{Action: crdt.OpDelete, Node: crdt.Node{ID: deleted}})
		return newCursor
	case tcell.KeyEnter:
		return insertRune(rga, clientID, broadcast, cursor, '\n')
	case tcell.KeyRune:
		return insertRune(rga, clientID, broadcast, cursor, r)
	}
	return cursor
}

// insertRune inserts r after cursor, broadcasts the op, and returns the new
// anchor (the id of the freshly inserted node).
func insertRune(rga *crdt.RGA, clientID string, broadcast BroadcastFunc, cursor crdt.NodeID, r rune) crdt.NodeID {
	id := rga.Insert(cursor, r, clientID)
	broadcast(crdt.Op{
		Action: crdt.OpInsert,
		PrevID: cursor,
		Node:   crdt.Node{ID: id, Char: r},
	})
	return id
}

// prevVisible returns the NodeID of the visible node immediately before
// cursor, or the zero NodeID if cursor is at (or before) the first visible
// node.
func prevVisible(rga *crdt.RGA, cursor crdt.NodeID) crdt.NodeID {
	if cursor == (crdt.NodeID{}) {
		return cursor
	}
	visible := rga.VisibleNodes()
	for i, n := range visible {
		if n.ID == cursor {
			if i == 0 {
				return crdt.NodeID{}
			}
			return visible[i-1].ID
		}
	}
	// Cursor anchor was tombstoned out from under us; fall back to start.
	return crdt.NodeID{}
}

// nextVisible returns the NodeID of the visible node immediately after
// cursor, or cursor unchanged if it is already at the end.
func nextVisible(rga *crdt.RGA, cursor crdt.NodeID) crdt.NodeID {
	visible := rga.VisibleNodes()
	if len(visible) == 0 {
		return cursor
	}
	if cursor == (crdt.NodeID{}) {
		return visible[0].ID
	}
	for i, n := range visible {
		if n.ID == cursor && i+1 < len(visible) {
			return visible[i+1].ID
		}
	}
	return cursor
}

// logicalPos is the (line, column) where the cursor sits when anchored to a
// given node. Lines are split on '\n' runes; column is the count of non-\n
// runes on the line preceding the cursor.
type logicalPos struct{ line, col int }

// layout computes the logical cursor position for every visible node. The
// returned slice is parallel to visible: positions[i] is where the cursor
// lands when anchored to visible[i].
func layout(visible []crdt.Node) []logicalPos {
	out := make([]logicalPos, len(visible))
	line, col := 0, 0
	for i, n := range visible {
		if n.Char == '\n' {
			line++
			col = 0
		} else {
			col++
		}
		out[i] = logicalPos{line, col}
	}
	return out
}

// currentPos returns the logical position of cursor. The zero NodeID maps to
// the start of the document. An unknown cursor (tombstoned out of view) also
// maps to the start.
func currentPos(visible []crdt.Node, positions []logicalPos, cursor crdt.NodeID) logicalPos {
	if cursor == (crdt.NodeID{}) {
		return logicalPos{0, 0}
	}
	for i, n := range visible {
		if n.ID == cursor {
			return positions[i]
		}
	}
	return logicalPos{0, 0}
}

// moveByLine returns the anchor for moving the cursor delta lines (negative
// for up, positive for down) while staying as close to the current column as
// the target line allows. Returns cursor unchanged if the target line does
// not exist.
func moveByLine(rga *crdt.RGA, cursor crdt.NodeID, delta int) crdt.NodeID {
	visible := rga.VisibleNodes()
	positions := layout(visible)
	cur := currentPos(visible, positions, cursor)
	target := cur.line + delta
	if target < 0 {
		return cursor
	}

	bestIdx := -1
	bestCol := -1
	targetExists := false
	for i, p := range positions {
		if p.line != target {
			continue
		}
		targetExists = true
		if p.col <= cur.col && p.col > bestCol {
			bestCol = p.col
			bestIdx = i
		}
	}
	if !targetExists {
		return cursor
	}
	if bestIdx < 0 {
		// Target line exists but every node on it has col > cur.col, which
		// can only happen for line 0 (no preceding \n seeds a col=0 anchor).
		return crdt.NodeID{}
	}
	return visible[bestIdx].ID
}

// draw renders the document starting at row 0 and places the terminal cursor
// immediately after the anchor node. Newline runes advance to the next row;
// content past the terminal width is clipped on its row.
func draw(screen tcell.Screen, rga *crdt.RGA, cursor crdt.NodeID) {
	screen.Clear()
	width, _ := screen.Size()
	visible := rga.VisibleNodes()

	x, y := 0, 0
	cursorX, cursorY := 0, 0
	for _, n := range visible {
		if n.Char == '\n' {
			y++
			x = 0
		} else {
			if x < width {
				screen.SetContent(x, y, n.Char, nil, tcell.StyleDefault)
			}
			x++
		}
		if n.ID == cursor {
			cursorX, cursorY = x, y
		}
	}
	if cursorX > width {
		cursorX = width
	}
	screen.ShowCursor(cursorX, cursorY)
	screen.Show()
}
