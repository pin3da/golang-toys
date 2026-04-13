package main_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	. "otel"
)

func TestStore_SetAndGetAll(t *testing.T) {
	s := NewStore()

	keyA := SeriesKey{Name: "http.requests", Attributes: "method=GET,status=200"}
	keyB := SeriesKey{Name: "http.requests", Attributes: "method=POST,status=500"}

	s.Set(keyA, 10)
	s.Set(keyB, 3)

	got := s.GetAll()
	want := map[SeriesKey]float64{
		keyA: 10,
		keyB: 3,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("GetAll() mismatch (-want +got):\n%s", diff)
	}
}

func TestStore_SetOverwrites(t *testing.T) {
	s := NewStore()

	key := SeriesKey{Name: "cpu.usage", Attributes: "host=a"}
	s.Set(key, 50)
	s.Set(key, 75)

	got := s.GetAll()
	if got[key] != 75 {
		t.Errorf("Set() overwrite: got %v, want 75", got[key])
	}
}

func TestStore_GetAllReturnsCopy(t *testing.T) {
	s := NewStore()

	key := SeriesKey{Name: "reqs", Attributes: ""}
	s.Set(key, 1)

	snapshot := s.GetAll()
	snapshot[key] = 999 // mutate the copy

	got := s.GetAll()
	if got[key] != 1 {
		t.Errorf("GetAll() returned a reference, not a copy: got %v, want 1", got[key])
	}
}

func TestStore_ConcurrentDistinctKeys(t *testing.T) {
	const goroutines = 50

	s := NewStore()
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			key := SeriesKey{Name: "metric", Attributes: fmt.Sprintf("id=%d", i)}
			s.Set(key, float64(i))
		}(i)
	}
	wg.Wait()

	got := s.GetAll()
	if len(got) != goroutines {
		t.Errorf("GetAll() returned %d series, want %d", len(got), goroutines)
	}
}

func TestStore_ConcurrentSameKey(t *testing.T) {
	const goroutines = 50

	s := NewStore()
	key := SeriesKey{Name: "metric", Attributes: "host=a"}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			s.Set(key, float64(i))
		}(i)
	}
	wg.Wait()

	got := s.GetAll()
	v, ok := got[key]
	if !ok {
		t.Fatal("key missing from store after concurrent writes")
	}
	if v < 0 || v >= goroutines {
		t.Errorf("value %v is outside the range of written values [0, %d)", v, goroutines)
	}
}

func TestStore_ConcurrentReadsAndWrites(t *testing.T) {
	const goroutines = 20

	s := NewStore()
	key := SeriesKey{Name: "metric", Attributes: ""}

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			s.Set(key, float64(i))
		}(i)

		go func() {
			defer wg.Done()
			_ = s.GetAll()
		}()
	}

	wg.Wait()
}
