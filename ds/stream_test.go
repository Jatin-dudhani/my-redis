package ds

import (
	"testing"
)

func TestStreamXAddAndXLen(t *testing.T) {
	st := NewStream()
	id1, err := st.XAdd("100-0", "sensor", "1", "temp", "24.5")
	if err != nil {
		t.Fatalf("xadd failed: %v", err)
	}
	if id1.String() != "100-0" {
		t.Fatalf("expected 100-0, got %s", id1.String())
	}

	id2, err := st.XAdd("100-1", "sensor", "2", "temp", "25.0")
	if err != nil {
		t.Fatalf("xadd failed: %v", err)
	}
	if id2.String() != "100-1" {
		t.Fatalf("expected 100-1, got %s", id2.String())
	}

	if st.XLen() != 2 {
		t.Fatalf("expected len 2, got %d", st.XLen())
	}

	// invalid id smaller than top item
	_, err = st.XAdd("99-0", "foo", "bar")
	if err == nil {
		t.Fatal("expected error for smaller ID")
	}
}

func TestStreamXRange(t *testing.T) {
	st := NewStream()
	st.XAdd("100-0", "k1", "v1")
	st.XAdd("200-0", "k2", "v2")
	st.XAdd("300-0", "k3", "v3")

	res := st.XRange("-", "+")
	if len(res) != 3 {
		t.Fatalf("expected 3 items in range, got %d", len(res))
	}

	sub := st.XRange("150-0", "250-0")
	if len(sub) != 1 || sub[0].ID.String() != "200-0" {
		t.Fatalf("expected only 200-0, got %v", sub)
	}
}

func TestStreamXReadAfter(t *testing.T) {
	st := NewStream()
	st.XAdd("100-0", "k1", "v1")
	st.XAdd("200-0", "k2", "v2")
	st.XAdd("300-0", "k3", "v3")

	res := st.XReadAfter("100-0", 1)
	if len(res) != 1 || res[0].ID.String() != "200-0" {
		t.Fatalf("expected 200-0, got %v", res)
	}
}
