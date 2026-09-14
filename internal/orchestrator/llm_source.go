package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/llm"
)

const DefaultSignalTimeout = 10 * time.Second

// LLMSignalSource adapts the llm.DecisionEngine to the orchestrator SignalSource
// interface, applying a per-call timeout and logging the LLM latency.
type LLMSignalSource struct {
	engine  *llm.DecisionEngine
	timeout time.Duration
	logger  *log.Logger
}

func NewLLMSignalSource(engine *llm.DecisionEngine, timeout time.Duration, logger *log.Logger) *LLMSignalSource {
	if timeout <= 0 {
		timeout = DefaultSignalTimeout
	}
	if logger == nil {
		logger = log.Default()
	}
	return &LLMSignalSource{
		engine:  engine,
		timeout: timeout,
		logger:  logger,
	}
}

func (s *LLMSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	if s.engine == nil {
		return domain.TradeSignal{}, fmt.Errorf("orchestrator: llm engine is required")
	}

	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	started := time.Now()
	signal, err := s.engine.Generate(callCtx, feature)
	s.logger.Printf("orchestrator: llm latency for %s: %s", feature.Ticker, time.Since(started))
	return signal, err
}
