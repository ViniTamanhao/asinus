package aof

import (
	"bufio"
	"os"
	"sync"
)

// AOF provides append-only persistence for write commands.
// It is safe for concurrent use via a mutex.
type AOF struct {
	file *os.File
	mu   sync.Mutex
}

// New opens or creates the AOF file in append mode.
func New(path string) (*AOF, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &AOF{file: f}, nil
}

// Write appends a command line to the file and calls sync to flush.
func (a *AOF) Write(cmd string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.file.WriteString(cmd + "\n"); err != nil {
		return err
	}
	return a.file.Sync()
}

// Read relays the file from the beginning, passing each command line to fn.
// Called once at startup to rebuild the store.
func (a *AOF) Read(fn func(string)) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.file.Seek(0, 0); err != nil {
		return err
	}

	scanner := bufio.NewScanner(a.file)
	for scanner.Scan() {
		fn(scanner.Text())
	}
	return scanner.Err()
}

// Close closes the underlying file.
func (a *AOF) Close() error {
	return a.file.Close()
}
