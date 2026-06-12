# Project Goals

## Primary Goal

Build a lightweight, Redis-compatible key-value store whose core value proposition is **active expiry notification** — pushing events to a webhook the moment a key dies, rather than requiring clients to poll.

## Design Principles

**Event-driven over polling.** External services should never need to scan for expired keys. Asinus owns that responsibility and delivers expiry events asynchronously.

**Bounded memory, always.** The store must never grow unboundedly. LRU eviction enforces a hard cap per shard so the process footprint is predictable under sustained write load.

**Crash safety without sacrificing throughput.** The AOF provides durability on every write via `fsync`. Background compaction prevents indefinite file growth without blocking the hot path.

**No global contention.** 256-way sharding with per-shard locks ensures write throughput scales with concurrency without a single serialization point.

**Wire-compatible with Redis tooling.** By implementing RESP2, Asinus works with existing Redis clients (`redis-cli`, standard language SDKs) with zero client-side changes.

## Intended Use Cases

- **Delayed event routing** — set a key with a TTL and let the webhook trigger a downstream action when it expires (abandoned cart emails, session cleanup, scheduled notifications).
- **Auto-reversing state** — temporary bans, rate-limit windows, feature flags with automatic rollback.
- **Ephemeral token storage** — store short-lived auth tokens and receive a webhook on expiry to trigger revocation logic.

## Non-Goals

- Full Redis command compatibility (only `GET`, `SET`, `DEL` are implemented).
- Replication or clustering.
- Persistent snapshots (RDB-style); AOF is the only persistence mechanism.
- Per-key webhook configuration; one global webhook URL serves all expiry events.
