package lua

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/Jatin-dudhani/my-redis/resp"
	lua "github.com/yuin/gopher-lua"
)

// CommandExecutor is a callback into the Redis server engine.
type CommandExecutor func(args []resp.Value) resp.Value

// Engine manages compiled Lua scripts and script execution.
type Engine struct {
	mu      sync.RWMutex
	scripts map[string]string // sha1 -> script
}

// NewEngine creates a new Lua scripting engine.
func NewEngine() *Engine {
	return &Engine{
		scripts: make(map[string]string),
	}
}

// ComputeSHA1 returns the hexadecimal SHA1 hash of a script.
func ComputeSHA1(script string) string {
	h := sha1.New()
	h.Write([]byte(script))
	return hex.EncodeToString(h.Sum(nil))
}

// Load compiles/registers a script and returns its SHA1 digest.
func (e *Engine) Load(script string) string {
	digest := ComputeSHA1(script)
	e.mu.Lock()
	e.scripts[digest] = script
	e.mu.Unlock()
	return digest
}

// Exists checks if scripts exist in the cache.
func (e *Engine) Exists(digests []string) []int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	res := make([]int64, len(digests))
	for i, d := range digests {
		if _, ok := e.scripts[d]; ok {
			res[i] = 1
		} else {
			res[i] = 0
		}
	}
	return res
}

// Flush clears all cached scripts.
func (e *Engine) Flush() {
	e.mu.Lock()
	e.scripts = make(map[string]string)
	e.mu.Unlock()
}

// GetScript retrieves a cached script by SHA1.
func (e *Engine) GetScript(digest string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.scripts[digest]
	return s, ok
}

// Execute runs a Lua script with keys and argv, given an executor callback for redis.call.
func (e *Engine) Execute(script string, keys []string, argv []string, exec CommandExecutor) (resp.Value, error) {
	L := lua.NewState(lua.Options{
		SkipOpenLibs: false,
	})
	defer L.Close()

	// Register KEYS table
	keysTable := L.NewTable()
	for i, k := range keys {
		keysTable.RawSetInt(i+1, lua.LString(k))
	}
	L.SetGlobal("KEYS", keysTable)

	// Register ARGV table
	argvTable := L.NewTable()
	for i, a := range argv {
		argvTable.RawSetInt(i+1, lua.LString(a))
	}
	L.SetGlobal("ARGV", argvTable)

	// Register redis module
	redisModule := L.NewTable()

	// redis.call
	L.SetField(redisModule, "call", L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		if top == 0 {
			L.RaiseError("ERR redis.call() requires at least one argument")
			return 0
		}

		var cmdArgs []resp.Value
		for i := 1; i <= top; i++ {
			val := L.Get(i)
			cmdArgs = append(cmdArgs, resp.BulkString(val.String()))
		}

		reply := exec(cmdArgs)
		if reply.Typ == resp.TypeError {
			L.RaiseError("%s", reply.Str)
			return 0
		}

		lVal := respToLua(L, reply)
		L.Push(lVal)
		return 1
	}))

	// redis.pcall
	L.SetField(redisModule, "pcall", L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		if top == 0 {
			tbl := L.NewTable()
			tbl.RawSetString("err", lua.LString("ERR redis.pcall() requires at least one argument"))
			L.Push(tbl)
			return 1
		}

		var cmdArgs []resp.Value
		for i := 1; i <= top; i++ {
			val := L.Get(i)
			cmdArgs = append(cmdArgs, resp.BulkString(val.String()))
		}

		reply := exec(cmdArgs)
		if reply.Typ == resp.TypeError {
			tbl := L.NewTable()
			tbl.RawSetString("err", lua.LString(reply.Str))
			L.Push(tbl)
			return 1
		}

		lVal := respToLua(L, reply)
		L.Push(lVal)
		return 1
	}))

	// redis.sha1hex
	L.SetField(redisModule, "sha1hex", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		L.Push(lua.LString(ComputeSHA1(str)))
		return 1
	}))

	// redis.error_reply
	L.SetField(redisModule, "error_reply", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		tbl := L.NewTable()
		tbl.RawSetString("err", lua.LString(str))
		L.Push(tbl)
		return 1
	}))

	// redis.status_reply
	L.SetField(redisModule, "status_reply", L.NewFunction(func(L *lua.LState) int {
		str := L.CheckString(1)
		tbl := L.NewTable()
		tbl.RawSetString("ok", lua.LString(str))
		L.Push(tbl)
		return 1
	}))

	L.SetGlobal("redis", redisModule)

	// Execute Lua script
	if err := L.DoString(script); err != nil {
		return resp.Error(fmt.Sprintf("ERR Error running script: %v", err)), nil
	}

	// Retrieve return value
	top := L.GetTop()
	if top == 0 {
		return resp.Null(), nil
	}
	retVal := L.Get(top)
	return luaToRESP(retVal), nil
}

func respToLua(L *lua.LState, v resp.Value) lua.LValue {
	switch v.Typ {
	case resp.TypeSimpleString, resp.TypeBulkString:
		return lua.LString(v.Str)
	case resp.TypeInteger:
		return lua.LNumber(v.Num)
	case resp.TypeNull:
		return lua.LFalse
	case resp.TypeArray:
		tbl := L.NewTable()
		for i, item := range v.Array {
			tbl.RawSetInt(i+1, respToLua(L, item))
		}
		return tbl
	case resp.TypeError:
		tbl := L.NewTable()
		tbl.RawSetString("err", lua.LString(v.Str))
		return tbl
	default:
		return lua.LNil
	}
}

func luaToRESP(lv lua.LValue) resp.Value {
	switch lv.Type() {
	case lua.LTNil:
		return resp.Null()
	case lua.LTBool:
		if lv == lua.LTrue {
			return resp.Integer(1)
		}
		return resp.Null()
	case lua.LTNumber:
		return resp.Integer(int64(lv.(lua.LNumber)))
	case lua.LTString:
		return resp.BulkString(string(lv.(lua.LString)))
	case lua.LTTable:
		tbl := lv.(*lua.LTable)
		// Check for error_reply {err = ...}
		errVal := tbl.RawGetString("err")
		if errVal != lua.LNil {
			return resp.Error(errVal.String())
		}
		// Check for status_reply {ok = ...}
		okVal := tbl.RawGetString("ok")
		if okVal != lua.LNil {
			return resp.SimpleString(okVal.String())
		}

		// Array representation
		maxIdx := tbl.MaxN()
		var arr []resp.Value
		for i := 1; i <= maxIdx; i++ {
			arr = append(arr, luaToRESP(tbl.RawGetInt(i)))
		}
		return resp.Array(arr)
	default:
		return resp.Null()
	}
}

// ErrNoScript is returned when EVALSHA cannot find the script.
var ErrNoScript = errors.New("NOSCRIPT No matching script. Please use EVAL.")
