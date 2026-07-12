package resp

import (
	"bytes"
	"testing"
)

func roundTrip(t *testing.T, v Value, expectedWire string) {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(v); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != expectedWire {
		t.Fatalf("wire: expected %q, got %q", expectedWire, buf.String())
	}

	r := NewReader(&buf)
	parsed, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Typ != v.Typ {
		t.Fatalf("type: expected %c, got %c", v.Typ, parsed.Typ)
	}
}

func TestSimpleString(t *testing.T) {
	roundTrip(t, SimpleString("OK"), "+OK\r\n")
}

func TestError(t *testing.T) {
	roundTrip(t, Error("ERR unknown command"), "-ERR unknown command\r\n")
}

func TestInteger(t *testing.T) {
	roundTrip(t, Integer(42), ":42\r\n")
	roundTrip(t, Integer(0), ":0\r\n")
	roundTrip(t, Integer(-1), ":-1\r\n")
}

func TestBulkString(t *testing.T) {
	roundTrip(t, BulkString("hello"), "$5\r\nhello\r\n")
	roundTrip(t, BulkString(""), "$0\r\n\r\n")
}

func TestNull(t *testing.T) {
	roundTrip(t, Null(), "$-1\r\n")
}

func TestArray(t *testing.T) {
	v := Array([]Value{
		BulkString("SET"),
		BulkString("name"),
		BulkString("Nikhil"),
	})
	expected := "*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$6\r\nNikhil\r\n"
	roundTrip(t, v, expected)
}

func TestNestedArray(t *testing.T) {
	v := Array([]Value{
		Array([]Value{BulkString("a"), BulkString("b")}),
		SimpleString("c"),
	})
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(v); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	r := NewReader(&buf)
	parsed, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Typ != TypeArray || len(parsed.Array) != 2 {
		t.Fatal("expected array of 2")
	}
}

func TestReadPing(t *testing.T) {
	wire := "*1\r\n$4\r\nPING\r\n"
	r := NewReader(bytes.NewReader([]byte(wire)))
	v, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if v.Typ != TypeArray || len(v.Array) != 1 {
		t.Fatalf("expected array of 1, got type=%c len=%d", v.Typ, len(v.Array))
	}
	if v.Array[0].Typ != TypeBulkString || v.Array[0].Str != "PING" {
		t.Fatalf("expected PING bulk string, got %+v", v.Array[0])
	}
}

func TestReadSetCommand(t *testing.T) {
	wire := "*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$6\r\nNikhil\r\n"
	r := NewReader(bytes.NewReader([]byte(wire)))
	v, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if v.Typ != TypeArray || len(v.Array) != 3 {
		t.Fatalf("expected array of 3, got %+v", v)
	}
	if v.Array[0].Str != "SET" || v.Array[1].Str != "name" || v.Array[2].Str != "Nikhil" {
		t.Fatalf("unexpected values: %+v", v.Array)
	}
}

func TestRoundTripComplex(t *testing.T) {
	original := Array([]Value{
		BulkString("HSET"),
		BulkString("user:1"),
		BulkString("name"),
		BulkString("Alice"),
		BulkString("age"),
		BulkString("30"),
	})
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(original); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	r := NewReader(&buf)
	parsed, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Array) != 6 {
		t.Fatalf("expected 6 elements, got %d", len(parsed.Array))
	}
	for i, elem := range parsed.Array {
		if elem.Typ != TypeBulkString {
			t.Fatalf("element %d: expected bulk string, got %c", i, elem.Typ)
		}
	}
	if parsed.Array[1].Str != "user:1" {
		t.Fatalf("expected user:1, got %s", parsed.Array[1].Str)
	}
}

func TestNullBulkStringInArray(t *testing.T) {
	v := Array([]Value{
		BulkString("GET"),
		Null(),
	})
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(v); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	r := NewReader(&buf)
	parsed, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Array[1].Typ != TypeNull {
		t.Fatalf("expected null, got %c", parsed.Array[1].Typ)
	}
}

func TestEmptyArray(t *testing.T) {
	roundTrip(t, Array([]Value{}), "*0\r\n")
}

func TestMultipleReads(t *testing.T) {
	wire := "+OK\r\n+OK\r\n+OK\r\n"
	r := NewReader(bytes.NewReader([]byte(wire)))
	for i := 0; i < 3; i++ {
		v, err := r.Read()
		if err != nil {
			t.Fatal(err)
		}
		if v.Typ != TypeSimpleString || v.Str != "OK" {
			t.Fatalf("expected OK, got %+v", v)
		}
	}
}
