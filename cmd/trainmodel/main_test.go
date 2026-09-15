package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

type trainingSource struct {
	series map[string][]moex.Candle
}

func (s trainingSource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	return s.series[ticker], nil
}

func testCandles(n int) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range candles {
		price := decimal.NewFromInt(int64(100 + i))
		candles[i] = moex.Candle{
			Open:   price,
			Close:  price.Add(decimal.NewFromFloat(0.5)),
			High:   price.Add(decimal.NewFromInt(2)),
			Low:    price.Sub(decimal.NewFromInt(1)),
			Volume: decimal.NewFromInt(1000),
			Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		}
	}
	return candles
}

func TestParseOptionsDefaults(t *testing.T) {
	opts, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.configPath != defaultConfigPath {
		t.Fatalf("config = %q, want %q", opts.configPath, defaultConfigPath)
	}
	if opts.horizonDays != defaultHorizonDays {
		t.Fatalf("horizonDays = %d, want %d", opts.horizonDays, defaultHorizonDays)
	}
	if opts.deadbandPct != defaultDeadbandPct {
		t.Fatalf("deadbandPct = %v, want %v", opts.deadbandPct, defaultDeadbandPct)
	}
	if opts.learningRate != model.DefaultTrainConfig().LearningRate {
		t.Fatalf("learningRate = %v, want %v", opts.learningRate, model.DefaultTrainConfig().LearningRate)
	}
	if opts.l2Lambda != model.DefaultTrainConfig().L2Lambda {
		t.Fatalf("l2Lambda = %v, want %v", opts.l2Lambda, model.DefaultTrainConfig().L2Lambda)
	}
	if opts.epochs != model.DefaultTrainConfig().Epochs {
		t.Fatalf("epochs = %d, want %d", opts.epochs, model.DefaultTrainConfig().Epochs)
	}
	if opts.valDays != defaultValDays {
		t.Fatalf("valDays = %d, want %d", opts.valDays, defaultValDays)
	}
	if opts.outPath != defaultOutPath {
		t.Fatalf("outPath = %q, want %q", opts.outPath, defaultOutPath)
	}
	if opts.maxLots != 0 {
		t.Fatalf("maxLots = %d, want config fallback sentinel 0", opts.maxLots)
	}
}

func TestParseOptionsOverrides(t *testing.T) {
	opts, err := parseOptions([]string{
		"-config", "other.yaml",
		"-tickers", "SBER, OZON",
		"-from", "2024-01-01",
		"-till", "2025-01-01",
		"-horizon-days", "10",
		"-deadband-pct", "0.8",
		"-learning-rate", "0.02",
		"-l2-lambda", "0.01",
		"-epochs", "250",
		"-split-date", "2024-07-01",
		"-val-days", "30",
		"-max-lots", "3",
		"-out", "/tmp/model.json",
	})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.configPath != "other.yaml" {
		t.Fatalf("config = %q, want other.yaml", opts.configPath)
	}
	if opts.tickersFlag != "SBER, OZON" {
		t.Fatalf("tickers = %q, want SBER, OZON", opts.tickersFlag)
	}
	if opts.fromStr != "2024-01-01" || opts.tillStr != "2025-01-01" {
		t.Fatalf("dates = %q..%q, want 2024-01-01..2025-01-01", opts.fromStr, opts.tillStr)
	}
	if opts.horizonDays != 10 || opts.deadbandPct != 0.8 {
		t.Fatalf("horizon/deadband = (%d, %v), want (10, 0.8)", opts.horizonDays, opts.deadbandPct)
	}
	if opts.learningRate != 0.02 || opts.l2Lambda != 0.01 || opts.epochs != 250 {
		t.Fatalf("train config = (%v, %v, %d), want (0.02, 0.01, 250)", opts.learningRate, opts.l2Lambda, opts.epochs)
	}
	if opts.splitDateStr != "2024-07-01" || opts.valDays != 30 {
		t.Fatalf("split/val = (%q, %d), want (2024-07-01, 30)", opts.splitDateStr, opts.valDays)
	}
	if opts.maxLots != 3 || opts.outPath != "/tmp/model.json" {
		t.Fatalf("maxLots/out = (%d, %q), want (3, /tmp/model.json)", opts.maxLots, opts.outPath)
	}
}

func TestParseOptionsRejectsPositionalArgs(t *testing.T) {
	if _, err := parseOptions([]string{"unexpected"}); err == nil {
		t.Fatal("parseOptions() error = nil, want unexpected argument error")
	}
}

func TestComputeSplitDateFromValDays(t *testing.T) {
	till := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	got, err := computeSplitDate(till, "", 30)
	if err != nil {
		t.Fatalf("computeSplitDate() error = %v", err)
	}
	want := time.Date(2024, 5, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("computeSplitDate() = %v, want %v", got, want)
	}
}

func TestComputeSplitDateExplicit(t *testing.T) {
	till := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	got, err := computeSplitDate(till, "2024-04-15", 30)
	if err != nil {
		t.Fatalf("computeSplitDate() error = %v", err)
	}
	want := time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("computeSplitDate() = %v, want %v", got, want)
	}
}

func TestComputeSplitDateErrors(t *testing.T) {
	till := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := computeSplitDate(till, "", 0); err == nil {
		t.Fatal("computeSplitDate() error = nil for non-positive val-days")
	}
	if _, err := computeSplitDate(till, "not-a-date", 30); err == nil {
		t.Fatal("computeSplitDate() error = nil for invalid split-date")
	}
}

func TestResolveWindowDefaults(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 30, 0, 0, time.UTC)
	opts := options{valDays: 10}
	from, till, split, err := resolveWindow(opts, now)
	if err != nil {
		t.Fatalf("resolveWindow() error = %v", err)
	}
	if !from.Equal(now.AddDate(-2, 0, 0)) {
		t.Fatalf("from = %v, want %v", from, now.AddDate(-2, 0, 0))
	}
	if !till.Equal(now) {
		t.Fatalf("till = %v, want %v", till, now)
	}
	if !split.Equal(now.AddDate(0, 0, -10)) {
		t.Fatalf("split = %v, want %v", split, now.AddDate(0, 0, -10))
	}
}

func TestRunPipelineEndToEnd(t *testing.T) {
	candles := testCandles(140)
	source := trainingSource{series: map[string][]moex.Candle{"TEST": candles}}
	outPath := filepath.Join(t.TempDir(), "model.json")
	from := candles[0].Begin
	till := candles[0].Begin.AddDate(0, 0, 200)
	split := candles[0].Begin.AddDate(0, 0, 90)
	var stdout bytes.Buffer

	weights, result, err := runPipeline(context.Background(), pipelineConfig{
		tickers:        []string{"TEST"},
		from:           from,
		till:           till,
		split:          split,
		horizonDays:    5,
		deadbandPct:    0.5,
		trainCfg:       model.DefaultTrainConfig(),
		maxLots:        1,
		deposit:        decimal.NewFromInt(100000),
		commissionRate: decimal.Zero,
		outPath:        outPath,
		now: func() time.Time {
			return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
		},
	}, source, &stdout)
	if err != nil {
		t.Fatalf("runPipeline() error = %v", err)
	}
	if weights == nil || result == nil {
		t.Fatal("runPipeline() returned nil weights or result")
	}
	if !strings.Contains(stdout.String(), "# Backtest Report") {
		t.Fatalf("stdout = %q, want markdown report", stdout.String())
	}
	if weights.Training.TrainSamples == 0 || weights.Training.ValSamples == 0 {
		t.Fatalf("sample split = %d train / %d val, want both non-zero", weights.Training.TrainSamples, weights.Training.ValSamples)
	}

	loaded, err := model.LoadWeights(outPath)
	if err != nil {
		t.Fatalf("LoadWeights() error = %v", err)
	}
	if len(loaded.FeatureOrder) == 0 || len(loaded.FeatureOrder) != len(loaded.Coef) {
		t.Fatalf("loaded feature order/coef = %d/%d", len(loaded.FeatureOrder), len(loaded.Coef))
	}
	if loaded.Training.ValSharpe != result.Sharpe || loaded.Training.ValHitRate != result.HitRate || loaded.Training.ValMaxDrawdownPct != result.MaxDrawdownPct {
		t.Fatalf("loaded validation metrics = (%v, %v, %v), result = (%v, %v, %v)",
			loaded.Training.ValSharpe, loaded.Training.ValHitRate, loaded.Training.ValMaxDrawdownPct,
			result.Sharpe, result.HitRate, result.MaxDrawdownPct)
	}
}

func TestSplitTrainValNoLabelLeakage(t *testing.T) {
	split := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	samples := []model.LabeledSample{
		{Label: 1, LabelDate: split.AddDate(0, 0, -3)},
		{Label: 1, LabelDate: split.AddDate(0, 0, -1)},
		{Label: 0, LabelDate: split},
		{Label: 0, LabelDate: split.AddDate(0, 0, 2)},
	}

	train, val := splitTrainVal(samples, split)
	if len(train) != 2 || len(val) != 2 {
		t.Fatalf("split = %d train / %d val, want 2/2", len(train), len(val))
	}
	for _, sample := range train {
		if !sample.LabelDate.Before(split) {
			t.Fatalf("train sample LabelDate %v is not before split %v: label leaked into training", sample.LabelDate, split)
		}
	}
	for _, sample := range val {
		if sample.LabelDate.Before(split) {
			t.Fatalf("val sample LabelDate %v is before split %v: sample misclassified as validation", sample.LabelDate, split)
		}
	}
}

func TestRunPipelineRejectsInvalidConfig(t *testing.T) {
	_, _, err := runPipeline(context.Background(), pipelineConfig{
		tickers:     []string{"TEST"},
		horizonDays: -1,
		outPath:     "model.json",
	}, trainingSource{}, nil)
	if err == nil {
		t.Fatal("runPipeline() error = nil, want invalid horizon error")
	}
}
