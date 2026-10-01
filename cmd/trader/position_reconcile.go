package main

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/risk"
)

const (
	positionReconcileTTL   = time.Minute
	rejectedOrderThreshold = 3
	rejectedOrderCooldown  = 15 * time.Minute
)

type portfolioLotsSource interface {
	PositionLots(ctx context.Context, tickers []string) (map[string]int, error)
}

type reconcilingPositionReader struct {
	source   portfolioLotsSource
	fallback risk.PositionReader
	tickers  []string
	ttl      time.Duration
	now      func() time.Time
	logger   *log.Logger

	mu          sync.Mutex
	cached      map[string]int
	attemptedAt time.Time
	lastErr     error
}

func newReconcilingPositionReader(source portfolioLotsSource, fallback risk.PositionReader, tickers []string, ttl time.Duration, now func() time.Time, logger *log.Logger) *reconcilingPositionReader {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = positionReconcileTTL
	}
	if logger == nil {
		logger = log.Default()
	}
	return &reconcilingPositionReader{
		source:   source,
		fallback: fallback,
		tickers:  append([]string(nil), tickers...),
		ttl:      ttl,
		now:      now,
		logger:   logger,
	}
}

func (r *reconcilingPositionReader) CurrentLots(ctx context.Context, ticker string) (int, error) {
	lots, ok := r.snapshot(ctx)
	if ok {
		return lots[strings.ToUpper(strings.TrimSpace(ticker))], nil
	}
	return r.fallback.CurrentLots(ctx, ticker)
}

// NetExposure reports signed net position notional from the local fill ledger
// (the broker gives lots, not prices, so average-cost pricing is the only
// consistent exposure read here). It is the same ledger the fallback
// CurrentLots uses, so the two never disagree.
func (r *reconcilingPositionReader) NetExposure(ctx context.Context) (decimal.Decimal, error) {
	if reader, ok := r.fallback.(risk.NetExposureReader); ok {
		return reader.NetExposure(ctx)
	}
	return decimal.Zero, nil
}

// ExposureByTicker delegates to the fallback's per-ticker book. Preferred by
// the risk gate over NetExposure, which would cancel longs against shorts.
func (r *reconcilingPositionReader) ExposureByTicker(ctx context.Context) (map[string]decimal.Decimal, error) {
	if reader, ok := r.fallback.(risk.ExposureReader); ok {
		return reader.ExposureByTicker(ctx)
	}
	return map[string]decimal.Decimal{}, nil
}

func (r *reconcilingPositionReader) snapshot(ctx context.Context) (map[string]int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	if !r.attemptedAt.IsZero() && now.Sub(r.attemptedAt) < r.ttl {
		if r.cached != nil {
			return r.cached, true
		}
		return nil, false
	}
	r.attemptedAt = now

	lots, err := r.source.PositionLots(ctx, r.tickers)
	if err != nil {
		r.cached = nil
		r.lastErr = err
		r.logger.Printf("trader: broker position reconcile failed, falling back to local fills: %v", err)
		return nil, false
	}
	r.cached = lots
	r.lastErr = nil
	return lots, true
}
