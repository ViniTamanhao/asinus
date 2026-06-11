package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"asinus/internal/aof"
	"asinus/internal/resp"
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
	log.Printf("asinus tcp server listening on :%s", port)

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

// handleConnection reads RESP commands from conn, dispatches them, and writes RESP responses back.
// Runs its own goroutine
func (srv *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer srv.wg.Done()
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	r := resp.NewParser(conn)
	w := resp.NewWriter(bw)

	done := make(chan struct{})

	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			conn.SetReadDeadline(time.Now())
		case <-done:
		}
	}()

	for {
		args, err := r.ReadCommand()
		if err != nil {
			return
		}

		srv.Dispatch(args, w)
		bw.Flush()
	}
}

// SetReplaying controls whether AOF writes are supressed.
// Used during startup AOF replay to avoid self-deadlock.
func (srv *Server) SetReplaying(v bool) {
	srv.replaying = v
}

// Dispatch executes a parssed RESP command against the store and writes the RESP response to w.
// It also persists write commands to the AOF.
func (srv *Server) Dispatch(args []string, w *resp.Writer) {
	if len(args) == 0 {
		w.WriteError(errors.New("ERR empty command"))
		return
	}

	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "GET":
		if len(args) != 2 {
			w.WriteError(errors.New("ERR wrong number of arguments for GET"))
			return
		}
		val, ok := srv.store.Get(args[1])
		if !ok {
			w.WriteBulk(nil)
			return
		}
		w.WriteBulk([]byte(val))

	case "SET":
		if len(args) < 3 {
			w.WriteError(errors.New("ERR wrong number of arguments for SET"))
			return
		}
		key := args[1]
		value := args[2]
		var ttl time.Duration
		if len(args) >= 4 {
			sec, err := strconv.Atoi(args[3])
			if err != nil {
				w.WriteError(errors.New("ERR invalid TTL (must be integer seconds)"))
				return
			}
			if sec < 0 {
				w.WriteError(errors.New("ERR invalid TTL (must be positive))"))
				return
			}
			ttl = time.Duration(sec) * time.Second
		}
		srv.store.Set(key, value, ttl)

		if srv.aof != nil && !srv.replaying {
			if err := srv.aof.Write(resp.EncodeCommand(args)); err != nil {
				log.Printf("aof write error: %v", err)
			}
		}

		w.WriteSimpleString("OK")

	case "DEL":
		if len(args) != 2 {
			w.WriteError(errors.New("ERR wrong number of arguments for DEL"))
			return
		}
		deleted := srv.store.Delete(args[1])

		if srv.aof != nil && !srv.replaying {
			if err := srv.aof.Write(resp.EncodeCommand(args)); err != nil {
				log.Printf("aof write error: %v", err)
			}
		}

		if deleted {
			w.WriteInt(1)
		} else {
			w.WriteInt(0)
		}

	default:
		w.WriteError(fmt.Errorf("ERR unknown command %q", cmd))
	}
}
