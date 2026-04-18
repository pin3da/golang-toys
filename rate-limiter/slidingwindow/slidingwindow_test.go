package slidingwindow_test

import (
	"testing"
	"time"

	"github.com/pin3da/golang-toys/rate-limiter/slidingwindow"
)

func TestAllowWithinLimit(t *testing.T) {
	l := slidingwindow.New(3, time.Second)
	now := time.Unix(100, 0)
	for i := range 3 {
		if !l.Allow("k", now) {
			t.Fatalf("Allow #%d = false, want true", i+1)
		}
	}
}

func TestRejectsOverLimit(t *testing.T) {
	l := slidingwindow.New(2, time.Second)
	now := time.Unix(100, 0)
	l.Allow("k", now)
	l.Allow("k", now)
	if l.Allow("k", now) {
		t.Fatalf("Allow past limit = true, want false")
	}
}

// TestPreviousWindowWeighting pins the core behavior: a request early in the
// new window still counts most of the previous window's load, so the
// remaining budget is small but nonzero.
func TestPreviousWindowWeighting(t *testing.T) {
	l := slidingwindow.New(10, time.Second)
	prev := time.Unix(100, 0)
	for range 10 {
		if !l.Allow("k", prev) {
			t.Fatalf("fill previous window: admit must succeed")
		}
	}
	// 100ms into the next window: previous weight = 0.9, estimate = 9.0, so
	// exactly one admit fits before budget runs out.
	t1 := prev.Add(time.Second + 100*time.Millisecond)
	if !l.Allow("k", t1) {
		t.Fatalf("Allow 100ms into new window = false; remaining budget should be 1.0")
	}
	if l.Allow("k", t1) {
		t.Fatalf("Allow second call at 100ms = true; estimate would be 10.0")
	}
	// 900ms into new window: previous weight = 0.1, estimate = 0.1*10 + 1 = 2.0.
	t2 := prev.Add(time.Second + 900*time.Millisecond)
	if !l.Allow("k", t2) {
		t.Fatalf("Allow 900ms into new window = false; estimate should be 2.0 with 8 budget remaining")
	}
}

// TestIdleGapClearsHistory ensures that when the previous and current
// windows are both older than one window ago, both counters are reset.
func TestIdleGapClearsHistory(t *testing.T) {
	l := slidingwindow.New(1, time.Second)
	t0 := time.Unix(100, 0)
	if !l.Allow("k", t0) {
		t.Fatal("first admit failed")
	}
	// Jump far ahead -- previous window should no longer influence.
	far := t0.Add(10 * time.Second)
	if !l.Allow("k", far) {
		t.Fatal("after long idle, budget must be fresh")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := slidingwindow.New(1, time.Second)
	now := time.Unix(100, 0)
	if !l.Allow("a", now) || !l.Allow("b", now) {
		t.Fatal("independent keys must each get their own budget")
	}
	if l.Allow("a", now) {
		t.Fatal("key a should be exhausted")
	}
}
