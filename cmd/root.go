package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"asinus/internal/aof"
	"asinus/internal/kicker"
	"asinus/internal/server"
	"asinus/internal/store"

	"github.com/spf13/cobra"
)

var (
	port    string
	webhook string
	workers int
	aofPath string
)

var rootCmd = &cobra.Command{
	Use:   "asinus",
	Short: "KickBack - an in-memory key-value store with TTL webhooks",
	Run:   run,
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&port, "port", "p", "6379", "port to listen on")
	rootCmd.PersistentFlags().StringVarP(&webhook, "webhook", "w", "", "webhook to call on expire")
	rootCmd.PersistentFlags().IntVarP(&workers, "workers", "j", 5, "number of workers")
	rootCmd.PersistentFlags().StringVarP(&aofPath, "aof", "a", "", "AOF file path for persistence (empty = disabled)")
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var onExpire func(key, value string)
	if webhook != "" {
		k := kicker.New(webhook, workers)
		k.Start(ctx)
		onExpire = k.Fire
		defer k.Wait()
	}

	st := store.New(onExpire)

	var a *aof.AOF
	if aofPath != "" {
		var err error
		a, err = aof.New(aofPath)
		if err != nil {
			log.Fatalf("failed to open AOF: %v", err)
		}
		defer a.Close()
	}

	srv := server.New(st, a)

	if a != nil {
		if err := a.Read(func(line string) {
			srv.Dispatch(line)
		}); err != nil {
			log.Printf("aof replay error: %v", err)
		}
		log.Printf("replayed AOF from %s", aofPath)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		st.StartSweeper(ctx, time.Second)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received %v, shutting down...", sig)
		cancel()
	}()

	if err := srv.Start(ctx, port); err != nil {
		log.Printf("server exited: %v", err)
	}

	wg.Wait()
}
