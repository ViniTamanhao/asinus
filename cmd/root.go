package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"asinus/internal/kicker"
	"asinus/internal/server"
	"asinus/internal/store"

	"github.com/spf13/cobra"
)

var (
	port    string
	webhook string
	workers int
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

	var wg sync.WaitGroup
	wg.Go(func() {
		st.StartSweeper(ctx, time.Second)
	})

	srv := server.New(st)

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
