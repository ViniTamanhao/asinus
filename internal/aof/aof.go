package aof

import (
	"bufio"
	"io"
	"os"
	"sync"
)

// AOF provides append-only persistence for write commands.
// It is safe for concurrent use via a mutex.
type AOF struct {
	file *os.File
	mu   sync.Mutex
	path string
}

// New opens or creates the AOF file in append mode.
func New(path string) (*AOF, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &AOF{file: f, path: path}, nil
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

// Rewrite rewrites the file with the contents of dumpFunc.
func (a *AOF) Rewrite(dumpFunc func(io.Writer) error) error {
	tmpPath := a.path + ".aof.tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	if err = dumpFunc(tmpFile); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	if err = tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.file.Close()

	if err = os.Rename(tmpPath, a.path); err != nil {
		return err
	}

	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	a.file = f
	return nil
}

// Close closes the file.
func (a *AOF) Close() error {
	return a.file.Close()
}
