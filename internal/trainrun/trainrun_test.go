package trainrun

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

func testIndexCandles(n int) []moex.Candle {
	candles := make([]moex.Candle, n)
	for i := range candles {
		candles[i] = moex.Candle{
			Open:   decimal.NewFromInt(3000),
			Close:  decimal.NewFromInt(3000),
			High:   decimal.NewFromInt(3010),
			Low:    decimal.NewFromInt(2990),
			Volume: decimal.NewFromInt(1000000),
			Begin:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			End:    time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC).AddDate(0, 0, i),
		}
	}
	return candles
}

func TestRunEndToEnd(t *testing.T) {
	candles := testCandles(140)
	source := trainingSource{series: map[string][]moex.Candle{
		"TEST":  candles,
		"IMOEX": testIndexCandles(140),
	}}
	outPath := filepath.Join(t.TempDir(), "model.json")
	from := candles[0].Begin
	till := candles[0].Begin.AddDate(0, 0, 200)
	split := candles[0].Begin.AddDate(0, 0, 90)
	var stdout bytes.Buffer

	weights, result, err := Run(context.Background(), Config{
		Tickers:        []string{"TEST"},
		From:           from,
		Till:           till,
		Split:          split,
		HorizonDays:    5,
		DeadbandPct:    0.5,
		TrainCfg:       model.DefaultTrainConfig(),
		BuyThreshold:   0.55,
		SellThreshold:  0.45,
		MaxLots:        1,
		Deposit:        decimal.NewFromInt(100000),
		CommissionRate: decimal.Zero,
		OutPath:        outPath,
		Now: func() time.Time {
			return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
		},
	}, source, &stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if weights == nil || result == nil {
		t.Fatal("Run() returned nil weights or result")
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

	train, val := SplitTrainVal(samples, split)
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

func TestRunRejectsInvalidConfig(t *testing.T) {
	_, _, err := Run(context.Background(), Config{
		Tickers:     []string{"TEST"},
		HorizonDays: -1,
		OutPath:     "model.json",
	}, trainingSource{}, nil)
	if err == nil {
		t.Fatal("Run() error = nil, want invalid horizon error")
	}
}
