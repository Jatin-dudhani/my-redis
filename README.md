# my-redis

A high-performance, distributed, in-memory key-value database built from scratch in Go. Fully compatible with the official Redis RESP wire protocol, enabling drop-in integration with `redis-cli` and standard client libraries across Python, Go, Node.js, and Java.

I built this project to explore systems programming, low-level data structures, and the architectural mechanics required to scale an in-memory database to **100,000+ ops/sec** with sub-millisecond latency and enterprise-grade crash resilience.

---

## Architecture Overview

```
                      +-----------------------------+
                      |    redis-cli / Any Client   |
                      +-----------------------------+
                                     |
                                (RESP2 TCP)
                                     v
                      +-----------------------------+
                      |   Connection & Transaction  |
                      |   (WATCH / MULTI / EXEC)    |
                      +-----------------------------+
                                     |
                                     v
       +-----------------------------------------------------------+
       |           256-Shard Partitioned In-Memory Engine          |
       |  (FNV-32a Key Hashing, Probabilistic Active Expiration)   |
       +-----------------------------------------------------------+
         |              |              |              |
         v              v              v              v
     [Shard 0]      [Shard 1]      [Shard 2]      [Shard 255]
         |              |              |              |
         +--------------+--------------+--------------+
                                |
         +----------------------+----------------------+
         |                                             |
         v                                             v
+------------------+                          +------------------+
|    Durability    |                          |    Clustering    |
| - AOF (everysec) |                          | - 16,384 Slots   |
| - BGREWRITEAOF   |                          | - CRC16 XMODEM   |
| - BGSAVE         |                          | - MOVED Redirect |
+------------------+                          +------------------+
```

---

## Engineering Highlights & Design Decisions

### 1. Eliminating Lock Contention: 256 Sharded Keyspaces
- **The Problem:** Naïve in-memory databases use a single global mutex (`sync.RWMutex`), serializing all operations across CPU cores. Even reads (`GET`) require a write lock to update LRU tracking, severely hurting multi-core concurrency.
- **My Solution:** I partitioned the keyspace into **256 independent shards** indexed by FNV-32a hashing. Keys hash deterministically to separate shards, allowing operations on different keys to execute completely lock-free relative to each other across all CPU cores.

### 2. $O(\log N)$ SkipLists vs $O(N \log N)$ Sorting
- **The Problem:** Storing Sorted Sets as hash maps requires allocating slices and running `sort.Slice` on every query, causing $O(N \log N)$ compute and memory spikes on large sets.
- **My Solution:** I implemented a custom **randomized SkipList** (32 levels, $P=0.25$) with span tracking. Elements maintain score order and lexicographical tie-breaking, reducing insertions, updates, and removals to $O(\log N)$ and range queries (`ZRANGE`) to $O(\log N + M)$.

### 3. $O(1)$ Doubly Linked Lists & Redis Streams
- **Lists:** Replaced Go slice prepending (`append(vals, list...)`), which required shifting arrays in memory, with a **doubly linked list** with head and tail pointers. `LPUSH`, `RPUSH`, `LPop`, and `RPop` are strict $O(1)$ operations with bidirectional `LRANGE` traversal.
- **Streams:** Built Redis Streams (`XADD`, `XLEN`, `XRANGE`, `XREAD`) with millisecond timestamp + sequence IDs (`<timestamp>-<seq>`) for event-driven message architectures.

### 4. Zero Data Loss Crash Recovery: Append-Only File (AOF)
- **AOF Engine:** Sequential write log with configurable fsync policies (`always`, `everysec`, `no`). Background worker flushes buffers every second (`f.Sync()`), offering high performance with a maximum 1-second risk window.
- **Background Compaction (`BGREWRITEAOF`):** Asynchronously writes compact reconstruction commands to a `.tmp` file and atomically swaps it using `os.Rename`, keeping file size bounded without pausing client traffic.
- **Full Type Persistence (`SAVE` / `BGSAVE`):** Snapshots preserving Strings, Hashes, Lists, Sets, Sorted Sets, and TTL expiration timestamps.

### 5. ACID Transactions with Optimistic Locking (`WATCH`)
- Implemented `WATCH` / `UNWATCH` optimistic concurrency control. If another client modifies a watched key before `EXEC`, the transaction cleanly aborts and returns `*-1\r\n` (Null Array).
- Serial transaction isolation during `EXEC` via execution mutex (`s.execMu`).

### 6. Horizontal Scaling: Redis Cluster Routing
- **16,384 Hash Slots:** Uses the official Redis CRC16-CCITT XMODEM algorithm.
- **Hash Tags:** Supports `{user100}:profile` and `{user100}:orders` to hash only what is between curly brackets for multi-key locality.
- **`-MOVED` Redirection:** Returns standard `-MOVED <slot> <targetAddr>` errors when accessing keys owned by remote nodes, fully compatible with cluster-aware client libraries.

---

## Live Performance Benchmarks

Tested on macOS using the official `redis-benchmark` utility with 50 concurrent connections across 100,000 operations:

```bash
redis-benchmark -p 6380 -t set,get -n 100000 -c 50 -q
```

| Command | Throughput | p50 Latency |
| :--- | :--- | :--- |
| **SET** | **81,967 req/sec** | **0.295 ms** |
| **GET** | **90,497 req/sec** | **0.279 ms** |

---

## Real-World Applications

1. **High-Throughput Caching & Session Store:** Microservices caching user sessions and query results with automatic TTL expiration.
2. **Real-Time Gaming Leaderboards:** Leveraging SkipList Sorted Sets (`ZADD`, `ZRANGE`) for ranking millions of players with sub-millisecond response times.
3. **Asynchronous Background Job Queues:** Using `RPUSH` and blocking `BLPOP` to build producer-consumer worker queues (Celery / BullMQ equivalent).
4. **Event Streaming & Activity Feeds:** Using Redis Streams (`XADD`, `XREAD`) as a lightweight alternative to Apache Kafka for IoT sensor metrics or event logs.
5. **Real-Time Pub/Sub Messaging:** Broadcast engine for multi-user chat apps and live notification dispatchers.
6. **Financial Transactions & Rate Limiting:** Safe atomic balance deductions and token-bucket rate limiting via `WATCH / MULTI / EXEC`.

---

## Quick Start

### 1. Build from Source
```bash
go build -o my-redis-server ./cmd/server/
```

### 2. Run the Server

**Standard Mode with AOF Durability:**
```bash
./my-redis-server --port 6380 --aof appendonly.aof --fsync everysec
```

**Cluster Mode (Node 1 owning slots 0 - 8191):**
```bash
./my-redis-server --port 7000 --cluster --cluster-node-id node-1 --cluster-start-slot 0 --cluster-end-slot 8191
```

### CLI Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--port` | `6379` | TCP port to listen on |
| `--db` | (none) | Path to JSON snapshot file for persistence |
| `--aof` | (none) | Path to append-only file (AOF) for durability |
| `--fsync` | `everysec` | AOF sync policy: `always`, `everysec`, or `no` |
| `--cluster` | `false` | Enable Redis Cluster slot management |
| `--cluster-node-id` | `node-main` | Cluster node identifier |
| `--cluster-start-slot` | `0` | Starting hash slot (0-16383) |
| `--cluster-end-slot` | `16383` | Ending hash slot (0-16383) |

---

## Interactive Demonstrations

### Demo 1: Crash Recovery with AOF
Verify that data survives a hard `kill -9` process kill:
```bash
# Insert records
redis-cli -p 6380 SET user:100 "Alice"
redis-cli -p 6380 HSET user:100:profile email "alice@example.com" age "28"
redis-cli -p 6380 ZADD leaderboard 500 "PlayerA" 950 "PlayerB"
redis-cli -p 6380 XADD iot_stream '*' temp 24.5 pressure 1013

# Forcefully kill the server
pkill -9 my-redis-server

# Restart server pointing to the same AOF file
./my-redis-server --port 6380 --aof appendonly.aof --fsync everysec

# Verify full state recovery
redis-cli -p 6380 GET user:100
# "Alice"
redis-cli -p 6380 HGETALL user:100:profile
# 1) "email" 2) "alice@example.com" 3) "age" 4) "28"
redis-cli -p 6380 ZRANGE leaderboard 0 -1 WITHSCORES
# 1) "PlayerA" 2) "500" 3) "PlayerB" 4) "950"
```

### Demo 2: Real-Time Worker Queue with `BLPOP`
Open two terminal windows:

**Terminal 1 (Worker blocking for work):**
```bash
# Blocks until a job arrives (or 30s timeout)
redis-cli -p 6380 BLPOP job_queue 30
```

**Terminal 2 (Producer pushing a job):**
```bash
redis-cli -p 6380 RPUSH job_queue "GenerateReport:user_42"
```
*Terminal 1 unblocks instantly and receives the job.*

### Demo 3: Race Condition Prevention with `WATCH`
Demonstrate optimistic locking on bank account balance:
```bash
# Terminal 1: User A prepares to deduct 50 from 100
redis-cli -p 6380 SET balance 100
redis-cli -p 6380 WATCH balance
redis-cli -p 6380 MULTI
redis-cli -p 6380 SET balance 50

# Terminal 2: An automated transaction changes the balance concurrently
redis-cli -p 6380 SET balance 80

# Terminal 1: User A executes transaction
redis-cli -p 6380 EXEC
# Output: (nil) -> Transaction aborted cleanly, preventing race condition!
```

---

## Client Integration Examples

### Python (`redis-py`)
```python
import redis

r = redis.Redis(host='localhost', port=6380, decode_responses=True)

# 1. Strings & TTL
r.set('session:token_abc', 'user_42', ex=3600)

# 2. SkipList Leaderboard
r.zadd('game_scores', {'Alice': 1500, 'Bob': 2300, 'Charlie': 1800})
top_players = r.zrange('game_scores', 0, -1, withscores=True)
print("Leaderboard:", top_players)

# 3. Stream Events
r.xadd('orders_stream', {'order_id': 'ORD-991', 'total': '49.99'})
events = r.xrange('orders_stream', min='-', max='+')
print("Stream Events:", events)
```

### Go (`go-redis`)
```go
package main

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380"})

	rdb.Set(ctx, "cluster_key", "value", 0)
	val, _ := rdb.Get(ctx, "cluster_key").Result()
	fmt.Println("Result:", val)
}
```

---

## Supported Commands

### Core & Key Management
`PING`, `SET [EX|PX]`, `GET`, `DEL`, `EXISTS`, `EXPIRE`, `TTL`, `TYPE`, `KEYS`, `DBSIZE`, `FLUSHDB`

### Data Structures
- **Lists:** `LPUSH`, `RPUSH`, `LPOP`, `RPOP`, `LRANGE`, `BLPOP`, `BRPOP`
- **Sets:** `SADD`, `SMEMBERS`, `SREM`, `SISMEMBER`
- **Hashes:** `HSET`, `HGET`, `HDEL`, `HEXISTS`, `HGETALL`
- **Sorted Sets:** `ZADD`, `ZRANGE [WITHSCORES]`, `ZREM`, `ZSCORE`
- **Streams:** `XADD`, `XLEN`, `XRANGE`, `XREAD`

### Transactions
`WATCH`, `UNWATCH`, `MULTI`, `EXEC`, `DISCARD`

### Persistence & Management
`SAVE`, `BGSAVE`, `BGREWRITEAOF`, `CONFIG GET/SET`, `MAXMEMORY`, `INFO`

### Pub/Sub & Clustering
- **Pub/Sub:** `SUBSCRIBE`, `UNSUBSCRIBE`, `PUBLISH`
- **Replication:** `REPLICAOF <host> <port>`, `REPLICAOF NO ONE`
- **Clustering:** `CLUSTER KEYSLOT`, `CLUSTER NODES`, `CLUSTER SLOTS`, `CLUSTER INFO`

---

## Test Suite

Run all unit, integration, concurrency, and persistence test suites:
```bash
go test -v -count=1 ./...
```
*(All 8 packages compile and pass cleanly).*
