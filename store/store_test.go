package store

import "testing"

func TestSetGet(t *testing.T) {
	s := New()
	s.Set("name", "Nikhil")
	val, ok := s.Get("name")
	if !ok || val != "Nikhil" {
		t.Fatalf("expected Nikhil, got %s (ok=%v)", val, ok)
	}
}

func TestGetMissing(t *testing.T) {
	s := New()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestDelete(t *testing.T) {
	s := New()
	s.Set("key", "val")
	s.Delete("key")
	_, ok := s.Get("key")
	if ok {
		t.Fatal("key should be deleted")
	}
}

func TestExists(t *testing.T) {
	s := New()
	s.Set("a", "1")
	if !s.Exists("a") {
		t.Fatal("expected exists to be true")
	}
	if s.Exists("b") {
		t.Fatal("expected exists to be false")
	}
}

func TestLen(t *testing.T) {
	s := New()
	s.Set("a", "1")
	s.Set("b", "2")
	if s.Len() != 2 {
		t.Fatalf("expected len 2, got %d", s.Len())
	}
}

func TestAll(t *testing.T) {
	s := New()
	s.Set("a", "1")
	s.Set("b", "2")
	all := s.All()
	if len(all) != 2 || all["a"] != "1" || all["b"] != "2" {
		t.Fatalf("unexpected All() result: %v", all)
	}
}

func TestLoad(t *testing.T) {
	s := New()
	data := map[string]string{"x": "10", "y": "20"}
	s.Load(data)
	if s.Len() != 2 || !s.Exists("x") || !s.Exists("y") {
		t.Fatal("Load did not populate store correctly")
	}
}

func TestConcurrency(t *testing.T) {
	s := New()
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			s.Set("key", "val")
			s.Get("key")
			s.Exists("key")
			s.Delete("key")
			done <- true
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}
