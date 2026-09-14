package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/storage"
	"github.com/olegsidorkin/moex-trader/internal/verifier"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		dbPath   = flag.String("db", "trader.db", "path to the SQLite database")
		sinceDur = flag.Duration("since", time.Hour, "how far back each report looks")
		interval = flag.Duration("interval", time.Hour, "wait between passes; 0 runs a single pass")
		outPath  = flag.String("out", "", "write the markdown report to this file instead of stdout")
	)
	flag.Parse()

	if *dbPath == "" {
		return fmt.Errorf("db must not be empty")
	}
	if *sinceDur <= 0 {
		return fmt.Errorf("since must be positive")
	}
	if *interval < 0 {
		return fmt.Errorf("interval must not be negative")
	}

	store, err := storage.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close storage: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ver := verifier.New(store, time.Now)
	for {
		report, err := ver.Run(ctx, time.Now().Add(-*sinceDur))
		if err != nil {
			return fmt.Errorf("verify: %w", err)
		}
		if err := emit(*outPath, report.Markdown()); err != nil {
			return err
		}
		if *interval == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(*interval):
		}
	}
}

func emit(path, content string) error {
	if path == "" {
		fmt.Print(content)
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
