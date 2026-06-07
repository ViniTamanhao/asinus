package store

import (
	"sync"
	"testing"
	"time"
)

func TestSetGet(t *testing.T) {
	s := New(nil)
	s.Set("key", "val", 0)
	got, ok := s.Get("key")
	if !ok || got != "val" {
		t.Fatalf(`expected "val", got %q (ok=%v)`, got, ok)
	}
}

func TestGetMissing(t *testing.T) {
	s := New(nil)
	_, ok := s.Get("nope")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestDelete(t *testing.T) {
	s := New(nil)
	s.Set("key", "val", 0)
	if !s.Delete("key") {
		t.Fatal("expected true after deleting existing key")
	}
	if _, ok := s.Get("key"); ok {
		t.Fatal("expected key to be gone after delete")
	}
}

func TestDeleteMissing(t *testing.T) {
	s := New(nil)
	if s.Delete("nope") {
		t.Fatal("expected false for missing key")
	}
}

func TestSetOverwrite(t *testing.T) {
	s := New(nil)
	s.Set("key", "old", 0)
	s.Set("key", "new", 0)
	got, ok := s.Get("key")
	if !ok || got != "new" {
		t.Fatalf(`expected "new", got %q`, got)
	}
}

func TestTTLNotExpired(t *testing.T) {
	s := New(nil)
	s.Set("key", "val", 10*time.Second)
	got, ok := s.Get("key")
	if !ok || got != "val" {
		t.Fatalf(`expected "val", got %q (ok=%v)`, got, ok)
	}
}

func TestTTLExpired(t *testing.T) {
	s := New(nil)
	s.Set("key", "val", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok := s.Get("key")
	if ok {
		t.Fatal("expected key to be expired")
	}
}

func TestConcurrentGoroutines(t *testing.T) {
	s := New(nil)
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
	s := New(func(key, value string) {
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
	s := New(nil)
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
