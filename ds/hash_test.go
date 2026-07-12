package ds

import "testing"

func TestNewHash(t *testing.T) {
	h := NewHash()
	if h.HLen() != 0 {
		t.Fatalf("expected len 0, got %d", h.HLen())
	}
}

func TestHSetAndHGet(t *testing.T) {
	h := NewHash()
	h.HSet("name", "Alice")
	val, ok := h.HGet("name")
	if !ok || val != "Alice" {
		t.Fatalf("expected Alice, got %v", val)
	}
}

func TestHGetMissing(t *testing.T) {
	h := NewHash()
	_, ok := h.HGet("missing")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestHSetOverwrite(t *testing.T) {
	h := NewHash()
	h.HSet("key", "old")
	h.HSet("key", "new")
	val, _ := h.HGet("key")
	if val != "new" {
		t.Fatalf("expected new, got %s", val)
	}
}

func TestHDel(t *testing.T) {
	h := NewHash()
	h.HSet("a", "1")
	h.HSet("b", "2")
	h.HSet("c", "3")
	n := h.HDel("b", "c")
	if n != 2 {
		t.Fatalf("expected 2 deleted, got %d", n)
	}
	if h.HLen() != 1 {
		t.Fatalf("expected len 1, got %d", h.HLen())
	}
}

func TestHDelNonExistent(t *testing.T) {
	h := NewHash()
	h.HSet("a", "1")
	n := h.HDel("b")
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestHExists(t *testing.T) {
	h := NewHash()
	h.HSet("key", "val")
	if !h.HExists("key") {
		t.Fatal("expected key to exist")
	}
	if h.HExists("missing") {
		t.Fatal("expected missing to not exist")
	}
}

func TestHGetAll(t *testing.T) {
	h := NewHash()
	h.HSet("a", "1")
	h.HSet("b", "2")
	all := h.HGetAll()
	if len(all) != 2 || all["a"] != "1" || all["b"] != "2" {
		t.Fatalf("expected {a:1 b:2}, got %v", all)
	}
}

func TestHGetAllEmpty(t *testing.T) {
	h := NewHash()
	all := h.HGetAll()
	if len(all) != 0 {
		t.Fatalf("expected empty, got %v", all)
	}
}

func TestHGetAllIsolation(t *testing.T) {
	h := NewHash()
	h.HSet("key", "val")
	all := h.HGetAll()
	all["key"] = "modified"
	val, _ := h.HGet("key")
	if val != "val" {
		t.Fatalf("expected original val, got %s", val)
	}
}
