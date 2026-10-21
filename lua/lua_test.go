package lua

import (
	"strings"
	"testing"

	"github.com/Jatin-dudhani/my-redis/resp"
)

func TestLuaReturnPrimitives(t *testing.T) {
	engine := NewEngine()
	mockExec := func(args []resp.Value) resp.Value {
		return resp.SimpleString("OK")
	}

	// Number return
	res, err := engine.Execute("return 42", nil, nil, mockExec)
	if err != nil || res.Typ != resp.TypeInteger || res.Num != 42 {
		t.Fatalf("expected 42, got %v (err: %v)", res, err)
	}

	// String return
	res, err = engine.Execute("return 'hello world'", nil, nil, mockExec)
	if err != nil || res.Typ != resp.TypeBulkString || res.Str != "hello world" {
		t.Fatalf("expected 'hello world', got %v (err: %v)", res, err)
	}

	// Array return
	res, err = engine.Execute("return {'a', 'b', 3}", nil, nil, mockExec)
	if err != nil || res.Typ != resp.TypeArray || len(res.Array) != 3 {
		t.Fatalf("expected array of 3, got %v", res)
	}
	if res.Array[0].Str != "a" || res.Array[1].Str != "b" || res.Array[2].Num != 3 {
		t.Fatalf("array elements mismatch: %v", res.Array)
	}
}

func TestLuaKeysAndArgv(t *testing.T) {
	engine := NewEngine()
	mockExec := func(args []resp.Value) resp.Value {
		return resp.SimpleString("OK")
	}

	script := `return {KEYS[1], KEYS[2], ARGV[1], ARGV[2]}`
	res, err := engine.Execute(script, []string{"key1", "key2"}, []string{"arg1", "arg2"}, mockExec)
	if err != nil || res.Typ != resp.TypeArray || len(res.Array) != 4 {
		t.Fatalf("expected array of 4, got %v (err: %v)", res, err)
	}
	if res.Array[0].Str != "key1" || res.Array[2].Str != "arg1" {
		t.Fatalf("keys or argv mismatch: %v", res.Array)
	}
}

func TestLuaRedisCall(t *testing.T) {
	engine := NewEngine()
	kv := make(map[string]string)

	mockExec := func(args []resp.Value) resp.Value {
		cmd := strings.ToUpper(args[0].Str)
		if cmd == "SET" {
			kv[args[1].Str] = args[2].Str
			return resp.SimpleString("OK")
		} else if cmd == "GET" {
			if v, ok := kv[args[1].Str]; ok {
				return resp.BulkString(v)
			}
			return resp.Null()
		}
		return resp.Error("unknown command")
	}

	script := `
		redis.call("SET", KEYS[1], ARGV[1])
		local val = redis.call("GET", KEYS[1])
		return val
	`
	res, err := engine.Execute(script, []string{"mykey"}, []string{"secret123"}, mockExec)
	if err != nil || res.Typ != resp.TypeBulkString || res.Str != "secret123" {
		t.Fatalf("expected 'secret123', got %v (err: %v)", res, err)
	}
	if kv["mykey"] != "secret123" {
		t.Fatalf("mock state not updated: %v", kv)
	}
}

func TestLuaScriptCache(t *testing.T) {
	engine := NewEngine()
	script := "return 1"
	sha := engine.Load(script)

	exists := engine.Exists([]string{sha, "nonexistent"})
	if exists[0] != 1 || exists[1] != 0 {
		t.Fatalf("expected [1, 0], got %v", exists)
	}

	s, ok := engine.GetScript(sha)
	if !ok || s != script {
		t.Fatalf("expected script %s, got %s", script, s)
	}

	engine.Flush()
	exists = engine.Exists([]string{sha})
	if exists[0] != 0 {
		t.Fatalf("expected 0 after flush, got %d", exists[0])
	}
}
