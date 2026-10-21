package ds

import (
	"bytes"
	"testing"
)

func TestBitmapGetSetBit(t *testing.T) {
	var b []byte

	// Initial Get on empty slice
	if bit := GetBit(b, 0); bit != 0 {
		t.Fatalf("expected 0, got %d", bit)
	}

	// Set bit 0 to 1 -> byte 0 becomes 0x80 (128)
	var old int
	b, old = SetBit(b, 0, 1)
	if old != 0 {
		t.Fatalf("expected old 0, got %d", old)
	}
	if b[0] != 0x80 {
		t.Fatalf("expected 0x80, got 0x%x", b[0])
	}
	if bit := GetBit(b, 0); bit != 1 {
		t.Fatalf("expected bit 0 to be 1, got %d", bit)
	}

	// Set bit 7 to 1 -> byte 0 becomes 0x81 (129)
	b, old = SetBit(b, 7, 1)
	if old != 0 {
		t.Fatalf("expected old 0, got %d", old)
	}
	if b[0] != 0x81 {
		t.Fatalf("expected 0x81, got 0x%x", b[0])
	}

	// Set bit 10 (byte 1, bit 2: 0x20 = 32)
	b, old = SetBit(b, 10, 1)
	if old != 0 {
		t.Fatalf("expected old 0, got %d", old)
	}
	if len(b) != 2 {
		t.Fatalf("expected len 2, got %d", len(b))
	}
	if GetBit(b, 10) != 1 {
		t.Fatalf("expected bit 10 to be 1")
	}

	// Reset bit 0 to 0
	b, old = SetBit(b, 0, 0)
	if old != 1 {
		t.Fatalf("expected old 1, got %d", old)
	}
	if GetBit(b, 0) != 0 {
		t.Fatalf("expected bit 0 to be 0")
	}
}

func TestBitmapCount(t *testing.T) {
	// "foobar" in ASCII:
	// 'f' = 01100110 (4 ones)
	// 'o' = 01101111 (6 ones)
	// 'o' = 01101111 (6 ones)
	// 'b' = 01100010 (3 ones)
	// 'a' = 01100001 (3 ones)
	// 'r' = 01110010 (4 ones)
	// Total: 26 ones
	b := []byte("foobar")

	if count := BitCount(b, 0, 0, false, false); count != 26 {
		t.Fatalf("expected 26, got %d", count)
	}

	// Range [0, 0] -> first byte 'f' -> 4
	if count := BitCount(b, 0, 0, true, false); count != 4 {
		t.Fatalf("expected 4, got %d", count)
	}

	// Range [1, 1] -> second byte 'o' -> 6
	if count := BitCount(b, 1, 1, true, false); count != 6 {
		t.Fatalf("expected 6, got %d", count)
	}

	// Bit range [0, 7] -> byte 0 -> 4
	if count := BitCount(b, 0, 7, true, true); count != 4 {
		t.Fatalf("expected 4, got %d", count)
	}
}

func TestBitmapPos(t *testing.T) {
	// byte 0 = 0x00, byte 1 = 0x00, byte 2 = 0x80 (bit 16 is 1)
	b := []byte{0x00, 0x00, 0x80}

	pos := BitPos(b, 1, 0, 0, false, false, false)
	if pos != 16 {
		t.Fatalf("expected 16, got %d", pos)
	}

	pos0 := BitPos(b, 0, 0, 0, false, false, false)
	if pos0 != 0 {
		t.Fatalf("expected 0, got %d", pos0)
	}
}

func TestBitmapOp(t *testing.T) {
	s1 := []byte{0x0F} // 0000 1111
	s2 := []byte{0x33} // 0011 0011

	and := BitOp("AND", [][]byte{s1, s2})
	if !bytes.Equal(and, []byte{0x03}) { // 0000 0011
		t.Fatalf("AND failed: got %x", and)
	}

	or := BitOp("OR", [][]byte{s1, s2})
	if !bytes.Equal(or, []byte{0x3F}) { // 0011 1111
		t.Fatalf("OR failed: got %x", or)
	}

	xor := BitOp("XOR", [][]byte{s1, s2})
	if !bytes.Equal(xor, []byte{0x3C}) { // 0011 1100
		t.Fatalf("XOR failed: got %x", xor)
	}

	not := BitOp("NOT", [][]byte{s1})
	if !bytes.Equal(not, []byte{0xF0}) { // 1111 0000
		t.Fatalf("NOT failed: got %x", not)
	}
}
