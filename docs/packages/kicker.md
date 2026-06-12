# Package: `internal/kicker`

HTTP webhook worker pool for delivering key-expiry events to an external URL.

## Overview

When a key expires or is LRU-evicted, the store calls `onExpire(key, value)`. `Kicker.Fire` is wired as this callback. It enqueues an event on a buffered channel without blocking; a pool of goroutines drain the channel and POST JSON payloads to the configured webhook URL.

This design keeps the store's sweeper fast — it never waits on network I/O.

## Webhook Payload

```json
{ "key": "session:abc123", "value": "user:42" }
```

Delivered as an HTTP POST with `Content-Type: application/json`.

## API

### `New(targetURL string, workerCount int) *Kicker`

Creates a `Kicker` with a 1024-element buffered job channel and an `http.Client` with a 10-second timeout. Workers are not started until `Start` is called.

### `Start(ctx context.Context)`

Launches `workerCount` goroutines. Each worker blocks on the jobs channel and calls `post` for every received event. Workers stop when `ctx` is cancelled or the channel is closed.

### `Fire(key, value string)`

Enqueues an expiry event using a **non-blocking send**. If the channel is full, the event is dropped and a warning is logged. This guarantees the sweeper is never stalled by a full queue or slow webhook consumers.

### `Wait()`

Closes the jobs channel (signals workers to drain and exit) and blocks until all workers finish. Call during graceful shutdown after cancelling the context.

## Backpressure and Drop Policy

The 1024-slot buffer absorbs bursts of expiry events. Under sustained overload (more expirations per second than the worker pool can POST), events are dropped. This is an explicit trade-off: **availability of the store over guaranteed webhook delivery**.

If guaranteed delivery is required, the caller should implement retry logic (e.g., a dead-letter queue) in the webhook handler.

## Concurrency Notes

- `Fire` is safe to call from multiple goroutines simultaneously (channel send is safe).
- `post` is called exclusively within worker goroutines; the `http.Client` is shared and is safe for concurrent use.
- `Wait` must only be called once, after `Start`.
