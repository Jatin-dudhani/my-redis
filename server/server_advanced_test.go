package server

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jatin-dudhani/my-redis/aof"
	"github.com/Jatin-dudhani/my-redis/resp"
)

func dialTestServer(t *testing.T, s *Server) net.Conn {
	t.Helper()
	if s.ln == nil {
		go s.Start()
	}
	s.WaitReady()
	conn, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	return conn
}

func sendCmd(conn net.Conn, parts ...string) resp.Value {
	arr := make([]resp.Value, len(parts))
	for i, p := range parts {
		arr[i] = resp.BulkString(p)
	}
	w := resp.NewWriter(conn)
	w.Write(resp.Array(arr))
	w.Flush()

	rd := resp.NewReader(conn)
	v, _ := rd.Read()
	return v
}

func TestWatchOptimisticLocking(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()

	// client 1 watches key
	c1 := dialTestServer(t, s)
	defer c1.Close()

	// client 2 modifies key
	c2 := dialTestServer(t, s)
	defer c2.Close()

	sendCmd(c1, "SET", "balance", "100")

	// c1 watches balance
	r := sendCmd(c1, "WATCH", "balance")
	if r.Str != "OK" {
		t.Fatalf("watch failed: %v", r)
	}

	// c1 starts transaction
	sendCmd(c1, "MULTI")
	sendCmd(c1, "SET", "balance", "150")

	// c2 modifies balance in the middle
	sendCmd(c2, "SET", "balance", "200")

	// c1 tries to EXEC -> must abort!
	execRes := sendCmd(c1, "EXEC")
	if execRes.Typ != resp.TypeNull {
		t.Fatalf("expected transaction to abort with null array, got %v", execRes)
	}

	// verify final balance is 200
	getRes := sendCmd(c1, "GET", "balance")
	if getRes.Str != "200" {
		t.Fatalf("expected balance 200, got %s", getRes.Str)
	}
}

func TestBLPop(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()

	c1 := dialTestServer(t, s)
	defer c1.Close()

	c2 := dialTestServer(t, s)
	defer c2.Close()

	// Push in background after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		sendCmd(c2, "RPUSH", "tasks", "job1")
	}()

	// c1 blocks on BLPOP
	res := sendCmd(c1, "BLPOP", "tasks", "2")
	if res.Typ != resp.TypeArray || len(res.Array) != 2 {
		t.Fatalf("expected [tasks job1], got %v", res)
	}
	if res.Array[0].Str != "tasks" || res.Array[1].Str != "job1" {
		t.Fatalf("unexpected blpop values: %v", res.Array)
	}
}

func TestStreamsEndToEnd(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()

	c := dialTestServer(t, s)
	defer c.Close()

	// XADD
	idRes := sendCmd(c, "XADD", "mystream", "100-0", "sensor", "temp", "val", "21.5")
	if idRes.Str != "100-0" {
		t.Fatalf("expected 100-0, got %s", idRes.Str)
	}

	sendCmd(c, "XADD", "mystream", "100-1", "sensor", "pressure", "val", "1013")

	// XLEN
	lenRes := sendCmd(c, "XLEN", "mystream")
	if lenRes.Num != 2 {
		t.Fatalf("expected xlen 2, got %d", lenRes.Num)
	}

	// XRANGE
	rangeRes := sendCmd(c, "XRANGE", "mystream", "-", "+")
	if rangeRes.Typ != resp.TypeArray || len(rangeRes.Array) != 2 {
		t.Fatalf("expected 2 stream items, got %v", rangeRes)
	}

	// XREAD
	readRes := sendCmd(c, "XREAD", "COUNT", "1", "STREAMS", "mystream", "100-0")
	if readRes.Typ != resp.TypeArray || len(readRes.Array) != 1 {
		t.Fatalf("expected xread 1 stream, got %v", readRes)
	}
}

func TestTypeAndInfo(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()

	c := dialTestServer(t, s)
	defer c.Close()

	sendCmd(c, "SET", "strkey", "val")
	sendCmd(c, "LPUSH", "listkey", "item")
	sendCmd(c, "HSET", "hashkey", "fld", "val")

	if tRes := sendCmd(c, "TYPE", "strkey"); tRes.Str != "string" {
		t.Fatalf("expected string, got %s", tRes.Str)
	}
	if tRes := sendCmd(c, "TYPE", "listkey"); tRes.Str != "list" {
		t.Fatalf("expected list, got %s", tRes.Str)
	}
	if tRes := sendCmd(c, "TYPE", "hashkey"); tRes.Str != "hash" {
		t.Fatalf("expected hash, got %s", tRes.Str)
	}

	infoRes := sendCmd(c, "INFO")
	if !strings.Contains(infoRes.Str, "redis_version") {
		t.Fatalf("info response missing redis_version: %s", infoRes.Str)
	}
}

func TestAOFIntegration(t *testing.T) {
	tmpAof := t.TempDir() + "/server.aof"
	s := New(":0", "")
	if err := s.EnableAOF(tmpAof, aof.FsyncAlways); err != nil {
		t.Fatalf("enable aof failed: %v", err)
	}

	c := dialTestServer(t, s)
	sendCmd(c, "SET", "user:10", "Alice")
	sendCmd(c, "HSET", "user:10:profile", "email", "alice@example.com")
	s.Stop()
	c.Close()

	// Restart server and replay AOF
	s2 := New(":0", "")
	defer s2.Stop()
	if err := s2.EnableAOF(tmpAof, aof.FsyncAlways); err != nil {
		t.Fatalf("enable aof on restart failed: %v", err)
	}

	c2 := dialTestServer(t, s2)
	defer c2.Close()

	val := sendCmd(c2, "GET", "user:10")
	if val.Str != "Alice" {
		t.Fatalf("expected Alice after AOF recovery, got %s", val.Str)
	}

	hval := sendCmd(c2, "HGET", "user:10:profile", "email")
	if hval.Str != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %s", hval.Str)
	}
}

func TestClusterSlotAndMoved(t *testing.T) {
	s := New(":0", "")
	c := dialTestServer(t, s)
	defer c.Close()
	defer s.Stop()

	// node only owns slots 0 to 5000
	s.EnableCluster("node-alpha", 0, 5000)

	// CLUSTER KEYSLOT
	slotRes := sendCmd(c, "CLUSTER", "KEYSLOT", "testkey")
	if slotRes.Typ != resp.TypeInteger {
		t.Fatalf("expected integer slot, got %v", slotRes)
	}

	// Set a key that maps outside 0..5000 -> must return MOVED error!
	var remoteKey string
	for i := 0; i < 500; i++ {
		k := fmt.Sprintf("remotekey%d", i)
		if slotRes := sendCmd(c, "CLUSTER", "KEYSLOT", k); slotRes.Num > 5000 {
			remoteKey = k
			break
		}
	}

	movedRes := sendCmd(c, "SET", remoteKey, "someval")
	if movedRes.Typ != resp.TypeError || !strings.HasPrefix(movedRes.Str, "MOVED") {
		t.Fatalf("expected -MOVED error, got %v", movedRes)
	}
}
