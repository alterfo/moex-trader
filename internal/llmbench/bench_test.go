package llmbench

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/llm"
)

type fakeChatClient struct {
	responses []llm.ChatResponse
	errs      []error
	calls     int
}

func (c *fakeChatClient) Chat(ctx context.Context, messages []llm.Message) (llm.ChatResponse, error) {
	index := c.calls
	c.calls++
	if index < len(c.errs) && c.errs[index] != nil {
		return llm.ChatResponse{}, c.errs[index]
	}
	if index < len(c.responses) {
		return c.responses[index], nil
	}
	return llm.ChatResponse{}, context.DeadlineExceeded
}

func validChatResponse(content string) llm.ChatResponse {
	return llm.ChatResponse{
		Model: "qwen3.8",
		Message: llm.Message{
			Role:    "assistant",
			Content: content,
		},
		Done: true,
	}
}

func TestRunnerRecordsSuccessAndFailures(t *testing.T) {
	client := &fakeChatClient{
		responses: []llm.ChatResponse{
			validChatResponse(`{"ticker":"SBER","action":"BUY","confidence":0.8,"target_lots":1,"reasoning":"positive"}`),
			validChatResponse(`not-json`),
		},
		errs: []error{nil, nil, context.DeadlineExceeded},
	}
	runner := NewRunner(client, llm.NewPromptBuilder())
	results := runner.Run(context.Background(), []Sample{
		{Name: "first", Feature: domain.FeatureContext{Ticker: "SBER"}},
		{Name: "second", Feature: domain.FeatureContext{Ticker: "GAZP"}},
		{Name: "third", Feature: domain.FeatureContext{Ticker: "OZON"}},
	})

	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("first result error = %v, want nil", results[0].Err)
	}
	if results[0].Signal.Action != domain.ActionBuy || results[0].Signal.Ticker != "SBER" {
		t.Fatalf("first signal = %+v", results[0].Signal)
	}
	if results[1].Err == nil || !strings.Contains(results[1].Err.Error(), "parse trade signal JSON") {
		t.Fatalf("second result error = %v, want parse error", results[1].Err)
	}
	if results[2].Err != context.DeadlineExceeded {
		t.Fatalf("third result error = %v, want deadline exceeded", results[2].Err)
	}
	if client.calls != 3 {
		t.Fatalf("Chat calls = %d, want 3", client.calls)
	}
}

func TestSummarizeStats(t *testing.T) {
	results := []Result{
		{Name: "a", Latency: time.Millisecond, Err: nil},
		{Name: "b", Latency: 2 * time.Millisecond, Err: nil},
		{Name: "c", Latency: 3 * time.Millisecond, Err: context.DeadlineExceeded},
		{Name: "d", Latency: 4 * time.Millisecond, Err: nil},
	}

	stats := Summarize(results)
	if stats.Total != 4 {
		t.Fatalf("Total = %d, want 4", stats.Total)
	}
	if stats.Success != 3 || stats.Failed != 1 {
		t.Fatalf("Success/Failed = %d/%d, want 3/1", stats.Success, stats.Failed)
	}
	if stats.SuccessRate != 0.75 {
		t.Fatalf("SuccessRate = %v, want 0.75", stats.SuccessRate)
	}
	if stats.AvgLatency != 2500*time.Microsecond {
		t.Fatalf("AvgLatency = %s, want 2.5ms", stats.AvgLatency)
	}
	if stats.P50 != 2*time.Millisecond {
		t.Fatalf("P50 = %s, want 2ms", stats.P50)
	}
	if stats.P90 != 4*time.Millisecond {
		t.Fatalf("P90 = %s, want 4ms", stats.P90)
	}
	if stats.P95 != 4*time.Millisecond {
		t.Fatalf("P95 = %s, want 4ms", stats.P95)
	}
	if stats.P99 != 4*time.Millisecond {
		t.Fatalf("P99 = %s, want 4ms", stats.P99)
	}
}

func TestDefaultSamples(t *testing.T) {
	samples := DefaultSamples()
	if len(samples) == 0 {
		t.Fatal("DefaultSamples() returned no samples")
	}
	for _, sample := range samples {
		if sample.Name == "" || sample.Feature.Ticker == "" {
			t.Fatalf("invalid sample: %+v", sample)
		}
		if !sample.Feature.LastPrice.IsPositive() {
			t.Fatalf("sample %s last price is not positive", sample.Name)
		}
	}
}
