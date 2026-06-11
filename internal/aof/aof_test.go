package aof

import (
	"asinus/internal/resp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.aof")
	a, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	cmds := [][]string{
		{"SET", "a", "1"},
		{"SET", "b", "2"},
		{"DEL", "a"},
	}
	for _, c := range cmds {
		if err = a.Write(resp.EncodeCommand(c)); err != nil {
			t.Fatal(err)
		}
	}
	a.Close()

	a2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Close()

	var got []string
	if err := a2.Read(func(args []string) {
		got = append(got, strings.Join(args, " "))
	}); err != nil {
		t.Fatal(err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(got))
	}
	expected := []string{"SET a 1", "SET b 2", "DEL a"}
	for i, c := range expected {
		if got[i] != c {
			t.Fatalf("line %d: expected %q, got %q", i, c, got[i])
		}
	}
}

func TestFileCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.aof")
	a, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected file to be created")
	}
}

func TestReadEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.aof")
	a, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	count := 0
	if err := a.Read(func(args []string) {
		count++
	}); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 lines, got %d", count)
	}
}

func TestConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.aof")
	a, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.Write(resp.EncodeCommand([]string{"SET", "key", "value"}))
		}()
	}
	wg.Wait()

	var lines []string
	a.Read(func(args []string) {
		lines = append(lines, strings.Join(args, " "))
	})
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines, got %d", len(lines))
	}
}
