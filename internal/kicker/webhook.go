package kicker

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type expiryEvent struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Kicker manages a pool of worker goroutines that dispatch expiry webhooks via HTTP POST.
// Bounded workers ensure the store's sweeper is never blocked by slow HTTP requests.
type Kicker struct {
	targetURL   string
	client      *http.Client
	jobs        chan expiryEvent
	wg          sync.WaitGroup
	workerCount int
}

// New creates a Kicker that posts to the given URL.
func New(targetURL string, workerCount int) *Kicker {
	return &Kicker{
		targetURL: targetURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		jobs:        make(chan expiryEvent, 1024),
		workerCount: workerCount,
	}
}

// Start launches workerCount goroutines that read from the jobs channel and POST to the webhook URL.
// Returns once all workers are running. Workers exit cleanly when ctx is cancelled.
func (k *Kicker) Start(ctx context.Context) {
	for i := 0; i < k.workerCount; i++ {
		k.wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case evt, ok := <-k.jobs:
					if !ok {
						return
					}
					k.post(evt)
				}
			}
		})
	}
}

// post sends a single JSON POST request for an expiry event.
func (k *Kicker) post(evt expiryEvent) {
	body, err := json.Marshal(evt)
	if err != nil {
		log.Printf("kicker: json marshal error: %v", err)
		return
	}
	resp, err := k.client.Post(k.targetURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("kicker: post to %s failed for key %q: %v", k.targetURL, evt.Key, err)
		return
	}
	resp.Body.Close()
}

// Fire enqueues an expiry event using a non-blocking send.
// If the channel is full the event is dropped and a warning i logged - the sweeper is never fully blocked.
func (k *Kicker) Fire(key, value string) {
	evt := expiryEvent{Key: key, Value: value}
	select {
	case k.jobs <- evt:
	default:
		log.Printf("kicker: job channel full, dropping event for key %q", key)
	}
}

// Wait blocks untill all workers have finished processing.
// Call after cancelling the context during graceful shutdown.
func (k *Kicker) Wait() {
	k.wg.Wait()
}
