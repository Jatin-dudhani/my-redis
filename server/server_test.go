package server

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"
)

func dial(t *testing.T, s *Server) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", s.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func startServer(t *testing.T) *Server {
	t.Helper()
	s := New(":0")
	go func() {
		s.Start()
	}()
	time.Sleep(50 * time.Millisecond)
	return s
}

func send(t *testing.T, conn net.Conn, cmd string) string {
	t.Helper()
	fmt.Fprintln(conn, cmd)
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestPingPong(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := send(t, conn, "PING")
	if reply != "PONG\n" {
		t.Fatalf("expected PONG, got %q", reply)
	}
}

func TestUnknownCommand(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := send(t, conn, "FOOBAR")
	expected := "ERR unknown command 'FOOBAR'\n"
	if reply != expected {
		t.Fatalf("expected %q, got %q", expected, reply)
	}
}

func TestSetGet(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	reply := send(t, conn, "SET name Nikhil")
	if reply != "OK\n" {
		t.Fatalf("expected OK, got %q", reply)
	}

	reply = send(t, conn, "GET name")
	if reply != "Nikhil\n" {
		t.Fatalf("expected Nikhil, got %q", reply)
	}
}

func TestGetMissing(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	reply := send(t, conn, "GET nonexistent")
	if reply != "(nil)\n" {
		t.Fatalf("expected (nil), got %q", reply)
	}
}

func TestSetGetMultiWord(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	send(t, conn, "SET greeting hello world")
	reply := send(t, conn, "GET greeting")
	if reply != "hello world\n" {
		t.Fatalf("expected 'hello world', got %q", reply)
	}
}

func TestDel(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	send(t, conn, "SET a 1")
	send(t, conn, "SET b 2")

	reply := send(t, conn, "DEL a")
	if reply != "1\n" {
		t.Fatalf("expected 1, got %q", reply)
	}

	reply = send(t, conn, "GET a")
	if reply != "(nil)\n" {
		t.Fatalf("expected (nil), got %q", reply)
	}

	reply = send(t, conn, "DEL a")
	if reply != "0\n" {
		t.Fatalf("expected 0, got %q", reply)
	}
}

func TestDelMultiple(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	send(t, conn, "SET a 1")
	send(t, conn, "SET b 2")
	send(t, conn, "SET c 3")

	reply := send(t, conn, "DEL a b c")
	if reply != "3\n" {
		t.Fatalf("expected 3, got %q", reply)
	}
}

func TestExists(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	send(t, conn, "SET a 1")

	reply := send(t, conn, "EXISTS a")
	if reply != "1\n" {
		t.Fatalf("expected 1, got %q", reply)
	}

	reply = send(t, conn, "EXISTS b")
	if reply != "0\n" {
		t.Fatalf("expected 0, got %q", reply)
	}
}

func TestMultiplePings(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	for i := 0; i < 5; i++ {
		reply := send(t, conn, "PING")
		if reply != "PONG\n" {
			t.Fatalf("iteration %d: expected PONG, got %q", i, reply)
		}
	}
}
