package ds

import "testing"

func TestNewSSet(t *testing.T) {
	z := NewSSet()
	if z.ZCard() != 0 {
		t.Fatalf("expected card 0, got %d", z.ZCard())
	}
}

func TestZAddAndZScore(t *testing.T) {
	z := NewSSet()
	z.ZAdd(10, "alice")
	score, ok := z.ZScore("alice")
	if !ok || score != 10 {
		t.Fatalf("expected score 10, got %v", score)
	}
}

func TestZScoreMissing(t *testing.T) {
	z := NewSSet()
	_, ok := z.ZScore("nobody")
	if ok {
		t.Fatal("expected false for missing member")
	}
}

func TestZAddOverwrite(t *testing.T) {
	z := NewSSet()
	z.ZAdd(10, "player")
	z.ZAdd(20, "player")
	score, _ := z.ZScore("player")
	if score != 20 {
		t.Fatalf("expected score 20, got %v", score)
	}
}

func TestZRem(t *testing.T) {
	z := NewSSet()
	z.ZAdd(1, "a")
	z.ZAdd(2, "b")
	z.ZAdd(3, "c")
	n := z.ZRem("b", "c")
	if n != 2 {
		t.Fatalf("expected 2 removed, got %d", n)
	}
	if z.ZCard() != 1 {
		t.Fatalf("expected card 1, got %d", z.ZCard())
	}
}

func TestZRemNonExistent(t *testing.T) {
	z := NewSSet()
	z.ZAdd(1, "a")
	n := z.ZRem("b")
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestZRangeOrdered(t *testing.T) {
	z := NewSSet()
	z.ZAdd(30, "c")
	z.ZAdd(10, "a")
	z.ZAdd(20, "b")
	items := z.ZRange(0, -1, false)
	if len(items) != 3 || items[0] != "a" || items[1] != "b" || items[2] != "c" {
		t.Fatalf("expected [a b c], got %v", items)
	}
}

func TestZRangeWithScores(t *testing.T) {
	z := NewSSet()
	z.ZAdd(10, "a")
	z.ZAdd(20, "b")
	items := z.ZRange(0, -1, true)
	if len(items) != 4 || items[0] != "a" || items[1] != "10" || items[2] != "b" || items[3] != "20" {
		t.Fatalf("expected [a 10 b 20], got %v", items)
	}
}

func TestZRangeSubset(t *testing.T) {
	z := NewSSet()
	z.ZAdd(10, "a")
	z.ZAdd(20, "b")
	z.ZAdd(30, "c")
	items := z.ZRange(1, 2, false)
	if len(items) != 2 || items[0] != "b" || items[1] != "c" {
		t.Fatalf("expected [b c], got %v", items)
	}
}

func TestZRangeEmpty(t *testing.T) {
	z := NewSSet()
	items := z.ZRange(0, -1, false)
	if items != nil {
		t.Fatalf("expected nil, got %v", items)
	}
}

func TestZRangeNegativeIndices(t *testing.T) {
	z := NewSSet()
	z.ZAdd(10, "a")
	z.ZAdd(20, "b")
	z.ZAdd(30, "c")
	items := z.ZRange(-2, -1, false)
	if len(items) != 2 || items[0] != "b" || items[1] != "c" {
		t.Fatalf("expected [b c], got %v", items)
	}
}

func TestZCard(t *testing.T) {
	z := NewSSet()
	z.ZAdd(1, "a")
	z.ZAdd(2, "b")
	if z.ZCard() != 2 {
		t.Fatalf("expected 2, got %d", z.ZCard())
	}
}
