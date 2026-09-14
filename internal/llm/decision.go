package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const DefaultMaxAttempts = 3

const StageDecisionFailure = "llm"

type ChatClient interface {
	Chat(ctx context.Context, messages []Message) (ChatResponse, error)
}

type AuditSink interface {
	InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error
}

type DecisionEngine struct {
	client      ChatClient
	prompt      *PromptBuilder
	audit       AuditSink
	maxAttempts int
	now         func() time.Time
	logger      *log.Logger
}

func NewDecisionEngine(client ChatClient, prompt *PromptBuilder, audit AuditSink, now func() time.Time) *DecisionEngine {
	if prompt == nil {
		prompt = NewPromptBuilder()
	}
	if now == nil {
		now = time.Now
	}
	return &DecisionEngine{
		client:      client,
		prompt:      prompt,
		audit:       audit,
		maxAttempts: DefaultMaxAttempts,
		now:         now,
		logger:      log.Default(),
	}
}

func (e *DecisionEngine) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	if e.client == nil {
		return domain.TradeSignal{}, fmt.Errorf("llm: chat client is required")
	}

	messages := []Message{
		{Role: "system", Content: e.prompt.SystemPrompt(feature)},
		{Role: "user", Content: "Сформируй торговый сигнал для текущего инструмента."},
	}

	var lastErr error
	for attempt := 1; attempt <= e.maxAttempts; attempt++ {
		response, err := e.client.Chat(ctx, messages)
		if err != nil {
			lastErr = err
			continue
		}

		signal, err := ParseTradeSignal(response.Message.Content)
		if err != nil {
			lastErr = err
			continue
		}
		if signal.GeneratedAt.IsZero() {
			signal.GeneratedAt = e.now()
		}
		return signal, nil
	}

	fallback := domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		Confidence:  decimal.Zero,
		TargetLots:  0,
		Reasoning:   "LLM response was invalid or unavailable after retries",
		GeneratedAt: e.now(),
	}
	e.recordFailure(ctx, feature.Ticker, lastErr)
	return fallback, nil
}

func ParseTradeSignal(content string) (domain.TradeSignal, error) {
	var signal domain.TradeSignal
	content = strings.TrimSpace(content)
	if content == "" {
		return signal, fmt.Errorf("parse trade signal: empty response")
	}
	if err := json.Unmarshal([]byte(content), &signal); err != nil {
		return signal, fmt.Errorf("parse trade signal JSON: %w", err)
	}
	if err := signal.Validate(); err != nil {
		return signal, fmt.Errorf("validate trade signal: %w", err)
	}
	return signal, nil
}

func (e *DecisionEngine) recordFailure(ctx context.Context, ticker string, cause error) {
	if e.audit == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"error":       errorText(cause),
		"attempts":    e.maxAttempts,
		"fallback":    "HOLD",
		"target_lots": 0,
	})
	event := domain.AuditEvent{
		ID:        uuid.NewString(),
		Ticker:    ticker,
		Stage:     StageDecisionFailure,
		Payload:   string(payload),
		CreatedAt: e.now(),
	}
	if err := e.audit.InsertAuditEvent(ctx, event); err != nil {
		e.logger.Printf("llm: record decision failure for %s: %v", ticker, err)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
