package store

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSetGet(t *testing.T) {
	s := New(10, nil)
	s.Set("key", "val", 0)
	got, ok := s.Get("key")
	if !ok || got != "val" {
		t.Fatalf(`expected "val", got %q (ok=%v)`, got, ok)
	}
}

func TestGetMissing(t *testing.T) {
	s := New(10, nil)
	_, ok := s.Get("nope")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestDelete(t *testing.T) {
	s := New(10, nil)
	s.Set("key", "val", 0)
	if !s.Delete("key") {
		t.Fatal("expected true after deleting existing key")
	}
	if _, ok := s.Get("key"); ok {
		t.Fatal("expected key to be gone after delete")
	}
}

func TestDeleteMissing(t *testing.T) {
	s := New(10, nil)
	if s.Delete("nope") {
		t.Fatal("expected false for missing key")
	}
}

func TestSetOverwrite(t *testing.T) {
	s := New(10, nil)
	s.Set("key", "old", 0)
	s.Set("key", "new", 0)
	got, ok := s.Get("key")
	if !ok || got != "new" {
		t.Fatalf(`expected "new", got %q`, got)
	}
}

func TestTTLNotExpired(t *testing.T) {
	s := New(10, nil)
	s.Set("key", "val", 10*time.Second)
	got, ok := s.Get("key")
	if !ok || got != "val" {
		t.Fatalf(`expected "val", got %q (ok=%v)`, got, ok)
	}
}

func TestTTLExpired(t *testing.T) {
	s := New(10, nil)
	s.Set("key", "val", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok := s.Get("key")
	if ok {
		t.Fatal("expected key to be expired")
	}
}

func TestConcurrentGoroutines(t *testing.T) {
	s := New(10, nil)
	var wg sync.WaitGroup

	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n))
			s.Set(key, "val", 0)
			s.Get(key)
			s.Delete(key)
		}(i)
	}
	wg.Wait()
}

func TestOnExpireCallback(t *testing.T) {
	var calledKey, calledValue string
	s := New(10, func(key, value string) {
		calledKey = key
		calledValue = value
	})

	s.Set("ex", "pire", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	s.sweep()

	if calledKey != "ex" || calledValue != "pire" {
		t.Fatalf(`expected "ex" / "pire", got %q / %q`, calledKey, calledValue)
	}
}

func TestSweeperDeletesExpired(t *testing.T) {
	s := New(10, nil)
	s.Set("gone", "bye", 50*time.Millisecond)
	s.Set("stay", "here", 0)

	time.Sleep(100 * time.Millisecond)
	s.sweep()

	if _, ok := s.Get("gone"); ok {
		t.Fatal("expected expired key to be deleted")
	}
	if _, ok := s.Get("stay"); !ok {
		t.Fatal("expected permanent key to remain")
	}
}

func TestLRUEviction(t *testing.T) {
    var keys [4]string
    for i, n := 0, 0; n < 4; i++ {
        k := fmt.Sprintf("k%d", i)
        if fnvCompute(k)%nshards == 0 {
            keys[n] = k
            n++
        }
    }
    t.Logf("colliding keys: %v", keys)

    var evicted []string
    s := New(3, func(key, value string) {
        evicted = append(evicted, key)
    })

    for i := range 3 {
        s.Set(keys[i], "v", 0)
    }

    s.Set(keys[3], "v", 0)

    if len(evicted) != 1 || evicted[0] != keys[0] {
        t.Fatalf("expected %q to be evicted, got %v", keys[0], evicted)
    }

    for i, k := range keys {
        _, ok := s.Get(k)
        if i == 0 && ok {
            t.Fatalf("expected %q to be evicted", k)
        }
        if i > 0 && !ok {
            t.Fatalf("expected %q to exist", k)
        }
    }
}

func TestLRUPromotion(t *testing.T) {
    var keys [4]string
    for i, n := 0, 0; n < 4; i++ {
        k := fmt.Sprintf("k%d", i)
        if fnvCompute(k)%nshards == 0 {
            keys[n] = k
            n++
        }
    }
    t.Logf("colliding keys: %v", keys)

    var evicted []string
    s := New(3, func(key, value string) {
        evicted = append(evicted, key)
    })

    for i := range 3 {
        s.Set(keys[i], "v", 0)
    }

    s.Get(keys[1])

    s.Set(keys[3], "v", 0)

    if len(evicted) != 1 {
        t.Fatalf("expected 1 eviction, got %d: %v", len(evicted), evicted)
    }
    t.Logf("evicted: %s (was LRU at time of insert)", evicted[0])
}
