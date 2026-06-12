# Package: `internal/server`

TCP server that accepts Redis clients, parses RESP commands, dispatches them to the store, and writes RESP responses back.

## Overview

`Server` wraps a `Store` and an optional `AOF`. It opens a TCP listener, spawns one goroutine per client connection, and runs a background AOF compaction ticker (every 5 minutes) while persistence is enabled.

The only commands accepted are `GET`, `SET`, and `DEL` — a deliberate subset of the Redis command set. Unknown commands receive an `ERR unknown command` response.

## API

### `New(s *store.Store, a *aof.AOF) *Server`

Creates a `Server`. Pass `nil` for `a` to disable AOF persistence.

### `Start(ctx context.Context, port string) error`

Binds a TCP listener on `:port`, then:
- Starts the AOF rewrite background goroutine (if AOF is set).
- Accepts connections in a loop; each connection gets its own goroutine via `handleConnection`.
- Blocks until the listener is closed (triggered by context cancellation).
- Waits for all in-flight connection goroutines to finish before returning.

Returns `ctx.Err()` on clean shutdown, or a listener error on failure.

### `SetReplaying(v bool)`

When `true`, `Dispatch` skips AOF writes. Used during startup replay to avoid re-appending commands that are already in the file.

### `Dispatch(args []string, w *resp.Writer)`

Executes a parsed command and writes the RESP response. This method is also called directly during AOF replay (with `w` pointing to `io.Discard`).

| Command | Args | Success response | Error conditions |
|---|---|---|---|
| `GET key` | 2 | Bulk string value, or Null Bulk String if missing | Wrong arg count |
| `SET key value [ttl]` | 3–4 | `+OK` | Wrong arg count; non-integer or negative TTL |
| `DEL key` | 2 | `:1` (deleted) or `:0` (not found) | Wrong arg count |

Commands are matched case-insensitively.

## Connection Handling

Each `handleConnection` goroutine:
1. Wraps the connection in a `bufio.Writer` (batches response writes) and a `resp.Parser`.
2. Reads commands in a loop until the connection closes or an error occurs.
3. Listens for context cancellation on a separate goroutine; on cancel, sets a zero read deadline on the connection to unblock the parser and cause it to return an error, cleanly exiting the loop.

The `sync.WaitGroup` in `Server` tracks all active connection goroutines. `Start` waits on it before returning so the process does not exit with open connections mid-flight.

## AOF Compaction

While `Start` is running and AOF is enabled, a background goroutine calls `aof.Rewrite(store.Dump)` every 5 minutes. Errors are logged but do not stop the server. The compaction is atomic from the file system's perspective (`os.Rename`).
