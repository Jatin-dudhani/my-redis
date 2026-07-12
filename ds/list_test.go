package ds

import (
	"testing"
)

func TestNewList(t *testing.T) {
	l := NewList()
	if l.LLen() != 0 {
		t.Fatalf("expected len 0, got %d", l.LLen())
	}
}

func TestLPush(t *testing.T) {
	l := NewList()
	n := l.LPush("a", "b")
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
	if l.LLen() != 2 {
		t.Fatalf("expected len 2, got %d", l.LLen())
	}
	items := l.LRange(0, -1)
	if len(items) != 2 || items[0] != "b" || items[1] != "a" {
		t.Fatalf("expected [b a], got %v", items)
	}
}

func TestRPush(t *testing.T) {
	l := NewList()
	l.RPush("a", "b")
	items := l.LRange(0, -1)
	if len(items) != 2 || items[0] != "a" || items[1] != "b" {
		t.Fatalf("expected [a b], got %v", items)
	}
}

func TestLPop(t *testing.T) {
	l := NewList()
	l.RPush("a", "b", "c")
	val, ok := l.LPop()
	if !ok || val != "a" {
		t.Fatalf("expected a, got %v", val)
	}
	if l.LLen() != 2 {
		t.Fatalf("expected len 2, got %d", l.LLen())
	}
}

func TestLPopEmpty(t *testing.T) {
	l := NewList()
	_, ok := l.LPop()
	if ok {
		t.Fatal("expected false from empty list")
	}
}

func TestRPop(t *testing.T) {
	l := NewList()
	l.RPush("a", "b", "c")
	val, ok := l.RPop()
	if !ok || val != "c" {
		t.Fatalf("expected c, got %v", val)
	}
}

func TestRPopEmpty(t *testing.T) {
	l := NewList()
	_, ok := l.RPop()
	if ok {
		t.Fatal("expected false from empty list")
	}
}

func TestLRangeFull(t *testing.T) {
	l := NewList()
	l.RPush("a", "b", "c")
	items := l.LRange(0, -1)
	if len(items) != 3 || items[0] != "a" || items[1] != "b" || items[2] != "c" {
		t.Fatalf("expected [a b c], got %v", items)
	}
}

func TestLRangeSubset(t *testing.T) {
	l := NewList()
	l.RPush("a", "b", "c", "d", "e")
	items := l.LRange(1, 3)
	if len(items) != 3 || items[0] != "b" || items[1] != "c" || items[2] != "d" {
		t.Fatalf("expected [b c d], got %v", items)
	}
}

func TestLRangeNegativeIndices(t *testing.T) {
	l := NewList()
	l.RPush("a", "b", "c")
	items := l.LRange(-2, -1)
	if len(items) != 2 || items[0] != "b" || items[1] != "c" {
		t.Fatalf("expected [b c], got %v", items)
	}
}

func TestLRangeOutOfRange(t *testing.T) {
	l := NewList()
	l.RPush("a", "b")
	items := l.LRange(5, 10)
	if items != nil {
		t.Fatalf("expected nil, got %v", items)
	}
}

func TestLRangeEmpty(t *testing.T) {
	l := NewList()
	items := l.LRange(0, -1)
	if items != nil {
		t.Fatalf("expected nil from empty list, got %v", items)
	}
}
