package store

import (
	"context"
	"hash/fnv"
	"sync"
	"time"
)

// entry holds the value and an optional expiry key.
// A zero expiresAt means the key never expires.

// Node is a doubly-linked list node that holds a key-value pair,
// an optional expiry, and list pointers
type Node struct {
	key       string
	value     string
	expiresAt time.Time
	prev      *Node
	next      *Node
}

const nshards = 256

// shard is a single partition of the key space with its own lock
// and an LRU-linked list bounded by capacity.
type shard struct {
	mu       sync.RWMutex
	data     map[string]*Node
	head     *Node
	tail     *Node
	capacity int
	count    int
}

// addNodeToFront inserts n at the head of the LRU list.
// The caller MUST hold sh.mu.Lock().
func (sh *shard) addNodeToFront(n *Node) {
	if sh.head == nil {
		sh.head = n
		sh.tail = n
		return
	}

	n.next = sh.head
	sh.head.prev = n
	sh.head = n
}

// removeNode detaches n from the LRU list.
// The caller MUST hold sh.mu.Lock().
func (sh *shard) removeNode(n *Node) {
	if n == sh.head {
		sh.head = n.next
	}
	if n == sh.tail {
		sh.tail = n.prev
	}
	if n.prev != nil {
		n.prev.next = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	}
	n.prev = nil
	n.next = nil
}

// moveToFront promotes n to the head of the LRU list.
// The caller MUST hold sh.mu.Lock().
func (sh *shard) moveToFront(n *Node) {
	sh.removeNode(n)
	sh.addNodeToFront(n)
}

// Store is a thread-safe in-memory key-value store.
type Store struct {
	shards   [nshards]shard
	onExpire func(key, value string)
}

// New creates and returns a new empty Store.
func New(shardCapacity int, onExpire func(key, value string)) *Store {
	s := &Store{onExpire: onExpire}
	for i := range s.shards {
		s.shards[i].data = make(map[string]*Node)
		s.shards[i].capacity = shardCapacity
	}
	return s
}

// Set stores a value under the given key.
// If the key already exists, its value and TTL are updated and the node is promoted to the MRU position.
// If the shard exceeds capacity the LRU entry is evicted and onExpire is called immediately.
func (s *Store) Set(key, value string, ttl time.Duration) {
	sh := s.shardPointer(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}

	if node, ok := sh.data[key]; ok {
		node.value = value
		node.expiresAt = exp
		sh.moveToFront(node)
		return
	}

	node := &Node{
		key:       key,
		value:     value,
		expiresAt: exp,
	}

	sh.data[key] = node
	sh.addNodeToFront(node)
	sh.count++

	if sh.count > sh.capacity {
		victim := sh.tail
		sh.removeNode(victim)
		delete(sh.data, victim.key)
		sh.count--
		if s.onExpire != nil {
			s.onExpire(victim.key, victim.value)
		}
	}
}

// Get retrueves the value stored under the given key.
// On a cache hit, the node is promoted to the front of the LRU list
func (s *Store) Get(key string) (string, bool) {
	sh := s.shardPointer(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	node, ok := sh.data[key]
	if !ok {
		return "", false
	}

	if !node.expiresAt.IsZero() && time.Now().After(node.expiresAt) {
		return "", false
	}

	sh.moveToFront(node)
	return node.value, true
}

// Delete removes a key.
// It returns true if the key existed and was deleted.
func (s *Store) Delete(key string) bool {
	sh := s.shardPointer(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	node, ok := sh.data[key]
	if !ok {
		return false
	}

	sh.removeNode(node)
	delete(sh.data, key)
	sh.count--
	return true
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

// sweep removes all expired entries across every shard.
func (s *Store) sweep() {
	now := time.Now()
	type kv struct{ key, value string }

	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.Lock()

		var shardExpired []kv

		for k, node := range sh.data {
			if !node.expiresAt.IsZero() && now.After(node.expiresAt) {
				sh.removeNode(node)
				delete(sh.data, k)
				sh.count--
				shardExpired = append(shardExpired, kv{k, node.value})
			}
		}
		sh.mu.Unlock()

		for _, e := range shardExpired {
			if s.onExpire != nil {
				s.onExpire(e.key, e.value)
			}
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
