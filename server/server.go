package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

type Server struct {
	addr string
	ln   net.Listener
}

func New(addr string) *Server {
	return &Server{addr: addr}
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

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		reply := s.processCommand(line)
		_, err := fmt.Fprint(conn, reply+"\n")
		if err != nil {
			fmt.Printf("write to %s: %v\n", conn.RemoteAddr(), err)
			return
		}
	}
}

func (s *Server) processCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	switch strings.ToUpper(parts[0]) {
	case "PING":
		return "PONG"
	default:
		return fmt.Sprintf("ERR unknown command '%s'", parts[0])
	}
}
