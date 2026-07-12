# Understand — Build Your Own Redis in Go

## Table of Contents

1. [What is Redis?](#1-what-is-redis)
2. [What is This Project?](#2-what-is-this-project)
3. [Prerequisites](#3-prerequisites)
4. [How to Run This Project](#4-how-to-run-this-project)
5. [Project Structure](#5-project-structure)
6. [Phase-by-Phase Explanation](#6-phase-by-phase-explanation)
   - [Phase 0.1 — TCP Echo Server](#phase-01--tcp-echo-server)
   - [Phase 0.2 — Thread-Safe Key-Value Store](#phase-02--thread-safe-key-value-store)
   - [Phase 0.3 — Persistence (Save to Disk)](#phase-03--persistence-save-to-disk)
   - [Phase 1 — PING/PONG](#phase-1--pingpong)
   - [Phase 2 — SET/GET/DEL/EXISTS](#phase-2--setgetdelexists)
   - [Phase 3 — The RESP Protocol](#phase-3--the-resp-protocol)
   - [Phase 4 — Concurrent Clients](#phase-4--concurrent-clients)
   - [Phase 5 — Key Expiration (EXPIRE/TTL)](#phase-5--key-expiration-expirettl)
   - [Phase 6 — Persistence (SAVE + Auto-Load)](#phase-6--persistence-save--auto-load)
   - [Phase 7 — Data Structures](#phase-7--data-structures)
   - [Phase 8 — Transactions (MULTI/EXEC/DISCARD)](#phase-8--transactions-multiexecdiscard)
   - [Phase 9 — Pub/Sub (Publish/Subscribe)](#phase-9--pubsub-publishsubscribe)
   - [Phase 10 — Replication (REPLICAOF)](#phase-10--replication-replicaof)
   - [Phase 11 — LRU Eviction (MAXMEMORY)](#phase-11--lru-eviction-maxmemory)
   - [Phase 12 — Performance Optimization](#phase-12--performance-optimization)
7. [Important Redis Concepts](#7-important-redis-concepts)
8. [Troubleshooting](#8-troubleshooting)

---

## 1. What is Redis?

Redis (Remote Dictionary Server) is an **in-memory data structure store**. Think of it as a super fast database where everything lives in RAM (computer memory) instead of on a hard drive.

### What makes Redis special?

- **Speed**: RAM is thousands of times faster than disk. Redis can handle millions of operations per second.
- **Data Structures**: Unlike simple key-value stores, Redis supports Lists, Sets, Hashes, Sorted Sets, and more.
- **Persistent**: Even though data lives in RAM, Redis can save it to disk so it survives restarts.
- **Pub/Sub**: Real-time messaging between clients.
- **Replication**: Data can be copied to other Redis servers for backup/scale.
- **Transactions**: Execute multiple commands atomically.

### What is Redis used for?

- **Caching**: Store frequently accessed data (like database query results) for fast retrieval.
- **Session Storage**: Keep user login sessions in web applications.
- **Real-time Leaderboards**: Sorted Sets make ranking easy.
- **Message Queues**: Lists can act as task queues.
- **Rate Limiting**: Track API request counts.

### How does a client talk to Redis?

Clients (like `redis-cli` or your application) connect to Redis over **TCP** (a network protocol). They send commands as text, and Redis replies with a response. Redis uses a protocol called **RESP** (Redis Serialization Protocol) to format these messages. More on this in Phase 3.

---

## 2. What is This Project?

This project is a **complete Redis-compatible server** built from scratch in Go. It implements the core Redis functionality across 12 phases, each adding a new feature. By the end, it speaks the same RESP protocol as real Redis, so you can connect to it using the official `redis-cli` tool.

The goal is educational — to understand Redis deeply by building it yourself.

### What does "Redis-compatible" mean?

It means our server understands the same network protocol as real Redis. You can run:

```bash
redis-cli -p 6380 SET name Alice
# -> OK
redis-cli -p 6380 GET name
# -> "Alice"
```

And it will work exactly the same as with real Redis.

---

## 3. Prerequisites

To run this project, you need:

- **Go 1.21+** installed (`go version` to check)
- **redis-cli** (optional, for testing): Install with `brew install redis` (macOS) or `apt install redis-tools` (Linux)

To check if Go is installed:

```bash
go version
# Example output: go version go1.26.5 darwin/arm64
```

---

## 4. How to Run This Project

### Clone and Build

```bash
git clone https://github.com/Jatin-dudhani/my-redis.git
cd my-redis
go build -o my-redis-server ./cmd/server/
```

This creates an executable called `my-redis-server`.

### Start the Server

```bash
./my-redis-server
# -> server listening on :6379
```

By default, it listens on **port 6379** (the standard Redis port). If port 6379 is already in use (maybe you have real Redis running), use a different port:

```bash
./my-redis-server --port 6380
# -> server listening on :6380
```

### Connect with redis-cli

Open a second terminal and run:

```bash
redis-cli -p 6380 PING
# -> PONG
```

It works! You can now run any supported command:

```bash
redis-cli -p 6380 SET name "Alice"
# -> OK

redis-cli -p 6380 GET name
# -> "Alice"

redis-cli -p 6380 EXISTS name
# -> (integer) 1

redis-cli -p 6380 DEL name
# -> (integer) 1

redis-cli -p 6380 GET name
# -> (nil)
```

### Persistence (Save to File)

Start the server with a database file:

```bash
./my-redis-server --port 6380 --db data.json
```

Now when you run SAVE, your data persists to disk:

```bash
redis-cli -p 6380 SET name Alice
redis-cli -p 6380 SAVE
# -> OK
# Data is now saved to data.json
```

Restart the server, and the data is loaded automatically.

### Running Tests

```bash
go test ./...
```

### All Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 6379 | TCP port to listen on |
| `--db` | (empty) | Path to JSON file for persistence |

---

## 5. Project Structure

```
my-redis/
├── cmd/server/main.go     # Entry point — starts the server
├── server/
│   ├── server.go          # Core server logic, command handlers
│   └── handlers_ds.go     # Data structure command handlers (list, set, hash, zset)
├── store/
│   ├── store.go            # Thread-safe key-value store with TTL + LRU
│   └── persistence.go     # JSON save/load to disk
├── resp/
│   ├── value.go           # RESP value types + constructors
│   ├── reader.go          # RESP parser — reads from network
│   └── writer.go          # RESP writer — sends responses
├── ds/
│   ├── list.go            # Doubly-ended list (LPUSH/RPUSH/LPOP/RPOP)
│   ├── set.go             # Set data structure
│   ├── hash.go            # Hash (map) data structure
│   └── zset.go            # Sorted Set data structure
├── pubsub/
│   └── pubsub.go          # Publish/Subscribe messaging system
├── plan.md                # Original roadmap
└── understand.md          # This file
```

### Why are there different packages?

Each package is a **module** with a specific responsibility:

| Package | Responsibility |
|---------|---------------|
| `server` | Accepts TCP connections, reads commands, calls other packages, sends responses |
| `store` | The in-memory key-value database with expiration and LRU eviction |
| `resp` | Implements the RESP protocol — parsing incoming data and formatting outgoing data |
| `ds` | Data structure implementations (List, Set, Hash, Sorted Set) |
| `pubsub` | Publish/Subscribe message routing |
| `cmd/server` | The main program that starts everything |

---

## 6. Phase-by-Phase Explanation

Each phase builds on the previous one. Here's what was built in each phase and why.

### Phase 0.1 — TCP Echo Server

**Goal**: Get a basic TCP server running that accepts connections.

**What is TCP?** TCP (Transmission Control Protocol) is the fundamental way programs communicate over a network. One program acts as a **server** (listens for connections), and another acts as a **client** (connects to the server). Redis itself works over TCP.

**What was built**: A simple server that:
1. Opens a TCP port and listens for connections
2. When a client connects, reads whatever the client sends
3. Sends it right back (echoes it)

**Key code** (`server.Start` in `server/server.go`):

```go
ln, err := net.Listen("tcp", s.addr)
// net.Listen opens a TCP port. Think of it like
// putting a mailbox on your house. Now anyone can
// send letters to that address.

for {
    conn, err := ln.Accept()
    // Accept waits for someone to connect.
    // When they do, conn represents that connection.
    
    go s.handleConn(conn)
    // Handle each connection in its own goroutine.
    // "go" means run this concurrently — like hiring
    // a new employee for each customer.
}
```

**Why this matters**: Without a TCP server, nothing else works. This is the foundation.

### Phase 0.2 — Thread-Safe Key-Value Store

**Goal**: Build the core data structure — a dictionary (map) that stores key-value pairs safely with multiple users.

**What is a map?** A map (also called a dictionary or hash table) is like a real dictionary: you look up a word (the **key**) and get its definition (the **value**). In Go, `map[string]interface{}` maps strings to values of any type.

**What does thread-safe mean?** When multiple clients connect at the same time (concurrently), they all read and write to the same store. Without protection, two clients writing at the same time can corrupt the data — like two people trying to edit the same document simultaneously. A **mutex** (mutual exclusion lock, `sync.Mutex`) ensures only one client modifies the store at a time.

**Key types in Go used**:

```go
type Store struct {
    mu    sync.Mutex          // The lock — only one goroutine can hold it at a time
    data  map[string]interface{} // The actual key-value storage
}
```

**How the mutex works**:

```go
func (s *Store) Set(key string, value interface{}) {
    s.mu.Lock()              // I need exclusive access — wait if someone else has it
    defer s.mu.Unlock()      // When this function exits, release the lock
    s.data[key] = value      // Safe to modify now
}

func (s *Store) Get(key string) (interface{}, bool) {
    s.mu.Lock()              // Lock for reading too (we use a plain Mutex, not RWMutex)
    defer s.mu.Unlock()
    val, ok := s.data[key]   // Safe to read now
    return val, ok
}
```

**The Lock/Unlock pattern**: Always `Lock` before accessing shared data, and always `Unlock` when done. `defer` ensures the unlock happens even if the function crashes partway through — like a safety net.

**Why this matters**: Without thread safety, the server crashes or corrupts data under load.

### Phase 0.3 — Persistence (Save to Disk)

**Goal**: Save the in-memory data to a file so it survives a server restart.

**The problem**: RAM is volatile — when the computer turns off, everything in RAM disappears. A database needs to survive power loss.

**The solution**: Serialize the data to JSON and write it to a file.

**What is JSON?** JSON (JavaScript Object Notation) is a text format for structured data. It looks like this:

```json
{
  "name": "Alice",
  "age": "30",
  "city": "New York"
}
```

**How saving works** (`store/persistence.go`):

```go
func SaveToFile(s *Store, path string) error {
    data := s.AllStrings()           // Get all string values from the store
    f, err := os.Create(path)        // Open (or create) a file
    enc := json.NewEncoder(f)        // Create a JSON encoder that writes to the file
    return enc.Encode(data)          // Encode the data as JSON and write it
}
```

**How loading works**:

```go
func LoadFromFile(path string) (*Store, error) {
    f, err := os.Open(path)          // Open the file
    var data map[string]string       // Where to put the decoded data
    json.NewDecoder(f).Decode(&data) // Read the file and decode JSON
    s := New()                        // Create a new store
    s.LoadStrings(data)              // Put the loaded data into the store
    return s, nil
}
```

**Why this matters**: This is the foundation of data durability. Later phases wire this into actual Redis commands (SAVE, auto-load on startup).

### Phase 1 — PING/PONG

**Goal**: Implement the simplest Redis command — a health check.

**What PING does**: The client sends `PING`, the server replies `PONG`. It's a way to verify the server is alive and responding.

**How it works** (`server/server.go`):

```go
func (s *Server) respPing(args []resp.Value) resp.Value {
    if len(args) > 0 {
        return resp.BulkString(args[0].Str)  // PING hello -> "hello"
    }
    return resp.SimpleString("PONG")          // PING -> +PONG\r\n
}
```

**Why this matters**: It's the simplest command and shows the request-response pattern. Everything else follows this same pattern: parse command → execute → send response.

### Phase 2 — SET/GET/DEL/EXISTS

**Goal**: Implement basic read and write commands for key-value pairs.

**The commands**:

| Command | What it does | Example |
|---------|-------------|---------|
| `SET key value` | Store a value | `SET name Alice` |
| `GET key` | Retrieve a value | `GET name` -> "Alice" |
| `DEL key1 key2...` | Delete one or more keys | `DEL name` |
| `EXISTS key1 key2...` | Check if keys exist | `EXISTS name` -> 1 |

**How SET works**:

```go
func (s *Server) respSet(args []resp.Value) resp.Value {
    key := args[0].Str
    value := args[1].Str
    s.store.Set(key, value)
    return resp.SimpleString("OK")
}
```

It calls `store.Set()` which stores the key-value pair in the map.

**How GET works**:

```go
func (s *Server) respGet(args []resp.Value) resp.Value {
    val, ok := s.store.Get(args[0].Str)
    if !ok {
        return resp.Null()  // Key doesn't exist -> nil
    }
    return resp.BulkString(str)  // Return the value
}
```

**How DEL works**: It loops through all provided keys and deletes each one, counting how many were actually removed.

**How EXISTS works**: Similar to DEL but counts keys that exist instead of deleting them.

**Why this matters**: These are the most fundamental Redis commands. Everything else builds on this read/write capability.

### Phase 3 — The RESP Protocol

**Goal**: Speak the same wire format as real Redis so official `redis-cli` works.

**What is RESP?** RESP (Redis Serialization Protocol) is the way Redis formats messages over TCP. Every message is either a **command** (client → server) or a **reply** (server → client).

**RESP data types**:

| Type | First Byte | Example (wire format) | Meaning |
|------|-----------|----------------------|---------|
| Simple String | `+` | `+OK\r\n` | A simple non-binary string |
| Error | `-` | `-ERR unknown command\r\n` | An error message |
| Integer | `:` | `:1\r\n` | An integer |
| Bulk String | `$` | `$5\r\nhello\r\n` | A binary-safe string |
| Array | `*` | `*2\r\n...` | A list of other values |
| Null | `$-1\r\n` | `$-1\r\n` | A null/nil value |

**Example**: The command `SET name Alice` in RESP is:

```
*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$5\r\nAlice\r\n
```

Let's decode that:
- `*3\r\n` — Array of 3 elements
- `$3\r\nSET\r\n` — Bulk string of length 3: "SET"
- `$4\r\nname\r\n` — Bulk string of length 4: "name"
- `$5\r\nAlice\r\n` — Bulk string of length 5: "Alice"

**How the RESP reader works** (`resp/reader.go`):

```go
func (r *Reader) Read() (Value, error) {
    b, _ := r.rd.ReadByte()  // Read the first byte to determine the type
    switch b {
    case '+': return r.readSimpleString()  // Starts with +
    case '-': return r.readError()          // Starts with -
    case ':': return r.readInteger()        // Starts with :
    case '$': return r.readBulkString()     // Starts with $
    case '*': return r.readArray()          // Starts with *
    }
}
```

Each reader method parses the type-specific content. For example, reading a bulk string:

```go
func (r *Reader) readBulkString() (Value, error) {
    line, _ := r.readLine()      // Read "$5"
    n, _ := strconv.Atoi(line)   // Parse "5" as integer
    buf := make([]byte, n)       // Create buffer of 5 bytes
    io.ReadFull(r.rd, buf)       // Read exactly 5 bytes: "hello"
    r.readLine()                 // Read the trailing \r\n
    return Value{Typ: TypeBulkString, Str: string(buf)}, nil
}
```

**How the RESP writer works** (`resp/writer.go`):

```go
func (wr *Writer) writeSimpleString(v Value) error {
    _, err := fmt.Fprintf(wr.w, "+%s\r\n", v.Str)
    return err
}

func (wr *Writer) writeBulkString(v Value) error {
    _, err := fmt.Fprintf(wr.w, "$%d\r\n%s\r\n", len(v.Str), v.Str)
    return err
}
```

**Why this matters**: The RESP protocol is what makes our server compatible with redis-cli and any Redis library. Without it, clients can't talk to our server.

### Phase 4 — Concurrent Clients

**Goal**: Handle multiple clients connecting simultaneously.

**The problem**: If the server handles one client at a time, slow clients block fast ones.

**The solution**: **Goroutines**. Go makes concurrent programming simple. When a client connects, we spawn a new goroutine to handle it:

```go
go s.handleConn(conn)
```

**What is a goroutine?** A lightweight thread. Think of it as a separate worker that can run independently. Unlike OS threads, goroutines are cheap — you can have thousands of them.

**Challenges of concurrency**:
1. **Race conditions**: Two clients writing to the same key simultaneously could corrupt data.
2. **Solution**: The mutex (`sync.Mutex`) in the store ensures only one modification happens at a time.

**The read goroutine pattern** (`server/server.go`):

```go
func (s *Server) handleConn(conn net.Conn) {
    vch := make(chan resp.Value, 1)  // Channel to receive commands
    errch := make(chan error, 1)     // Channel to receive errors
    
    // One goroutine reads from the network
    go func() {
        for {
            v, err := rd.Read()
            if err != nil { errch <- err; return }
            vch <- v
        }
    }()
    
    // Main loop reads from the channel
    for {
        select {
        case v := <-vch:
            reply := processRESP(v)
            wr.Write(reply)
        case <-errch:
            return  // Connection closed
        }
    }
}
```

Channels (`chan`) are Go's way of communicating between goroutines. The `select` statement waits for either a new command or a disconnection.

**Why this matters**: A server that only handles one client at a time is useless in practice.

### Phase 5 — Key Expiration (EXPIRE/TTL)

**Goal**: Support keys that auto-delete after a timeout.

**The commands**:

| Command | What it does | Example |
|---------|-------------|---------|
| `EXPIRE key seconds` | Set a TTL on a key | `EXPIRE session 3600` |
| `TTL key` | Get remaining seconds | `TTL session` -> 1234 |
| `SET key value EX seconds` | Set with expiration | `SET code 1234 EX 60` |

**How TTL tracking works**: The store has an `expires` map:

```go
type Store struct {
    data    map[string]interface{}   // key -> value
    expires map[string]time.Time     // key -> expiry time
}
```

**How EXPIRE works**:

```go
func (s *Store) Expire(key string, ttl time.Duration) bool {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, ok := s.data[key]; !ok {
        return false  // Key doesn't exist
    }
    s.expires[key] = time.Now().Add(ttl)  // Set expiry time = now + ttl
    return true
}
```

**How TTL works**:

```go
func (s *Store) TTL(key string) int64 {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, ok := s.data[key]; !ok {
        return -2  // Key does not exist
    }
    exp, ok := s.expires[key]
    if !ok {
        return -1  // Key has no expiration
    }
    rem := time.Until(exp)
    if rem <= 0 {
        return -2  // Already expired
    }
    return int64(rem.Seconds())
}
```

**How expired keys are cleaned up**: A background goroutine periodically scans for expired keys and deletes them:

```go
func (s *Store) StartCleanup(interval time.Duration) {
    s.stopCh = make(chan struct{})
    go func() {
        ticker := time.NewTicker(interval)
        for {
            select {
            case <-ticker.C:
                s.deleteExpired()  // Every 100ms, check for expired keys
            case <-s.stopCh:
                return
            }
        }
    }()
}
```

**Lazy expiration**: Even before the cleanup runs, `Get()` and `Exists()` check if a key is expired before returning it:

```go
func (s *Store) Get(key string) (interface{}, bool) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.isExpired(key, time.Now()) {
        return nil, false  // Expired? Pretend it doesn't exist
    }
    val, ok := s.data[key]
    return val, ok
}
```

**Why this matters**: Expiration is essential for caching. You can store data that automatically disappears after a set time, like cached web pages that refresh every hour.

### Phase 6 — Persistence (SAVE + Auto-Load)

**Goal**: Make saving and loading data available through Redis commands.

**The SAVE command**:

```go
func (s *Server) respSave(args []resp.Value) resp.Value {
    if err := store.SaveToFile(s.store, s.dbPath); err != nil {
        return resp.Error(fmt.Sprintf("ERR saving DB: %v", err))
    }
    return resp.SimpleString("OK")
}
```

**Auto-load on startup** (`server.go` `New` → `loadDB`):

```go
func (s *Server) loadDB() {
    if s.dbPath == "" {
        return  // No DB file configured
    }
    loaded, err := store.LoadFromFile(s.dbPath)
    if err != nil {
        return  // File doesn't exist yet — that's ok
    }
    s.store = loaded  // Replace empty store with loaded data
    fmt.Printf("loaded %d keys\n", s.store.Len())
}
```

**The full persistence flow**:
1. Start: `./my-redis-server --db data.json`
2. Client runs `SET name Alice` → stored in RAM
3. Client runs `SAVE` → RAM data written to `data.json`
4. Server restarts → `data.json` is read → data is back in RAM

**Why this matters**: Without persistence, all data is lost when the server restarts. This makes Redis useful as a real database, not just a cache.

### Phase 7 — Data Structures

**Goal**: Implement Redis's rich data structure types beyond simple strings.

#### Lists

A **List** is an ordered sequence of strings. Think of it like an array or a queue.

**Operations**:
- `LPUSH key val1 val2...` — Insert at the beginning (left)
- `RPUSH key val1 val2...` — Insert at the end (right)
- `LPOP key` — Remove and return the first element
- `RPOP key` — Remove and return the last element
- `LRANGE key start stop` — Get a range of elements

**How List is implemented** (`ds/list.go`):

```go
type List struct {
    mu    sync.Mutex
    items []string  // Go slice (dynamic array)
}

func (l *List) LPush(vals ...string) int {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.items = append(vals, l.items...)  // Prepend: put new items before existing
    return len(l.items)
}

func (l *List) LPop() (string, bool) {
    l.mu.Lock()
    defer l.mu.Unlock()
    if len(l.items) == 0 {
        return "", false
    }
    val := l.items[0]           // Take first element
    l.items = l.items[1:]       // Remove it from the slice
    return val, true
}
```

**Use case**: A List can be used as a queue (RPUSH + LPOP) or a stack (LPUSH + LPOP).

#### Sets

A **Set** is an unordered collection of unique strings. No duplicates allowed.

**Operations**:
- `SADD key member1 member2...` — Add members
- `SMEMBERS key` — Get all members
- `SREM key member1 member2...` — Remove members
- `SISMEMBER key member` — Check if member exists

**How Set is implemented** (`ds/set.go`):

```go
type Set struct {
    mu    sync.RWMutex
    items map[string]struct{}  // Go map with empty struct values (memory-efficient)
}

func (s *Set) SAdd(vals ...string) int {
    s.mu.Lock()
    defer s.mu.Unlock()
    count := 0
    for _, v := range vals {
        if _, ok := s.items[v]; !ok {
            s.items[v] = struct{}{}  // struct{}{} takes zero bytes
            count++
        }
    }
    return count  // Return how many were actually NEW
}
```

The key insight: We use `map[string]struct{}` instead of `map[string]bool` because `struct{}{}` takes zero memory, while `bool` takes 1 byte.

**Use case**: Sets are great for tracking unique things — tags on a blog post, followers of a user, etc.

#### Hashes

A **Hash** maps field names to field values within a key. Think of it as a mini-dictionary inside a Redis key.

**Operations**:
- `HSET key field value [field value...]` — Set one or more fields
- `HGET key field` — Get a field's value
- `HDEL key field [field...]` — Delete fields
- `HEXISTS key field` — Check if field exists
- `HGETALL key` — Get all fields and values

**How Hash is implemented** (`ds/hash.go`):

```go
type Hash struct {
    mu    sync.RWMutex
    items map[string]string  // field -> value
}

func (h *Hash) HSet(field, value string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.items[field] = value
}

func (h *Hash) HGetAll() map[string]string {
    h.mu.RLock()
    defer h.mu.RUnlock()
    copy := make(map[string]string, len(h.items))
    for k, v := range h.items {
        copy[k] = v
    }
    return copy
}
```

Note: HGetAll returns a **copy** of the map, not the original. This prevents callers from modifying the internal data directly.

**Use case**: Hashes are perfect for storing objects — a user profile with name, email, age, etc.

#### Sorted Sets

A **Sorted Set** is like a Set where each member has a score, and members are ordered by score.

**Operations**:
- `ZADD key score member [score member...]` — Add members with scores
- `ZRANGE key start stop [WITHSCORES]` — Get members by rank (ordered by score)
- `ZREM key member [member...]` — Remove members
- `ZSCORE key member` — Get a member's score

**How Sorted Set is implemented** (`ds/zset.go`):

```go
type SSet struct {
    mu    sync.RWMutex
    items map[string]float64  // member -> score
}
```

For `ZRANGE`, we sort by score (and alphabetically for ties):

```go
func (z *SSet) ZRange(start, stop int, withScores bool) []string {
    // ... get all entries
    sort.Slice(entries, func(i, j int) bool {
        if entries[i].Score != entries[j].Score {
            return entries[i].Score < entries[j].Score
        }
        return entries[i].Member < entries[j].Member  // Alphabetical tiebreaker
    })
    // ... slice by start/stop indices
}
```

**Use case**: Leaderboards! Player scores in a game, product ratings, etc.

**Why this matters**: Data structures are what make Redis more than just a simple key-value store. They enable complex server-side operations that would be slow and complex on the client side.

### Phase 8 — Transactions (MULTI/EXEC/DISCARD)

**Goal**: Allow clients to group multiple commands into a single atomic unit.

**What are transactions?** A transaction is a group of commands that execute **sequentially** and **without interruption**. Either all commands execute, or none do (though our implementation doesn't roll back individual command failures).

**How transactions work**:

1. `MULTI` — Start a transaction
2. `SET key1 val1` — Commands are queued, not executed
3. `SET key2 val2` — Still queued
4. `EXEC` — Execute all queued commands at once
5. `DISCARD` — Cancel the transaction instead

**Example**:
```
redis-cli -p 6380 MULTI
-> OK
redis-cli -p 6380 SET a 1
-> QUEUED
redis-cli -p 6380 SET b 2
-> QUEUED
redis-cli -p 6380 EXEC
1) OK
2) OK
```

**Implementation** (`server.go`):

```go
type clientState struct {
    inTx  bool          // Are we in a transaction?
    queue []resp.Value  // Queued commands
}

// In processRESP:
if cmd == "MULTI" {
    cs.inTx = true
    cs.queue = nil
    return resp.SimpleString("OK")
}

if cs.inTx {
    cs.queue = append(cs.queue, v)  // Queue the command
    return resp.SimpleString("QUEUED")
}

// In EXEC:
queue := cs.queue
cs.inTx = false
cs.queue = nil
results := make([]resp.Value, len(queue))
for i, qv := range queue {
    results[i] = s.executeCommand(qv)  // Execute one by one
}
return resp.Array(results)
```

**What does atomic mean?** While commands in a transaction are executing, no other client's commands can interleave. Our store's mutex ensures this, but EXEC doesn't hold the lock across the entire transaction — each command acquires and releases the lock independently. A true Redis transaction guarantees that no other client's commands execute between MULTI and EXEC.

**Why this matters**: Transactions are crucial for data integrity. For example, transferring money between two accounts requires debiting one and crediting the other — if the server crashes mid-way, you'd lose money without transactions.

### Phase 9 — Pub/Sub (Publish/Subscribe)

**Goal**: Implement a real-time messaging system where clients can subscribe to channels and receive messages.

**The concept**: Pub/Sub is like a radio broadcast:
- **Publishers** send messages to channels (like a radio station transmitting on a frequency)
- **Subscribers** listen to channels (like people tuning their radio to that frequency)
- Multiple subscribers can listen to the same channel

**Commands**:
| Command | What it does |
|---------|-------------|
| `SUBSCRIBE channel1 channel2...` | Start listening to channels |
| `PUBLISH channel message` | Send a message to a channel |
| `UNSUBSCRIBE channel1 channel2...` | Stop listening |

**How Pub/Sub is implemented** (`pubsub/pubsub.go`):

```go
type Hub struct {
    mu       sync.RWMutex
    channels map[string]map[*Subscriber]struct{}  // channel -> set of subscribers
}

type Subscriber struct {
    Messages chan Message  // Buffered channel to receive messages
}
```

**How subscription works**:

```go
func (h *Hub) Subscribe(channel string, sub *Subscriber) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if h.channels[channel] == nil {
        h.channels[channel] = make(map[*Subscriber]struct{})
    }
    h.channels[channel][sub] = struct{}{}
}
```

**How publishing works**:

```go
func (h *Hub) Publish(channel, message string) int {
    h.mu.RLock()
    defer h.mu.RUnlock()
    subs := h.channels[channel]
    count := 0
    for sub := range subs {
        select {
        case sub.Messages <- Message{Channel: channel, Payload: message}:
            count++
        default:
            // Channel buffer full — skip this subscriber
        }
    }
    return count
}
```

**Subscribed mode**: When a client enters subscribe mode, the server stops processing normal commands and instead listens for:
1. Incoming messages from subscribed channels (sent to the client)
2. Subscription management commands (SUBSCRIBE/UNSUBSCRIBE/QUIT)

```go
for {
    if cs.subscribed {
        select {
        case msg := <-cs.sub.Messages:
            // Forward the published message to the client
            wr.Write(formatPubSubMessage(msg))
        case v := <-vch:
            // Handle SUBSCRIBE/UNSUBSCRIBE/QUIT
            s.handleSubscribedCommand(v, cs, wr)
        }
    }
}
```

**Why this matters**: Pub/Sub enables real-time features like chat applications, live notifications, and event-driven architectures.

### Phase 10 — Replication (REPLICAOF)

**Goal**: Allow our server to act as a replica (slave) of another Redis server, keeping data in sync.

**What is replication?** Replication creates copies of data across multiple servers. A **master** server accepts writes, and **replicas** (sometimes called slaves) copy the master's data. If the master fails, a replica can take over.

**How replication works in our implementation**:

1. **The REPLICAOF command**: A client tells our server to become a replica of a master:
   ```
   REPLICAOF 127.0.0.1 6379
   ```

2. **Connecting to the master**: Our server opens a TCP connection to the master.

3. **Command propagation**: Every write command sent to the master is forwarded to all connected replicas.

**Implementation details** (`server.go`):

```go
type Server struct {
    replicas   map[net.Conn]struct{}  // Set of replica connections
    isReplica  bool                    // Are we a replica?
    masterConn net.Conn                // Connection to our master
}
```

**Write commands** are tracked:

```go
var writeCommands = map[string]bool{
    "SET": true, "DEL": true, "EXPIRE": true,
    "LPUSH": true, "RPUSH": true, "LPOP": true, "RPOP": true,
    "SADD": true, "SREM": true, "HSET": true, "HDEL": true,
    "ZADD": true, "ZREM": true,
}
```

After executing a write command, propagate it:

```go
func (s *Server) executeCommand(v resp.Value) resp.Value {
    // ... execute the command ...
    
    if writeCommands[cmd] {
        s.propagate(v)  // Forward to all replicas
    }
    return reply
}
```

**How propagation works**:

```go
func (s *Server) propagate(v resp.Value) {
    s.replicaMu.Lock()
    defer s.replicaMu.Unlock()
    for conn := range s.replicas {
        conn.Write([]byte(data))  // Send the raw RESP command
    }
}
```

**REPLICAOF NO ONE**: To make a replica become a master again.

**Why this matters**: Replication provides:
- **Data redundancy**: Backups on other machines
- **Read scaling**: Replicas can handle read queries
- **High availability**: If master fails, promote a replica

### Phase 11 — LRU Eviction (MAXMEMORY)

**Goal**: Limit memory usage by evicting the least recently used keys.

**The problem**: RAM is limited. If a client keeps adding data, eventually the server runs out of memory.

**The solution**: When the number of keys exceeds `maxKeys`, evict (remove) the **Least Recently Used** (LRU) keys until we're under the limit.

**What is LRU?** LRU stands for "Least Recently Used". The idea is simple: the keys you haven't accessed in the longest time are the ones most likely to be evicted. If you have a bookshelf with room for 10 books, and you buy an 11th, you'd remove the book you haven't read in the longest time.

**How LRU is implemented** (`store/store.go`):

We use a **doubly-linked list** + **hash map** for O(1) operations:

```go
type Store struct {
    maxKeys  int                       // Maximum number of keys
    lruList  *list.List                // Doubly-linked list (key order: front=recent, back=old)
    lruIndex map[string]*list.Element  // Quick lookup: key -> position in list
}
```

**The linked list maintains access order**:
- Front of list = Most recently used
- Back of list = Least recently used (candidate for eviction)

**When a key is accessed** (`touchLocked`):

```go
func (s *Store) touchLocked(key string) {
    if elem, ok := s.lruIndex[key]; ok {
        s.lruList.MoveToFront(elem)  // Move to front = "this was just used"
    }
}
```

**When we need to evict** (`evictLocked`):

```go
func (s *Store) evictLocked() {
    for len(s.data) > s.maxKeys {
        elem := s.lruList.Back()        // Get the least recently used
        entry := elem.Value.(lruEntry)
        delete(s.data, entry.key)       // Remove from data map
        delete(s.expires, entry.key)    // Remove from expires map
        delete(s.lruIndex, entry.key)   // Remove from index
        s.lruList.Remove(elem)          // Remove from list
    }
}
```

**MAXMEMORY command**: Allows clients to set the limit dynamically:

```
redis-cli -p 6380 MAXMEMORY 1000
-> OK
```

**Touching is called on**: `Set`, `Get`, `Exists`, `Expire` — any operation that uses a key marks it as recently used.

**Why this matters**: Without eviction, the server crashes when memory fills up. LRU is smart eviction — it keeps the most useful data and discards the least useful.

### Phase 12 — Performance Optimization

**Goal**: Make the server faster.

**Optimizations implemented**:

1. **Buffered Writer** (`resp/writer.go`): Instead of writing to the network one small chunk at a time (many system calls), we use a buffered writer. Data accumulates in a 64KB buffer and is flushed all at once.

   ```go
   type Writer struct {
       w *bufio.Writer  // Instead of raw io.Writer
   }
   ```

2. **Larger Reader Buffer** (`resp/reader.go`): Increased from the default 4KB to 64KB, reducing the number of read system calls.

   ```go
   func NewReader(rd io.Reader) *Reader {
       return &Reader{rd: bufio.NewReaderSize(rd, 65536)}  // 64KB
   }
   ```

3. **Byte Buffer Pool** (`resp/reader.go`): Instead of allocating new memory for every bulk string, we reuse buffers from a `sync.Pool`.

   ```go
   var bufPool = sync.Pool{
       New: func() interface{} {
           return make([]byte, 0, 1024)
       },
   }
   
   // In readBulkString:
   buf := bufPool.Get().([]byte)  // Get a reused buffer
   // ... use it ...
   bufPool.Put(buf[:0])            // Return it for reuse
   ```

   `sync.Pool` automatically reuses memory allocations, reducing garbage collection (GC) pressure.

4. **CONFIG command stub**: Added minimal support for `CONFIG GET` so `redis-benchmark` compatibility improves.

**Benchmark results** (20 clients, 5000 ops each):

| Command | Throughput |
|---------|-----------|
| SET | 56,818 ops/sec |
| GET | 68,493 ops/sec |
| LPUSH | 45,871 ops/sec |
| LPOP | 60,976 ops/sec |
| SADD | 75,758 ops/sec |
| HSET | 67,568 ops/sec |

**Why this matters**: Real-world Redis handles millions of ops/sec on good hardware. These optimizations reduce latency and improve throughput, making the server usable for real workloads.

---

## 7. Important Redis Concepts

### Key-Value Model

Redis is fundamentally a **key-value store**. Every piece of data is stored with a unique key (a string). Keys are looked up quickly using a hash table (map).

Good key naming convention: `object:id:field`, e.g., `user:1000:name`, `post:42:likes`.

### TTL (Time To Live)

Keys can have a **time-to-live** — an automatic expiry time. After the TTL expires, the key disappears.

- `EXPIRE key seconds` — set TTL
- `TTL key` — check remaining time
- `-1` = no TTL, `-2` = key doesn't exist (or expired)

### The RESP Protocol

As discussed in Phase 3, RESP is Redis's wire protocol. Every message starts with a type byte:

```
+OK\r\n          -> Simple string (OK)
-ERR msg\r\n     -> Error
:42\r\n          -> Integer
$5\r\nhello\r\n  -> Bulk string (length-prefixed)
*3\r\n...\r\n    -> Array
```

### Atomicity

Operations in Redis are **atomic** — they cannot be interrupted mid-execution. Our server achieves this with mutexes. Transactions extend this by grouping multiple commands.

### Blocking vs Non-blocking

**Pub/Sub is blocking**: Once a client subscribes, the server stops processing normal commands until the client unsubscribes. The server's main loop enters a different code path that only handles subscription commands and forwarding messages.

**Normal commands are non-blocking**: The server processes one command, sends a response, and immediately waits for the next one. Multiple clients are handled concurrently via goroutines.

### Master-Replica Replication

- **Master**: Accepts writes, propagates changes to replicas
- **Replica**: Connects to master, receives propagated commands
- **REPLICAOF**: Tells a server to become a replica of another
- **REPLICAOF NO ONE**: Promotes a replica back to master

---

## 8. Troubleshooting

### Port already in use

```
listen on :6379: listen tcp :6379: bind: address already in use
```

Use a different port:

```bash
./my-redis-server --port 6380
```

### redis-cli can't connect

Make sure the server is running and listening on the port you specified:

```bash
redis-cli -p 6380 PING
# If you get: Could not connect to Redis at 127.0.0.1:6380: Connection refused
# Then the server isn't running.
```

### SAVE fails

Make sure you started the server with `--db` flag:

```bash
./my-redis-server --port 6380 --db mydata.json
# Now SAVE will work
redis-cli -p 6380 SAVE
```

### Tests fail

```bash
go clean -testcache && go test ./...
```

### Build fails

Make sure you have Go installed and are in the right directory:

```bash
go version
cd /path/to/my-redis
go build ./...
```

### Connection drops with redis-benchmark

redis-benchmark sends connections very aggressively. Our server is not as optimized as Redis — try with fewer parallel connections:

```bash
redis-benchmark -p 6380 -n 5000 -c 20 -q -t SET,GET
```

---

*This document was created as part of the "Build Your Own Redis" project — a hands-on journey to understand Redis by building it from scratch in Go.*
