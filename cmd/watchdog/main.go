package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/alert/telegram"
	"github.com/olegsidorkin/moex-trader/internal/failover"
)

func main() {
	configPath := flag.String("config", "watchdog.yaml", "path to the watchdog config")
	flag.Parse()
	if err := run(*configPath); err != nil {
		log.Fatal(err)
	}
}

func run(configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	env, err := failover.LoadEnvFile(cfg.EnvFile)
	if err != nil {
		return err
	}
	witnessToken := lookupEnv(env, cfg.WitnessTokenEnv)
	if witnessToken == "" {
		return fmt.Errorf("witness token %s is not set (env file %q or process env)", cfg.WitnessTokenEnv, cfg.EnvFile)
	}
	heartbeatToken := lookupEnv(env, cfg.HeartbeatTokenEnv)
	if heartbeatToken == "" {
		return fmt.Errorf("heartbeat token %s is not set (env file %q or process env)", cfg.HeartbeatTokenEnv, cfg.EnvFile)
	}

	telegramHTTPClient, err := newProxyHTTPClient(cfg.Telegram.Proxy)
	if err != nil {
		return err
	}
	telegramClient := telegram.New(lookupEnv(env, cfg.TelegramTokenEnv), cfg.Telegram.ChatID, telegramHTTPClient)

	witness := failover.NewWitnessClient(cfg.Witness.URL, witnessToken, nil)
	peer := failover.NewPeerClient(cfg.PeerURL, heartbeatToken, nil)
	node := newWatchdog(cfg, witness, peer, telegramClient, log.Default())

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           node.routes(heartbeatToken),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("watchdog: http listening on %s", cfg.Listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runErr := make(chan error, 1)
	go func() {
		runErr <- node.run(ctx)
	}()

	var result error
	select {
	case err := <-serverErr:
		result = err
		stop()
	case err := <-runErr:
		result = err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("watchdog: http shutdown: %v", err)
	}
	return result
}

func newProxyHTTPClient(proxy string) (*http.Client, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return nil, nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse telegram proxy %q: %w", proxy, err)
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}, nil
}
