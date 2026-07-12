package ds

import (
	"sort"
	"testing"
)

func TestNewSet(t *testing.T) {
	s := NewSet()
	if s.SCard() != 0 {
		t.Fatalf("expected card 0, got %d", s.SCard())
	}
}

func TestSAddNew(t *testing.T) {
	s := NewSet()
	n := s.SAdd("a", "b", "c")
	if n != 3 {
		t.Fatalf("expected 3 new, got %d", n)
	}
}

func TestSAddDuplicates(t *testing.T) {
	s := NewSet()
	s.SAdd("a", "b")
	n := s.SAdd("b", "c")
	if n != 1 {
		t.Fatalf("expected 1 new (c), got %d", n)
	}
}

func TestSMembers(t *testing.T) {
	s := NewSet()
	s.SAdd("a", "b", "c")
	members := s.SMembers()
	sort.Strings(members)
	if len(members) != 3 || members[0] != "a" || members[1] != "b" || members[2] != "c" {
		t.Fatalf("expected [a b c], got %v", members)
	}
}

func TestSMembersEmpty(t *testing.T) {
	s := NewSet()
	members := s.SMembers()
	if len(members) != 0 {
		t.Fatalf("expected empty, got %v", members)
	}
}

func TestSRem(t *testing.T) {
	s := NewSet()
	s.SAdd("a", "b", "c")
	n := s.SRem("b", "c")
	if n != 2 {
		t.Fatalf("expected 2 removed, got %d", n)
	}
	if s.SCard() != 1 {
		t.Fatalf("expected card 1, got %d", s.SCard())
	}
}

func TestSRemNonExistent(t *testing.T) {
	s := NewSet()
	s.SAdd("a")
	n := s.SRem("b")
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestSIsMember(t *testing.T) {
	s := NewSet()
	s.SAdd("a")
	if !s.SIsMember("a") {
		t.Fatal("expected a to be a member")
	}
	if s.SIsMember("b") {
		t.Fatal("expected b not to be a member")
	}
}

func TestSCard(t *testing.T) {
	s := NewSet()
	if s.SCard() != 0 {
		t.Fatalf("expected 0, got %d", s.SCard())
	}
	s.SAdd("a", "b")
	if s.SCard() != 2 {
		t.Fatalf("expected 2, got %d", s.SCard())
	}
}
