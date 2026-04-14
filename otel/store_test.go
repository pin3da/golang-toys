package main_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "otel"
)

var t0 = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

func TestWindowedStore_SetAndGet(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	key := SeriesKey{Name: "http.requests", Attributes: "method=GET"}

	store.Set(key, 42, t0)

	windows, total := store.GetWindows(1, t0)

	if len(windows) != 1 {
		t.Fatalf("GetWindows(1) = %d windows, want 1", len(windows))
	}
	if got := windows[0].Series[key]; got != 42 {
		t.Errorf("windows[0][key] = %v, want 42", got)
	}
	if got := total[key]; got != 42 {
		t.Errorf("total[key] = %v, want 42", got)
	}
}

func TestWindowedStore_OverwriteInSameWindow(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	key := SeriesKey{Name: "cpu.usage", Attributes: "host=a"}

	store.Set(key, 10, t0)
	store.Set(key, 99, t0.Add(30*time.Second))

	_, total := store.GetWindows(1, t0)
	if got := total[key]; got != 99 {
		t.Errorf("total[key] = %v, want 99", got)
	}
}

func TestWindowedStore_MultipleWindowsOldestFirst(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	key := SeriesKey{Name: "reqs", Attributes: ""}

	for i := range 3 {
		store.Set(key, float64(i+1), t0.Add(time.Duration(i)*time.Minute))
	}

	windows, total := store.GetWindows(3, t0.Add(2*time.Minute))

	if len(windows) != 3 {
		t.Fatalf("GetWindows(3) = %d windows, want 3", len(windows))
	}
	for i, w := range windows {
		want := float64(i + 1)
		if got := w.Series[key]; got != want {
			t.Errorf("windows[%d][key] = %v, want %v", i, got, want)
		}
	}
	if got := total[key]; got != 6 {
		t.Errorf("total[key] = %v, want 6", got)
	}
}

func TestWindowedStore_WindowStartTimestamp(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	key := SeriesKey{Name: "x", Attributes: ""}

	store.Set(key, 1, t0)

	windows, _ := store.GetWindows(1, t0)
	if len(windows) != 1 {
		t.Fatalf("GetWindows(1) = %d windows, want 1", len(windows))
	}

	wantStart := t0.Truncate(time.Minute)
	if got := windows[0].Start; !got.Equal(wantStart) {
		t.Errorf("windows[0].Start = %v, want %v", got, wantStart)
	}
}

func TestWindowedStore_NowBoundsQuery(t *testing.T) {
	store := NewWindowedStore(4, time.Minute)
	key := SeriesKey{Name: "reqs", Attributes: ""}

	store.Set(key, 10, t0)
	store.Set(key, 20, t0.Add(1*time.Minute))
	store.Set(key, 30, t0.Add(2*time.Minute))
	store.Set(key, 40, t0.Add(3*time.Minute))

	windows, total := store.GetWindows(4, t0.Add(2*time.Minute))

	if len(windows) != 3 {
		t.Fatalf("GetWindows(4) at epoch 2 = %d windows, want 3", len(windows))
	}
	if got := windows[0].Series[key]; got != 10 {
		t.Errorf("windows[0][key] = %v, want 10", got)
	}
	if got := windows[1].Series[key]; got != 20 {
		t.Errorf("windows[1][key] = %v, want 20", got)
	}
	if got := windows[2].Series[key]; got != 30 {
		t.Errorf("windows[2][key] = %v, want 30", got)
	}
	if got := total[key]; got != 60 {
		t.Errorf("total[key] = %v, want 60", got)
	}
}

func TestWindowedStore_BucketRotation(t *testing.T) {
	const numBuckets = 3
	store := NewWindowedStore(numBuckets, time.Minute)
	key := SeriesKey{Name: "metric", Attributes: ""}

	store.Set(key, float64(10), t0.Add(time.Duration(0)*time.Minute))
	store.Set(key, float64(11), t0.Add(time.Duration(1)*time.Minute))
	store.Set(key, float64(12), t0.Add(time.Duration(2)*time.Minute))

	windows, _ := store.GetWindows(numBuckets, t0.Add(2*time.Minute))
	if len(windows) != 3 {
		t.Errorf("GetWindows(%d) = %d, want 3", numBuckets, len(windows))
	}

	store.Set(key, 99, t0.Add(4*time.Minute)) // slot 1, should trigger eviction for 0.
	windows, _ = store.GetWindows(numBuckets, t0.Add(2*time.Minute))
	if len(windows) != 1 {
		t.Errorf("GetWindows(%d) = %d, want 1", numBuckets, len(windows))
	}

	for _, w := range windows {
		if v, ok := w.Series[key]; ok && v == 10 {
			t.Errorf("evicted value 10 is still visible in window starting at %v", w.Start)
		}
	}
}

func TestWindowedStore_GetWindowsCapped(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	key := SeriesKey{Name: "x", Attributes: ""}

	store.Set(key, 1, t0)

	windows, _ := store.GetWindows(100, t0)
	if len(windows) != 1 {
		t.Errorf("GetWindows(100) = %d windows, want 1", len(windows))
	}
}

func TestWindowedStore_EmptyStore(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)

	windows, total := store.GetWindows(5, t0)

	if len(windows) != 0 {
		t.Errorf("got %d windows, want 0", len(windows))
	}
	if len(total) != 0 {
		t.Errorf("total has %d entries, want 0", len(total))
	}
}

func TestWindowedStore_ConcurrentDistinctKeys(t *testing.T) {
	const goroutines = 50

	store := NewWindowedStore(5, time.Minute)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			key := SeriesKey{Name: "metric", Attributes: fmt.Sprintf("id=%d", i)}
			store.Set(key, float64(i), t0)
		}(i)
	}
	wg.Wait()

	_, total := store.GetWindows(1, t0)
	if len(total) != goroutines {
		t.Errorf("total has %d series, want %d", len(total), goroutines)
	}
}

func TestWindowedStore_ConcurrentReadsAndWrites(t *testing.T) {
	const goroutines = 20

	store := NewWindowedStore(5, time.Minute)
	key := SeriesKey{Name: "metric", Attributes: ""}

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			store.Set(key, float64(i), t0.Add(time.Duration(i%2)*time.Minute))
		}(i)
		go func() {
			defer wg.Done()
			store.GetWindows(5, t0.Add(3*time.Minute))
		}()
	}
	wg.Wait()
}

func TestNewWindowedStore_PanicsOnInvalidArgs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		buckets  int
		duration time.Duration
	}{
		{"zero buckets", 0, time.Minute},
		{"negative buckets", -1, time.Minute},
		{"zero duration", 1, 0},
		{"negative duration", 1, -time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			NewWindowedStore(tc.buckets, tc.duration)
		})
	}
}

func TestWindowedStore_SingleBucketEviction(t *testing.T) {
	store := NewWindowedStore(1, time.Minute)
	key := SeriesKey{Name: "x", Attributes: ""}

	store.Set(key, 1, t0)
	store.Set(key, 2, t0.Add(time.Minute)) // writing to a new epoch evicts the only slot

	windows, total := store.GetWindows(1, t0.Add(time.Minute))
	if len(windows) != 1 {
		t.Fatalf("got %d windows, want 1", len(windows))
	}
	if got := total[key]; got != 2 {
		t.Errorf("total = %v, want 2", got)
	}
}

func TestWindowedStore_LargeGapEvictsAll(t *testing.T) {
	store := NewWindowedStore(3, time.Minute)
	key := SeriesKey{Name: "x", Attributes: ""}

	store.Set(key, 1, t0)
	store.Set(key, 2, t0.Add(10*time.Minute)) // gap >> numBuckets

	windows, total := store.GetWindows(3, t0.Add(10*time.Minute))
	for _, w := range windows {
		if v, ok := w.Series[key]; ok && v == 1 {
			t.Errorf("stale value 1 survived large eviction gap at %v", w.Start)
		}
	}
	if got := total[key]; got != 2 {
		t.Errorf("total = %v, want 2 (new value must be present)", got)
	}
}

func TestWindowedStore_GetWindowsZero(t *testing.T) {
	store := NewWindowedStore(5, time.Minute)
	store.Set(SeriesKey{Name: "x", Attributes: ""}, 1, t0)

	windows, total := store.GetWindows(0, t0)
	if len(windows) != 0 {
		t.Errorf("GetWindows(0) = %d windows, want 0", len(windows))
	}
	if len(total) != 0 {
		t.Errorf("GetWindows(0) total has %d entries, want 0", len(total))
	}
}
