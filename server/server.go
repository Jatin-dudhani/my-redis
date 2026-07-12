package server

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/macbook/my-redis/resp"
	"github.com/macbook/my-redis/store"
)

type Server struct {
	addr   string
	dbPath string
	ln     net.Listener
	store  *store.Store
}

func New(addr, dbPath string) *Server {
	s := &Server{
		addr:   addr,
		dbPath: dbPath,
		store:  store.New(),
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

	rd := resp.NewReader(conn)
	wr := resp.NewWriter(conn)

	for {
		v, err := rd.Read()
		if err != nil {
			fmt.Printf("read from %s: %v\n", conn.RemoteAddr(), err)
			return
		}
		reply := s.processRESP(v)
		if err := wr.Write(reply); err != nil {
			fmt.Printf("write to %s: %v\n", conn.RemoteAddr(), err)
			return
		}
	}
}

func (s *Server) processRESP(v resp.Value) resp.Value {
	if v.Typ != resp.TypeArray || len(v.Array) == 0 {
		return resp.Error("ERR protocol error: expected array")
	}
	cmd := strings.ToUpper(v.Array[0].Str)
	args := v.Array[1:]

	switch cmd {
	case "PING":
		return s.respPing(args)
	case "SET":
		return s.respSet(args)
	case "GET":
		return s.respGet(args)
	case "DEL":
		return s.respDel(args)
	case "EXISTS":
		return s.respExists(args)
	case "EXPIRE":
		return s.respExpire(args)
	case "TTL":
		return s.respTTL(args)
	case "SAVE":
		return s.respSave(args)
	default:
		return resp.Error(fmt.Sprintf("ERR unknown command '%s'", cmd))
	}
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
	return resp.BulkString(val)
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
