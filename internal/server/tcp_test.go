package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"

	"asinus/internal/resp"
	"asinus/internal/store"
)

// dispatchArgs is a test helper that calls Dispatch and returns the raw RESP output.
func dispatchArgs(t *testing.T, srv *Server, args []string) string {
	t.Helper()
	var buf bytes.Buffer
	w := resp.NewWriter(&buf)
	srv.Dispatch(args, w)
	return buf.String()
}

func setupTest(t *testing.T) (*Server, context.CancelFunc) {
	t.Helper()
	_, cancel := context.WithCancel(context.Background())
	s := store.New(10, nil)
	return New(s, nil), cancel
}

func TestDispatchSet(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := dispatchArgs(t, srv, []string{"SET", "color", "blue"})
	if reply != "+OK\r\n" {
		t.Fatalf(`expected "+OK\r\n", got %q`, reply)
	}
}

func TestDispatchGet(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	srv.Dispatch([]string{"SET", "color", "blue"}, resp.NewWriter(io.Discard))
	reply := dispatchArgs(t, srv, []string{"GET", "color"})
	if reply != "$4\r\nblue\r\n" {
		t.Fatalf(`expected "$4\r\nblue\r\n", got %q`, reply)
	}
}

func TestDispatchGetMissing(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := dispatchArgs(t, srv, []string{"GET", "nope"})
	if reply != "$-1\r\n" {
		t.Fatalf(`expected "$-1\r\n", got %q`, reply)
	}
}

func TestDispatchDel(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	srv.Dispatch([]string{"SET", "color", "blue"}, resp.NewWriter(io.Discard))
	reply := dispatchArgs(t, srv, []string{"DEL", "color"})
	if reply != ":1\r\n" {
		t.Fatalf(`expected ":1\r\n", got %q`, reply)
	}
}

func TestDispatchDelMissing(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := dispatchArgs(t, srv, []string{"DEL", "nope"})
	if reply != ":0\r\n" {
		t.Fatalf(`expected ":0\r\n", got %q`, reply)
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := dispatchArgs(t, srv, []string{"FOO", "bar"})
	if !strings.HasPrefix(reply, "-ERR") {
		t.Fatalf(`expected "-ERR..." prefix, got %q`, reply)
	}
}

func TestDispatchSetWithTTL(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := dispatchArgs(t, srv, []string{"SET", "color", "blue", "60"})
	if reply != "+OK\r\n" {
		t.Fatalf(`expected "+OK\r\n", got %q`, reply)
	}
}

func TestDispatchSetWrongArgs(t *testing.T) {
	srv, cancel := setupTest(t)
	defer cancel()

	reply := dispatchArgs(t, srv, []string{"SET"})
	if !strings.HasPrefix(reply, "-ERR") {
		t.Fatalf(`expected "-ERR..." prefix, got %q`, reply)
	}
}

// writeRESP sends a RESP Array of Bulk Strings to conn.
func writeRESP(t *testing.T, conn net.Conn, args ...string) {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("*")
	buf.WriteString(itoa(len(args)))
	buf.WriteString("\r\n")
	for _, arg := range args {
		buf.WriteString("$")
		buf.WriteString(itoa(len(arg)))
		buf.WriteString("\r\n")
		buf.WriteString(arg)
		buf.WriteString("\r\n")
	}
	conn.Write(buf.Bytes())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestFullTCPRoundTrip(t *testing.T) {
	ctx := t.Context()

	s := store.New(10, nil)
	srv := New(s, nil)

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { <-ctx.Done(); ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			srv.wg.Add(1)
			go srv.handleConnection(ctx, conn)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// --- SET via RESP ---
	writeRESP(t, conn, "SET", "color", "blue")
	r := bufio.NewReader(conn)
	got, _ := r.ReadString('\n')
	got = strings.TrimRight(got, "\r\n")
	if got != "+OK" {
		t.Fatalf(`expected "+OK", got %q`, got)
	}

	// --- GET via RESP ---
	writeRESP(t, conn, "GET", "color")
	header, _ := r.ReadString('\n')
	if header != "$4\r\n" {
		t.Fatalf(`expected "$4\r\n", got %q`, header)
	}
	body := make([]byte, 4)
	io.ReadFull(r, body)
	if string(body) != "blue" {
		t.Fatalf(`expected "blue", got %q`, string(body))
	}
	trailer := make([]byte, 2)
	io.ReadFull(r, trailer)
	if string(trailer) != "\r\n" {
		t.Fatalf(`expected "\r\n", got %q`, string(trailer))
	}
}

func TestHandleConnectionExitsOnClose(t *testing.T) {
	ctx := t.Context()

	s := store.New(10, nil)
	srv := New(s, nil)

	client, server := net.Pipe()
	srv.wg.Add(1)
	go srv.handleConnection(ctx, server)

	client.Close()
}
