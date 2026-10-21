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

func TestBitmapsOverTCP(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()
	c := dialTestServer(t, s)
	defer c.Close()

	// SETBIT key 7 1 -> byte 0 becomes 0x01
	r := sendCmd(c, "SETBIT", "mybitmap", "7", "1")
	if r.Typ != resp.TypeInteger || r.Num != 0 {
		t.Fatalf("expected old bit 0, got %v", r)
	}

	// GETBIT key 7 -> 1
	r = sendCmd(c, "GETBIT", "mybitmap", "7")
	if r.Typ != resp.TypeInteger || r.Num != 1 {
		t.Fatalf("expected bit 1, got %v", r)
	}

	// BITCOUNT
	r = sendCmd(c, "BITCOUNT", "mybitmap")
	if r.Typ != resp.TypeInteger || r.Num != 1 {
		t.Fatalf("expected count 1, got %v", r)
	}

	// BITPOS
	r = sendCmd(c, "BITPOS", "mybitmap", "1")
	if r.Typ != resp.TypeInteger || r.Num != 7 {
		t.Fatalf("expected pos 7, got %v", r)
	}

	// BITOP AND
	sendCmd(c, "SETBIT", "bm2", "7", "1")
	r = sendCmd(c, "BITOP", "AND", "bm3", "mybitmap", "bm2")
	if r.Typ != resp.TypeInteger || r.Num != 1 {
		t.Fatalf("expected bitop len 1, got %v", r)
	}
	r = sendCmd(c, "GETBIT", "bm3", "7")
	if r.Num != 1 {
		t.Fatalf("expected bit 7 to be 1 in bm3, got %d", r.Num)
	}
}

func TestHyperLogLogOverTCP(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()
	c := dialTestServer(t, s)
	defer c.Close()

	// PFADD
	r := sendCmd(c, "PFADD", "hll1", "apple", "banana", "cherry")
	if r.Typ != resp.TypeInteger || r.Num != 1 {
		t.Fatalf("expected 1 from PFADD, got %v", r)
	}

	// Duplicate PFADD returns 0
	r = sendCmd(c, "PFADD", "hll1", "apple", "banana")
	if r.Typ != resp.TypeInteger || r.Num != 0 {
		t.Fatalf("expected 0 from duplicate PFADD, got %v", r)
	}

	// PFCOUNT
	r = sendCmd(c, "PFCOUNT", "hll1")
	if r.Typ != resp.TypeInteger || r.Num != 3 {
		t.Fatalf("expected count 3, got %v", r)
	}

	// PFMERGE
	sendCmd(c, "PFADD", "hll2", "date", "fig")
	r = sendCmd(c, "PFMERGE", "hll_union", "hll1", "hll2")
	if r.Typ != resp.TypeSimpleString || r.Str != "OK" {
		t.Fatalf("expected OK from PFMERGE, got %v", r)
	}

	r = sendCmd(c, "PFCOUNT", "hll_union")
	if r.Typ != resp.TypeInteger || r.Num != 5 {
		t.Fatalf("expected 5 in merged HLL, got %v", r)
	}
}

func TestGeospatialOverTCP(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()
	c := dialTestServer(t, s)
	defer c.Close()

	// GEOADD Sicily 13.361389 38.115556 Palermo 15.087269 37.502669 Catania
	r := sendCmd(c, "GEOADD", "Sicily", "13.361389", "38.115556", "Palermo", "15.087269", "37.502669", "Catania")
	if r.Typ != resp.TypeInteger || r.Num != 2 {
		t.Fatalf("expected 2 added, got %v", r)
	}

	// GEODIST Sicily Palermo Catania km
	r = sendCmd(c, "GEODIST", "Sicily", "Palermo", "Catania", "km")
	if r.Typ != resp.TypeBulkString || !strings.HasPrefix(r.Str, "166.") {
		t.Fatalf("expected ~166km distance, got %s", r.Str)
	}

	// GEOPOS Sicily Palermo
	r = sendCmd(c, "GEOPOS", "Sicily", "Palermo")
	if r.Typ != resp.TypeArray || len(r.Array) != 1 {
		t.Fatalf("expected array of 1 for Palermo pos, got %v", r)
	}

	// GEOSEARCH Sicily FROMLONLAT 13.361389 38.115556 BYRADIUS 100 km
	r = sendCmd(c, "GEOSEARCH", "Sicily", "FROMLONLAT", "13.361389", "38.115556", "BYRADIUS", "100", "km")
	if r.Typ != resp.TypeArray || len(r.Array) != 1 || r.Array[0].Str != "Palermo" {
		t.Fatalf("expected only Palermo within 100km, got %v", r)
	}

	// GEOHASH Sicily Palermo
	r = sendCmd(c, "GEOHASH", "Sicily", "Palermo")
	if r.Typ != resp.TypeArray || len(r.Array) != 1 || len(r.Array[0].Str) != 11 {
		t.Fatalf("expected 11-char geohash string, got %v", r)
	}
}

func TestLuaScriptingOverTCP(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()
	c := dialTestServer(t, s)
	defer c.Close()

	// EVAL script with redis.call
	script := `
		redis.call("SET", KEYS[1], ARGV[1])
		return redis.call("GET", KEYS[1])
	`
	r := sendCmd(c, "EVAL", script, "1", "score:user:1", "950")
	if r.Typ != resp.TypeBulkString || r.Str != "950" {
		t.Fatalf("expected '950', got %v", r)
	}

	// SCRIPT LOAD
	rLoad := sendCmd(c, "SCRIPT", "LOAD", script)
	if rLoad.Typ != resp.TypeBulkString || len(rLoad.Str) != 40 {
		t.Fatalf("expected 40-char SHA1, got %v", rLoad)
	}
	sha := rLoad.Str

	// SCRIPT EXISTS
	rExists := sendCmd(c, "SCRIPT", "EXISTS", sha)
	if rExists.Typ != resp.TypeArray || rExists.Array[0].Num != 1 {
		t.Fatalf("expected script to exist, got %v", rExists)
	}

	// EVALSHA
	rSHA := sendCmd(c, "EVALSHA", sha, "1", "score:user:2", "990")
	if rSHA.Typ != resp.TypeBulkString || rSHA.Str != "990" {
		t.Fatalf("expected '990' from EVALSHA, got %v", rSHA)
	}
}

func TestEnterpriseACLOverTCP(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()
	c := dialTestServer(t, s)
	defer c.Close()

	// ACL WHOAMI
	r := sendCmd(c, "ACL", "WHOAMI")
	if r.Typ != resp.TypeBulkString || r.Str != "default" {
		t.Fatalf("expected default whoami, got %v", r)
	}

	// Configure user 'alice' with password and permissions
	r = sendCmd(c, "ACL", "SETUSER", "alice", "on", ">secret123", "+@read", "~cache:*")
	if r.Typ != resp.TypeSimpleString || r.Str != "OK" {
		t.Fatalf("expected OK from ACL SETUSER, got %v", r)
	}

	// Authenticate as alice
	r = sendCmd(c, "AUTH", "alice", "secret123")
	if r.Typ != resp.SimpleString("OK").Typ || r.Str != "OK" {
		t.Fatalf("expected auth success, got %v", r)
	}

	// Alice tries write command -> should be rejected by ACL!
	r = sendCmd(c, "SET", "cache:1", "data")
	if r.Typ != resp.TypeError || !strings.HasPrefix(r.Str, "NOPERM") {
		t.Fatalf("expected NOPERM error on SET, got %v", r)
	}

	// Alice tries to read forbidden key -> should be rejected by ACL key pattern!
	r = sendCmd(c, "GET", "secret:key")
	if r.Typ != resp.TypeError || !strings.HasPrefix(r.Str, "NOPERM") {
		t.Fatalf("expected NOPERM error on non-cache key, got %v", r)
	}

	// Re-authenticate as default user
	r = sendCmd(c, "AUTH", "default", "")
	if r.Str != "OK" {
		t.Fatalf("expected re-auth as default, got %v", r)
	}
}

func TestTelemetryHTTPService(t *testing.T) {
	s := New(":0", "")
	defer s.Stop()
	c := dialTestServer(t, s)
	defer c.Close()

	sendCmd(c, "SET", "demo_key", "demo_value")
	sendCmd(c, "GET", "demo_key")

	err := s.EnableHTTPTelemetry("127.0.0.1:0")
	if err != nil {
		t.Fatalf("enable http telemetry failed: %v", err)
	}

	// Give HTTP server a split second to start
	time.Sleep(50 * time.Millisecond)

	// Verify metrics snapshot
	snap := s.Metrics().Snapshot()
	if snap.TotalCommands < 2 {
		t.Fatalf("expected at least 2 commands recorded in telemetry, got %d", snap.TotalCommands)
	}
	if snap.CacheHits < 1 {
		t.Fatalf("expected at least 1 cache hit in telemetry, got %d", snap.CacheHits)
	}
}

func TestClusterGossipMeetOverTCP(t *testing.T) {
	s1 := New(":0", "")
	s1.EnableCluster("node-alpha", 0, 8191)
	err := s1.EnableClusterBus("127.0.0.1:0")
	if err != nil {
		t.Fatalf("enable cluster bus s1 failed: %v", err)
	}
	defer s1.Stop()

	s2 := New(":0", "")
	s2.EnableCluster("node-beta", 8192, 16383)
	err = s2.EnableClusterBus("127.0.0.1:0")
	if err != nil {
		t.Fatalf("enable cluster bus s2 failed: %v", err)
	}
	defer s2.Stop()

	c1 := dialTestServer(t, s1)
	defer c1.Close()

	// Parse host:port of s2's gossip bus
	s2BusAddr := s2.GossipBus().BusAddr()
	host, port, _ := net.SplitHostPort(s2BusAddr)

	// CLUSTER MEET via Redis TCP protocol
	r := sendCmd(c1, "CLUSTER", "MEET", host, port, port)
	if r.Typ != resp.TypeSimpleString || r.Str != "OK" {
		t.Fatalf("expected OK from CLUSTER MEET, got %v", r)
	}

	// Verify node 1 recorded peer
	time.Sleep(50 * time.Millisecond)
	rInfo := sendCmd(c1, "CLUSTER", "INFO")
	if !strings.Contains(rInfo.Str, "cluster_known_nodes:2") {
		t.Fatalf("expected cluster_known_nodes:2 in info, got %s", rInfo.Str)
	}
}

