package server

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestPingPong(t *testing.T) {
	s := New(":0")
	go func() {
		if err := s.Start(); err != nil {
			return
		}
	}()
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", s.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprintln(conn, "PING")
	reply, _ := bufio.NewReader(conn).ReadString('\n')
	if reply != "PONG\n" {
		t.Fatalf("expected PONG, got %q", reply)
	}
}

func TestUnknownCommand(t *testing.T) {
	s := New(":0")
	go func() {
		if err := s.Start(); err != nil {
			return
		}
	}()
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", s.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprintln(conn, "FOOBAR")
	reply, _ := bufio.NewReader(conn).ReadString('\n')
	expected := "ERR unknown command 'FOOBAR'\n"
	if reply != expected {
		t.Fatalf("expected %q, got %q", expected, reply)
	}
}

func TestMultiplePings(t *testing.T) {
	s := New(":0")
	go func() {
		if err := s.Start(); err != nil {
			return
		}
	}()
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", s.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	for i := 0; i < 5; i++ {
		fmt.Fprintln(conn, "PING")
		reply, _ := bufio.NewReader(conn).ReadString('\n')
		if reply != "PONG\n" {
			t.Fatalf("iteration %d: expected PONG, got %q", i, reply)
		}
	}
}
