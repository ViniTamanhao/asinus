package store

import (
	"context"
	"sync"
	"time"
)

// entry holds the value and an optional expiry key.
// A zero expiresAt means the keu never expires.
type entry struct {
	value     string
	expiresAt time.Time
}

// Store is a thread-safe in-memory key-value store.
type Store struct {
	mu       sync.RWMutex
	data     map[string]entry
	onExpire func(key, value string)
}

// New creates and returns a new empty Store.
func New(onExpire func(key, value string)) *Store {
	return &Store{
		data:     make(map[string]entry),
		onExpire: onExpire,
	}
}

// Set stores a value under the given key.
// It acquires a write lock because we're mutating the map.
func (s *Store) Set(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	s.data[key] = entry{value: value, expiresAt: exp}
}

// Get retrieves the value stored under the given key. The bool return is false
// when the key does not exist. Multiple readers can run in parallel thanks to RLock.
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
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
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[key]
	delete(s.data, key)
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
	s.mu.Lock()
	now := time.Now()
	type kv struct{ key, value string }
	var expired []kv

	for k, e := range s.data {
		if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
			expired = append(expired, kv{k, e.value})
			delete(s.data, k)
		}
	}
	s.mu.Unlock()

	for _, e := range expired {
		if s.onExpire != nil {
			s.onExpire(e.key, e.value)
		}
	}
}
