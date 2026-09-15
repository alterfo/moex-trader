package backtest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

// RetryingSignalSource retries the inner source on context deadline (the LLM
// host being slow or hung) with exponential backoff. After maxConsecutive
// consecutive timeouts it opens a circuit: subsequent Generate calls return
// HOLD without touching the host, so a whole batch does not burn hours against
// an unreachable model.
type RetryingSignalSource struct {
	inner                  SignalSource
	attempts               int
	baseBackoff            time.Duration
	maxBackoff             time.Duration
	maxConsecutiveTimeouts int
	logger                 *log.Logger

	consecutiveTimeouts int
}

func NewRetryingSignalSource(inner SignalSource, attempts int, baseBackoff, maxBackoff time.Duration, maxConsecutiveTimeouts int, logger *log.Logger) *RetryingSignalSource {
	if attempts <= 0 {
		attempts = 3
	}
	if baseBackoff <= 0 {
		baseBackoff = 5 * time.Second
	}
	if maxBackoff < baseBackoff {
		maxBackoff = baseBackoff
	}
	if maxConsecutiveTimeouts <= 0 {
		maxConsecutiveTimeouts = 3
	}
	if logger == nil {
		logger = log.Default()
	}
	return &RetryingSignalSource{
		inner:                  inner,
		attempts:               attempts,
		baseBackoff:            baseBackoff,
		maxBackoff:             maxBackoff,
		maxConsecutiveTimeouts: maxConsecutiveTimeouts,
		logger:                 logger,
	}
}

func (s *RetryingSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	if s.consecutiveTimeouts >= s.maxConsecutiveTimeouts {
		s.logger.Printf("backtest: circuit open (%d consecutive LLM timeouts), %s %s -> HOLD",
			s.consecutiveTimeouts, feature.Ticker, feature.GeneratedAt.Format("2006-01-02"))
		return holdSignal(feature), nil
	}

	var lastErr error
	backoff := s.baseBackoff
	for attempt := 1; attempt <= s.attempts; attempt++ {
		if attempt > 1 {
			s.logger.Printf("backtest: retrying %s %s (attempt %d/%d) after %s",
				feature.Ticker, feature.GeneratedAt.Format("2006-01-02"), attempt, s.attempts, backoff)
			if err := sleepCtx(ctx, backoff); err != nil {
				return domain.TradeSignal{}, err
			}
		}
		signal, err := s.inner.Generate(ctx, feature)
		if err == nil {
			s.consecutiveTimeouts = 0
			return signal, nil
		}
		lastErr = err
		if !errors.Is(err, context.DeadlineExceeded) {
			return domain.TradeSignal{}, err
		}
		s.consecutiveTimeouts++
		if s.consecutiveTimeouts >= s.maxConsecutiveTimeouts {
			s.logger.Printf("backtest: circuit OPEN for %s %s after %d consecutive LLM timeouts",
				feature.Ticker, feature.GeneratedAt.Format("2006-01-02"), s.consecutiveTimeouts)
			break
		}
		backoff = nextBackoff(backoff*2, s.maxBackoff)
	}

	return domain.TradeSignal{}, fmt.Errorf("backtest: llm unreachable for %s on %s: %w", feature.Ticker, feature.GeneratedAt.Format("2006-01-02"), lastErr)
}

func holdSignal(feature domain.FeatureContext) domain.TradeSignal {
	return domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		TargetLots:  0,
		Reasoning:   "LLM host unreachable (circuit breaker)",
		GeneratedAt: feature.GeneratedAt,
		HoldReason:  domain.HoldReasonTimeout,
	}
}

func sleepCtx(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func nextBackoff(proposed, max time.Duration) time.Duration {
	if proposed > max {
		return max
	}
	return proposed
}
