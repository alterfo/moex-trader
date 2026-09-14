package llm

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestPromptBuilderSystemPrompt(t *testing.T) {
	builder := NewPromptBuilder()

	tests := []struct {
		name string
		ctx  domain.FeatureContext
	}{
		{
			name: "SBER",
			ctx: domain.FeatureContext{
				Ticker:             "SBER",
				GeneratedAt:        time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC),
				LastPrice:          decimal.RequireFromString("310.5"),
				PrevClose:          decimal.RequireFromString("305.2"),
				Bid:                decimal.RequireFromString("310.4"),
				Ask:                decimal.RequireFromString("310.6"),
				ReturnPct:          decimal.RequireFromString("1.74"),
				RealizedVolatility: decimal.RequireFromString("0.9"),
				NewsSentiment:      decimal.RequireFromString("0.7"),
				NewsCount:          4,
			},
		},
		{
			name: "GAZP",
			ctx: domain.FeatureContext{
				Ticker:             "GAZP",
				GeneratedAt:        time.Date(2026, 9, 14, 11, 1, 0, 0, time.UTC),
				LastPrice:          decimal.RequireFromString("128.4"),
				PrevClose:          decimal.RequireFromString("131.0"),
				ReturnPct:          decimal.RequireFromString("-1.98"),
				RealizedVolatility: decimal.RequireFromString("1.4"),
				NewsSentiment:      decimal.RequireFromString("-0.6"),
				NewsCount:          5,
			},
		},
		{
			name: "OZON",
			ctx: domain.FeatureContext{
				Ticker:      "OZON",
				GeneratedAt: time.Date(2026, 9, 14, 11, 2, 0, 0, time.UTC),
				LastPrice:   decimal.RequireFromString("4210.0"),
				PrevClose:   decimal.RequireFromString("4198.0"),
			},
		},
	}

	common := []string{
		StrictJSONInstruction,
		`"type":"object"`,
		`"properties"`,
		`"action"`,
		"Примеры (вход -> выход):",
		"GAZP",
		"OZON",
		"LKOH",
		"YDEX",
		`"action":"BUY"`,
		`"action":"SELL"`,
		`"action":"HOLD"`,
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prompt := builder.SystemPrompt(test.ctx)
			for _, want := range common {
				if !strings.Contains(prompt, want) {
					t.Errorf("SystemPrompt() missing %q in:\n%s", want, prompt)
				}
			}
			if !strings.Contains(prompt, `"ticker":"`+test.ctx.Ticker+`"`) {
				t.Errorf("SystemPrompt() missing current ticker %q", test.ctx.Ticker)
			}
			if !strings.Contains(prompt, `"last_price":`+test.ctx.LastPrice.String()) {
				t.Errorf("SystemPrompt() missing numeric last_price %s", test.ctx.LastPrice.String())
			}
		})
	}
}

func TestFewShotExamplesVariety(t *testing.T) {
	examples := fewShotExamples()
	if len(examples) < 3 || len(examples) > 5 {
		t.Fatalf("fewShotExamples() = %d, want 3..5", len(examples))
	}

	data, err := json.Marshal(examples)
	if err != nil {
		t.Fatalf("marshal fewShotExamples: %v", err)
	}
	text := string(data)

	seenTickers := map[string]bool{}
	seenActions := map[string]bool{}
	for _, example := range examples {
		input, ok := example.Input["ticker"].(string)
		if !ok {
			t.Fatalf("example input ticker has type %T", example.Input["ticker"])
		}
		output, ok := example.Output["ticker"].(string)
		if !ok {
			t.Fatalf("example output ticker has type %T", example.Output["ticker"])
		}
		if input != output {
			t.Errorf("example input/output ticker mismatch: %q != %q", input, output)
		}
		action, ok := example.Output["action"].(string)
		if !ok {
			t.Fatalf("example output action has type %T", example.Output["action"])
		}
		seenTickers[input] = true
		seenActions[action] = true
	}
	if len(seenTickers) < 3 {
		t.Errorf("few-shot examples use %d distinct tickers, want at least 3", len(seenTickers))
	}
	for _, action := range []string{"BUY", "SELL", "HOLD"} {
		if !seenActions[action] {
			t.Errorf("few-shot examples missing action %s", action)
		}
	}

	// Ensure numeric rendering stays numeric rather than quoted decimal strings.
	if !strings.Contains(text, `"confidence":0.`) {
		t.Errorf("few-shot examples confidence should be numeric JSON, got %s", text)
	}
	if !strings.Contains(text, `"target_lots":`) {
		t.Errorf("few-shot examples should contain target_lots, got %s", text)
	}
}
