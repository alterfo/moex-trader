package llm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/shopspring/decimal"
)

type scriptedChatClient struct {
	responses []ChatResponse
	errs      []error
	calls     int
}

func (c *scriptedChatClient) Chat(ctx context.Context, messages []Message) (ChatResponse, error) {
	index := c.calls
	c.calls++
	if index < len(c.errs) && c.errs[index] != nil {
		return ChatResponse{}, c.errs[index]
	}
	if index < len(c.responses) {
		return c.responses[index], nil
	}
	return ChatResponse{}, fmt.Errorf("no scripted response for call %d", index)
}

type recordingAuditSink struct {
	events []domain.AuditEvent
}

func (s *recordingAuditSink) InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestDecisionEngineValidOnFirstTry(t *testing.T) {
	client := &scriptedChatClient{
		responses: []ChatResponse{validChatResponse(`{"ticker":"SBER","action":"BUY","confidence":0.8,"target_lots":1,"reasoning":"positive","generated_at":"2026-09-14T10:30:00Z"}`)},
	}
	audit := &recordingAuditSink{}
	engine := newTestDecisionEngine(client, audit)

	signal, err := engine.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("Chat calls = %d, want 1", client.calls)
	}
	if signal.Ticker != "SBER" || signal.Action != domain.ActionBuy || signal.TargetLots != 1 {
		t.Fatalf("unexpected signal: %+v", signal)
	}
	if signal.Confidence.String() != "0.8" {
		t.Fatalf("confidence = %s, want 0.8", signal.Confidence.String())
	}
	if len(audit.events) != 0 {
		t.Fatalf("audit events = %d, want 0", len(audit.events))
	}
}

func TestDecisionEngineRetriesInvalidThenValid(t *testing.T) {
	client := &scriptedChatClient{
		responses: []ChatResponse{
			validChatResponse(`not-json`),
			validChatResponse(`{"ticker":"SBER","action":"SELL","confidence":0.7,"target_lots":1,"reasoning":"negative","generated_at":"2026-09-14T10:30:00Z"}`),
		},
	}
	audit := &recordingAuditSink{}
	engine := newTestDecisionEngine(client, audit)

	signal, err := engine.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("Chat calls = %d, want 2", client.calls)
	}
	if signal.Action != domain.ActionSell {
		t.Fatalf("action = %q, want SELL", signal.Action)
	}
	if len(audit.events) != 0 {
		t.Fatalf("audit events = %d, want 0", len(audit.events))
	}
}

func TestDecisionEngineExhaustedRetriesFallsBackToHold(t *testing.T) {
	client := &scriptedChatClient{
		responses: []ChatResponse{
			validChatResponse(`not-json`),
			validChatResponse(`{"ticker":"SBER","action":"HODL","confidence":0.5,"target_lots":1,"reasoning":"bad action"}`),
			validChatResponse(`{"ticker":"","action":"BUY","confidence":0.5,"target_lots":1,"reasoning":"bad ticker"}`),
		},
	}
	audit := &recordingAuditSink{}
	now := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	engine := NewDecisionEngine(client, NewPromptBuilder(), audit, func() time.Time { return now })

	signal, err := engine.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if client.calls != 3 {
		t.Fatalf("Chat calls = %d, want 3", client.calls)
	}
	if signal.Ticker != "SBER" {
		t.Fatalf("fallback ticker = %q, want SBER", signal.Ticker)
	}
	if signal.Action != domain.ActionHold {
		t.Fatalf("fallback action = %q, want HOLD", signal.Action)
	}
	if signal.TargetLots != 0 {
		t.Fatalf("fallback target lots = %d, want 0", signal.TargetLots)
	}
	if !signal.GeneratedAt.Equal(now) {
		t.Fatalf("fallback generated at = %v, want %v", signal.GeneratedAt, now)
	}
	if err := signal.Validate(); err != nil {
		t.Fatalf("fallback Validate() error = %v", err)
	}

	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(audit.events))
	}
	event := audit.events[0]
	if event.Ticker != "SBER" {
		t.Fatalf("audit ticker = %q, want SBER", event.Ticker)
	}
	if event.Stage != StageDecisionFailure {
		t.Fatalf("audit stage = %q, want %q", event.Stage, StageDecisionFailure)
	}
	if !strings.Contains(event.Payload, `"fallback":"HOLD"`) {
		t.Fatalf("audit payload does not note fallback: %q", event.Payload)
	}
	if !strings.Contains(event.Payload, `"attempts":3`) {
		t.Fatalf("audit payload does not note attempts: %q", event.Payload)
	}
}

func TestParseTradeSignalValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "valid", content: `{"ticker":"SBER","action":"BUY","confidence":0.8,"target_lots":1,"reasoning":"up"}`},
		{name: "empty", content: ``, wantErr: "empty response"},
		{name: "malformed json", content: `{`, wantErr: "parse trade signal JSON"},
		{name: "empty ticker", content: `{"ticker":"","action":"BUY","confidence":0.8,"target_lots":1,"reasoning":"up"}`, wantErr: "ticker must not be empty"},
		{name: "bad action", content: `{"ticker":"SBER","action":"HODL","confidence":0.8,"target_lots":1,"reasoning":"up"}`, wantErr: "action must be one of BUY, SELL, HOLD"},
		{name: "confidence above one", content: `{"ticker":"SBER","action":"BUY","confidence":1.01,"target_lots":1,"reasoning":"up"}`, wantErr: "confidence must be in [0,1]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signal, err := ParseTradeSignal(test.content)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseTradeSignal() error = %v", err)
				}
				if signal.Ticker != "SBER" || signal.Action != domain.ActionBuy {
					t.Fatalf("unexpected signal: %+v", signal)
				}
				return
			}
			if err == nil {
				t.Fatal("ParseTradeSignal() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ParseTradeSignal() error = %q, want contains %q", err.Error(), test.wantErr)
			}
		})
	}
}

func TestDecisionEngineNilClientReturnsError(t *testing.T) {
	engine := NewDecisionEngine(nil, NewPromptBuilder(), &recordingAuditSink{}, nil)
	_, err := engine.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err == nil {
		t.Fatal("Generate() error = nil, want client required error")
	}
	if !strings.Contains(err.Error(), "chat client is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validChatResponse(content string) ChatResponse {
	return ChatResponse{
		Model: "qwen3.8",
		Message: Message{
			Role:    "assistant",
			Content: content,
		},
		Done: true,
	}
}

func newTestDecisionEngine(client ChatClient, audit AuditSink) *DecisionEngine {
	return NewDecisionEngine(client, NewPromptBuilder(), audit, func() time.Time {
		return time.Date(2026, 9, 14, 10, 30, 0, 0, time.UTC)
	})
}

func TestDecisionEngineDecimalConfidenceRoundTrip(t *testing.T) {
	client := &scriptedChatClient{
		responses: []ChatResponse{validChatResponse(`{"ticker":"GAZP","action":"BUY","confidence":0.55,"target_lots":1,"reasoning":"up"}`)},
	}
	audit := &recordingAuditSink{}
	engine := newTestDecisionEngine(client, audit)

	signal, err := engine.Generate(context.Background(), domain.FeatureContext{Ticker: "GAZP"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Confidence.Cmp(decimal.NewFromFloat(0.55)) != 0 {
		t.Fatalf("confidence = %s, want 0.55", signal.Confidence.String())
	}
	if signal.GeneratedAt.IsZero() {
		t.Fatal("generated_at was not defaulted")
	}
}
