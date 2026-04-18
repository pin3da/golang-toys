package fixedwindow_test

import (
	"sync"
	"testing"
	"time"

	"github.com/pin3da/golang-toys/rate-limiter/fixedwindow"
)

func TestAllowWithinLimit(t *testing.T) {
	l := fixedwindow.New(3, time.Second)
	now := time.Unix(100, 0)
	for i := range 3 {
		if !l.Allow("k", now) {
			t.Fatalf("Allow(k, %v) #%d = false, want true", now, i+1)
		}
	}
}

func TestRejectsOverLimit(t *testing.T) {
	l := fixedwindow.New(2, time.Second)
	now := time.Unix(100, 0)
	l.Allow("k", now)
	l.Allow("k", now)
	if l.Allow("k", now) {
		t.Fatalf("Allow(k) past limit = true, want false")
	}
}

func TestWindowRollover(t *testing.T) {
	l := fixedwindow.New(1, time.Second)
	t0 := time.Unix(100, 0)
	if !l.Allow("k", t0) {
		t.Fatalf("Allow at t0 = false, want true")
	}
	if l.Allow("k", t0.Add(500*time.Millisecond)) {
		t.Fatalf("Allow inside same window = true, want false")
	}
	if !l.Allow("k", t0.Add(time.Second)) {
		t.Fatalf("Allow in next window = false, want true")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := fixedwindow.New(1, time.Second)
	now := time.Unix(100, 0)
	if !l.Allow("a", now) || !l.Allow("b", now) {
		t.Fatalf("independent keys must each get their own budget")
	}
	if l.Allow("a", now) {
		t.Fatalf("key a should be exhausted")
	}
}

// TestConcurrentAllowDoesNotExceedLimit validates that the mutex prevents
// overcounting under concurrent access: with limit N, at most N calls in a
// single window may return true, regardless of goroutine count.
func TestConcurrentAllowDoesNotExceedLimit(t *testing.T) {
	const limit = 100
	const goroutines = 500
	l := fixedwindow.New(limit, time.Hour)
	now := time.Unix(100, 0)

	var wg sync.WaitGroup
	var allowed int64
	var mu sync.Mutex
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
