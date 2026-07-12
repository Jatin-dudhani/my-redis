package store

import (
	"testing"
	"time"
)

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

func TestSetWithTTL(t *testing.T) {
	s := New()
	s.SetWithTTL("key", "val", 50*time.Millisecond)

	val, ok := s.Get("key")
	if !ok || val != "val" {
		t.Fatalf("expected key to exist immediately")
	}

	time.Sleep(60 * time.Millisecond)
	_, ok = s.Get("key")
	if ok {
		t.Fatal("expected key to be expired")
	}
}

func TestExpire(t *testing.T) {
	s := New()
	s.Set("key", "val")
	if !s.Expire("key", 50*time.Millisecond) {
		t.Fatal("Expire should return true")
	}

	time.Sleep(60 * time.Millisecond)
	if s.Exists("key") {
		t.Fatal("key should be expired")
	}
}

func TestExpireNonExistent(t *testing.T) {
	s := New()
	if s.Expire("nonexistent", 1*time.Second) {
		t.Fatal("Expire on missing key should return false")
	}
}

func TestTTL(t *testing.T) {
	s := New()
	s.Set("key", "val")

	ttl := s.TTL("key")
	if ttl != -1 {
		t.Fatalf("expected -1 for key without expiry, got %d", ttl)
	}

	s.Expire("key", 60*time.Second)
	ttl = s.TTL("key")
	if ttl <= 0 || ttl > 60 {
		t.Fatalf("expected TTL between 1 and 60, got %d", ttl)
	}

	ttl = s.TTL("nonexistent")
	if ttl != -2 {
		t.Fatalf("expected -2 for nonexistent key, got %d", ttl)
	}
}

func TestSetRemovesTTL(t *testing.T) {
	s := New()
	s.SetWithTTL("key", "val", 60*time.Second)
	s.Set("key", "newval") // Set without TTL should remove expiry

	if ttl := s.TTL("key"); ttl != -1 {
		t.Fatalf("expected -1 after Set, got %d", ttl)
	}
}

func TestOverwriteTTL(t *testing.T) {
	s := New()
	s.SetWithTTL("key", "val", 60*time.Second)
	s.SetWithTTL("key", "newval", 120*time.Second)

	ttl := s.TTL("key")
	if ttl <= 0 || ttl > 120 {
		t.Fatalf("expected TTL around 120, got %d", ttl)
	}

	val, ok := s.Get("key")
	if !ok || val != "newval" {
		t.Fatalf("expected newval, got %s", val)
	}
}

func TestBackgroundCleanup(t *testing.T) {
	s := New()
	s.StartCleanup(10 * time.Millisecond)
	defer s.StopCleanup()

	s.SetWithTTL("key", "val", 20*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	if s.Exists("key") {
		t.Fatal("key should have been cleaned up by background worker")
	}
}
