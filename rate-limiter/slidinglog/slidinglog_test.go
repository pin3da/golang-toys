package slidinglog_test

import (
	"sync"
	"testing"
	"time"

	"github.com/pin3da/golang-toys/rate-limiter/slidinglog"
)

func TestAllowWithinLimit(t *testing.T) {
	l := slidinglog.New(3, time.Second)
	now := time.Unix(100, 0)
	for i := range 3 {
		if !l.Allow("k", now) {
			t.Fatalf("Allow #%d = false, want true", i+1)
		}
	}
}

func TestRejectsOverLimit(t *testing.T) {
	l := slidinglog.New(2, time.Second)
	now := time.Unix(100, 0)
	l.Allow("k", now)
	l.Allow("k", now)
	if l.Allow("k", now) {
		t.Fatalf("Allow past limit = true, want false")
	}
}

// TestSlidingBoundaryIsExact pins the core property that distinguishes a
// sliding log from a fixed-window counter: exactly window after the oldest
// event expires, one new event becomes available -- not at the next fixed
// boundary.
func TestSlidingBoundaryIsExact(t *testing.T) {
	l := slidinglog.New(2, time.Second)
	t0 := time.Unix(100, 0)
	l.Allow("k", t0)
	l.Allow("k", t0.Add(500*time.Millisecond))

	// Budget is exhausted at t0+600ms; oldest (t0) expires at t0+1s.
	if l.Allow("k", t0.Add(999*time.Millisecond)) {
		t.Fatalf("Allow before oldest expires = true, want false")
	}
	if !l.Allow("k", t0.Add(1001*time.Millisecond)) {
		t.Fatalf("Allow after oldest expires = false, want true")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := slidinglog.New(1, time.Second)
	now := time.Unix(100, 0)
	if !l.Allow("a", now) || !l.Allow("b", now) {
		t.Fatalf("independent keys must each get their own budget")
	}
	if l.Allow("a", now) {
		t.Fatalf("key a should be exhausted")
	}
}

// TestRingBufferWrapsCorrectly exercises many evict/admit cycles on a
// small-limit bucket to ensure the ring buffer's head/tail arithmetic
// stays consistent across wrap-around.
func TestRingBufferWrapsCorrectly(t *testing.T) {
	l := slidinglog.New(3, time.Second)
	t0 := time.Unix(100, 0)
	for i := range 10 {
		ts := t0.Add(time.Duration(i) * 400 * time.Millisecond)
		if !l.Allow("k", ts) {
			t.Fatalf("step %d at %v = false; rolling-limit admits should always succeed", i, ts)
		}
	}
}

func TestConcurrentAllowDoesNotExceedLimit(t *testing.T) {
	const limit = 100
	const goroutines = 500
	l := slidinglog.New(limit, time.Hour)
	now := time.Unix(100, 0)

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("k", now) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != limit {
		t.Fatalf("allowed = %d, want %d", allowed, limit)
	}
}
