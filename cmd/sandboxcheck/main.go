package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	brokertinkoff "github.com/olegsidorkin/moex-trader/internal/broker/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config YAML")
	ticker := flag.String("ticker", "", "ticker to check; empty checks every configured ticker")
	placeOrder := flag.Bool("place-order", false, "place a market buy and sell for one lot (requires -ticker and an open trading session)")
	timeout := flag.Duration("timeout", 180*time.Second, "overall timeout")
	flag.Parse()

	if err := run(*configPath, *ticker, *placeOrder, *timeout); err != nil {
		log.Fatal(err)
	}
}

func run(configPath, ticker string, placeOrder bool, timeout time.Duration) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if cfg.Tinkoff.Token == "" {
		return fmt.Errorf("MOEX_TRADER_TINKOFF_TOKEN is not set (env or .env)")
	}
	if !cfg.Tinkoff.Sandbox {
		return fmt.Errorf("tinkoff.sandbox must be true for the sandbox check")
	}
	if placeOrder && ticker == "" {
		return fmt.Errorf("-ticker is required with -place-order")
	}
	tickers := cfg.Tickers
	if ticker != "" {
		tickers = []string{ticker}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := tinkoff.New(ctx, tinkoff.Config{
		Endpoint: cfg.Tinkoff.Endpoint,
		Token:    cfg.Tinkoff.Token,
		AppName:  "moex-trader-sandboxcheck",
	})
	if err != nil {
		return fmt.Errorf("dial tinkoff sandbox: %w", err)
	}
	defer client.Close()

	accounts, err := client.SandboxAccounts(ctx)
	if err != nil {
		return fmt.Errorf("sandbox accounts: %w", err)
	}
	log.Printf("sandbox accounts: %d", len(accounts))

	sandbox, err := brokertinkoff.NewSandbox(client, brokertinkoff.Config{
		AccountID: cfg.Tinkoff.AccountID,
		PayIn:     cfg.Tinkoff.PayIn,
		Now:       time.Now,
	})
	if err != nil {
		return err
	}
	accountID, err := sandbox.EnsureAccount(ctx)
	if err != nil {
		return fmt.Errorf("ensure sandbox account: %w", err)
	}
	log.Printf("sandbox account ready: %s", accountID)

	snapshot, err := sandbox.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("portfolio snapshot: %w", err)
	}
	log.Printf("risk snapshot: deposit=%s day_start=%s equity=%s", snapshot.Deposit, snapshot.DayStartEquity, snapshot.CurrentEquity)

	orders, err := client.GetSandboxOrders(ctx, accountID)
	if err != nil {
		return fmt.Errorf("sandbox orders: %w", err)
	}
	log.Printf("open sandbox orders: %d", len(orders))

	available := 0
	resolved := 0
	for _, current := range tickers {
		uid, err := client.ResolveInstrumentUID(ctx, current)
		if err != nil {
			log.Printf("%-12s resolve failed: %v", current, err)
			continue
		}
		resolved++
		status, err := client.TradingStatus(ctx, uid)
		if err != nil {
			log.Printf("%-12s uid=%s status failed: %v", current, uid, err)
			continue
		}
		if status.GetMarketOrderAvailableFlag() {
			available++
		}
		log.Printf("%-12s uid=%s status=%s limit=%v market=%v api=%v",
			current, uid, status.GetTradingStatus(), status.GetLimitOrderAvailableFlag(),
			status.GetMarketOrderAvailableFlag(), status.GetApiTradeAvailableFlag())
	}
	log.Printf("tickers checked: %d, resolved: %d, market orders available: %d", len(tickers), resolved, available)

	if !placeOrder {
		log.Printf("connectivity check passed; pass -ticker <TICKER> -place-order during the MOEX trading session to verify order execution")
		return nil
	}
	return placeOrderCheck(ctx, cfg, client, sandbox, accountID, ticker)
}

func placeOrderCheck(ctx context.Context, cfg *config.Config, client *tinkoff.Client, sandbox *brokertinkoff.Sandbox, accountID, ticker string) error {
	uid, err := client.ResolveInstrumentUID(ctx, ticker)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", ticker, err)
	}
	status, err := client.TradingStatus(ctx, uid)
	if err != nil {
		return fmt.Errorf("trading status %s: %w", ticker, err)
	}
	if !status.GetMarketOrderAvailableFlag() {
		return fmt.Errorf("%s is not available for market orders right now (%s); retry during the MOEX trading session", ticker, status.GetTradingStatus())
	}

	lastPrice, err := client.LastPrice(ctx, uid)
	if err != nil {
		return fmt.Errorf("last price %s: %w", ticker, err)
	}
	log.Printf("%s last price: %s", ticker, lastPrice.Price)

	store, cleanupStore, err := openTempStore()
	if err != nil {
		return err
	}
	defer cleanupStore()

	exec, err := executor.NewLiveExecutor(sandbox, executor.LiveConfig{
		AccountID:           accountID,
		ResolveInstrumentID: sandbox.ResolveInstrumentID,
		OrderType:           pb.OrderType_ORDER_TYPE_MARKET,
		Store:               store,
		Now:                 time.Now,
		CommissionRate:      cfg.Commission.Rate,
	})
	if err != nil {
		return err
	}

	buyFill, err := exec.Execute(ctx, smokeSignal(ticker, domain.ActionBuy), lastPrice.Price)
	if err != nil {
		return fmt.Errorf("buy order: %w", err)
	}
	fmt.Printf("BUY fill: id=%s lots=%d price=%s commission=%s\n", buyFill.ID, buyFill.Lots, buyFill.Price, buyFill.Commission)

	sellFill, err := exec.Execute(ctx, smokeSignal(ticker, domain.ActionSell), lastPrice.Price)
	if err != nil {
		return fmt.Errorf("sell order: %w", err)
	}
	fmt.Printf("SELL fill: id=%s lots=%d price=%s commission=%s\n", sellFill.ID, sellFill.Lots, sellFill.Price, sellFill.Commission)

	after, err := sandbox.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("portfolio snapshot after orders: %w", err)
	}
	log.Printf("risk snapshot after orders: equity=%s", after.CurrentEquity)
	return nil
}

func smokeSignal(ticker string, action domain.Action) domain.TradeSignal {
	return domain.TradeSignal{
		Ticker:      ticker,
		Action:      action,
		Confidence:  decimal.RequireFromString("0.9"),
		TargetLots:  1,
		Reasoning:   "sandboxcheck",
		GeneratedAt: time.Now(),
	}
}

func openTempStore() (*storage.Store, func(), error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("moex-sandboxcheck-%d.db", time.Now().UnixNano()))
	store, err := storage.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open temp storage: %w", err)
	}
	cleanup := func() {
		if err := store.Close(); err != nil {
			log.Printf("close temp storage: %v", err)
		}
		_ = os.Remove(path)
	}
	return store, cleanup, nil
}
