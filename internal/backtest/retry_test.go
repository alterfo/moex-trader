package backtest

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type timeoutSource struct {
	calls int
}

func (t *timeoutSource) Generate(_ context.Context, _ domain.FeatureContext) (domain.TradeSignal, error) {
	t.calls++
	return domain.TradeSignal{}, context.DeadlineExceeded
}

func TestRetrying_CircuitBreaker(t *testing.T) {
	maxConsec := 3
	src := NewRetryingSignalSource(
		&timeoutSource{},
		3,                  // attempts per call
		1*time.Millisecond, // base backoff (fast for test)
		10*time.Millisecond,
		maxConsec,
		log.Default(),
	)

	feat := domain.FeatureContext{Ticker: "TEST", GeneratedAt: time.Now()}

	_, err := src.Generate(context.Background(), feat)
	if err == nil {
		t.Fatal("first call should fail as circuit trips")
	}

	for i := 0; i < 30; i++ {
		signal, err := src.Generate(context.Background(), feat)
		if err != nil {
			t.Fatalf("iter %d: expected nil error once circuit is open, got err=%v", i, err)
		}
		if signal.Action != domain.ActionHold {
			t.Fatalf("iter %d: expected HOLD from circuit, got %s", i, signal.Action)
		}
	}
}
