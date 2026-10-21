package server

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jatin-dudhani/my-redis/acl"
	"github.com/Jatin-dudhani/my-redis/cluster"
	"github.com/Jatin-dudhani/my-redis/ds"
	"github.com/Jatin-dudhani/my-redis/lua"
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
	case *ds.HLL:
		return resp.SimpleString("string")
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
	case "MEET":
		if len(args) < 3 {
			return resp.Error("ERR wrong number of arguments for 'CLUSTER MEET' command")
		}
		ip := args[1].Str
		port := args[2].Str
		busPort := ""
		if len(args) >= 4 {
			busPort = args[3].Str
		} else {
			pNum, err := strconv.Atoi(port)
			if err == nil {
				busPort = strconv.Itoa(pNum + 10000)
			} else {
				busPort = "16379"
			}
		}
		remoteBusAddr := fmt.Sprintf("%s:%s", ip, busPort)
		if s.gossipBus != nil {
			if err := s.gossipBus.Meet(remoteBusAddr); err != nil {
				return resp.Error(fmt.Sprintf("ERR cluster meet failed: %v", err))
			}
		}
		return resp.SimpleString("OK")
	case "MYID":
		if s.cluster != nil {
			return resp.BulkString(s.cluster.NodeID())
		}
		return resp.BulkString("07c37dfeb235213b87975015de450f364e42bf9d")
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
		known := 1
		if s.gossipBus != nil {
			known = 1 + s.gossipBus.PeerCount()
		}
		info := fmt.Sprintf("cluster_state:ok\r\ncluster_slots_assigned:16384\r\ncluster_slots_ok:16384\r\ncluster_known_nodes:%d\r\ncluster_size:%d\r\n", known, known)
		return resp.BulkString(info)
	default:
		return resp.Error(fmt.Sprintf("ERR unknown subcommand '%s' for 'CLUSTER'", subCmd))
	}
}

// --- Bitmaps & Bitfields ---

func (s *Server) respSetBit(args []resp.Value) resp.Value {
	if len(args) != 3 {
		return resp.Error("ERR wrong number of arguments for 'SETBIT' command")
	}
	key := args[0].Str
	offset, err := strconv.Atoi(args[1].Str)
	if err != nil || offset < 0 {
		return resp.Error("ERR bit offset is not an integer or out of range")
	}
	if offset > 512*1024*1024*8 {
		return resp.Error("ERR bit offset is out of range")
	}
	val, err := strconv.Atoi(args[2].Str)
	if err != nil || (val != 0 && val != 1) {
		return resp.Error("ERR bit is not an integer or out of range")
	}

	var bytes []byte
	existing, ok := s.store.Get(key)
	if ok && existing != nil {
		if str, isStr := existing.(string); isStr {
			bytes = []byte(str)
		} else {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
	}

	newBytes, oldBit := ds.SetBit(bytes, offset, val)
	s.store.Set(key, string(newBytes))
	s.touchWatchedKey(key)
	return resp.Integer(int64(oldBit))
}

func (s *Server) respGetBit(args []resp.Value) resp.Value {
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'GETBIT' command")
	}
	key := args[0].Str
	offset, err := strconv.Atoi(args[1].Str)
	if err != nil || offset < 0 {
		return resp.Error("ERR bit offset is not an integer or out of range")
	}

	existing, ok := s.store.Get(key)
	if !ok || existing == nil {
		return resp.Integer(0)
	}

	str, isStr := existing.(string)
	if !isStr {
		return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	bit := ds.GetBit([]byte(str), offset)
	return resp.Integer(int64(bit))
}

func (s *Server) respBitCount(args []resp.Value) resp.Value {
	if len(args) < 1 || len(args) > 4 {
		return resp.Error("ERR wrong number of arguments for 'BITCOUNT' command")
	}
	key := args[0].Str

	existing, ok := s.store.Get(key)
	if !ok || existing == nil {
		return resp.Integer(0)
	}

	str, isStr := existing.(string)
	if !isStr {
		return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	hasRange := false
	start := 0
	end := 0
	isBit := false

	if len(args) >= 3 {
		var err error
		start, err = strconv.Atoi(args[1].Str)
		if err != nil {
			return resp.Error("ERR value is not an integer or out of range")
		}
		end, err = strconv.Atoi(args[2].Str)
		if err != nil {
			return resp.Error("ERR value is not an integer or out of range")
		}
		hasRange = true
	}

	if len(args) == 4 {
		unit := strings.ToUpper(args[3].Str)
		if unit == "BIT" {
			isBit = true
		} else if unit != "BYTE" {
			return resp.Error("ERR syntax error")
		}
	}

	count := ds.BitCount([]byte(str), start, end, hasRange, isBit)
	return resp.Integer(count)
}

func (s *Server) respBitPos(args []resp.Value) resp.Value {
	if len(args) < 2 || len(args) > 5 {
		return resp.Error("ERR wrong number of arguments for 'BITPOS' command")
	}
	key := args[0].Str
	bitVal, err := strconv.Atoi(args[1].Str)
	if err != nil || (bitVal != 0 && bitVal != 1) {
		return resp.Error("ERR bit is not an integer or out of range")
	}

	existing, ok := s.store.Get(key)
	var b []byte
	if ok && existing != nil {
		str, isStr := existing.(string)
		if !isStr {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		b = []byte(str)
	}

	hasRange := false
	hasEnd := false
	start := 0
	end := 0
	isBit := false

	if len(args) >= 3 {
		start, err = strconv.Atoi(args[2].Str)
		if err != nil {
			return resp.Error("ERR value is not an integer or out of range")
		}
		hasRange = true
	}
	if len(args) >= 4 {
		end, err = strconv.Atoi(args[3].Str)
		if err != nil {
			return resp.Error("ERR value is not an integer or out of range")
		}
		hasEnd = true
	}
	if len(args) == 5 {
		unit := strings.ToUpper(args[4].Str)
		if unit == "BIT" {
			isBit = true
		} else if unit != "BYTE" {
			return resp.Error("ERR syntax error")
		}
	}

	pos := ds.BitPos(b, bitVal, start, end, hasRange, hasEnd, isBit)
	return resp.Integer(pos)
}

func (s *Server) respBitOp(args []resp.Value) resp.Value {
	if len(args) < 3 {
		return resp.Error("ERR wrong number of arguments for 'BITOP' command")
	}
	op := strings.ToUpper(args[0].Str)
	destKey := args[1].Str
	srcKeys := args[2:]

	if op == "NOT" && len(srcKeys) != 1 {
		return resp.Error("ERR BITOP NOT must be called with a single source key")
	}
	if op != "AND" && op != "OR" && op != "XOR" && op != "NOT" {
		return resp.Error("ERR syntax error")
	}

	var sources [][]byte
	for _, k := range srcKeys {
		val, ok := s.store.Get(k.Str)
		if ok && val != nil {
			if str, isStr := val.(string); isStr {
				sources = append(sources, []byte(str))
			} else {
				return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
			}
		} else {
			sources = append(sources, nil)
		}
	}

	res := ds.BitOp(op, sources)
	s.store.Set(destKey, string(res))
	s.touchWatchedKey(destKey)
	return resp.Integer(int64(len(res)))
}

// --- HyperLogLog ---

func (s *Server) respPFAdd(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'PFADD' command")
	}
	key := args[0].Str
	elements := args[1:]

	var hll *ds.HLL
	val, ok := s.store.Get(key)
	if ok && val != nil {
		var isHLL bool
		hll, isHLL = val.(*ds.HLL)
		if !isHLL {
			return resp.Error("WRONGTYPE Key is not a valid HyperLogLog string value")
		}
	} else {
		hll = ds.NewHLL()
	}

	altered := false
	for _, el := range elements {
		if hll.Add([]byte(el.Str)) {
			altered = true
		}
	}

	s.store.Set(key, hll)
	s.touchWatchedKey(key)

	if altered {
		return resp.Integer(1)
	}
	return resp.Integer(0)
}

func (s *Server) respPFCount(args []resp.Value) resp.Value {
	if len(args) < 1 {
		return resp.Error("ERR wrong number of arguments for 'PFCOUNT' command")
	}

	if len(args) == 1 {
		key := args[0].Str
		val, ok := s.store.Get(key)
		if !ok || val == nil {
			return resp.Integer(0)
		}
		hll, isHLL := val.(*ds.HLL)
		if !isHLL {
			return resp.Error("WRONGTYPE Key is not a valid HyperLogLog string value")
		}
		return resp.Integer(int64(hll.Count()))
	}

	// Multiple keys union
	mergedRegisters := make([]uint8, ds.HLLRegisters)
	for _, k := range args {
		val, ok := s.store.Get(k.Str)
		if !ok || val == nil {
			continue
		}
		hll, isHLL := val.(*ds.HLL)
		if !isHLL {
			return resp.Error("WRONGTYPE Key is not a valid HyperLogLog string value")
		}
		bytes := hll.Bytes()
		for i := 0; i < ds.HLLRegisters; i++ {
			if bytes[i] > mergedRegisters[i] {
				mergedRegisters[i] = bytes[i]
			}
		}
	}

	card := ds.EstimateRegisters(mergedRegisters)
	return resp.Integer(int64(card))
}

func (s *Server) respPFMerge(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'PFMERGE' command")
	}
	destKey := args[0].Str
	srcKeys := args[1:]

	destHLL := ds.NewHLL()
	val, ok := s.store.Get(destKey)
	if ok && val != nil {
		if h, isHLL := val.(*ds.HLL); isHLL {
			destHLL.Merge(h)
		} else {
			return resp.Error("WRONGTYPE Key is not a valid HyperLogLog string value")
		}
	}

	for _, k := range srcKeys {
		val, ok := s.store.Get(k.Str)
		if !ok || val == nil {
			continue
		}
		hll, isHLL := val.(*ds.HLL)
		if !isHLL {
			return resp.Error("WRONGTYPE Key is not a valid HyperLogLog string value")
		}
		destHLL.Merge(hll)
	}

	s.store.Set(destKey, destHLL)
	s.touchWatchedKey(destKey)
	return resp.SimpleString("OK")
}

// --- Geospatial Indexing ---

func (s *Server) respGeoAdd(args []resp.Value) resp.Value {
	if len(args) < 4 || (len(args)-1)%3 != 0 {
		return resp.Error("ERR wrong number of arguments for 'GEOADD' command")
	}
	key := args[0].Str

	var zset *ds.SSet
	val, ok := s.store.Get(key)
	if ok && val != nil {
		var isZSet bool
		zset, isZSet = val.(*ds.SSet)
		if !isZSet {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
	} else {
		zset = ds.NewSSet()
	}

	added := 0
	for i := 1; i < len(args); i += 3 {
		lon, err1 := strconv.ParseFloat(args[i].Str, 64)
		lat, err2 := strconv.ParseFloat(args[i+1].Str, 64)
		member := args[i+2].Str

		if err1 != nil || err2 != nil {
			return resp.Error("ERR value is not a valid float")
		}

		hash, err := ds.EncodeGeohash52(lon, lat)
		if err != nil {
			return resp.Error(fmt.Sprintf("ERR invalid longitude/latitude for member %s", member))
		}

		if _, exists := zset.ZScore(member); !exists {
			added++
		}
		zset.ZAdd(float64(hash), member)
	}

	s.store.Set(key, zset)
	s.touchWatchedKey(key)
	return resp.Integer(int64(added))
}

func (s *Server) respGeoPos(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'GEOPOS' command")
	}
	key := args[0].Str
	members := args[1:]

	val, ok := s.store.Get(key)
	var zset *ds.SSet
	if ok && val != nil {
		var isZSet bool
		zset, isZSet = val.(*ds.SSet)
		if !isZSet {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
	}

	results := make([]resp.Value, len(members))
	for i, m := range members {
		if zset == nil {
			results[i] = resp.Null()
			continue
		}
		score, exists := zset.ZScore(m.Str)
		if !exists {
			results[i] = resp.Null()
			continue
		}
		lon, lat := ds.DecodeGeohash52(uint64(score))
		results[i] = resp.Array([]resp.Value{
			resp.BulkString(fmt.Sprintf("%.6f", lon)),
			resp.BulkString(fmt.Sprintf("%.6f", lat)),
		})
	}

	return resp.Array(results)
}

func (s *Server) respGeoDist(args []resp.Value) resp.Value {
	if len(args) < 3 || len(args) > 4 {
		return resp.Error("ERR wrong number of arguments for 'GEODIST' command")
	}
	key := args[0].Str
	m1 := args[1].Str
	m2 := args[2].Str
	unit := "m"
	if len(args) == 4 {
		unit = args[3].Str
	}

	multiplier, err := ds.DistanceUnitMultiplier(unit)
	if err != nil {
		return resp.Error(err.Error())
	}

	val, ok := s.store.Get(key)
	if !ok || val == nil {
		return resp.Null()
	}
	zset, isZSet := val.(*ds.SSet)
	if !isZSet {
		return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	s1, ok1 := zset.ZScore(m1)
	s2, ok2 := zset.ZScore(m2)
	if !ok1 || !ok2 {
		return resp.Null()
	}

	lon1, lat1 := ds.DecodeGeohash52(uint64(s1))
	lon2, lat2 := ds.DecodeGeohash52(uint64(s2))
	distMeters := ds.HaversineDistance(lon1, lat1, lon2, lat2)

	return resp.BulkString(fmt.Sprintf("%.4f", distMeters/multiplier))
}

func (s *Server) respGeoHash(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'GEOHASH' command")
	}
	key := args[0].Str
	members := args[1:]

	val, ok := s.store.Get(key)
	var zset *ds.SSet
	if ok && val != nil {
		var isZSet bool
		zset, isZSet = val.(*ds.SSet)
		if !isZSet {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
	}

	results := make([]resp.Value, len(members))
	for i, m := range members {
		if zset == nil {
			results[i] = resp.Null()
			continue
		}
		score, exists := zset.ZScore(m.Str)
		if !exists {
			results[i] = resp.Null()
			continue
		}
		results[i] = resp.BulkString(ds.GeohashString(uint64(score)))
	}
	return resp.Array(results)
}

func (s *Server) respGeoSearch(args []resp.Value) resp.Value {
	if len(args) < 5 {
		return resp.Error("ERR wrong number of arguments for 'GEOSEARCH' command")
	}
	key := args[0].Str
	val, ok := s.store.Get(key)
	if !ok || val == nil {
		return resp.Array(nil)
	}
	zset, isZSet := val.(*ds.SSet)
	if !isZSet {
		return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	var centerLon, centerLat float64
	var radiusMeters float64
	withDist := false
	withCoord := false
	withHash := false
	asc := true
	count := 0

	idx := 1
	for idx < len(args) {
		opt := strings.ToUpper(args[idx].Str)
		switch opt {
		case "FROMMEMBER":
			if idx+1 >= len(args) {
				return resp.Error("ERR syntax error")
			}
			mem := args[idx+1].Str
			score, exists := zset.ZScore(mem)
			if !exists {
				return resp.Error(fmt.Sprintf("ERR could not decode requested zset member '%s'", mem))
			}
			centerLon, centerLat = ds.DecodeGeohash52(uint64(score))
			idx += 2
		case "FROMLONLAT":
			if idx+2 >= len(args) {
				return resp.Error("ERR syntax error")
			}
			var err1, err2 error
			centerLon, err1 = strconv.ParseFloat(args[idx+1].Str, 64)
			centerLat, err2 = strconv.ParseFloat(args[idx+2].Str, 64)
			if err1 != nil || err2 != nil {
				return resp.Error("ERR value is not a valid float")
			}
			idx += 3
		case "BYRADIUS":
			if idx+2 >= len(args) {
				return resp.Error("ERR syntax error")
			}
			rad, err := strconv.ParseFloat(args[idx+1].Str, 64)
			if err != nil {
				return resp.Error("ERR value is not a valid float")
			}
			unit := args[idx+2].Str
			mul, err := ds.DistanceUnitMultiplier(unit)
			if err != nil {
				return resp.Error(err.Error())
			}
			radiusMeters = rad * mul
			idx += 3
		case "WITHDIST":
			withDist = true
			idx++
		case "WITHCOORD":
			withCoord = true
			idx++
		case "WITHHASH":
			withHash = true
			idx++
		case "ASC":
			asc = true
			idx++
		case "DESC":
			asc = false
			idx++
		case "COUNT":
			if idx+1 >= len(args) {
				return resp.Error("ERR syntax error")
			}
			c, err := strconv.Atoi(args[idx+1].Str)
			if err == nil {
				count = c
			}
			idx += 2
		default:
			idx++
		}
	}

	searchResults := ds.SearchRadius(zset, centerLon, centerLat, radiusMeters, asc, count)
	if !withDist && !withCoord && !withHash {
		respArray := make([]resp.Value, len(searchResults))
		for i, r := range searchResults {
			respArray[i] = resp.BulkString(r.Member)
		}
		return resp.Array(respArray)
	}

	respArray := make([]resp.Value, len(searchResults))
	for i, r := range searchResults {
		var item []resp.Value
		item = append(item, resp.BulkString(r.Member))
		if withDist {
			item = append(item, resp.BulkString(fmt.Sprintf("%.4f", r.Distance)))
		}
		if withHash {
			item = append(item, resp.Integer(int64(r.Hash)))
		}
		if withCoord {
			item = append(item, resp.Array([]resp.Value{
				resp.BulkString(fmt.Sprintf("%.6f", r.Lon)),
				resp.BulkString(fmt.Sprintf("%.6f", r.Lat)),
			}))
		}
		respArray[i] = resp.Array(item)
	}

	return resp.Array(respArray)
}

func (s *Server) respGeoRadius(args []resp.Value) resp.Value {
	if len(args) < 5 {
		return resp.Error("ERR wrong number of arguments for 'GEORADIUS' command")
	}
	key := args[0].Str
	lon := args[1].Str
	lat := args[2].Str
	rad := args[3].Str
	unit := args[4].Str

	searchArgs := []resp.Value{
		resp.BulkString(key),
		resp.BulkString("FROMLONLAT"),
		resp.BulkString(lon),
		resp.BulkString(lat),
		resp.BulkString("BYRADIUS"),
		resp.BulkString(rad),
		resp.BulkString(unit),
	}
	searchArgs = append(searchArgs, args[5:]...)
	return s.respGeoSearch(searchArgs)
}

// --- Lua Scripting Engine ---

func (s *Server) respEval(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'EVAL' command")
	}
	script := args[0].Str
	numKeys, err := strconv.Atoi(args[1].Str)
	if err != nil || numKeys < 0 {
		return resp.Error("ERR value is not an integer or out of range")
	}
	if len(args) < 2+numKeys {
		return resp.Error("ERR Number of keys can't be greater than number of args")
	}

	var keys []string
	for i := 0; i < numKeys; i++ {
		keys = append(keys, args[2+i].Str)
	}

	var argv []string
	for i := 2 + numKeys; i < len(args); i++ {
		argv = append(argv, args[i].Str)
	}

	if s.luaEngine == nil {
		s.luaEngine = lua.NewEngine()
	}

	res, err := s.luaEngine.Execute(script, keys, argv, func(cmdArgs []resp.Value) resp.Value {
		return s.executeCommand(resp.Array(cmdArgs))
	})
	if err != nil {
		return resp.Error(err.Error())
	}
	return res
}

func (s *Server) respEvalSHA(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'EVALSHA' command")
	}
	sha := strings.ToLower(args[0].Str)
	if s.luaEngine == nil {
		s.luaEngine = lua.NewEngine()
	}

	script, ok := s.luaEngine.GetScript(sha)
	if !ok {
		return resp.Error("NOSCRIPT No matching script. Please use EVAL.")
	}

	evalArgs := append([]resp.Value{resp.BulkString(script)}, args[1:]...)
	return s.respEval(evalArgs)
}

func (s *Server) respScript(args []resp.Value) resp.Value {
	if len(args) == 0 {
		return resp.Error("ERR wrong number of arguments for 'SCRIPT' command")
	}
	if s.luaEngine == nil {
		s.luaEngine = lua.NewEngine()
	}

	sub := strings.ToUpper(args[0].Str)
	switch sub {
	case "LOAD":
		if len(args) != 2 {
			return resp.Error("ERR wrong number of arguments for 'SCRIPT LOAD' command")
		}
		sha := s.luaEngine.Load(args[1].Str)
		return resp.BulkString(sha)
	case "EXISTS":
		if len(args) < 2 {
			return resp.Error("ERR wrong number of arguments for 'SCRIPT EXISTS' command")
		}
		var shas []string
		for _, a := range args[1:] {
			shas = append(shas, strings.ToLower(a.Str))
		}
		res := s.luaEngine.Exists(shas)
		respArr := make([]resp.Value, len(res))
		for i, v := range res {
			respArr[i] = resp.Integer(v)
		}
		return resp.Array(respArr)
	case "FLUSH":
		s.luaEngine.Flush()
		return resp.SimpleString("OK")
	default:
		return resp.Error(fmt.Sprintf("ERR unknown subcommand '%s' for 'SCRIPT'", sub))
	}
}

// --- Enterprise Security & ACLs ---

func (s *Server) handleAuth(args []resp.Value, cs *clientState) resp.Value {
	if len(args) < 1 || len(args) > 2 {
		return resp.Error("ERR wrong number of arguments for 'AUTH' command")
	}
	username := "default"
	password := args[0].Str
	if len(args) == 2 {
		username = args[0].Str
		password = args[1].Str
	}

	if s.acl == nil {
		s.acl = acl.NewManager()
	}

	if s.acl.Authenticate(username, password) {
		cs.mu.Lock()
		cs.authenticatedUser = username
		cs.mu.Unlock()
		return resp.SimpleString("OK")
	}

	return resp.Error("WRONGPASS invalid username-password pair or user is disabled.")
}

func (s *Server) respACL(args []resp.Value) resp.Value {
	if len(args) == 0 {
		return resp.Error("ERR wrong number of arguments for 'ACL' command")
	}
	if s.acl == nil {
		s.acl = acl.NewManager()
	}

	sub := strings.ToUpper(args[0].Str)
	switch sub {
	case "WHOAMI":
		return resp.BulkString("default")
	case "LIST":
		lines := s.acl.DescribeUsers()
		respArr := make([]resp.Value, len(lines))
		for i, l := range lines {
			respArr[i] = resp.BulkString(l)
		}
		return resp.Array(respArr)
	case "USERS":
		users := s.acl.ListUsers()
		respArr := make([]resp.Value, len(users))
		for i, u := range users {
			respArr[i] = resp.BulkString(u)
		}
		return resp.Array(respArr)
	case "SETUSER":
		if len(args) < 2 {
			return resp.Error("ERR wrong number of arguments for 'ACL SETUSER' command")
		}
		user := args[1].Str
		var rules []string
		for _, r := range args[2:] {
			rules = append(rules, r.Str)
		}
		if err := s.acl.SetUser(user, rules); err != nil {
			return resp.Error(err.Error())
		}
		return resp.SimpleString("OK")
	case "DELUSER":
		if len(args) < 2 {
			return resp.Error("ERR wrong number of arguments for 'ACL DELUSER' command")
		}
		var names []string
		for _, a := range args[1:] {
			names = append(names, a.Str)
		}
		count := s.acl.DelUser(names...)
		return resp.Integer(int64(count))
	case "CAT":
		if len(args) == 1 {
			var cats []string
			for c := range acl.Categories {
				cats = append(cats, c)
			}
			sort.Strings(cats)
			respArr := make([]resp.Value, len(cats))
			for i, c := range cats {
				respArr[i] = resp.BulkString(c)
			}
			return resp.Array(respArr)
		}
		catName := strings.ToLower(args[1].Str)
		cmds, ok := acl.Categories[catName]
		if !ok {
			return resp.Error(fmt.Sprintf("ERR Unknown category '%s'", catName))
		}
		respArr := make([]resp.Value, len(cmds))
		for i, c := range cmds {
			respArr[i] = resp.BulkString(c)
		}
		return resp.Array(respArr)
	default:
		return resp.Error(fmt.Sprintf("ERR unknown subcommand '%s' for 'ACL'", sub))
	}
}

