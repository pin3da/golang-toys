package tokenbucket_test

import (
	"testing"
	"time"

	"github.com/pin3da/golang-toys/rate-limiter/tokenbucket"
)

func TestAllowWithinCapacity(t *testing.T) {
	l := tokenbucket.New(3, 1)
	now := time.Unix(100, 0)
	for i := range 3 {
		if !l.Allow("k", now) {
			t.Fatalf("Allow #%d = false, want true", i+1)
		}
	}
}

func TestRejectsWhenEmpty(t *testing.T) {
	l := tokenbucket.New(2, 1)
	now := time.Unix(100, 0)
	l.Allow("k", now)
	l.Allow("k", now)
	if l.Allow("k", now) {
		t.Fatalf("Allow past capacity = true, want false")
	}
}

// TestRefillRestoresTokens pins the core behavior: after the bucket is
// drained, waiting long enough at the configured rate yields new tokens.
func TestRefillRestoresTokens(t *testing.T) {
	l := tokenbucket.New(2, 2) // 2 tokens/sec
	t0 := time.Unix(100, 0)
	l.Allow("k", t0)
	l.Allow("k", t0)
	if l.Allow("k", t0) {
		t.Fatal("bucket should be empty")
	}
	// 500ms later -> 1 token refilled.
	t1 := t0.Add(500 * time.Millisecond)
	if !l.Allow("k", t1) {
		t.Fatal("expected 1 token after 500ms at 2/s")
	}
	if l.Allow("k", t1) {
		t.Fatal("only 1 token should have refilled")
	}
}

// TestRefillCapsAtCapacity ensures long idle periods do not accumulate
// tokens beyond capacity.
func TestRefillCapsAtCapacity(t *testing.T) {
	l := tokenbucket.New(3, 1)
	t0 := time.Unix(100, 0)
	l.Allow("k", t0)
	l.Allow("k", t0)
	l.Allow("k", t0)
	// Idle for an hour: refill is capped at capacity = 3.
	far := t0.Add(time.Hour)
	for i := range 3 {
		if !l.Allow("k", far) {
			t.Fatalf("Allow #%d after idle = false, want true", i+1)
		}
	}
	if l.Allow("k", far) {
		t.Fatal("bucket should be capped at capacity = 3")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := tokenbucket.New(1, 1)
	now := time.Unix(100, 0)
	if !l.Allow("a", now) || !l.Allow("b", now) {
		t.Fatal("independent keys must each get their own bucket")
	}
	if l.Allow("a", now) {
		t.Fatal("key a should be exhausted")
	}
}
