package store

import (
	"os"
	"testing"
	"time"

	"github.com/Jatin-dudhani/my-redis/ds"
)

func TestSaveAndLoad(t *testing.T) {
	s := New()
	s.Set("a", "1")
	s.Set("b", "2")

	tmp := t.TempDir() + "/store.json"
	if err := SaveToFile(s, tmp); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFromFile(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Len() != 2 {
		t.Fatalf("expected len 2, got %d", loaded.Len())
	}

	val, ok := loaded.Get("a")
	if !ok || val != "1" {
		t.Fatalf("expected a=1, got %s (ok=%v)", val, ok)
	}

	val, ok = loaded.Get("b")
	if !ok || val != "2" {
		t.Fatalf("expected b=2, got %s (ok=%v)", val, ok)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path.json")
	if !os.IsNotExist(err) {
		t.Fatalf("expected file not found error, got %v", err)
	}
}

func TestSaveAndLoadAllTypes(t *testing.T) {
	s := New()
	s.Set("str", "hello")

	l := ds.NewList()
	l.RPush("a", "b", "c")
	s.Set("list", l)

	st := ds.NewSet()
	st.SAdd("x", "y")
	s.Set("set", st)

	h := ds.NewHash()
	h.HSet("field1", "val1")
	s.Set("hash", h)

	zs := ds.NewSSet()
	zs.ZAdd(10.5, "player1")
	zs.ZAdd(20.0, "player2")
	s.Set("zset", zs)

	s.SetWithTTL("temp", "expiring", 10*time.Minute)

	tmp := t.TempDir() + "/all_types.json"
	if err := SaveToFile(s, tmp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadFromFile(tmp)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Len() != 6 {
		t.Fatalf("expected len 6, got %d", loaded.Len())
	}

	// verify list
	val, ok := loaded.Get("list")
	if !ok {
		t.Fatal("missing list")
	}
	loadedList, ok := val.(*ds.List)
	if !ok || loadedList.LLen() != 3 {
		t.Fatalf("expected list of len 3, got %v", val)
	}
	rangeItems := loadedList.LRange(0, -1)
	if len(rangeItems) != 3 || rangeItems[0] != "a" || rangeItems[2] != "c" {
		t.Fatalf("unexpected list items: %v", rangeItems)
	}

	// verify zset
	zval, ok := loaded.Get("zset")
	if !ok {
		t.Fatal("missing zset")
	}
	loadedZSet, ok := zval.(*ds.SSet)
	if !ok || loadedZSet.ZCard() != 2 {
		t.Fatalf("expected zset card 2, got %v", zval)
	}
	score, ok := loadedZSet.ZScore("player1")
	if !ok || score != 10.5 {
		t.Fatalf("expected player1 score 10.5, got %f", score)
	}

	// verify ttl preserved
	ttl := loaded.TTL("temp")
	if ttl <= 0 || ttl > 600 {
		t.Fatalf("expected TTL near 600, got %d", ttl)
	}
}

