# my-redis — Build Your Own Redis End-to-End

**Language:** Go  
**Repo:** `my-redis`  
**Estimated time:** 6–8 weeks  

## Workflow

1. Build one small thing
2. Test it manually or with a script
3. Commit + push to GitHub
4. Understand before moving on

---

## Phase 0 — Prerequisites

| # | What | File(s) |
|---|------|---------|
| 0.1 | TCP echo server on `:6379` | `cmd/echoserver/main.go` |
| 0.2 | In-memory `map[string]string` with thread-safe CRUD | `store/store.go` |
| 0.3 | Save/load store as JSON | `store/persistence.go` |

## Phase 1 — TCP Server + PING/PONG

- Proper server package
- Handle PING → PONG
- Multiple connections

## Phase 2 — SET/GET/DEL/EXISTS

- Wire store into TCP handler
- Plain-text command parsing

## Phase 3 — RESP Protocol

- RESP parser and encoder
- Replace plain text with RESP wire format

## Phase 4 — Concurrent Clients

- `sync.RWMutex` in store
- Scale to 100+ clients

## Phase 5 — Expiration (EXPIRE/TTL)

- Background goroutine for key expiry
- `SET key val EX 30`, `EXPIRE`, `TTL`

## Phase 6 — Persistence (RDB/AOF)

- `SAVE` / load snapshot
- Append-only file logging

## Phase 7 — More Data Structures

- Lists, Sets, Hashes, Sorted Sets

## Phase 8 — Transactions (MULTI/EXEC/DISCARD)

- Command queueing + atomic execution

## Phase 9 — Pub/Sub

- `SUBSCRIBE`, `PUBLISH`, `UNSUBSCRIBE`

## Phase 10 — Replication

- `REPLICAOF`
- Full sync + partial sync

## Phase 11 — LRU Cache / Eviction

- `MAXMEMORY` policy
- LRU eviction

## Phase 12 — Performance

- Benchmarking
- Memory pooling, zero-copy, faster parser
