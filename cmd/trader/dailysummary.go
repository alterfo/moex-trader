package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/dailysummary"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

// dailySummaryConfig wires the nightly evening digest. It runs inside the
// lease-holding trader only, so the standby never publishes a duplicate.
type dailySummaryConfig struct {
	store   *storage.Store
	history dailysummary.CandleSource
	tickers []string
	alerter signalFailureAlerter
	fire    time.Duration
	now     func() time.Time
	logger  *log.Logger
}

// runDailySummary fires the evening digest once per MSK trading day at the
// configured time. Weekends and exchange holidays are detected via the IMOEX
// candle for the current day and skipped silently.
func runDailySummary(ctx context.Context, cfg dailySummaryConfig) {
	if cfg.alerter == nil || cfg.history == nil || cfg.store == nil || cfg.logger == nil {
		return
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	loc := dailysummary.MSKLocation()
	for {
		next := dailysummary.NextFire(cfg.now(), cfg.fire, loc)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := sendDailySummary(ctx, cfg, loc); err != nil {
				cfg.logger.Printf("dailysummary: %v", err)
			}
		}
	}
}

func sendDailySummary(ctx context.Context, cfg dailySummaryConfig, loc *time.Location) error {
	now := cfg.now()

	closes, err := dailysummary.DayCloses(ctx, cfg.history, cfg.tickers, now.In(loc))
	if err != nil {
		return fmt.Errorf("fetch daily closes: %w", err)
	}
	imoex := closes["IMOEX"]
	anchor, isToday := dailysummary.AnchorDay(imoex, now, loc)
	if !isToday {
		cfg.logger.Printf("dailysummary: %s is not a trading day (latest IMOEX candle %s), skipping",
			now.In(loc).Format("2006-01-02"), anchor)
		return nil
	}

	events, err := cfg.store.ListAllAuditEvents(ctx)
	if err != nil {
		return fmt.Errorf("load fills: %w", err)
	}
	bot := dailysummary.Account(dailysummary.FillsFromAuditEvents(events), now, loc, closes)
	market := dailysummary.MarketDirection(imoex, closes, cfg.tickers, anchor)

	text := dailysummary.Message(now.In(loc), bot, market, loc)
	if err := cfg.alerter.Send(ctx, text); err != nil {
		return fmt.Errorf("send daily summary: %w", err)
	}
	cfg.logger.Printf("dailysummary: sent evening digest (trades=%d day=%s₽ realized=%s₽ MTM=%s₽ IMOEX=%s up=%d/%d)",
		bot.Trades, bot.PnL().StringFixed(2), bot.Realized.StringFixed(2), bot.MTMChange().StringFixed(2),
		market.IndexPct.StringFixed(2), market.UpTickers, market.TotalTickers)
	return nil
}
