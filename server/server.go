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

	"github.com/Jatin-dudhani/my-redis/pubsub"
	"github.com/Jatin-dudhani/my-redis/resp"
	"github.com/Jatin-dudhani/my-redis/store"
)

type Server struct {
	addr    string
	dbPath  string
	ln      net.Listener
	store   *store.Store
	pubsub  *pubsub.Hub

	replicas   map[net.Conn]struct{}
	replicaMu  sync.Mutex
	isReplica  bool
	masterConn net.Conn
}

type clientState struct {
	inTx       bool
	queue      []resp.Value
	mu         sync.Mutex
	subscribed bool
	sub        *pubsub.Subscriber
	channels   []string
	isReplica  bool
}

func New(addr, dbPath string) *Server {
	s := &Server{
		addr:     addr,
		dbPath:   dbPath,
		store:    store.New(),
		pubsub:   pubsub.NewHub(),
		replicas: make(map[net.Conn]struct{}),
	}
	s.loadDB()
	return s
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
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	fmt.Printf("new connection from %s\n", conn.RemoteAddr())

	rd := resp.NewReaderSize(conn, 65536)
	wr := resp.NewWriterSize(conn, 65536)
	cs := &clientState{}
	vch := make(chan resp.Value, 1)
	errch := make(chan error, 1)

	// goroutine to read from network
	go func() {
		for {
			v, err := rd.Read()
			if err != nil {
				errch <- err
				return
			}
			vch <- v
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
			// remove from replicas on disconnect
			s.replicaMu.Lock()
			delete(s.replicas, conn)
			s.replicaMu.Unlock()
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
				fmt.Printf("write error: %v\n", err)
				return
			}
			if err := wr.Flush(); err != nil {
				fmt.Printf("flush error: %v\n", err)
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

	if cmd == "MULTI" {
		cs.mu.Lock()
		cs.inTx = true
		cs.queue = nil
		cs.mu.Unlock()
		return resp.SimpleString("OK")
	}

	if cmd == "EXEC" {
		cs.mu.Lock()
		if !cs.inTx {
			cs.mu.Unlock()
			return resp.Error("ERR EXEC without MULTI")
		}
		queue := cs.queue
		cs.inTx = false
		cs.queue = nil
		cs.mu.Unlock()
		if len(queue) == 0 {
			return resp.Array(nil)
		}
		results := make([]resp.Value, len(queue))
		for i, qv := range queue {
			results[i] = s.executeCommand(qv)
		}
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
}

func (s *Server) executeCommand(v resp.Value) resp.Value {
	cmd := strings.ToUpper(v.Array[0].Str)
	args := v.Array[1:]

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
	default:
		return resp.Error(fmt.Sprintf("ERR unknown command '%s'", cmd))
	}

	if writeCommands[cmd] {
		s.propagate(v)
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
	data := buf.String()
	for conn := range s.replicas {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write([]byte(data)); err != nil {
			fmt.Printf("replica write error: %v\n", err)
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
	// Minimally respond to GET/SET for benchmark compatibility
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
		// REPLICAOF NO ONE — become master
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
			fmt.Printf("replica connection lost: %v\n", err)
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
			return resp.SimpleString("OK")
		}
	}

	s.store.Set(key, value)
	return resp.SimpleString("OK")
}

func (s *Server) respGet(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'GET' command")
	}
	val, ok := s.store.Get(args[0].Str)
	if !ok {
		return resp.Null()
	}
	str, ok := val.(string)
	if !ok {
		return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
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
