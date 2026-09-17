package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type sequenceSignalSource struct {
	next func(ticker string) (domain.TradeSignal, error)
}

func (s *sequenceSignalSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	return s.next(feature.Ticker)
}

func signal(action domain.Action, lots int) domain.TradeSignal {
	return domain.TradeSignal{Ticker: "SBER", Action: action, TargetLots: lots}
}

func TestSignalHysteresisFirstPollCommitsImmediately(t *testing.T) {
	source := &sequenceSignalSource{next: func(string) (domain.TradeSignal, error) {
		return signal(domain.ActionBuy, 5), nil
	}}
	h := NewSignalHysteresisSource(source, DefaultSignalHysteresisPolls)

	got, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != domain.ActionBuy || got.TargetLots != 5 {
		t.Fatalf("first poll should commit immediately, got %v lots=%d", got.Action, got.TargetLots)
	}
}

func TestSignalHysteresisRequiresTwoConsecutivePollsToSwitch(t *testing.T) {
	sequence := []domain.TradeSignal{
		signal(domain.ActionBuy, 5),
		signal(domain.ActionBuy, 5),
		signal(domain.ActionSell, 5),
		signal(domain.ActionSell, 5),
	}
	i := 0
	source := &sequenceSignalSource{next: func(string) (domain.TradeSignal, error) {
		sig := sequence[i]
		i++
		return sig, nil
	}}
	h := NewSignalHysteresisSource(source, DefaultSignalHysteresisPolls)

	got1, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	got2, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	got3, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	got4, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}

	if got1.Action != domain.ActionBuy || got2.Action != domain.ActionBuy {
		t.Fatalf("first two BUY polls should commit BUY, got %v, %v", got1.Action, got2.Action)
	}
	if got3.Action != domain.ActionBuy {
		t.Fatalf("first SELL poll should be suppressed to BUY, got %v", got3.Action)
	}
	if got4.Action != domain.ActionSell {
		t.Fatalf("second consecutive SELL poll should switch to SELL, got %v", got4.Action)
	}
}

func TestSignalHysteresisInterleavedPollsDoNotSwitch(t *testing.T) {
	sequence := []domain.TradeSignal{
		signal(domain.ActionBuy, 5),
		signal(domain.ActionSell, 5),
		signal(domain.ActionHold, 0),
		signal(domain.ActionSell, 5),
	}
	i := 0
	source := &sequenceSignalSource{next: func(string) (domain.TradeSignal, error) {
		sig := sequence[i]
		i++
		return sig, nil
	}}
	h := NewSignalHysteresisSource(source, DefaultSignalHysteresisPolls)

	for idx := 0; idx < len(sequence); idx++ {
		got, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Action != domain.ActionBuy {
			t.Fatalf("poll %d: SELL/HOLD were never consecutive, want BUY, got %v", idx, got.Action)
		}
	}
}

func TestSignalHysteresisRefreshSameBandKeepsLatestSignal(t *testing.T) {
	sequence := []domain.TradeSignal{
		signal(domain.ActionBuy, 5),
		signal(domain.ActionBuy, 7),
	}
	i := 0
	source := &sequenceSignalSource{next: func(string) (domain.TradeSignal, error) {
		sig := sequence[i]
		i++
		return sig, nil
	}}
	h := NewSignalHysteresisSource(source, DefaultSignalHysteresisPolls)

	_, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != domain.ActionBuy || got.TargetLots != 7 {
		t.Fatalf("same-band refresh should keep latest target lots, got %v lots=%d", got.Action, got.TargetLots)
	}
}

func TestSignalHysteresisErrorResetsState(t *testing.T) {
	boom := errors.New("signal failure")
	calls := 0
	source := &sequenceSignalSource{next: func(string) (domain.TradeSignal, error) {
		calls++
		if calls == 1 {
			return signal(domain.ActionBuy, 5), nil
		}
		if calls == 2 {
			return domain.TradeSignal{}, boom
		}
		return signal(domain.ActionSell, 5), nil
	}}
	h := NewSignalHysteresisSource(source, DefaultSignalHysteresisPolls)

	if _, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"}); !errors.Is(err, boom) {
		t.Fatalf("expected error to propagate, got %v", err)
	}
	got, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != domain.ActionSell {
		t.Fatalf("after an error the state should reset and the next poll commit immediately, got %v", got.Action)
	}
}

func TestSignalHysteresisSinglePollDisablesHysteresis(t *testing.T) {
	sequence := []domain.TradeSignal{
		signal(domain.ActionBuy, 5),
		signal(domain.ActionSell, 5),
	}
	i := 0
	source := &sequenceSignalSource{next: func(string) (domain.TradeSignal, error) {
		sig := sequence[i]
		i++
		return sig, nil
	}}
	h := NewSignalHysteresisSource(source, 1)

	_, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != domain.ActionSell {
		t.Fatalf("single-poll hysteresis should switch immediately, got %v", got.Action)
	}
}
