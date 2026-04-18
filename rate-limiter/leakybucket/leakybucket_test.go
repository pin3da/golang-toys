package leakybucket_test

import (
	"testing"
	"time"

	"github.com/pin3da/golang-toys/rate-limiter/leakybucket"
)

func TestAllowWithinCapacity(t *testing.T) {
	l := leakybucket.New(3, 1)
	now := time.Unix(100, 0)
	for i := range 3 {
		if !l.Allow("k", now) {
			t.Fatalf("Allow #%d = false, want true", i+1)
		}
	}
}

func TestRejectsWhenFull(t *testing.T) {
	l := leakybucket.New(2, 1)
	now := time.Unix(100, 0)
	l.Allow("k", now)
	l.Allow("k", now)
	if l.Allow("k", now) {
		t.Fatalf("Allow past capacity = true, want false")
	}
}

// TestLeakFreesSlots pins the core behavior: after the bucket fills,
// waiting lets it drain at leakRate so new events can be admitted.
func TestLeakFreesSlots(t *testing.T) {
	l := leakybucket.New(2, 2) // 2 events/sec drain
	t0 := time.Unix(100, 0)
	l.Allow("k", t0)
	l.Allow("k", t0)
	if l.Allow("k", t0) {
		t.Fatal("bucket should be full")
	}
	// 500ms later -> 1 slot freed.
	t1 := t0.Add(500 * time.Millisecond)
	if !l.Allow("k", t1) {
		t.Fatal("expected 1 slot after 500ms at 2/s")
	}
	if l.Allow("k", t1) {
		t.Fatal("only 1 slot should have drained")
	}
}

// TestLeakFloorsAtEmpty ensures long idle periods do not create
// negative levels that would later admit more than capacity.
func TestLeakFloorsAtEmpty(t *testing.T) {
	l := leakybucket.New(3, 1)
	t0 := time.Unix(100, 0)
	l.Allow("k", t0)
	// Idle for an hour: bucket drains to empty, not negative.
	far := t0.Add(time.Hour)
	for i := range 3 {
		if !l.Allow("k", far) {
			t.Fatalf("Allow #%d after idle = false, want true", i+1)
		}
	}
	if l.Allow("k", far) {
		t.Fatal("bucket should be full at capacity = 3")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := leakybucket.New(1, 1)
	now := time.Unix(100, 0)
	if !l.Allow("a", now) || !l.Allow("b", now) {
		t.Fatal("independent keys must each get their own bucket")
	}
	if l.Allow("a", now) {
		t.Fatal("key a should be full")
	}
}
