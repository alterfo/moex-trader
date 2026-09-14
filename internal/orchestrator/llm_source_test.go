package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/llm"
)

type mockedChatClient struct {
	calls atomic.Int32
}

func (m *mockedChatClient) Chat(ctx context.Context, messages []llm.Message) (llm.ChatResponse, error) {
	m.calls.Add(1)
	ticker := tickerFromSystemPrompt(messages)
	if ticker == "" {
		ticker = "SBER"
	}
	content := fmt.Sprintf(`{"ticker":"%s","action":"BUY","confidence":0.8,"target_lots":1,"reasoning":"positive"}`, ticker)
	return llm.ChatResponse{
		Model:   "qwen3.8",
		Message: llm.Message{Role: "assistant", Content: content},
		Done:    true,
	}, nil
}

func tickerFromSystemPrompt(messages []llm.Message) string {
	if len(messages) == 0 {
		return ""
	}
	const marker = `"ticker":"`
	content := messages[0].Content
	index := strings.Index(content, marker)
	if index < 0 {
		return ""
	}
	rest := content[index+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func TestOrchestratorIntegrationWithMockedLLMClient(t *testing.T) {
	store := openTestStore(t)
	now := func() time.Time { return time.Date(2024, 1, 11, 12, 30, 0, 0, time.UTC) }
	ingestor := &fakeIngestor{inputs: map[string]features.Input{
		"SBER": fixtureInput("SBER"),
		"YDEX": fixtureInput("YDEX"),
	}}
	client := &mockedChatClient{}
	decision := llm.NewDecisionEngine(client, llm.NewPromptBuilder(), store, now)
	source := NewLLMSignalSource(decision, time.Second, log.New(io.Discard, "", 0))
	exec := &fakeExecutor{store: store, now: now}
	orch := newTestOrchestrator(t, store, ingestor, source, now, exec)

	orch.RunOnce(context.Background())

	if client.calls.Load() != 2 {
		t.Fatalf("LLM calls = %d, want 2", client.calls.Load())
	}
	if exec.callCount() != 2 {
		t.Fatalf("executor calls = %d, want 2", exec.callCount())
	}

	events, err := store.ListAuditEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	for _, ticker := range []string{"SBER", "YDEX"} {
		if got := countEvents(events, ticker, StageSignal); got != 1 {
			t.Fatalf("signal events for %s = %d, want 1", ticker, got)
		}
	}
	for _, event := range events {
		if event.Stage != StageSignal {
			continue
		}
		if !strings.Contains(event.Payload, `"action":"BUY"`) {
			t.Fatalf("signal payload does not contain BUY action: %s", event.Payload)
		}
	}
}

func TestLLMSignalSourceLogsLatency(t *testing.T) {
	client := &mockedChatClient{}
	decision := llm.NewDecisionEngine(client, llm.NewPromptBuilder(), nil, time.Now)
	var buf bytes.Buffer
	source := NewLLMSignalSource(decision, time.Second, log.New(&buf, "", 0))

	signal, err := source.Generate(context.Background(), domain.FeatureContext{Ticker: "SBER"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if signal.Ticker != "SBER" || signal.Action != domain.ActionBuy {
		t.Fatalf("unexpected signal: %+v", signal)
	}
	if !strings.Contains(buf.String(), "orchestrator: llm latency for SBER:") {
		t.Fatalf("latency log missing: %q", buf.String())
	}
}
