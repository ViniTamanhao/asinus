# Package: `internal/aof`

Append-Only File (AOF) persistence — durably records every write command and compacts the log periodically.

## Overview

The AOF provides crash recovery: on every `SET` or `DEL`, the RESP-encoded command is appended to a file and immediately `fsync`'d to disk. On restart, the file is replayed from the beginning to rebuild the in-memory store.

To prevent the file from growing without bound, the server triggers `Rewrite` every 5 minutes. `Rewrite` calls `Store.Dump` to serialize only the current live state, writes it to a temporary file, then atomically renames it over the live AOF — no data is lost during the swap.

## API

### `New(path string) (*AOF, error)`

Opens or creates the file at `path` in append+read-write mode (`O_APPEND | O_CREATE | O_RDWR`). Returns an `*AOF` ready for use.

### `Write(cmd []byte) error`

Appends `cmd` to the file and calls `file.Sync()` (forces an `fsync` syscall). Holds the mutex for the duration. This is intentionally synchronous — durability is prioritised over write throughput.

### `Read(fn func([]string)) error`

Seeks to the beginning of the file and replays every persisted command by passing each parsed `[]string` to `fn`. Called once at startup with `srv.Dispatch` as the callback. Returns `nil` on `io.EOF` (normal end of file).

### `Rewrite(dumpFunc func(io.Writer) error) error`

Atomic log compaction:
1. Creates `<path>.aof.tmp`.
2. Calls `dumpFunc` (i.e. `Store.Dump`) to write all live keys to the temp file.
3. Closes and renames the temp file over the live file under the mutex.
4. Re-opens the live file in append mode.

If `dumpFunc` or the rename fails, the temp file is removed and the original AOF is left intact.

### `Close() error`

Closes the underlying file. Called during graceful shutdown.

## Concurrency

All operations are protected by a single `sync.Mutex`. `Read` and `Write` both acquire the lock, so replay at startup and concurrent writes during normal operation are serialized safely.

`Rewrite` acquires the lock only for the file-swap step, so `dumpFunc` can read the store (under its own per-shard locks) concurrently with live writes. A write that arrives between the end of `dumpFunc` and the rename will be included in the next `Write` call after the new file is open.

## Durability Trade-off

`fsync` on every `Write` means a server crash will lose at most the in-flight write, not multiple commands. Applications that can tolerate some data loss in exchange for higher throughput would instead batch writes or skip `fsync` — that is a deliberate non-goal for this project.
