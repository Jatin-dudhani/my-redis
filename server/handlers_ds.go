package server

import (
	"math"
	"strconv"

	"github.com/macbook/my-redis/ds"
	"github.com/macbook/my-redis/resp"
)

func (s *Server) respLPush(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'LPUSH' command")
	}
	key := args[0].Str
	val, _ := s.store.Get(key)
	list, ok := val.(*ds.List)
	if !ok {
		if val != nil {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		list = ds.NewList()
	}
	vals := make([]string, len(args)-1)
	for i, a := range args[1:] {
		vals[i] = a.Str
	}
	n := list.LPush(vals...)
	s.store.Set(key, list)
	return resp.Integer(int64(n))
}

func (s *Server) respRPush(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'RPUSH' command")
	}
	key := args[0].Str
	val, _ := s.store.Get(key)
	list, ok := val.(*ds.List)
	if !ok {
		if val != nil {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		list = ds.NewList()
	}
	vals := make([]string, len(args)-1)
	for i, a := range args[1:] {
		vals[i] = a.Str
	}
	n := list.RPush(vals...)
	s.store.Set(key, list)
	return resp.Integer(int64(n))
}

func (s *Server) respLPop(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'LPOP' command")
	}
	val, _ := s.store.Get(args[0].Str)
	list, ok := val.(*ds.List)
	if !ok || list == nil {
		return resp.Null()
	}
	elem, ok := list.LPop()
	if !ok {
		return resp.Null()
	}
	if list.LLen() == 0 {
		s.store.Delete(args[0].Str)
	} else {
		s.store.Set(args[0].Str, list)
	}
	return resp.BulkString(elem)
}

func (s *Server) respRPop(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'RPOP' command")
	}
	val, _ := s.store.Get(args[0].Str)
	list, ok := val.(*ds.List)
	if !ok || list == nil {
		return resp.Null()
	}
	elem, ok := list.RPop()
	if !ok {
		return resp.Null()
	}
	if list.LLen() == 0 {
		s.store.Delete(args[0].Str)
	} else {
		s.store.Set(args[0].Str, list)
	}
	return resp.BulkString(elem)
}

func (s *Server) respLRange(args []resp.Value) resp.Value {
	if len(args) != 3 {
		return resp.Error("ERR wrong number of arguments for 'LRANGE' command")
	}
	val, _ := s.store.Get(args[0].Str)
	list, ok := val.(*ds.List)
	if !ok || list == nil {
		return resp.Array(nil)
	}
	start, err := strconv.Atoi(args[1].Str)
	if err != nil {
		return resp.Error("ERR value is not an integer or out of range")
	}
	stop, err := strconv.Atoi(args[2].Str)
	if err != nil {
		return resp.Error("ERR value is not an integer or out of range")
	}
	items := list.LRange(start, stop)
	vals := make([]resp.Value, len(items))
	for i, item := range items {
		vals[i] = resp.BulkString(item)
	}
	return resp.Array(vals)
}

func (s *Server) respSAdd(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'SADD' command")
	}
	key := args[0].Str
	val, _ := s.store.Get(key)
	set, ok := val.(*ds.Set)
	if !ok {
		if val != nil {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		set = ds.NewSet()
	}
	members := make([]string, len(args)-1)
	for i, a := range args[1:] {
		members[i] = a.Str
	}
	n := set.SAdd(members...)
	s.store.Set(key, set)
	return resp.Integer(int64(n))
}

func (s *Server) respSMembers(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'SMEMBERS' command")
	}
	val, _ := s.store.Get(args[0].Str)
	set, ok := val.(*ds.Set)
	if !ok || set == nil {
		return resp.Array(nil)
	}
	members := set.SMembers()
	vals := make([]resp.Value, len(members))
	for i, m := range members {
		vals[i] = resp.BulkString(m)
	}
	return resp.Array(vals)
}

func (s *Server) respSRem(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'SREM' command")
	}
	val, _ := s.store.Get(args[0].Str)
	set, ok := val.(*ds.Set)
	if !ok || set == nil {
		return resp.Integer(0)
	}
	members := make([]string, len(args)-1)
	for i, a := range args[1:] {
		members[i] = a.Str
	}
	n := set.SRem(members...)
	if set.SCard() == 0 {
		s.store.Delete(args[0].Str)
	} else {
		s.store.Set(args[0].Str, set)
	}
	return resp.Integer(int64(n))
}

func (s *Server) respSIsMember(args []resp.Value) resp.Value {
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'SISMEMBER' command")
	}
	val, _ := s.store.Get(args[0].Str)
	set, ok := val.(*ds.Set)
	if !ok || set == nil {
		return resp.Integer(0)
	}
	if set.SIsMember(args[1].Str) {
		return resp.Integer(1)
	}
	return resp.Integer(0)
}

func (s *Server) respHSet(args []resp.Value) resp.Value {
	if len(args) < 3 || (len(args)-1)%2 != 0 {
		return resp.Error("ERR wrong number of arguments for 'HSET' command")
	}
	key := args[0].Str
	val, _ := s.store.Get(key)
	hash, ok := val.(*ds.Hash)
	if !ok {
		if val != nil {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		hash = ds.NewHash()
	}
	count := int64(0)
	for i := 1; i < len(args); i += 2 {
		field := args[i].Str
		value := args[i+1].Str
		if !hash.HExists(field) {
			count++
		}
		hash.HSet(field, value)
	}
	s.store.Set(key, hash)
	return resp.Integer(count)
}

func (s *Server) respHGet(args []resp.Value) resp.Value {
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'HGET' command")
	}
	val, _ := s.store.Get(args[0].Str)
	hash, ok := val.(*ds.Hash)
	if !ok || hash == nil {
		return resp.Null()
	}
	v, ok := hash.HGet(args[1].Str)
	if !ok {
		return resp.Null()
	}
	return resp.BulkString(v)
}

func (s *Server) respHDel(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'HDEL' command")
	}
	val, _ := s.store.Get(args[0].Str)
	hash, ok := val.(*ds.Hash)
	if !ok || hash == nil {
		return resp.Integer(0)
	}
	fields := make([]string, len(args)-1)
	for i, a := range args[1:] {
		fields[i] = a.Str
	}
	n := hash.HDel(fields...)
	if hash.HLen() == 0 {
		s.store.Delete(args[0].Str)
	} else {
		s.store.Set(args[0].Str, hash)
	}
	return resp.Integer(int64(n))
}

func (s *Server) respHExists(args []resp.Value) resp.Value {
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'HEXISTS' command")
	}
	val, _ := s.store.Get(args[0].Str)
	hash, ok := val.(*ds.Hash)
	if !ok || hash == nil {
		return resp.Integer(0)
	}
	if hash.HExists(args[1].Str) {
		return resp.Integer(1)
	}
	return resp.Integer(0)
}

func (s *Server) respHGetAll(args []resp.Value) resp.Value {
	if len(args) != 1 {
		return resp.Error("ERR wrong number of arguments for 'HGETALL' command")
	}
	val, _ := s.store.Get(args[0].Str)
	hash, ok := val.(*ds.Hash)
	if !ok || hash == nil {
		return resp.Array(nil)
	}
	pairs := hash.HGetAll()
	vals := make([]resp.Value, 0, len(pairs)*2)
	for k, v := range pairs {
		vals = append(vals, resp.BulkString(k), resp.BulkString(v))
	}
	return resp.Array(vals)
}

func (s *Server) respZAdd(args []resp.Value) resp.Value {
	if len(args) < 3 || (len(args)-1)%2 != 0 {
		return resp.Error("ERR wrong number of arguments for 'ZADD' command")
	}
	key := args[0].Str
	val, _ := s.store.Get(key)
	zset, ok := val.(*ds.SSet)
	if !ok {
		if val != nil {
			return resp.Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		zset = ds.NewSSet()
	}
	count := int64(0)
	for i := 1; i < len(args); i += 2 {
		score, err := strconv.ParseFloat(args[i].Str, 64)
		if err != nil {
			return resp.Error("ERR value is not a valid float")
		}
		member := args[i+1].Str
		if _, ok := zset.ZScore(member); !ok {
			count++
		} else if math.IsInf(score, 0) || math.IsNaN(score) {
			return resp.Error("ERR score is not a valid float")
		}
		zset.ZAdd(score, member)
	}
	s.store.Set(key, zset)
	return resp.Integer(count)
}

func (s *Server) respZRange(args []resp.Value) resp.Value {
	if len(args) < 3 {
		return resp.Error("ERR wrong number of arguments for 'ZRANGE' command")
	}
	val, _ := s.store.Get(args[0].Str)
	zset, ok := val.(*ds.SSet)
	if !ok || zset == nil {
		return resp.Array(nil)
	}
	start, err := strconv.Atoi(args[1].Str)
	if err != nil {
		return resp.Error("ERR value is not an integer or out of range")
	}
	stop, err := strconv.Atoi(args[2].Str)
	if err != nil {
		return resp.Error("ERR value is not an integer or out of range")
	}
	withScores := false
	if len(args) >= 4 && args[3].Str == "WITHSCORES" {
		withScores = true
	}
	items := zset.ZRange(start, stop, withScores)
	vals := make([]resp.Value, len(items))
	for i, item := range items {
		vals[i] = resp.BulkString(item)
	}
	return resp.Array(vals)
}

func (s *Server) respZRem(args []resp.Value) resp.Value {
	if len(args) < 2 {
		return resp.Error("ERR wrong number of arguments for 'ZREM' command")
	}
	val, _ := s.store.Get(args[0].Str)
	zset, ok := val.(*ds.SSet)
	if !ok || zset == nil {
		return resp.Integer(0)
	}
	members := make([]string, len(args)-1)
	for i, a := range args[1:] {
		members[i] = a.Str
	}
	n := zset.ZRem(members...)
	if zset.ZCard() == 0 {
		s.store.Delete(args[0].Str)
	} else {
		s.store.Set(args[0].Str, zset)
	}
	return resp.Integer(int64(n))
}

func (s *Server) respZScore(args []resp.Value) resp.Value {
	if len(args) != 2 {
		return resp.Error("ERR wrong number of arguments for 'ZSCORE' command")
	}
	val, _ := s.store.Get(args[0].Str)
	zset, ok := val.(*ds.SSet)
	if !ok || zset == nil {
		return resp.Null()
	}
	score, ok := zset.ZScore(args[1].Str)
	if !ok {
		return resp.Null()
	}
	if score == float64(int64(score)) {
		return resp.BulkString(strconv.FormatInt(int64(score), 10))
	}
	return resp.BulkString(strconv.FormatFloat(score, 'f', -1, 64))
}
