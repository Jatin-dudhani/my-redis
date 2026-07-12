package server

import (
	"net"
	"testing"
	"time"

	"github.com/macbook/my-redis/resp"
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
	s := New(":0", "")
	go func() {
		s.Start()
	}()
	time.Sleep(50 * time.Millisecond)
	return s
}

func sendRESP(t *testing.T, conn net.Conn, v resp.Value) resp.Value {
	t.Helper()
	w := resp.NewWriter(conn)
	if err := w.Write(v); err != nil {
		t.Fatal(err)
	}
	r := resp.NewReader(conn)
	reply, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func cmd(args ...string) resp.Value {
	vals := make([]resp.Value, len(args))
	for i, a := range args {
		vals[i] = resp.BulkString(a)
	}
	return resp.Array(vals)
}

func TestPingPong(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, cmd("PING"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "PONG" {
		t.Fatalf("expected +PONG, got %+v", reply)
	}
}

func TestPingWithArg(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, cmd("PING", "hello"))
	if reply.Typ != resp.TypeBulkString || reply.Str != "hello" {
		t.Fatalf("expected $5\\r\\nhello, got %+v", reply)
	}
}

func TestUnknownCommand(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, cmd("FOOBAR"))
	if reply.Typ != resp.TypeError {
		t.Fatalf("expected error, got %+v", reply)
	}
}

func TestSetGet(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	reply := sendRESP(t, conn, cmd("SET", "name", "Nikhil"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "OK" {
		t.Fatalf("expected +OK, got %+v", reply)
	}

	reply = sendRESP(t, conn, cmd("GET", "name"))
	if reply.Typ != resp.TypeBulkString || reply.Str != "Nikhil" {
		t.Fatalf("expected $6\\r\\nNikhil, got %+v", reply)
	}
}

func TestGetMissing(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, cmd("GET", "nonexistent"))
	if reply.Typ != resp.TypeNull {
		t.Fatalf("expected null, got %+v", reply)
	}
}

func TestDel(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	sendRESP(t, conn, cmd("SET", "a", "1"))
	sendRESP(t, conn, cmd("SET", "b", "2"))

	reply := sendRESP(t, conn, cmd("DEL", "a"))
	if reply.Typ != resp.TypeInteger || reply.Num != 1 {
		t.Fatalf("expected :1, got %+v", reply)
	}

	reply = sendRESP(t, conn, cmd("GET", "a"))
	if reply.Typ != resp.TypeNull {
		t.Fatalf("expected null, got %+v", reply)
	}
}

func TestDelMultiple(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	sendRESP(t, conn, cmd("SET", "a", "1"))
	sendRESP(t, conn, cmd("SET", "b", "2"))

	reply := sendRESP(t, conn, cmd("DEL", "a", "b"))
	if reply.Typ != resp.TypeInteger || reply.Num != 2 {
		t.Fatalf("expected :2, got %+v", reply)
	}
}

func TestExists(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	sendRESP(t, conn, cmd("SET", "a", "1"))

	reply := sendRESP(t, conn, cmd("EXISTS", "a"))
	if reply.Typ != resp.TypeInteger || reply.Num != 1 {
		t.Fatalf("expected :1, got %+v", reply)
	}

	reply = sendRESP(t, conn, cmd("EXISTS", "b"))
	if reply.Typ != resp.TypeInteger || reply.Num != 0 {
		t.Fatalf("expected :0, got %+v", reply)
	}
}

func TestConcurrentClients(t *testing.T) {
	s := startServer(t)
	defer s.Stop()

	start := make(chan struct{})
	errs := make(chan error, 50)

	for i := 0; i < 50; i++ {
		go func(n int) {
			<-start
			conn, err := net.Dial("tcp", s.ln.Addr().String())
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()

			w := resp.NewWriter(conn)
			r := resp.NewReader(conn)

			// SET
			key := resp.BulkString("SET")
			arg1 := resp.BulkString("key")
			arg2 := resp.BulkString("val")
			w.Write(resp.Array([]resp.Value{key, arg1, arg2}))
			reply, _ := r.Read()
			if reply.Typ != resp.TypeSimpleString || reply.Str != "OK" {
				errs <- nil
				return
			}

			// GET
			w.Write(resp.Array([]resp.Value{resp.BulkString("GET"), resp.BulkString("key")}))
			reply, _ = r.Read()
			if reply.Typ != resp.TypeBulkString || reply.Str != "val" {
				errs <- nil
				return
			}

			errs <- nil
		}(i)
	}

	close(start)
	for i := 0; i < 50; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestSetWithEX(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	reply := sendRESP(t, conn, cmd("SET", "key", "val", "EX", "1"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "OK" {
		t.Fatalf("expected +OK, got %+v", reply)
	}

	// Immediately readable
	reply = sendRESP(t, conn, cmd("GET", "key"))
	if reply.Typ != resp.TypeBulkString || reply.Str != "val" {
		t.Fatalf("expected val, got %+v", reply)
	}

	// Wait for expiry and check again (use a separate connection)
	time.Sleep(1100 * time.Millisecond)
	conn2 := dial(t, s)
	defer conn2.Close()
	reply = sendRESP(t, conn2, cmd("GET", "key"))
	if reply.Typ != resp.TypeNull {
		t.Fatalf("expected null after expiry, got %+v", reply)
	}
}

func TestExpireCommand(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	sendRESP(t, conn, cmd("SET", "key", "val"))

	reply := sendRESP(t, conn, cmd("EXPIRE", "key", "1"))
	if reply.Typ != resp.TypeInteger || reply.Num != 1 {
		t.Fatalf("expected :1, got %+v", reply)
	}

	time.Sleep(1100 * time.Millisecond)
	conn2 := dial(t, s)
	defer conn2.Close()
	reply = sendRESP(t, conn2, cmd("GET", "key"))
	if reply.Typ != resp.TypeNull {
		t.Fatalf("expected null after expire, got %+v", reply)
	}
}

func TestExpireMissingKey(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	reply := sendRESP(t, conn, cmd("EXPIRE", "nonexistent", "10"))
	if reply.Typ != resp.TypeInteger || reply.Num != 0 {
		t.Fatalf("expected :0, got %+v", reply)
	}
}

func TestTTLCommand(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	// No expiry
	sendRESP(t, conn, cmd("SET", "key", "val"))
	reply := sendRESP(t, conn, cmd("TTL", "key"))
	if reply.Typ != resp.TypeInteger || reply.Num != -1 {
		t.Fatalf("expected :-1, got %+v", reply)
	}

	// With expiry
	sendRESP(t, conn, cmd("EXPIRE", "key", "60"))
	reply = sendRESP(t, conn, cmd("TTL", "key"))
	if reply.Typ != resp.TypeInteger || reply.Num <= 0 || reply.Num > 60 {
		t.Fatalf("expected TTL 1-60, got %+v", reply)
	}

	// Missing key
	reply = sendRESP(t, conn, cmd("TTL", "nokey"))
	if reply.Typ != resp.TypeInteger || reply.Num != -2 {
		t.Fatalf("expected :-2, got %+v", reply)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/dump.json"

	// Start server with db path, set some keys, save
	s1 := New(":0", dbPath)
	go s1.Start()
	time.Sleep(50 * time.Millisecond)
	defer s1.Stop()

	conn := dial(t, s1)
	sendRESP(t, conn, cmd("SET", "a", "1"))
	sendRESP(t, conn, cmd("SET", "b", "2"))
	reply := sendRESP(t, conn, cmd("SAVE"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "OK" {
		t.Fatalf("expected +OK, got %+v", reply)
	}
	conn.Close()

	// Start a new server with same db path, verify data loaded
	s2 := New(":0", dbPath)
	go s2.Start()
	time.Sleep(50 * time.Millisecond)
	defer s2.Stop()

	conn2 := dial(t, s2)
	reply = sendRESP(t, conn2, cmd("GET", "a"))
	if reply.Typ != resp.TypeBulkString || reply.Str != "1" {
		t.Fatalf("expected $1\\r\\n1, got %+v", reply)
	}
	reply = sendRESP(t, conn2, cmd("GET", "b"))
	if reply.Typ != resp.TypeBulkString || reply.Str != "2" {
		t.Fatalf("expected $1\\r\\n2, got %+v", reply)
	}
	conn2.Close()
}

func TestSaveWithoutPath(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, cmd("SAVE"))
	if reply.Typ != resp.TypeError {
		t.Fatalf("expected error for SAVE without db path, got %+v", reply)
	}
}

func TestTransactionExec(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	reply := sendRESP(t, conn, cmd("MULTI"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "OK" {
		t.Fatalf("expected +OK, got %+v", reply)
	}

	reply = sendRESP(t, conn, cmd("SET", "a", "1"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "QUEUED" {
		t.Fatalf("expected QUEUED, got %+v", reply)
	}

	reply = sendRESP(t, conn, cmd("SET", "b", "2"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "QUEUED" {
		t.Fatalf("expected QUEUED, got %+v", reply)
	}

	reply = sendRESP(t, conn, cmd("EXEC"))
	if reply.Typ != resp.TypeArray || len(reply.Array) != 2 {
		t.Fatalf("expected array of 2, got %+v", reply)
	}
	if reply.Array[0].Typ != resp.TypeSimpleString || reply.Array[0].Str != "OK" {
		t.Fatalf("expected OK, got %+v", reply.Array[0])
	}
	if reply.Array[1].Typ != resp.TypeSimpleString || reply.Array[1].Str != "OK" {
		t.Fatalf("expected OK, got %+v", reply.Array[1])
	}

	// Verify values were set
	reply = sendRESP(t, conn, cmd("GET", "a"))
	if reply.Typ != resp.TypeBulkString || reply.Str != "1" {
		t.Fatalf("expected 1, got %+v", reply)
	}
}

func TestTransactionDiscard(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()

	sendRESP(t, conn, cmd("MULTI"))
	sendRESP(t, conn, cmd("SET", "x", "100"))

	reply := sendRESP(t, conn, cmd("DISCARD"))
	if reply.Typ != resp.TypeSimpleString || reply.Str != "OK" {
		t.Fatalf("expected +OK, got %+v", reply)
	}

	// x should not have been set
	reply = sendRESP(t, conn, cmd("GET", "x"))
	if reply.Typ != resp.TypeNull {
		t.Fatalf("expected null, got %+v", reply)
	}
}

func TestExecWithoutMulti(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, cmd("EXEC"))
	if reply.Typ != resp.TypeError {
		t.Fatalf("expected error, got %+v", reply)
	}
}

func TestDiscardWithoutMulti(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, cmd("DISCARD"))
	if reply.Typ != resp.TypeError {
		t.Fatalf("expected error, got %+v", reply)
	}
}

func TestProtocolError(t *testing.T) {
	s := startServer(t)
	defer s.Stop()
	conn := dial(t, s)
	defer conn.Close()
	reply := sendRESP(t, conn, resp.SimpleString("INVALID"))
	if reply.Typ != resp.TypeError {
		t.Fatalf("expected error for non-array input, got %+v", reply)
	}
}
