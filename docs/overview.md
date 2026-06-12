# Asinus — Project Overview

Asinus is a Redis-compatible, in-memory key-value store written in Go. Its defining feature is **event-driven expiry**: when a key's TTL expires or it is evicted by the LRU policy, Asinus fires an HTTP POST webhook so downstream services can react immediately rather than polling.

## Key Properties

| Property | Detail |
|---|---|
| Protocol | RESP2 (Redis Serialization Protocol v2) — wire-compatible with standard Redis clients |
| Supported commands | `GET`, `SET` (with optional TTL in seconds), `DEL` |
| Concurrency model | 256-way sharded store; each shard has its own `sync.RWMutex` |
| Eviction | Per-shard LRU doubly-linked list; bounded by `--shard-capacity` |
| TTL enforcement | Background sweeper ticks every second; lazy check on `GET` |
| Persistence | Append-Only File (AOF) with `fsync` on every write; atomic rewrite every 5 minutes |
| Webhook delivery | Fixed-size worker pool (configurable); non-blocking `Fire` drops events when the channel is full |

## Component Map

```
cmd/root.go           — CLI entry point; wires Kicker → Store → AOF → Server
internal/resp         — RESP2 parser and writer
internal/store        — sharded in-memory store with LRU eviction and TTL sweeper
internal/aof          — append-only file persistence and compaction
internal/kicker       — HTTP webhook worker pool
internal/server       — TCP listener and RESP command dispatcher
```

## Request Lifecycle

```
TCP client
  │  RESP-encoded command
  ▼
internal/server  ─── ParseCommand ──► internal/resp
  │
  │  Dispatch (GET / SET / DEL)
  ▼
internal/store
  │  (on SET/DEL) ──────────────────► internal/aof  (Write + fsync)
  │
  │  (on TTL expiry or LRU eviction)
  ▼
internal/kicker  ─── HTTP POST ──────► configured webhook URL
```

## Startup Sequence

1. Cobra CLI parses flags.
2. `Kicker` is created and workers are started (if `--webhook` is set).
3. `Store` is created with the `Kicker.Fire` callback wired as `onExpire`.
4. `AOF` file is opened (if `--aof` is set).
5. AOF is replayed with `srv.replaying = true` to rebuild in-memory state without re-writing commands.
6. `StartSweeper` is started.
7. TCP listener opens; server begins accepting connections.

## Shutdown Sequence

`SIGINT` or `SIGTERM` cancels the root context, which:
1. Closes the TCP listener (no new connections accepted).
2. Sets a read deadline on existing connections so they unblock and exit.
3. Cancels sweeper and AOF rewrite goroutines.
4. Calls `Kicker.Wait()` to drain in-flight webhook jobs.
