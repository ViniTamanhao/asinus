package server

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"asinus/internal/aof"
	"asinus/internal/store"
)

// Server holds a reference to the key-value store and manages TCP connections.
type Server struct {
	store     *store.Store
	aof       *aof.AOF // nil if persistence is disabled
	listener  net.Listener
	wg        sync.WaitGroup
	replaying bool
}

// New creates a new Server backed by the given Store.
func New(s *store.Store, a *aof.AOF) *Server {
	return &Server{store: s, aof: a}
}

// Start begins listening on the given TCP port and accepts connections in a loop.
// Each client gets its own goroutine.
func (srv *Server) Start(ctx context.Context, port string) error {
	var err error
	srv.listener, err = net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	log.Printf("kickback tcp server listening on :%s", port)

	go func() {
		<-ctx.Done()
		srv.listener.Close()
	}()

	if srv.aof != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return 
				case <-ticker.C:
					if err := srv.aof.Rewrite(srv.store.Dump); err != nil {
						log.Printf("aof rewrite error: %v", err)
					}
				}
			}
		}()
	}

	for {
		conn, err := srv.listener.Accept()
		if err != nil {
			break
		}
		srv.wg.Add(1)
		go srv.handleConnection(ctx, conn)
	}

	srv.wg.Wait()
	return ctx.Err()
}

// handleConnection is run in its own goroutine per client.
// It sends a welcome message and then closes the connection.
func (srv *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer srv.wg.Done()
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			// Unblock the scanner by making any pending Read return immediately.
			conn.SetReadDeadline(time.Now())
		case <-done:
		}
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		reply := srv.Dispatch(line)
		fmt.Fprintln(conn, reply)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("read error: %v", err)
	}
}

// SetReplaying controls whether AOF writes are supressed.
// Used during startup AOF replay to avoid self-deadlock.
func (srv *Server) SetReplaying(v bool) {
	srv.replaying = v
}

// Dispatch parses a single text command and executes it against the store;
// Supported commands: GET, SET, DEL.
func (srv *Server) Dispatch(line string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "-ERR empty command"
	}

	cmd := strings.ToUpper(parts[0])
	switch cmd {
	case "GET":
		if len(parts) != 2 {
			return "-ERR wrong number of arguments for GET"
		}
		val, ok := srv.store.Get(parts[1])
		if !ok {
			return "-ERR not found"
		}
		return "+" + val
	case "SET":
		if len(parts) < 3 {
			return "-ERR wrong number of arguments for SET"
		}
		key := parts[1]
		value := parts[2]
		var ttl time.Duration
		if len(parts) >= 4 {
			sec, err := strconv.Atoi(parts[3])
			if err != nil {
				return "-ERR invalid TTL (must be integer seconds)"
			}
			if sec < 0 {
				return "-ERR invalid TTL (must be positive)"
			}
			ttl = time.Duration(sec) * time.Second
		}
		srv.store.Set(key, value, ttl)

		if srv.aof != nil && !srv.replaying {
			if err := srv.aof.Write(line); err != nil {
				log.Printf("aof write error: %v", err)
			}
		}

		return "+OK"
	case "DEL":
		if len(parts) != 2 {
			return "-ERR wrong number of arguments for DEL"
		}
		srv.store.Delete(parts[1])

		if srv.aof != nil && !srv.replaying {
			if err := srv.aof.Write(line); err != nil {
				log.Printf("aof write error: %v", err)
			}
		}

		return "+OK"

	default:
		return fmt.Sprintf("-ERR unknown command %q", cmd)
	}
}
