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
