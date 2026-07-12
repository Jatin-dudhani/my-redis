package store

import (
	"os"
	"testing"
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
