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

const auditMaxTitles = 8

func (g *newsGateSignalSource) auditCheck(feature domain.FeatureContext, result incidentcheck.Result, outcome string, checkErr error) {
	if g.audit == nil {
		return
	}
	titles := feature.IncidentTitles
	if len(titles) > auditMaxTitles {
		titles = titles[:auditMaxTitles]
	}
	rec := incidentcheck.AuditRecord{
		At:        g.now().UTC(),
		Ticker:    feature.Ticker,
		Model:     result.Model,
		Digest:    result.Digest,
		PromptSHA: result.PromptSHA,
		Titles:    append([]string(nil), titles...),
		Raw:       result.Raw,
		Verdict:   string(result.Verdict),
		Reason:    result.Reason,
		Outcome:   outcome,
	}
	if checkErr != nil {
		rec.Error = checkErr.Error()
	}
	if err := g.audit.Append(rec); err != nil {
		g.logger.Printf("trader: incident check audit write failed for %s: %v", feature.Ticker, err)
	}
}

func (g *newsGateSignalSource) confirmDamage(ctx context.Context, feature domain.FeatureContext) damageOutcome {
	if g.checker == nil {
		return damageConfirmed
	}
	if len(feature.IncidentTitles) == 0 {
		g.logger.Printf("trader: incident short %s skipped, no headlines to check", feature.Ticker)
		g.auditCheck(feature, incidentcheck.Result{}, "skipped_no_headlines", nil)
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
		g.auditCheck(feature, result, "error", err)
		return damageRetry
	}
	g.logger.Printf("trader: incident check %s: %s (%s)", feature.Ticker, result.Verdict, result.Reason)
	if result.Verdict == incidentcheck.Damage {
		g.auditCheck(feature, result, "confirmed", nil)
		return damageConfirmed
	}
	g.auditCheck(feature, result, "rejected", nil)
	return damageRejected
}
