package latency

import (
	"math/rand"
	"time"
)

// treapNode is a node in the treap.
type treapNode struct {
	obs      Observation
	priority int
	size     int
	left     *treapNode
	right    *treapNode
}

// Treap is a randomized binary search tree that maintains order statistics.
// It uses latency as the key and a random priority to maintain balance.
type Treap struct {
	root *treapNode
	rng  *rand.Rand
}

// NewTreap creates a new Treap.
func NewTreap() *Treap {
	return &Treap{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Insert adds an observation to the treap.
func (t *Treap) Insert(obs Observation) {
	t.root = t.insert(t.root, obs)
}

// DeleteByTimestamp removes all observations with timestamp <= cutoff.
func (t *Treap) DeleteByTimestamp(cutoff time.Time) {
	t.root = t.deleteByTimestamp(t.root, cutoff)
}

// SelectByRank returns the latency at the given rank (0-indexed) in sorted order.
func (t *Treap) SelectByRank(rank int) time.Duration {
	return selectByRank(t.root, rank)
}

// Size returns the number of nodes in the treap.
func (t *Treap) Size() int {
	return size(t.root)
}

// insert adds a node and returns the new root.
func (t *Treap) insert(n *treapNode, obs Observation) *treapNode {
	if n == nil {
		return newTreapNode(obs, t.rng.Int())
	}

	if obs.Latency < n.obs.Latency {
		n.left = t.insert(n.left, obs)
		if n.left.priority > n.priority {
			n = rotateRight(n)
		}
	} else {
		n.right = t.insert(n.right, obs)
		if n.right.priority > n.priority {
			n = rotateLeft(n)
		}
	}

	n.recalc()
	return n
}

// deleteByTimestamp removes nodes with timestamp <= cutoff.
func (t *Treap) deleteByTimestamp(n *treapNode, cutoff time.Time) *treapNode {
	if n == nil {
		return nil
	}

	n.left = t.deleteByTimestamp(n.left, cutoff)
	n.right = t.deleteByTimestamp(n.right, cutoff)

	if !n.obs.Timestamp.After(cutoff) {
		return merge(n.left, n.right)
	}

	n.recalc()
	return n
}

// newTreapNode creates a node with the given priority.
func newTreapNode(obs Observation, priority int) *treapNode {
	return &treapNode{
		obs:      obs,
		priority: priority,
		size:     1,
	}
}

// recalc updates the size from children.
func (n *treapNode) recalc() {
	n.size = 1 + size(n.left) + size(n.right)
}

// size returns the size of a node's subtree (0 if nil).
func size(n *treapNode) int {
	if n == nil {
		return 0
	}
	return n.size
}

// rotateRight performs a right rotation.
func rotateRight(n *treapNode) *treapNode {
	left := n.left
	n.left = left.right
	left.right = n
	n.recalc()
	left.recalc()
	return left
}

// rotateLeft performs a left rotation.
func rotateLeft(n *treapNode) *treapNode {
	right := n.right
	n.right = right.left
	right.left = n
	n.recalc()
	right.recalc()
	return right
}

// merge combines two treaps where all keys in left are <= all keys in right.
func merge(left, right *treapNode) *treapNode {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}

	if left.priority > right.priority {
		left.right = merge(left.right, right)
		left.recalc()
		return left
	}

	right.left = merge(left, right.left)
	right.recalc()
	return right
}

// selectByRank finds the node at the given rank (0-indexed).
func selectByRank(n *treapNode, rank int) time.Duration {
	if n == nil || rank < 0 || rank >= n.size {
		return 0
	}

	leftSize := size(n.left)

	if rank < leftSize {
		return selectByRank(n.left, rank)
	}
	if rank == leftSize {
		return n.obs.Latency
	}
	return selectByRank(n.right, rank-leftSize-1)
}

// TreapWindowPercentile implements WindowPercentile using a treap.
type TreapWindowPercentile struct {
	window time.Duration
	treap  *Treap
}

// NewTreapWindowPercentile creates a new TreapWindowPercentile.
func NewTreapWindowPercentile(window time.Duration) *TreapWindowPercentile {
	return &TreapWindowPercentile{
		window: window,
		treap:  NewTreap(),
	}
}

// Record adds a new latency observation to the window.
func (twp *TreapWindowPercentile) Record(obs Observation) {
	twp.treap.Insert(obs)
}

// Percentile returns the p-th percentile (0.0 < p < 1.0) of latencies
// currently within the window relative to the provided now time.
func (twp *TreapWindowPercentile) Percentile(p float64, now time.Time) time.Duration {
	cutoff := now.Add(-twp.window)
	twp.treap.DeleteByTimestamp(cutoff)

	sz := twp.treap.Size()
	if sz == 0 {
		return 0
	}

	index := int(float64(sz-1) * p)
	return twp.treap.SelectByRank(index)
}
