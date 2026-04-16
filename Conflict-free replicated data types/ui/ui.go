// Package ui is the tcell-backed terminal front end for the collaborative
// editor. It owns the screen, translates key events into RGA mutations, and
// renders the RGA contents on every repaint.
package ui

import (
	"sync/atomic"

	"github.com/gdamore/tcell/v2"

	"crdt-practice/crdt"
)

// BroadcastFunc ships a locally produced [crdt.Op] to peers. The UI calls it
// synchronously after applying the op to the local RGA; implementations must
// not block.
type BroadcastFunc func(op crdt.Op)

// activeScreen holds the screen owned by the currently running [Start], or
// nil otherwise. Accessed by [PostRefresh] from arbitrary goroutines.
var activeScreen atomic.Pointer[tcell.Screen]

// Start runs the tcell event loop until the user hits Ctrl-C or the screen
// errors out. It takes ownership of stdin/stdout drawing.
//
// rga is the shared document; Start mutates it on local key events and reads
// it on every repaint. clientID is stamped into every locally minted NodeID.
// broadcast is invoked after each local mutation; pass a no-op to run offline.
//
// Not safe for concurrent use; call once from the main goroutine. Remote ops
// applied by other goroutines must trigger a repaint via [PostRefresh] and
// serialize their own RGA access against the UI goroutine.
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
		case *tcell.EventInterrupt:
			// Posted by PostRefresh; just redraw.
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC {
				return nil
			}
			cursor = handleKey(ev.Key(), ev.Rune(), rga, clientID, broadcast, cursor)
		}
		draw(screen, rga, cursor)
	}
}

// PostRefresh asks the running UI to repaint. Safe to call from any goroutine;
// a no-op if [Start] has not been called or has already returned.
func PostRefresh() {
	sp := activeScreen.Load()
	if sp == nil {
		return
	}
	(*sp).PostEvent(tcell.NewEventInterrupt(nil))
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
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if cursor == (crdt.NodeID{}) {
			return cursor
		}
		deleted := cursor
		newCursor := prevVisible(rga, cursor)
		rga.Delete(deleted)
		broadcast(crdt.Op{Action: crdt.OpDelete, Node: crdt.Node{ID: deleted}})
		return newCursor
	case tcell.KeyRune:
		id := rga.Insert(cursor, r, clientID)
		broadcast(crdt.Op{
			Action: crdt.OpInsert,
			PrevID: cursor,
			Node:   crdt.Node{ID: id, Char: r},
		})
		return id
	}
	return cursor
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

// draw renders the document on row 0 and places the terminal cursor one cell
// past the anchor node. Content past the terminal width is clipped.
func draw(screen tcell.Screen, rga *crdt.RGA, cursor crdt.NodeID) {
	screen.Clear()
	width, _ := screen.Size()
	visible := rga.VisibleNodes()

	cursorX := 0
	for i, n := range visible {
		if i >= width {
			break
		}
		screen.SetContent(i, 0, n.Char, nil, tcell.StyleDefault)
		if n.ID == cursor {
			cursorX = i + 1
		}
	}
	if cursorX > width {
		cursorX = width
	}
	screen.ShowCursor(cursorX, 0)
	screen.Show()
}
