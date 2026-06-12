# Package: `internal/store`

Thread-safe, sharded, in-memory key-value store with LRU eviction and TTL-based expiry.

## Overview

The store partitions the key space across **256 shards** using a FNV-1a 32-bit hash of the key. Each shard has its own `sync.RWMutex` so independent keys can be written concurrently without contention.

Within each shard, keys are arranged in a **doubly-linked list** ordered by access time (head = MRU, tail = LRU). When a shard's entry count exceeds `capacity`, the tail node (least recently used) is evicted synchronously inside `Set`.

## Types

### `Store`

```go
type Store struct {
    shards   [256]shard
    onExpire func(key, value string)
}
```

Created via `New(shardCapacity int, onExpire func(key, value string))`. `onExpire` is called for every evicted or expired key.

### `shard`

Internal type. Each shard holds:
- `data map[string]*Node` — O(1) key lookup.
- `head`, `tail *Node` — MRU/LRU list sentinels.
- `capacity`, `count int` — eviction threshold.
- `mu sync.RWMutex` — per-shard lock.

### `Node`

Internal list node: `key`, `value string`, `expiresAt time.Time`, `prev`/`next *Node`.

## API

### `Set(key, value string, ttl time.Duration)`

Stores or updates a key. Pass `ttl = 0` for a non-expiring key.

- Promotes an existing key to MRU without allocating a new node.
- If the shard is at capacity, evicts the LRU tail and calls `onExpire`.
- Holds the write lock for the duration.

### `Get(key string) (string, bool)`

Returns the value and `true` if the key exists and has not expired.

- Promotes the accessed node to MRU (cache hit updates recency).
- Returns `"", false` for missing or expired keys; expired keys are **not** actively deleted on `Get` (the sweeper handles that).

### `Delete(key string) bool`

Removes a key and returns `true` if it existed. Does **not** invoke `onExpire`.

### `Dump(w io.Writer) error`

Serializes all live, non-expired keys to `w` as RESP `SET` commands. Used by AOF compaction. Iterates shards sequentially, holding each shard's read lock for the duration of that shard's iteration.

### `StartSweeper(ctx context.Context, interval time.Duration)`

Runs a background loop that calls `sweep()` on every tick. Blocks until `ctx` is cancelled. Typically called in its own goroutine.

## TTL Enforcement

Two paths:
1. **Lazy** — `Get` checks `expiresAt` before returning a value; expired entries are invisible but not removed.
2. **Active** — `sweep()` scans every shard once per tick, removes expired nodes under the write lock, then calls `onExpire` outside the lock to avoid re-entrant deadlocks.

## Concurrency Notes

- `Set`, `Get`, `Delete` each acquire a single shard's lock — no cross-shard coordination needed.
- `Dump` holds each shard's **read** lock sequentially, so live writes to other shards are not blocked.
- `onExpire` is always called **after** releasing the shard lock to prevent a deadlock if the callback tries to write back to the store.
