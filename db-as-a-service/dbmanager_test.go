package main

import (
	"sync"
	"testing"
)

func TestDBManager_CreateAndGet(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if err := m.Create("mydb"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	db, err := m.Get("mydb")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if db == nil {
		t.Fatal("Get returned nil db")
	}
}

func TestDBManager_CreateDuplicate(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if err := m.Create("mydb"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := m.Create("mydb"); err == nil {
		t.Fatal("expected error on duplicate Create, got nil")
	}
}

func TestDBManager_GetNotFound(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if _, err := m.Get("missing"); err == nil {
		t.Fatal("expected error for missing database, got nil")
	}
}

func TestDBManager_List(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	for _, name := range []string{"b", "a", "c"} {
		if err := m.Create(name); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}

	got := m.List()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDBManager_Delete(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if err := m.Create("mydb"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Delete("mydb"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get("mydb"); err == nil {
		t.Fatal("expected error after Delete, got nil")
	}
}

func TestDBManager_DeleteNotFound(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if err := m.Delete("missing"); err == nil {
		t.Fatal("expected error deleting missing database, got nil")
	}
}

func TestDBManager_ListEmpty(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	got := m.List()
	if len(got) != 0 {
		t.Errorf("List() on empty manager = %v, want []", got)
	}
}

func TestDBManager_ListAfterDelete(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	for _, name := range []string{"a", "b"} {
		if err := m.Create(name); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}
	if err := m.Delete("a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got := m.List()
	if len(got) != 1 || got[0] != "b" {
		t.Errorf("List() after delete = %v, want [b]", got)
	}
}

func TestDBManager_CreateAfterDelete(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if err := m.Create("mydb"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := m.Delete("mydb"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := m.Create("mydb"); err != nil {
		t.Errorf("Create after Delete: %v", err)
	}
}

func TestDBManager_CloseAllEmpty(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if err := m.CloseAll(); err != nil {
		t.Errorf("CloseAll on empty manager: %v", err)
	}
}

func TestDBManager_CloseAll(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	for _, name := range []string{"a", "b", "c"} {
		if err := m.Create(name); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}
	if err := m.CloseAll(); err != nil {
		t.Errorf("CloseAll: %v", err)
	}
}

func TestDBManager_DataIsolation(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	for _, name := range []string{"db1", "db2"} {
		if err := m.Create(name); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}

	db1, _ := m.Get("db1")
	db2, _ := m.Get("db2")

	if err := db1.Put("key", "from-db1"); err != nil {
		t.Fatalf("db1.Put: %v", err)
	}

	_, ok, err := db2.Get("key")
	if err != nil {
		t.Fatalf("db2.Get: %v", err)
	}
	if ok {
		t.Error("key written to db1 is visible in db2: databases are not isolated")
	}
}

func TestDBManager_ConcurrentCreateGet(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	// Pre-create databases so Get calls don't race with Create.
	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		if err := m.Create(name); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		name := names[i%len(names)]
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			db, err := m.Get(n)
			if err != nil {
				t.Errorf("concurrent Get(%q): %v", n, err)
				return
			}
			if err := db.Put("k", "v"); err != nil {
				t.Errorf("concurrent Put on %q: %v", n, err)
			}
		}(name)
	}
	wg.Wait()
}

func TestDBManager_PutGetRoundtrip(t *testing.T) {
	m, err := NewDBManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewDBManager: %v", err)
	}

	if err := m.Create("mydb"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	db, err := m.Get("mydb")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if err := db.Put("hello", "world"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	val, ok, err := db.Get("hello")
	if err != nil {
		t.Fatalf("db.Get: %v", err)
	}
	if !ok {
		t.Fatal("key not found after Put")
	}
	if val != "world" {
		t.Errorf("db.Get = %q, want %q", val, "world")
	}
}
