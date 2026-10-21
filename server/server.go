package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jatin-dudhani/my-redis/acl"
	"github.com/Jatin-dudhani/my-redis/aof"
	"github.com/Jatin-dudhani/my-redis/cluster"
	"github.com/Jatin-dudhani/my-redis/ds"
	"github.com/Jatin-dudhani/my-redis/lua"
	"github.com/Jatin-dudhani/my-redis/pubsub"
	"github.com/Jatin-dudhani/my-redis/resp"
	"github.com/Jatin-dudhani/my-redis/store"
	"github.com/Jatin-dudhani/my-redis/telemetry"
)

type Server struct {
	addr      string
	dbPath    string
	aofPath   string
	aof       *aof.AOF
	ln        net.Listener
	ready     chan struct{}
	store     *store.Store
	pubsub    *pubsub.Hub
	cluster   *cluster.Cluster
	gossipBus *cluster.GossipBus
	startTime time.Time
	acl       *acl.Manager
	luaEngine *lua.Engine
	metrics   *telemetry.Metrics
	httpSrv   *telemetry.HTTPServer

	execMu    sync.Mutex
	clientsMu sync.RWMutex
	clients   map[*clientState]struct{}

	waiterMu sync.Mutex
	waiters  map[string][]chan string

	replicas   map[net.Conn]struct{}
	replicaMu  sync.Mutex
	isReplica  bool
	masterConn net.Conn
}

type clientState struct {
	inTx              bool
	queue             []resp.Value
	mu                sync.Mutex
	subscribed        bool
	sub               *pubsub.Subscriber
	channels          []string
	isReplica         bool
	watchedKeys       map[string]struct{}
	txDirty           bool
	authenticatedUser string
}

func New(addr, dbPath string) *Server {
	s := &Server{
		addr:      addr,
		dbPath:    dbPath,
		ready:     make(chan struct{}),
		store:     store.New(),
		pubsub:    pubsub.NewHub(),
		replicas:  make(map[net.Conn]struct{}),
		clients:   make(map[*clientState]struct{}),
		waiters:   make(map[string][]chan string),
		startTime: time.Now(),
		acl:       acl.NewManager(),
		luaEngine: lua.NewEngine(),
		metrics:   telemetry.NewMetrics(),
	}
	s.loadDB()
	return s
}

func (s *Server) EnableAOF(path string, policy aof.FsyncPolicy) error {
	a, err := aof.Open(path, policy)
	if err != nil {
		return err
	}
	s.aofPath = path
	err = a.Replay(func(v resp.Value) error {
		s.executeCommand(v)
		return nil
	})
	if err != nil {
		a.Close()
		return err
	}
	s.aof = a
	return nil
}

func (s *Server) EnableCluster(nodeID string, startSlot, endSlot uint16) {
	s.cluster = cluster.NewCluster(nodeID, s.addr, startSlot, endSlot)
}

func (s *Server) EnableClusterBus(busAddr string) error {
	if s.cluster == nil {
		s.EnableCluster("node-main", 0, 16383)
	}
	s.gossipBus = cluster.NewGossipBus(s.cluster, s.cluster.NodeID(), s.addr, busAddr)
	return s.gossipBus.Start()
}

func (s *Server) EnableHTTPTelemetry(httpAddr string) error {
	callbacks := telemetry.ServerCallbacks{
		GetClientsCount: func() int {
			s.clientsMu.RLock()
			defer s.clientsMu.RUnlock()
			return len(s.clients)
		},
		GetKeysCount: func() int {
			return s.store.Len()
		},
		GetKeysInfo: func() []telemetry.KeyInfo {
			dumps := s.store.DumpAll()
			var list []telemetry.KeyInfo
			for _, d := range dumps {
				typ := "unknown"
				switch d.Value.(type) {
				case string:
					typ = "string"
				case *ds.List:
					typ = "list"
				case *ds.Set:
					typ = "set"
				case *ds.Hash:
					typ = "hash"
				case *ds.SSet:
					typ = "zset"
				case *ds.Stream:
					typ = "stream"
				case *ds.HLL:
					typ = "hll"
				}
				ttl := int64(-1)
				if d.ExpiresAt != nil {
					rem := time.Until(*d.ExpiresAt)
					if rem > 0 {
						ttl = int64(rem.Seconds())
					}
				}
				list = append(list, telemetry.KeyInfo{
					Key:  d.Key,
					Type: typ,
					TTL:  ttl,
				})
			}
			return list
		},
		ExecuteCommand: func(cmdStr string) string {
			parts := strings.Fields(cmdStr)
			if len(parts) == 0 {
				return ""
			}
			var vals []resp.Value
			for _, p := range parts {
				vals = append(vals, resp.BulkString(p))
			}
			res := s.executeCommand(resp.Array(vals))
			switch res.Typ {
			case resp.TypeSimpleString, resp.TypeBulkString, resp.TypeError:
				return res.Str
			case resp.TypeInteger:
				return strconv.FormatInt(res.Num, 10)
			case resp.TypeNull:
				return "(nil)"
			case resp.TypeArray:
				var sb strings.Builder
				for i, item := range res.Array {
					sb.WriteString(fmt.Sprintf("%d) %s\n", i+1, item.Str))
				}
				return sb.String()
			default:
				return "OK"
			}
		},
	}
	s.httpSrv = telemetry.NewHTTPServer(httpAddr, s.metrics, callbacks)
	go func() {
		_ = s.httpSrv.Start()
	}()
	return nil
}

func (s *Server) Metrics() *telemetry.Metrics {
	return s.metrics
}

func (s *Server) ACL() *acl.Manager {
	return s.acl
}

func (s *Server) LuaEngine() *lua.Engine {
	return s.luaEngine
}

func (s *Server) GossipBus() *cluster.GossipBus {
	return s.gossipBus
}

func (s *Server) Cluster() *cluster.Cluster {
	return s.cluster
}

func (s *Server) Addr() net.Addr {
	<-s.ready
	return s.ln.Addr()
}

func (s *Server) WaitReady() {
	<-s.ready
}

func (s *Server) touchWatchedKey(key string) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	for cs := range s.clients {
		cs.mu.Lock()
		if cs.watchedKeys != nil {
			if _, ok := cs.watchedKeys[key]; ok {
				cs.txDirty = true
			}
		}
		cs.mu.Unlock()
	}
}

func (s *Server) notifyListWaiter(key, item string) bool {
	s.waiterMu.Lock()
	defer s.waiterMu.Unlock()
	chList := s.waiters[key]
	if len(chList) > 0 {
		ch := chList[0]
		s.waiters[key] = chList[1:]
		ch <- item
		return true
	}
	return false
}

func (s *Server) loadDB() {
	if s.dbPath == "" {
		return
	}
	loaded, err := store.LoadFromFile(s.dbPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("warning: failed to load DB: %v\n", err)
		}
		return
	}
	s.store = loaded
	fmt.Printf("loaded %d keys from %s\n", s.store.Len(), s.dbPath)
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	s.ln = ln
	select {
	case <-s.ready:
	default:
		close(s.ready)
	}

	fmt.Printf("server listening on %s\n", s.addr)
	if s.store == nil {
		s.store = store.New()
	}
	s.store.StartCleanup(100 * time.Millisecond)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			fmt.Printf("accept error: %v\n", err)
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) Stop() error {
	s.store.StopCleanup()
	if s.aof != nil {
		_ = s.aof.Close()
	}
	if s.httpSrv != nil {
		_ = s.httpSrv.Stop()
	}
	if s.gossipBus != nil {
		s.gossipBus.Stop()
	}
	if s.metrics != nil {
		s.metrics.Close()
	}
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	defer func() {
		s.replicaMu.Lock()
		delete(s.replicas, conn)
		s.replicaMu.Unlock()
	}()

	rd := resp.NewReaderSize(conn, 65536)
	wr := resp.NewWriterSize(conn, 65536)
	cs := &clientState{
		watchedKeys: make(map[string]struct{}),
	}

	s.clientsMu.Lock()
	s.clients[cs] = struct{}{}
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, cs)
		s.clientsMu.Unlock()
	}()

	vch := make(chan resp.Value, 1)
	errch := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			v, err := rd.Read()
			if err != nil {
				select {
				case errch <- err:
				case <-done:
				}
				return
			}
			select {
			case vch <- v:
			case <-done:
				return
			}
		}
	}()

	for {
		if cs.subscribed {
			select {
			case msg := <-cs.sub.Messages:
				reply := resp.Array([]resp.Value{
					resp.BulkString("message"),
					resp.BulkString(msg.Channel),
					resp.BulkString(msg.Payload),
				})
				if err := wr.Write(reply); err != nil {
					return
				}
				if err := wr.Flush(); err != nil {
					return
				}
				continue
			case v := <-vch:
				s.handleSubscribedCommand(v, cs, wr)
				continue
			case <-errch:
				return
			}
		}

		select {
		case <-errch:
			return
		case v := <-vch:
			if cs.isReplica {
				continue
			}

			if v.Typ == resp.TypeArray && len(v.Array) >= 3 {
				cmd := strings.ToUpper(v.Array[0].Str)
				if cmd == "REPLCONF" && strings.ToUpper(v.Array[1].Str) == "LISTENING-PORT" {
					s.replicaMu.Lock()
					s.replicas[conn] = struct{}{}
					s.replicaMu.Unlock()
					cs.isReplica = true
					if err := wr.Write(resp.SimpleString("OK")); err != nil {
						return
					}
					if err := wr.Flush(); err != nil {
						return
					}
					continue
				}
			}

			if s.isSubscribeCommand(v) {
				s.enterSubscribeMode(v, cs, wr)
				continue
			}

			reply := s.processRESP(v, cs)
			if err := wr.Write(reply); err != nil {
				return
			}
			if err := wr.Flush(); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleSubscribedCommand(v resp.Value, cs *clientState, wr *resp.Writer) {
	if v.Typ != resp.TypeArray || len(v.Array) == 0 {
		return
	}
	cmd := strings.ToUpper(v.Array[0].Str)
	args := v.Array[1:]

	switch cmd {
	case "SUBSCRIBE":
		for _, a := range args {
			ch := a.Str
			cs.channels = append(cs.channels, ch)
			s.pubsub.Subscribe(ch, cs.sub)
		}
		for _, a := range args {
			wr.Write(subscribeReply(a.Str, len(cs.channels)))
		}
	case "UNSUBSCRIBE":
		if len(args) == 0 {
			s.pubsub.UnsubscribeAll(cs.sub)
			for _, ch := range cs.channels {
				wr.Write(unsubscribeReply(ch, 0))
			}
			cs.channels = nil
		} else {
			for _, a := range args {
				ch := a.Str
				s.pubsub.Unsubscribe(ch, cs.sub)
				cs.channels = removeStr(cs.channels, ch)
				wr.Write(unsubscribeReply(ch, len(cs.channels)))
			}
		}
		if len(cs.channels) == 0 {
			cs.subscribed = false
		}
	case "PING":
		wr.Write(resp.SimpleString("PONG"))
	case "QUIT":
		wr.Write(resp.SimpleString("OK"))
		wr.Flush()
		return
	default:
		wr.Write(resp.Error(fmt.Sprintf("ERR unknown command '%s' in subscribed mode", cmd)))
	}
	wr.Flush()
}

func subscribeReply(channel string, count int) resp.Value {
	return resp.Array([]resp.Value{
		resp.BulkString("subscribe"),
		resp.BulkString(channel),
		resp.Integer(int64(count)),
	})
}

func unsubscribeReply(channel string, count int) resp.Value {
	return resp.Array([]resp.Value{
		resp.BulkString("unsubscribe"),
		resp.BulkString(channel),
		resp.Integer(int64(count)),
	})
}

func (s *Server) processRESP(v resp.Value, cs *clientState) resp.Value {
	if v.Typ != resp.TypeArray || len(v.Array) == 0 {
		return resp.Error("ERR protocol error: expected array")
	}
	cmd := strings.ToUpper(v.Array[0].Str)

	// Authentication & ACL checking
	if s.acl != nil {
		cs.mu.Lock()
		user := cs.authenticatedUser
		if user == "" {
			user = "default"
		}
		cs.mu.Unlock()

		if cmd == "AUTH" {
			return s.handleAuth(v.Array[1:], cs)
		}

		firstKey := ""
		if len(v.Array) > 1 {
			firstKey = v.Array[1].Str
		}
		if ok, reason := s.acl.CheckPermission(user, cmd, firstKey); !ok {
			return resp.Error(reason)
		}
	}

	if cmd == "WATCH" {
		if len(v.Array) < 2 {
			return resp.Error("ERR wrong number of arguments for 'WATCH' command")
		}
		cs.mu.Lock()
		for _, a := range v.Array[1:] {
			cs.watchedKeys[a.Str] = struct{}{}
		}
		cs.mu.Unlock()
		return resp.SimpleString("OK")
	}

	if cmd == "UNWATCH" {
		cs.mu.Lock()
		cs.watchedKeys = make(map[string]struct{})
		cs.mu.Unlock()
		return resp.SimpleString("OK")
	}

	if cmd == "MULTI" {
		cs.mu.Lock()
		cs.inTx = true
		cs.queue = nil
		cs.txDirty = false
		cs.mu.Unlock()
		return resp.SimpleString("OK")
	}

	if cmd == "EXEC" {
		cs.mu.Lock()
		if !cs.inTx {
			cs.mu.Unlock()
			return resp.Error("ERR EXEC without MULTI")
		}
		if cs.txDirty {
			cs.inTx = false
			cs.queue = nil
			cs.watchedKeys = make(map[string]struct{})
			cs.txDirty = false
			cs.mu.Unlock()
			return resp.Null() // *-1\r\n
		}
		queue := cs.queue
		cs.inTx = false
		cs.queue = nil
		cs.watchedKeys = make(map[string]struct{})
		cs.mu.Unlock()

		if len(queue) == 0 {
			return resp.Array(nil)
		}

		s.execMu.Lock()
		results := make([]resp.Value, len(queue))
		for i, qv := range queue {
			results[i] = s.executeCommand(qv)
		}
		s.execMu.Unlock()
		return resp.Array(results)
	}

	if cmd == "DISCARD" {
		cs.mu.Lock()
		if !cs.inTx {
			cs.mu.Unlock()
			return resp.Error("ERR DISCARD without MULTI")
		}
		cs.inTx = false
		cs.queue = nil
		cs.watchedKeys = make(map[string]struct{})
		cs.txDirty = false
		cs.mu.Unlock()
		return resp.SimpleString("OK")
	}

	cs.mu.Lock()
	if cs.inTx {
		cs.queue = append(cs.queue, v)
		cs.mu.Unlock()
		return resp.SimpleString("QUEUED")
	}
	cs.mu.Unlock()

	return s.executeCommand(v)
}

func (s *Server) isSubscribeCommand(v resp.Value) bool {
	if v.Typ != resp.TypeArray || len(v.Array) == 0 {
		return false
	}
	cmd := strings.ToUpper(v.Array[0].Str)
	return cmd == "SUBSCRIBE" || cmd == "PSUBSCRIBE"
}

func (s *Server) enterSubscribeMode(v resp.Value, cs *clientState, wr *resp.Writer) {
	cs.subscribed = true
	cs.sub = pubsub.NewSubscriber()
	cs.channels = nil

	for i := 1; i < len(v.Array); i++ {
		ch := v.Array[i].Str
		cs.channels = append(cs.channels, ch)
		s.pubsub.Subscribe(ch, cs.sub)
	}

	for _, ch := range cs.channels {
		wr.Write(subscribeReply(ch, len(cs.channels)))
	}
}

var writeCommands = map[string]bool{
	"SET": true, "DEL": true, "EXPIRE": true,
	"LPUSH": true, "RPUSH": true, "LPOP": true, "RPOP": true,
	"SADD": true, "SREM": true,
	"HSET": true, "HDEL": true,
	"ZADD": true, "ZREM": true,
	"XADD": true, "FLUSHDB": true,
	"SETBIT": true, "BITOP": true,
	"PFADD": true, "PFMERGE": true,
	"GEOADD": true,
}

func (s *Server) executeCommand(v resp.Value) resp.Value {
	cmd := strings.ToUpper(v.Array[0].Str)
	args := v.Array[1:]

	if s.metrics != nil {
		s.metrics.RecordCommand(cmd)
	}

	// Cluster redirection check if enabled
	if s.cluster != nil && len(args) > 0 {
		firstKey := args[0].Str
		if isLocal, targetAddr, slot := s.cluster.CheckSlot(firstKey); !isLocal {
			return resp.Error(fmt.Sprintf("MOVED %d %s", slot, targetAddr))
		}
	}

	var reply resp.Value

	switch cmd {
	case "PING":
		reply = s.respPing(args)
	case "PUBLISH":
		reply = s.respPublish(args)
	case "SET":
		reply = s.respSet(args)
	case "GET":
		reply = s.respGet(args)
	case "DEL":
		reply = s.respDel(args)
	case "EXISTS":
		reply = s.respExists(args)
	case "EXPIRE":
		reply = s.respExpire(args)
	case "TTL":
		reply = s.respTTL(args)
	case "SAVE":
		reply = s.respSave(args)
	case "BGSAVE":
		reply = s.respBGSave(args)
	case "BGREWRITEAOF":
		reply = s.respBGRewriteAOF(args)
	case "CONFIG":
		reply = s.respConfig(args)
	case "MAXMEMORY":
		reply = s.respMaxMemory(args)
	case "REPLICAOF":
		reply = s.respReplicaOf(args)
	case "LPUSH":
		reply = s.respLPush(args)
	case "RPUSH":
		reply = s.respRPush(args)
	case "LPOP":
		reply = s.respLPop(args)
	case "RPOP":
		reply = s.respRPop(args)
	case "BLPOP":
		reply = s.respBLPop(args)
	case "BRPOP":
		reply = s.respBRPop(args)
	case "LRANGE":
		reply = s.respLRange(args)
	case "SADD":
		reply = s.respSAdd(args)
	case "SMEMBERS":
		reply = s.respSMembers(args)
	case "SREM":
		reply = s.respSRem(args)
	case "SISMEMBER":
		reply = s.respSIsMember(args)
	case "HSET":
		reply = s.respHSet(args)
	case "HGET":
		reply = s.respHGet(args)
	case "HDEL":
		reply = s.respHDel(args)
	case "HEXISTS":
		reply = s.respHExists(args)
	case "HGETALL":
		reply = s.respHGetAll(args)
	case "ZADD":
		reply = s.respZAdd(args)
	case "ZRANGE":
		reply = s.respZRange(args)
	case "ZREM":
		reply = s.respZRem(args)
	case "ZSCORE":
		reply = s.respZScore(args)
	case "XADD":
		reply = s.respXAdd(args)
	case "XLEN":
		reply = s.respXLen(args)
	case "XRANGE":
		reply = s.respXRange(args)
	case "XREAD":
		reply = s.respXRead(args)
	case "TYPE":
		reply = s.respType(args)
	case "KEYS":
		reply = s.respKeys(args)
	case "DBSIZE":
		reply = s.respDBSize(args)
	case "FLUSHDB":
		reply = s.respFlushDB(args)
	case "INFO":
		reply = s.respInfo(args)
	case "CLUSTER":
		reply = s.respCluster(args)

	// Bitmaps
	case "SETBIT":
		reply = s.respSetBit(args)
	case "GETBIT":
		reply = s.respGetBit(args)
	case "BITCOUNT":
		reply = s.respBitCount(args)
	case "BITPOS":
		reply = s.respBitPos(args)
	case "BITOP":
		reply = s.respBitOp(args)

	// HyperLogLog
	case "PFADD":
		reply = s.respPFAdd(args)
	case "PFCOUNT":
		reply = s.respPFCount(args)
	case "PFMERGE":
		reply = s.respPFMerge(args)

	// Geospatial
	case "GEOADD":
		reply = s.respGeoAdd(args)
	case "GEOPOS":
		reply = s.respGeoPos(args)
	case "GEODIST":
		reply = s.respGeoDist(args)
	case "GEOSEARCH":
		reply = s.respGeoSearch(args)
	case "GEORADIUS":
		reply = s.respGeoRadius(args)
	case "GEOHASH":
		reply = s.respGeoHash(args)

	// Lua Scripting
	case "EVAL":
		reply = s.respEval(args)
	case "EVALSHA":
		reply = s.respEvalSHA(args)
	case "SCRIPT":
		reply = s.respScript(args)

	// Enterprise ACL & Auth
	case "ACL":
		reply = s.respACL(args)
	case "AUTH":
		if len(args) == 0 {
			reply = resp.Error("ERR wrong number of arguments for 'AUTH' command")
		} else {
			u := "default"
			p := args[0].Str
			if len(args) >= 2 {
				u = args[0].Str
				p = args[1].Str
			}
			if s.acl != nil && s.acl.Authenticate(u, p) {
				reply = resp.SimpleString("OK")
			} else {
				reply = resp.Error("WRONGPASS invalid username-password pair or user is disabled.")
			}
		}

	default:
		return resp.Error(fmt.Sprintf("ERR unknown command '%s'", cmd))
	}

	if writeCommands[cmd] {
		s.propagate(v)
		if s.aof != nil {
			_ = s.aof.Write(v)
		}
	}

	return reply
}

func (s *Server) propagate(v resp.Value) {
	s.replicaMu.Lock()
	defer s.replicaMu.Unlock()
	if len(s.replicas) == 0 {
		return
	}
	var buf strings.Builder
	w := resp.NewWriter(&buf)
	if err := w.Write(v); err != nil {
		return
	}
	_ = w.Flush()
	data := buf.String()
	for conn := range s.replicas {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write([]byte(data)); err != nil {
			conn.Close()
			delete(s.replicas, conn)
		}
	}
}

func (s *Server) respConfig(args []resp.Value) resp.Value {
	if len(args) == 2 && strings.ToUpper(args[0].Str) == "GET" {
		key := strings.ToUpper(args[1].Str)
		if key == "MAXMEMORY" {
			return resp.Array([]resp.Value{
				resp.BulkString("maxmemory"),
				resp.BulkString("0"),
			})
		}
	}
	return resp.SimpleString("OK")
}

func (s *Server) respMaxMemory(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'MAXMEMORY' command")
	}
	n, err := strconv.Atoi(args[0].Str)
	if err != nil {
		return resp.Error("ERR value is not an integer or out of range")
	}
	s.store.SetMaxKeys(n)
	return resp.SimpleString("OK")
}

func (s *Server) respReplicaOf(args []resp.Value) resp.Value {
	if len(args) == 1 && strings.ToUpper(args[0].Str) == "NO" {
		s.replicaMu.Lock()
		if s.masterConn != nil {
			s.masterConn.Close()
			s.masterConn = nil
		}
		s.isReplica = false
		s.replicaMu.Unlock()
		return resp.SimpleString("OK")
	}
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'REPLICAOF' command")
	}
	host := args[0].Str
	port := args[1].Str

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return resp.Error(fmt.Sprintf("ERR connecting to master: %v", err))
	}

	s.replicaMu.Lock()
	if s.masterConn != nil {
		s.masterConn.Close()
	}
	s.masterConn = conn
	s.isReplica = true
	s.replicaMu.Unlock()

	go s.replicaReceiver(conn)
	return resp.SimpleString("OK")
}

func (s *Server) replicaReceiver(conn net.Conn) {
	defer conn.Close()
	rd := resp.NewReader(conn)
	for {
		v, err := rd.Read()
		if err != nil {
			s.replicaMu.Lock()
			s.isReplica = false
			s.masterConn = nil
			s.replicaMu.Unlock()
			return
		}
		s.executeCommand(v)
	}
}

func (s *Server) respPublish(args []resp.Value) resp.Value {
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'PUBLISH' command")
	}
	count := s.pubsub.Publish(args[0].Str, args[1].Str)
	return resp.Integer(int64(count))
}

func (s *Server) respPing(args []resp.Value) resp.Value {
	if len(args) > 0 {
		return resp.BulkString(args[0].Str)
	}
	return resp.SimpleString("PONG")
}

func (s *Server) respSet(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'SET' command")
	}
	key := args[0].Str
	value := args[1].Str

	for i := 2; i < len(args); i++ {
		opt := strings.ToUpper(args[i].Str)
		switch opt {
		case "EX":
			if i+1 >= len(args) {
				return resp.Error("ERR syntax error")
			}
			secs, err := strconv.ParseInt(args[i+1].Str, 10, 64)
			if err != nil {
				return resp.Error("ERR value is not an integer or out of range")
			}
			s.store.SetWithTTL(key, value, time.Duration(secs)*time.Second)
			s.touchWatchedKey(key)
			return resp.SimpleString("OK")
		case "PX":
			if i+1 >= len(args) {
				return resp.Error("ERR syntax error")
			}
			millis, err := strconv.ParseInt(args[i+1].Str, 10, 64)
			if err != nil {
				return resp.Error("ERR value is not an integer or out of range")
			}
			s.store.SetWithTTL(key, value, time.Duration(millis)*time.Millisecond)
			s.touchWatchedKey(key)
			return resp.SimpleString("OK")
		}
	}

	s.store.Set(key, value)
	s.touchWatchedKey(key)
	return resp.SimpleString("OK")
}

func (s *Server) respGet(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'GET' command")
	}
	val, ok := s.store.Get(args[0].Str)
	if !ok || val == nil {
		if s.metrics != nil {
			s.metrics.RecordMiss()
		}
		return resp.Null()
	}
	str, ok := val.(string)
	if !ok {
		return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	if s.metrics != nil {
		s.metrics.RecordHit()
	}
	return resp.BulkString(str)
}

func (s *Server) respDel(args []resp.Value) resp.Value {
	if len(args) < 1 {
		return resp.Error("ERR wrong number of arguments for 'DEL' command")
	}
	count := int64(0)
	for _, arg := range args {
		if s.store.Exists(arg.Str) {
			s.store.Delete(arg.Str)
			s.touchWatchedKey(arg.Str)
			count++
		}
	}
	return resp.Integer(count)
}

func (s *Server) respExists(args []resp.Value) resp.Value {
	if len(args) < 1 {
		return resp.Error("ERR wrong number of arguments for 'EXISTS' command")
	}
	count := int64(0)
	for _, arg := range args {
		if s.store.Exists(arg.Str) {
			count++
		}
	}
	return resp.Integer(count)
}

func (s *Server) respExpire(args []resp.Value) resp.Value {
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'EXPIRE' command")
	}
	secs, err := strconv.ParseInt(args[1].Str, 10, 64)
	if err != nil {
		return resp.Error("ERR value is not an integer or out of range")
	}
	ok := s.store.Expire(args[0].Str, time.Duration(secs)*time.Second)
	if ok {
		s.touchWatchedKey(args[0].Str)
		return resp.Integer(1)
	}
	return resp.Integer(0)
}

func (s *Server) respTTL(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'TTL' command")
	}
	ttl := s.store.TTL(args[0].Str)
	return resp.Integer(ttl)
}

func (s *Server) respSave(args []resp.Value) resp.Value {
	if s.dbPath == "" {
		return resp.Error("ERR no DB path configured")
	}
	if err := store.SaveToFile(s.store, s.dbPath); err != nil {
		return resp.Error(fmt.Sprintf("ERR saving DB: %v", err))
	}
	return resp.SimpleString("OK")
}

func removeStr(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
