package server

import (
	"fmt"
	"net"
	"strings"

	"github.com/macbook/my-redis/resp"
	"github.com/macbook/my-redis/store"
)

type Server struct {
	addr  string
	ln    net.Listener
	store *store.Store
}

func New(addr string) *Server {
	return &Server{
		addr:  addr,
		store: store.New(),
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	s.ln = ln
	fmt.Printf("server listening on %s\n", s.addr)

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
	return s.ln.Close()
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
