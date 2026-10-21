package server

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Jatin-dudhani/my-redis/cluster"
	"github.com/Jatin-dudhani/my-redis/ds"
	"github.com/Jatin-dudhani/my-redis/resp"
	"github.com/Jatin-dudhani/my-redis/store"
)

// --- Key Management & Inspection ---

func (s *Server) respType(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'TYPE' command")
	}
	val, ok := s.store.Get(args[0].Str)
	if !ok || val == nil {
		return resp.SimpleString("none")
	}
	switch val.(type) {
	case string:
		return resp.SimpleString("string")
	case *ds.List:
		return resp.SimpleString("list")
	case *ds.Set:
		return resp.SimpleString("set")
	case *ds.Hash:
		return resp.SimpleString("hash")
	case *ds.SSet:
		return resp.SimpleString("zset")
	case *ds.Stream:
		return resp.SimpleString("stream")
	default:
		return resp.SimpleString("unknown")
	}
}

func (s *Server) respKeys(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'KEYS' command")
	}
	pattern := args[0].Str
	dumps := s.store.DumpAll()
	var matched []resp.Value

	for _, d := range dumps {
		if pattern == "*" {
			matched = append(matched, resp.BulkString(d.Key))
		} else if ok, _ := filepath.Match(pattern, d.Key); ok {
			matched = append(matched, resp.BulkString(d.Key))
		}
	}
	return resp.Array(matched)
}

func (s *Server) respDBSize(args []resp.Value) resp.Value {
	return resp.Integer(int64(s.store.Len()))
}

func (s *Server) respFlushDB(args []resp.Value) resp.Value {
	s.store = store.New()
	return resp.SimpleString("OK")
}

// --- Observability & Stats ---

func (s *Server) respInfo(args []resp.Value) resp.Value {
	uptime := int64(time.Since(s.startTime).Seconds())
	role := "master"
	s.replicaMu.Lock()
	if s.isReplica {
		role = "slave"
	}
	numReplicas := len(s.replicas)
	s.replicaMu.Unlock()

	s.clientsMu.RLock()
	numClients := len(s.clients)
	s.clientsMu.RUnlock()

	var b strings.Builder
	b.WriteString("# Server\r\n")
	b.WriteString("redis_version:7.2.0-highscale\r\n")
	b.WriteString("redis_mode:standalone\r\n")
	b.WriteString(fmt.Sprintf("uptime_in_seconds:%d\r\n", uptime))
	b.WriteString(fmt.Sprintf("tcp_port:%s\r\n", s.addr))
	b.WriteString("# Clients\r\n")
	b.WriteString(fmt.Sprintf("connected_clients:%d\r\n", numClients))
	b.WriteString("# Memory\r\n")
	b.WriteString(fmt.Sprintf("maxmemory:%d\r\n", s.store.MaxKeys()))
	b.WriteString("# Persistence\r\n")
	aofEnabled := 0
	if s.aof != nil {
		aofEnabled = 1
	}
	b.WriteString(fmt.Sprintf("aof_enabled:%d\r\n", aofEnabled))
	b.WriteString("# Replication\r\n")
	b.WriteString(fmt.Sprintf("role:%s\r\n", role))
	b.WriteString(fmt.Sprintf("connected_slaves:%d\r\n", numReplicas))
	b.WriteString("# Keyspace\r\n")
	b.WriteString(fmt.Sprintf("db0:keys=%d,expires=0\r\n", s.store.Len()))

	return resp.BulkString(b.String())
}

// --- Persistence Background Commands ---

func (s *Server) respBGSave(args []resp.Value) resp.Value {
	if s.dbPath == "" {
		return resp.Error("ERR no DB path configured")
	}
	go func() {
		_ = store.SaveToFile(s.store, s.dbPath)
	}()
	return resp.SimpleString("Background saving started")
}

func (s *Server) respBGRewriteAOF(args []resp.Value) resp.Value {
	if s.aof == nil {
		return resp.Error("ERR AOF not enabled")
	}
	go func() {
		dumps := s.store.DumpAll()
		var cmds []resp.Value
		for _, d := range dumps {
			switch val := d.Value.(type) {
			case string:
				cmds = append(cmds, resp.Array([]resp.Value{
					resp.BulkString("SET"),
					resp.BulkString(d.Key),
					resp.BulkString(val),
				}))
			case *ds.List:
				items := val.Items()
				if len(items) > 0 {
					arr := []resp.Value{resp.BulkString("RPUSH"), resp.BulkString(d.Key)}
					for _, item := range items {
						arr = append(arr, resp.BulkString(item))
					}
					cmds = append(cmds, resp.Array(arr))
				}
			case *ds.Set:
				members := val.SMembers()
				if len(members) > 0 {
					arr := []resp.Value{resp.BulkString("SADD"), resp.BulkString(d.Key)}
					for _, m := range members {
						arr = append(arr, resp.BulkString(m))
					}
					cmds = append(cmds, resp.Array(arr))
				}
			case *ds.Hash:
				all := val.HGetAll()
				if len(all) > 0 {
					arr := []resp.Value{resp.BulkString("HSET"), resp.BulkString(d.Key)}
					for fld, fv := range all {
						arr = append(arr, resp.BulkString(fld), resp.BulkString(fv))
					}
					cmds = append(cmds, resp.Array(arr))
				}
			case *ds.SSet:
				entries := val.Entries()
				if len(entries) > 0 {
					arr := []resp.Value{resp.BulkString("ZADD"), resp.BulkString(d.Key)}
					for _, e := range entries {
						arr = append(arr, resp.BulkString(fmt.Sprintf("%g", e.Score)), resp.BulkString(e.Member))
					}
					cmds = append(cmds, resp.Array(arr))
				}
			}
			if d.ExpiresAt != nil {
				rem := time.Until(*d.ExpiresAt)
				if rem > 0 {
					cmds = append(cmds, resp.Array([]resp.Value{
						resp.BulkString("EXPIRE"),
						resp.BulkString(d.Key),
						resp.BulkString(strconv.FormatInt(int64(rem.Seconds()), 10)),
					}))
				}
			}
		}
		_ = s.aof.Rewrite(cmds)
	}()
	return resp.SimpleString("Background append only file rewriting started")
}

// --- Blocking List Operations (BLPOP / BRPOP) ---

func (s *Server) respBLPop(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'BLPOP' command")
	}
	timeoutSec, err := strconv.ParseFloat(args[len(args)-1].Str, 64)
	if err != nil {
		return resp.Error("ERR timeout is not a float or out of range")
	}
	keys := args[:len(args)-1]

	// Check if any key already has elements
	for _, k := range keys {
		val, _ := s.store.Get(k.Str)
		if list, ok := val.(*ds.List); ok && list != nil {
			if elem, popped := list.LPop(); popped {
				if list.LLen() == 0 {
					s.store.Delete(k.Str)
				}
				s.touchWatchedKey(k.Str)
				return resp.Array([]resp.Value{
					resp.BulkString(k.Str),
					resp.BulkString(elem),
				})
			}
		}
	}

	if timeoutSec <= 0 {
		return resp.Null()
	}

	// Register waiter on keys
	ch := make(chan string, 1)
	s.waiterMu.Lock()
	for _, k := range keys {
		s.waiters[k.Str] = append(s.waiters[k.Str], ch)
	}
	s.waiterMu.Unlock()

	select {
	case elem := <-ch:
		// Return the first key that was signaled
		return resp.Array([]resp.Value{
			resp.BulkString(keys[0].Str),
			resp.BulkString(elem),
		})
	case <-time.After(time.Duration(timeoutSec * float64(time.Second))):
		return resp.Null()
	}
}

func (s *Server) respBRPop(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'BRPOP' command")
	}
	timeoutSec, err := strconv.ParseFloat(args[len(args)-1].Str, 64)
	if err != nil {
		return resp.Error("ERR timeout is not a float or out of range")
	}
	keys := args[:len(args)-1]

	// Check if any key already has elements
	for _, k := range keys {
		val, _ := s.store.Get(k.Str)
		if list, ok := val.(*ds.List); ok && list != nil {
			if elem, popped := list.RPop(); popped {
				if list.LLen() == 0 {
					s.store.Delete(k.Str)
				}
				s.touchWatchedKey(k.Str)
				return resp.Array([]resp.Value{
					resp.BulkString(k.Str),
					resp.BulkString(elem),
				})
			}
		}
	}

	if timeoutSec <= 0 {
		return resp.Null()
	}

	ch := make(chan string, 1)
	s.waiterMu.Lock()
	for _, k := range keys {
		s.waiters[k.Str] = append(s.waiters[k.Str], ch)
	}
	s.waiterMu.Unlock()

	select {
	case elem := <-ch:
		return resp.Array([]resp.Value{
			resp.BulkString(keys[0].Str),
			resp.BulkString(elem),
		})
	case <-time.After(time.Duration(timeoutSec * float64(time.Second))):
		return resp.Null()
	}
}

// --- Redis Streams Commands ---

func (s *Server) respXAdd(args []resp.Value) resp.Value {
	if len(args) < 4 || (len(args)-2)%2 != 0 {
		return resp.Error("ERR wrong number of arguments for 'XADD' command")
	}
	key := args[0].Str
	idStr := args[1].Str

	val, _ := s.store.Get(key)
	st, ok := val.(*ds.Stream)
	if !ok {
		if val != nil {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		st = ds.NewStream()
	}

	values := make([]string, len(args)-2)
	for i, a := range args[2:] {
		values[i] = a.Str
	}

	id, err := st.XAdd(idStr, values...)
	if err != nil {
		return resp.Error(err.Error())
	}
	s.store.Set(key, st)
	s.touchWatchedKey(key)
	return resp.BulkString(id.String())
}

func (s *Server) respXLen(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'XLEN' command")
	}
	val, _ := s.store.Get(args[0].Str)
	st, ok := val.(*ds.Stream)
	if !ok || st == nil {
		return resp.Integer(0)
	}
	return resp.Integer(int64(st.XLen()))
}

func (s *Server) respXRange(args []resp.Value) resp.Value {
	if len(args) < 3 {
		return resp.Error("ERR wrong number of arguments for 'XRANGE' command")
	}
	key := args[0].Str
	start := args[1].Str
	end := args[2].Str

	val, _ := s.store.Get(key)
	st, ok := val.(*ds.Stream)
	if !ok || st == nil {
		return resp.Array(nil)
	}

	msgs := st.XRange(start, end)
	res := make([]resp.Value, len(msgs))
	for i, m := range msgs {
		fieldVals := make([]resp.Value, len(m.Values))
		for j, fv := range m.Values {
			fieldVals[j] = resp.BulkString(fv)
		}
		res[i] = resp.Array([]resp.Value{
			resp.BulkString(m.ID.String()),
			resp.Array(fieldVals),
		})
	}
	return resp.Array(res)
}

func (s *Server) respXRead(args []resp.Value) resp.Value {
	// Simple parsing: XREAD STREAMS key [key...] ID [ID...]
	streamsIdx := -1
	count := 0
	for i := 0; i < len(args); i++ {
		upper := strings.ToUpper(args[i].Str)
		if upper == "COUNT" && i+1 < len(args) {
			c, _ := strconv.Atoi(args[i+1].Str)
			count = c
			i++
		} else if upper == "STREAMS" {
			streamsIdx = i
			break
		}
	}
	if streamsIdx == -1 {
		return resp.Error("ERR syntax error")
	}

	remArgs := args[streamsIdx+1:]
	if len(remArgs)%2 != 0 || len(remArgs) == 0 {
		return resp.Error("ERR syntax error in XREAD STREAMS")
	}
	numKeys := len(remArgs) / 2
	keys := remArgs[:numKeys]
	ids := remArgs[numKeys:]

	var streamResults []resp.Value
	for i := 0; i < numKeys; i++ {
		key := keys[i].Str
		idStr := ids[i].Str
		val, _ := s.store.Get(key)
		st, ok := val.(*ds.Stream)
		if !ok || st == nil {
			continue
		}
		msgs := st.XReadAfter(idStr, count)
		if len(msgs) == 0 {
			continue
		}
		msgVals := make([]resp.Value, len(msgs))
		for j, m := range msgs {
			fieldVals := make([]resp.Value, len(m.Values))
			for k, fv := range m.Values {
				fieldVals[k] = resp.BulkString(fv)
			}
			msgVals[j] = resp.Array([]resp.Value{
				resp.BulkString(m.ID.String()),
				resp.Array(fieldVals),
			})
		}
		streamResults = append(streamResults, resp.Array([]resp.Value{
			resp.BulkString(key),
			resp.Array(msgVals),
		}))
	}

	if len(streamResults) == 0 {
		return resp.Null()
	}
	return resp.Array(streamResults)
}

// --- Redis Cluster Commands ---

func (s *Server) respCluster(args []resp.Value) resp.Value {
	if len(args) == 0 {
		return resp.Error("ERR wrong number of arguments for 'CLUSTER' command")
	}
	subCmd := strings.ToUpper(args[0].Str)
	switch subCmd {
	case "KEYSLOT":
		if len(args) != 2 {
			return resp.Error("ERR wrong number of arguments for 'CLUSTER KEYSLOT' command")
		}
		slot := cluster.KeySlot(args[1].Str)
		return resp.Integer(int64(slot))
	case "NODES":
		if s.cluster != nil {
			return resp.BulkString(s.cluster.NodesInfo())
		}
		// Default standalone fallback node
		nodeInfo := fmt.Sprintf("07c37dfeb235213b87975015de450f364e42bf9d %s@16379 myself,master - 0 0 1 connected 0-16383\n", s.addr)
		return resp.BulkString(nodeInfo)
	case "SLOTS":
		// [ [startSlot, endSlot, [ip, port, nodeID]] ]
		return resp.Array([]resp.Value{
			resp.Array([]resp.Value{
				resp.Integer(0),
				resp.Integer(16383),
				resp.Array([]resp.Value{
					resp.BulkString("127.0.0.1"),
					resp.Integer(6379),
					resp.BulkString("07c37dfeb235213b87975015de450f364e42bf9d"),
				}),
			}),
		})
	case "INFO":
		info := "cluster_state:ok\r\ncluster_slots_assigned:16384\r\ncluster_slots_ok:16384\r\ncluster_known_nodes:1\r\ncluster_size:1\r\n"
		return resp.BulkString(info)
	default:
		return resp.Error(fmt.Sprintf("ERR unknown subcommand '%s' for 'CLUSTER'", subCmd))
	}
}
