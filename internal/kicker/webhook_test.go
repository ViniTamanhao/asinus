package kicker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFireQueuesJob(t *testing.T) {
	k := New("http://example.com/hook", 2)

	k.Fire("key1", "val1")
	k.Fire("key2", "val2")

	if len(k.jobs) != 2 {
		t.Fatalf("expected 2 jobs in channel, got %d", len(k.jobs))
	}

	evt1 := <-k.jobs
	if evt1.Key != "key1" || evt1.Value != "val1" {
		t.Fatalf(`expected key1/val1, got %q/%q`, evt1.Key, evt1.Value)
	}

	evt2 := <-k.jobs
	if evt2.Key != "key2" || evt2.Value != "val2" {
		t.Fatalf(`expected key2/val2, got %q/%q`, evt2.Key, evt2.Value)
	}
}

func TestFireDropsOnFullChannel(t *testing.T) {
	k := New("http://example.com/hook", 1)
	for i := 0; i < 1050; i++ {
		k.Fire("k", "v")
	}
	if len(k.jobs) != 1024 {
		t.Fatalf("expected 1024 buffered jobs, got %d", len(k.jobs))
	}
}

func TestWorkerProcessesJob(t *testing.T) {
	var mu sync.Mutex
	var received expiryEvent

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		mu.Lock()
		json.NewDecoder(r.Body).Decode(&received)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	k := New(ts.URL, 2)
	k.Start(ctx)

	k.Fire("testkey", "testval")

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if received.Key != "testkey" || received.Value != "testval" {
		t.Fatalf(`expected testkey/testval, got %q/%q`, received.Key, received.Value)
	}
	mu.Unlock()
}

func TestMultipleWorkers(t *testing.T) {
	var mu sync.Mutex
	var count int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	k := New(ts.URL, 3)
	k.Start(ctx)

	for i := 0; i < 10; i++ {
		k.Fire("k", "v")
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if count != 10 {
		t.Fatalf("expected 10 posts, got %d", count)
	}
	mu.Unlock()
}

func TestWorkersStopOnCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())

	k := New(ts.URL, 2)
	k.Start(ctx)

	k.Fire("k", "v")
	time.Sleep(50 * time.Millisecond)

	cancel()
	k.Wait()
}

func TestFireDoesNotBlock(t *testing.T) {
	k := New("http://example.com/hook", 1)
	for i := 0; i < 1024; i++ {
		k.Fire("k", "v")
	}

	done := make(chan struct{})
	go func() {
		k.Fire("should", "drop")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Fire blocked on full channel")
	}
}
