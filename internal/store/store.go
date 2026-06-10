package store

import (
	"context"
	"hash/fnv"
	"sync"
	"time"
)

// entry holds the value and an optional expiry key.
// A zero expiresAt means the keu never expires.
type entry struct {
	value     string
	expiresAt time.Time
}

const nshards = 256

// shard is a single partition of the key space with its own lock.
type shard struct {
	mu sync.RWMutex
	data map[string]entry
}

// Store is a thread-safe in-memory key-value store.
type Store struct {
	shards [nshards]shard
	onExpire func(key, value string)
}

// New creates and returns a new empty Store.
func New(onExpire func(key, value string)) *Store {
	s := &Store{onExpire: onExpire}
	for i := range s.shards {
		s.shards[i].data = make(map[string]entry)
	}
	
	return s
}

// Set stores a value under the given key.
// It acquires a write lock because we're mutating the map.
func (s *Store) Set(key, value string, ttl time.Duration) {
	sh := s.shardPointer(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}

	sh.data[key] = entry{value: value, expiresAt: exp}
}

// Get retrieves the value stored under the given key. The bool return is false
// when the key does not exist. Multiple readers can run in parallel thanks to RLock.
func (s *Store) Get(key string) (string, bool) {
	sh := s.shardPointer(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	e, ok := sh.data[key]
	if !ok {
		return "", false
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		return "", false
	}

	return e.value, true
}

// Delete removes a key. It returns true if the key existed and was deleted.
func (s *Store) Delete(key string) bool {
	sh := s.shardPointer(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	_, ok := sh.data[key]
	delete(sh.data, key)
	return ok
}

// StartSweeper runs a background loop that evicts expired keys at the given interval.
// It stops cleanly when ctx is cancelled.
func (s *Store) StartSweeper(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep()
		}
	}
}

// sweep locks the map and deletes all expired entries.
func (s *Store) sweep() {
	now := time.Now()
	type kv struct{ key, value string }
	var expired []kv

	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()
		for k, e := range sh.data {
			if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
				expired = append(expired, kv{k, e.value})
				delete(sh.data, k)
			}
		}
		sh.mu.Unlock()
	}

	for _, e := range expired {
		if s.onExpire != nil {
			s.onExpire(e.key, e.value)
		}
	}
}

// fnvCompute computes a FNV-1a 32-bit hash of the key.
func fnvCompute(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}

// shardPointer returns a pointer to the shard that owns the given key.
func (s *Store) shardPointer(key string) *shard {
	return &s.shards[fnvCompute(key)%nshards]
}
