# my-redis

A Redis-compatible in-memory database built from scratch in Go, supporting the RESP protocol so you can use it with the official `redis-cli`.

## Quick Start

```bash
go build -o my-redis-server ./cmd/server/
./my-redis-server --port 6380
```

```bash
redis-cli -p 6380 PING
# PONG

redis-cli -p 6380 SET name Alice
# OK

redis-cli -p 6380 GET name
# "Alice"
```

## Flags

| Flag   | Default | Description                     |
|--------|---------|---------------------------------|
| `--port` | 6379  | TCP port to listen on           |
| `--db`   | (none) | Path to JSON file for persistence |

## Supported Commands

### Core
`PING`, `SET [EX\|PX]`, `GET`, `DEL`, `EXISTS`, `EXPIRE`, `TTL`, `SAVE`

### Data Structures
- **Lists:** `LPUSH`, `RPUSH`, `LPOP`, `RPOP`, `LRANGE`
- **Sets:** `SADD`, `SMEMBERS`, `SREM`, `SISMEMBER`
- **Hashes:** `HSET`, `HGET`, `HDEL`, `HEXISTS`, `HGETALL`
- **Sorted Sets:** `ZADD`, `ZRANGE [WITHSCORES]`, `ZREM`, `ZSCORE`

### Transactions
`MULTI`, `EXEC`, `DISCARD`

### Pub/Sub
`SUBSCRIBE`, `UNSUBSCRIBE`, `PUBLISH`

### Replication
`REPLICAOF`, `REPLICAOF NO ONE`

### Eviction
`MAXMEMORY <count>`

## Persistence

```bash
./my-redis-server --port 6380 --db snapshot.json
redis-cli -p 6380 SET user:1 Alice
redis-cli -p 6380 SAVE
# restart server, data is reloaded automatically
```

## Project Layout

```
├── cmd/server/main.go    Entry point
├── server/               TCP server + all command handlers
├── store/                Thread-safe key-value store (TTL, LRU, persistence)
├── resp/                 RESP protocol reader/writer
├── ds/                   Data structures: List, Set, Hash, Sorted Set
└── pubsub/               Publish/Subscribe message hub
```

## Build & Test

```bash
go build ./...
go test ./...
```

## Performance

| Command | Throughput (20 clients) |
|---------|-----------------------|
| SET     | ~57,000 ops/sec       |
| GET     | ~69,000 ops/sec       |
| SADD    | ~76,000 ops/sec       |
| HSET    | ~68,000 ops/sec       |
| LPUSH   | ~46,000 ops/sec       |

## Architecture

1. **TCP Listener** accepts connections and spawns a goroutine per client
2. **RESP Reader** parses incoming data into typed `Value` structs
3. **Command Handler** dispatches to the appropriate function
4. **Store** holds all data in memory with mutex protection
5. **RESP Writer** formats and sends the response
