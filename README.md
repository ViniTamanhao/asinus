# Asinus

**An event-driven, sharded, in-memory key-value database built in Go.**

Asinus is a lightweight database that flips the traditional caching model on its head. Instead of passively waiting for a worker to poll for expired data, Asinus actively "kicks" data back to your architecture via asynchronous webhooks the moment a key's Time-To-Live (TTL) expires or it gets evicted from memory.

## Features

* **Event-Driven Expiry:** Triggers an HTTP POST to a configured webhook when a key dies, powered by a highly concurrent background worker pool.
* **Massive Throughput:** Eliminates global lock contention using 256-way map sharding via FNV-1a hashing.
* **Strict Memory Bounding:** Uses a custom Doubly-Linked List to enforce a Least Recently Used (LRU) eviction policy per shard.
* **Rock-Solid Persistence:** Survives server crashes using an Append-Only File (AOF) with automatic background compaction to prevent infinite disk growth.

## Why use Asinus?

Asinus philosophy is to provide a simple, yet powerful way to build caching while being natively built around expiry events. This makes Asinus perfect for:

* **Delayed Event Routing:** "If the user doesn't complete checkout in 15 minutes, trigger the Abandoned Cart email microservice."
* **Auto-Reversing Firewalls:** "Ban this IP address for exactly 600 seconds, then notify the WAF to unban them."
* **Ephemeral Sessions:** Store authentication tokens and automatically trigger a cleanup job when the session dies.

## Quick Start

### 1. Build and Run the Server

Compile the binary and start the server. You can configure the AOF persistence, worker pool size, target webhook URL and shard capacity via CLI flags.

```bash
go build -o asinus .

./asinus --port 6379 \
         --aof ./data/kickback.aof \
         --webhook [http://your-api.com/webhooks/cache](http://your-api.com/webhooks/cache) \
         --workers 10 \
         --shard-capacity 1000
