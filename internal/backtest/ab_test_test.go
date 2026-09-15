package backtest

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestReversalRuleSource_TradesOnThreshold(t *testing.T) {
	src := &ReversalRuleSource{Threshold: decimal.NewFromFloat(0.5), MaxLots: 1}
	ctx := context.Background()

	sig, err := src.Generate(ctx, domain.FeatureContext{Ticker: "SBER", Reversal1d: decimal.NewFromFloat(0.7)})
	if err != nil {
		t.Fatal(err)
	}
	if sig.Action != domain.ActionSell || sig.TargetLots != 1 {
		t.Fatalf("expected SELL above +threshold, got %v lots=%d", sig.Action, sig.TargetLots)
	}

	sig, err = src.Generate(ctx, domain.FeatureContext{Ticker: "SBER", Reversal1d: decimal.NewFromFloat(-0.6)})
	if err != nil {
		t.Fatal(err)
	}
	if sig.Action != domain.ActionBuy || sig.TargetLots != 1 {
		t.Fatalf("expected BUY below -threshold, got %v lots=%d", sig.Action, sig.TargetLots)
	}

	sig, err = src.Generate(ctx, domain.FeatureContext{Ticker: "SBER", Reversal1d: decimal.NewFromFloat(0.3)})
	if err != nil {
		t.Fatal(err)
	}
	if sig.Action != domain.ActionHold || sig.HoldReason != domain.HoldReasonModel {
		t.Fatalf("expected HOLD inside threshold, got %v reason=%q", sig.Action, sig.HoldReason)
	}

	sig, err = src.Generate(ctx, domain.FeatureContext{Ticker: "SBER", Reversal1d: decimal.NewFromFloat(0.5)})
	if err != nil {
		t.Fatal(err)
	}
	if sig.Action != domain.ActionSell {
		t.Fatalf("expected SELL at exact +threshold (>=), got %v", sig.Action)
	}
}

type fixedSignalSource struct {
	signal *domain.TradeSignal
	err    error
}

func (s *fixedSignalSource) Generate(_ context.Context, _ domain.FeatureContext) (domain.TradeSignal, error) {
	if s.err != nil {
		return domain.TradeSignal{}, s.err
	}
	return *s.signal, nil
}

func TestConfidenceGateSource_DemotesLowConfidence(t *testing.T) {
	inner := &fixedSignalSource{}
	gate := &ConfidenceGateSource{Inner: inner, MinConfidence: decimal.NewFromFloat(0.7)}
	ctx := context.Background()

	inner.signal = &domain.TradeSignal{
		Ticker: "SBER", Action: domain.ActionBuy, Confidence: decimal.NewFromFloat(0.95), TargetLots: 1,
	}
	sig, err := gate.Generate(ctx, domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	if sig.Action != domain.ActionBuy {
		t.Fatalf("expected BUY kept above threshold, got %v", sig.Action)
	}

	inner.signal = &domain.TradeSignal{
		Ticker: "SBER", Action: domain.ActionSell, Confidence: decimal.NewFromFloat(0.5), TargetLots: 1,
	}
	sig, err = gate.Generate(ctx, domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	if sig.Action != domain.ActionHold || sig.TargetLots != 0 || sig.HoldReason != domain.HoldReasonConfidence {
		t.Fatalf("expected HOLD demoted with reason=%q, got %v lots=%d reason=%q",
			domain.HoldReasonConfidence, sig.Action, sig.TargetLots, sig.HoldReason)
	}
}

func TestConfidenceGateSource_PassesThroughErrors(t *testing.T) {
	inner := &fixedSignalSource{err: context.DeadlineExceeded}
	gate := &ConfidenceGateSource{Inner: inner, MinConfidence: decimal.NewFromFloat(0.7)}
	_, err := gate.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != context.DeadlineExceeded {
		t.Fatalf("expected underlying error to propagate, got %v", err)
	}
}
