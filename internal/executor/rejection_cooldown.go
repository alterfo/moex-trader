package executor

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const shortPositionRejectionCode = "30034"

type RejectionCooldownExecutor struct {
	inner     Executor
	now       func() time.Time
	logger    *log.Logger
	threshold int
	cooldown  time.Duration

	mu       sync.Mutex
	failures map[string]int
	until    map[string]time.Time
}

func NewRejectionCooldownExecutor(inner Executor, now func() time.Time, threshold int, cooldown time.Duration, logger *log.Logger) *RejectionCooldownExecutor {
	if now == nil {
		now = time.Now
	}
	if threshold < 1 {
		threshold = 1
	}
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	if logger == nil {
		logger = log.Default()
	}
	return &RejectionCooldownExecutor{
		inner:     inner,
		now:       now,
		logger:    logger,
		threshold: threshold,
		cooldown:  cooldown,
		failures:  make(map[string]int),
		until:     make(map[string]time.Time),
	}
}

func (e *RejectionCooldownExecutor) Inner() Executor {
	return e.inner
}

func (e *RejectionCooldownExecutor) Execute(ctx context.Context, signal domain.TradeSignal, price decimal.Decimal) (Fill, error) {
	if signal.Action != domain.ActionBuy && signal.Action != domain.ActionSell {
		return e.inner.Execute(ctx, signal, price)
	}
	key := string(signal.Action) + "|" + signal.Ticker
	if until, ok := e.suppressedUntil(key); ok && e.now().Before(until) {
		e.logger.Printf("executor: skipping %s %s until %s after repeated %s rejections", signal.Action, signal.Ticker, until.Format(time.RFC3339), shortPositionRejectionCode)
		return suppressedFill(signal, price, e.now()), nil
	}
	fill, err := e.inner.Execute(ctx, signal, price)
	if err != nil {
		if isShortPositionRejection(err) {
			e.recordFailure(key)
		}
		return fill, err
	}
	e.clear(key)
	return fill, nil
}

func (e *RejectionCooldownExecutor) suppressedUntil(key string) (time.Time, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	until, ok := e.until[key]
	return until, ok
}

func (e *RejectionCooldownExecutor) recordFailure(key string) {
	e.mu.Lock()
	e.failures[key]++
	reached := e.failures[key] >= e.threshold
	if reached {
		e.until[key] = e.now().Add(e.cooldown)
		e.failures[key] = 0
	}
	e.mu.Unlock()
	if reached {
		e.logger.Printf("executor: %s rejections reached threshold, cooling down for %s", shortPositionRejectionCode, e.cooldown)
	}
}

func (e *RejectionCooldownExecutor) clear(key string) {
	e.mu.Lock()
	delete(e.failures, key)
	delete(e.until, key)
	e.mu.Unlock()
}

func isShortPositionRejection(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), shortPositionRejectionCode)
}

func suppressedFill(signal domain.TradeSignal, price decimal.Decimal, at time.Time) Fill {
	return Fill{
		ID:         "suppressed-" + signal.Ticker,
		Ticker:     signal.Ticker,
		Action:     domain.ActionHold,
		Lots:       0,
		Price:      price,
		ExecutedAt: at,
	}
}
