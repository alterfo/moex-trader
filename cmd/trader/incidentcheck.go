package main

import (
	"context"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/incidentcheck"
)

type damageOutcome int

const (
	damageConfirmed damageOutcome = iota
	damageRejected
	damageRetry
)

const (
	incidentCheckRetryDelay = 10 * time.Minute
	incidentCheckTimeout    = 60 * time.Second
)

func (g *newsGateSignalSource) confirmDamage(ctx context.Context, feature domain.FeatureContext) damageOutcome {
	if g.checker == nil {
		return damageConfirmed
	}
	if len(feature.IncidentTitles) == 0 {
		g.logger.Printf("trader: incident short %s skipped, no headlines to check", feature.Ticker)
		return damageRejected
	}
	g.alertMu.Lock()
	retryAt := g.checkRetryAt[feature.Ticker]
	g.alertMu.Unlock()
	if g.now().Before(retryAt) {
		return damageRetry
	}
	timeout := g.checkTimeout
	if timeout <= 0 {
		timeout = incidentCheckTimeout
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := g.checker.Check(checkCtx, feature.Ticker, feature.IncidentTitles)
	if err != nil {
		g.alertMu.Lock()
		g.checkRetryAt[feature.Ticker] = g.now().Add(incidentCheckRetryDelay)
		g.alertMu.Unlock()
		g.logger.Printf("trader: incident check %s failed, no short this cycle: %v", feature.Ticker, err)
		return damageRetry
	}
	g.logger.Printf("trader: incident check %s: %s (%s)", feature.Ticker, result.Verdict, result.Reason)
	if result.Verdict == incidentcheck.Damage {
		return damageConfirmed
	}
	return damageRejected
}
