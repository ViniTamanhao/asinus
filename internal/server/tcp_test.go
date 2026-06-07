package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"asinus/internal/store"
)

func setupTest(t *testing.T) (*Server, context.CancelFunc) {
	t.Helper()
	_, cancel := context.WithCancel(context.Background())
	s := store.New(nil)
	return New(s, nil), cancel
}

func TestDispatchSet(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := srv.Dispatch("SET color blue")
	if reply != "+OK" {
		t.Fatalf(`expected "+OK", got %q`, reply)
	}
}

func TestDispatchGet(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	srv.Dispatch("SET color blue")
	reply := srv.Dispatch("GET color")
	if reply != "+blue" {
		t.Fatalf(`expected "+blue", got %q`, reply)
	}
}

func TestDispatchGetMissing(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := srv.Dispatch("GET nope")
	if reply != "-ERR not found" {
		t.Fatalf(`expected "-ERR not found", got %q`, reply)
	}
}

func TestDispatchDel(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	srv.Dispatch("SET color blue")
	reply := srv.Dispatch("DEL color")
	if reply != "+OK" {
		t.Fatalf(`expected "+OK", got %q`, reply)
	}
}

func TestDispatchDelMissing(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := srv.Dispatch("DEL nope")
	if reply != "+OK" {
		t.Fatalf(`expected "+OK", got %q`, reply)
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := srv.Dispatch("FOO bar")
	if !strings.HasPrefix(reply, "-ERR") {
		t.Fatalf(`expected "-ERR..." prefix, got %q`, reply)
	}
}

func TestDispatchSetWithTTL(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := srv.Dispatch("SET color blue 60")
	if reply != "+OK" {
		t.Fatalf(`expected "+OK", got %q`, reply)
	}
}

func TestDispatchSetWrongArgs(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := srv.Dispatch("SET")
	if !strings.HasPrefix(reply, "-ERR") {
		t.Fatalf(`expected "-ERR..." prefix, got %q`, reply)
	}
}

func TestFullTCPRoundTrip(t *testing.T) {
	ctx := t.Context()

	s := store.New(nil)
	srv := New(s, nil)

	// start on a random port
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			srv.wg.Add(1)
			go srv.handleConnection(conn)
		}
	}()

	addr := ln.Addr().String()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprintln(conn, "SET color blue")
	got, _ := bufio.NewReader(conn).ReadString('\n')
	got = strings.TrimSpace(got)
	if got != "+OK" {
		t.Fatalf(`expected "+OK", got %q`, got)
	}

	fmt.Fprintln(conn, "GET color")
	got, _ = bufio.NewReader(conn).ReadString('\n')
	got = strings.TrimSpace(got)
	if got != "+blue" {
		t.Fatalf(`expected "+blue", got %q`, got)
	}
}

func TestHandleConnectionExitsOnClose(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := store.New(nil)
	srv := New(s, nil)

	client, server := net.Pipe()
	srv.wg.Add(1)
	go srv.handleConnection(server)

	client.Close()
	time.Sleep(50 * time.Millisecond)
}

