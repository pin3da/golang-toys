# Practice Project: Collaborative Text Editor

CRDT Model: **Replicated Growable Array (RGA)**  
Networking: **ZeroMQ (ZMQ_PAIR or ZMQ_PUB/SUB)**  
UI: **tcell (Terminal Window UI)**  


Goal: Build a terminal-based collaborative text editor using a Sequence CRDT to preserve character order concurrently.

## References

*   **RGA Background**: The Replicated Growable Array (RGA) was originally detailed mathematically by Roh et al. in ["Replicated abstract data types: Building blocks for collaborative applications" (2011)](https://hal.inria.fr/inria-00555588). 
*   **How it works**: Unlike a Set CRDT, a Sequence CRDT must preserve ordering. RGA achieves this by modelling the document as a linked list (or array) of characters. Every single character typed is a `Node` containing the character itself, a boolean `Tombstone` (used instead of actually deleting the node), and a unique `NodeID`. 
*   **ZMQ Guide**: [ZeroMQ Documentation](https://zguide.zeromq.org/)
*   **tcell Reference**: [gdamore/tcell GitHub](https://github.com/gdamore/tcell)

## Dependencies
```bash
go get github.com/pebbe/zmq4
go get github.com/gdamore/tcell/v2
go get github.com/google/uuid
```
*(Requires `libzmq` installed on host system. On Debian/Ubuntu, install it using: `sudo apt-get install libzmq3-dev`)*

## Implementation Goals

### 1. CRDT (`crdt/rga.go`)
*   **Node IDs**: A `NodeID` should combine a logical timestamp (or `time.Now().UnixNano()`) and a unique Client ID.
*   **Insertion**: When a user types a new character, insert it strictly *after* the `NodeID` where the cursor was located. If two peers insert after the *exact same* node concurrently, sort the ties based on `NodeID` (e.g. highest timestamp wins, break ties using Client ID lexicographically) so both users converge.
*   **Deletion (Tombstoning)**: When a backspace occurs, find the node before the cursor's current position and flip its `Tombstone` value to `true`. Never remove the node from the array. 

### 2. Networking (`server/server.go`)
*   **Sockets**: Use `pebbe/zmq4` to start either a `ZMQ_PAIR` socket strictly between you and a peer, or a `ZMQ_PUB`/`ZMQ_SUB` architecture. 
*   **Operations**: When `ui.go` records an insertion or a delete, serialize the minimal changes (e.g., `{Action: "INSERT", PrevID: ..., Node: ...}`) and send it over ZeroMQ. When receiving changes, deserialize and call the corresponding methods precisely on `localRGA`. 

### 3. Interface (`ui/ui.go`)
*   **Drawing**: Write a `Draw()` loop that iterates over your RGA document, ignoring nodes where `Tombstone == true`, and printing them sequentially to `tcell`'s screen buffer.
*   **Event Handling**: Catch key presses via `tcell.EventKey`.
    * If an arrow key: move your internal `cursor` relative to the *visible* (non-tombstoned) nodes.
    * If a standard Rune: Call `RGA.Insert(...)` and broadcast to network.
    * If a Backspace: Call `RGA.Delete(...)` and broadcast to network.

## Future improvements

Rough plan for the next iterations; each item is independent and
can be tackled on its own.

### CRDT / correctness
*   **Causal delivery**: today ops are applied in arrival order. An insert whose `PrevID` has not arrived yet will panic in `insertAfter`. Buffer such ops until their dependency is present, or promote `insertAfter` to tolerate unknown prev by queuing.
*   **Compaction**: tombstoned nodes accumulate forever. Add a causally-stable GC pass that drops tombstones once every peer has observed them (requires a version vector or similar).
*   **Undo/redo**: a local ring buffer of locally produced ops, rewound by issuing compensating ops.

### Networking
*   **State sync for late joiners**: PUB/SUB drops messages sent before a SUB connects. Add a JOIN handshake (REQ/REP or DEALER/ROUTER) that dumps the full RGA on connect so a third peer can catch up.
*   **N-peer mesh**: `--peer` currently takes a single address. Accept a comma-separated list and open one SUB per peer.
*   **Back-pressure**: tune ZMQ high-water marks and decide whether to block or drop when a peer is slow.
*   **Graceful shutdown**: close sockets and cancel the recv goroutine on Ctrl-C instead of relying on process exit.
*   **Wire-format versioning**: reserve a version byte at the head of every message so the format can evolve without breaking existing peers.

### UI
*   **Long-line handling**: lines past the terminal width clip today. Choose between soft-wrap (render next row) and horizontal scroll (shift viewport to keep cursor visible).
*   **Sticky column for Up/Down**: remember the user's desired column across successive vertical moves so a Down through a short line followed by Down does not collapse the column permanently.
*   **Home/End, PageUp/PageDown, word-wise navigation** (Ctrl+Left / Ctrl+Right).
*   **Selection and clipboard**: visual selection with Shift+arrows, copy/cut/paste that emits batched ops.
*   **Peer presence**: render remote cursors using their ClientID as a colour; requires broadcasting a lightweight cursor-position op separately from edits.
*   **Status line**: filename, peer count, last error, dirty indicator.

### Persistence
*   **Load/save**: read an initial document into the RGA on startup and flush the current state to disk on quit. Treat the on-disk file as a plain-text snapshot, not a log.
*   **Op log**: append every local op to a log file so a crashed session can be replayed.
