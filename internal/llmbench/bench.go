package llmbench

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/llm"
)

type Sample struct {
	Name    string
	Feature domain.FeatureContext
}

type Result struct {
	Name    string
	Latency time.Duration
	Signal  domain.TradeSignal
	Err     error
}

type Runner struct {
	client llm.ChatClient
	prompt *llm.PromptBuilder
}

func NewRunner(client llm.ChatClient, prompt *llm.PromptBuilder) *Runner {
	if prompt == nil {
		prompt = llm.NewPromptBuilder()
	}
	return &Runner{client: client, prompt: prompt}
}

func (r *Runner) Run(ctx context.Context, samples []Sample) []Result {
	results := make([]Result, 0, len(samples))
	for _, sample := range samples {
		result := Result{Name: sample.Name}
		messages := []llm.Message{
			{Role: "system", Content: r.prompt.SystemPrompt(sample.Feature)},
			{Role: "user", Content: "Сформируй торговый сигнал для текущего инструмента."},
		}

		started := time.Now()
		response, err := r.client.Chat(ctx, messages)
		result.Latency = time.Since(started)
		if err != nil {
			result.Err = err
			results = append(results, result)
			continue
		}

		signal, err := llm.ParseTradeSignal(response.Message.Content)
		if err != nil {
			result.Err = err
			results = append(results, result)
			continue
		}
		result.Signal = signal
		results = append(results, result)
	}
	return results
}

type Stats struct {
	Total       int
	Success     int
	Failed      int
	SuccessRate float64
	AvgLatency  time.Duration
	P50         time.Duration
	P90         time.Duration
	P95         time.Duration
	P99         time.Duration
}

func Summarize(results []Result) Stats {
	stats := Stats{Total: len(results)}
	latencies := make([]time.Duration, 0, len(results))

	var total time.Duration
	for _, result := range results {
		latencies = append(latencies, result.Latency)
		total += result.Latency
		if result.Err == nil {
			stats.Success++
		} else {
			stats.Failed++
		}
	}

	if stats.Total > 0 {
		stats.SuccessRate = float64(stats.Success) / float64(stats.Total)
		stats.AvgLatency = total / time.Duration(stats.Total)
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})
	stats.P50 = percentile(latencies, 0.50)
	stats.P90 = percentile(latencies, 0.90)
	stats.P95 = percentile(latencies, 0.95)
	stats.P99 = percentile(latencies, 0.99)
	return stats
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	rank := int(math.Ceil(p*float64(len(values)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(values) {
		rank = len(values) - 1
	}
	return values[rank]
}

func DefaultSamples() []Sample {
	return []Sample{
		{Name: "SBER", Feature: sampleContext("SBER", "310.5", "305.2", "1.74", "0.9", "0.7", 4)},
		{Name: "GAZP", Feature: sampleContext("GAZP", "128.4", "131.0", "-1.98", "1.4", "-0.6", 5)},
		{Name: "OZON", Feature: sampleContext("OZON", "4210.0", "4198.0", "0.29", "0.6", "0.0", 0)},
		{Name: "LKOH", Feature: sampleContext("LKOH", "6420.5", "6350.0", "1.11", "0.8", "0.4", 2)},
		{Name: "YDEX", Feature: sampleContext("YDEX", "4021.0", "4080.0", "-1.45", "1.1", "-0.3", 1)},
	}
}

func sampleContext(ticker, lastPrice, prevClose, returnPct, volatility, sentiment string, newsCount int) domain.FeatureContext {
	return domain.FeatureContext{
		Ticker:             ticker,
		GeneratedAt:        time.Date(2026, 9, 14, 10, 30, 0, 0, time.UTC),
		LastPrice:          mustDecimal(lastPrice),
		PrevClose:          mustDecimal(prevClose),
		ReturnPct:          mustDecimal(returnPct),
		RealizedVolatility: mustDecimal(volatility),
		NewsSentiment:      mustDecimal(sentiment),
		NewsCount:          newsCount,
	}
}

func mustDecimal(value string) decimal.Decimal {
	return decimal.RequireFromString(value)
}
