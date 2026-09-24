package trainrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

type Config struct {
	Tickers            []string
	From               time.Time
	Till               time.Time
	Split              time.Time
	HorizonDays        int
	DeadbandPct        float64
	LabelCommissionPct float64
	IntervalMin        int
	FeatureBPD         int
	TrainCfg           model.TrainConfig
	BuyThreshold       float64
	SellThreshold      float64
	MaxLots            int
	Deposit            decimal.Decimal
	CommissionRate     decimal.Decimal
	OutPath            string
	NewsHistory        string
	LabelMode          model.LabelMode
	Now                func() time.Time
}

func Run(ctx context.Context, cfg Config, source backtest.HistoricalSource, stdout io.Writer) (*model.Weights, *backtest.Result, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, nil, err
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	samples, err := model.BuildSamplesWithFeatureConfig(ctx, source, cfg.Tickers, cfg.From, cfg.Till, cfg.HorizonDays, cfg.DeadbandPct, cfg.LabelMode, cfg.LabelCommissionPct, features.PriceFeatureConfig{BarsPerDay: cfg.FeatureBPD})
	if err != nil {
		return nil, nil, fmt.Errorf("build samples: %w", err)
	}
	if len(samples) == 0 {
		return nil, nil, errors.New("no labeled samples in the selected window")
	}

	if cfg.NewsHistory != "" {
		records, err := model.LoadFinanalysNewsHistory(cfg.NewsHistory)
		if err != nil {
			return nil, nil, fmt.Errorf("load news history: %w", err)
		}
		news := model.AggregateDailySentiment(records)
		topics := model.AggregateDailyTopicSignals(records)
		applied := model.ApplyNewsOverride(samples, news)
		topicApplied := model.ApplyTopicSignalOverrides(samples, topics)
		log.Printf("trainrun: applied real news_sentiment/news_count to %d/%d samples and topic signals to %d/%d samples from %d records", applied, len(samples), topicApplied, len(samples), len(records))
	}

	trainSamples, valSamples := SplitTrainVal(samples, cfg.Split)
	if len(trainSamples) == 0 {
		return nil, nil, errors.New("no training samples before the validation split")
	}

	rows := make([]model.Sample, 0, len(trainSamples))
	names := make([]string, 0)
	for _, sample := range trainSamples {
		vector, order := model.ToVector(sample.Feature)
		names = order
		rows = append(rows, model.Sample{X: vector, Y: sample.Label})
	}
	mean, std := model.Standardize(rows)
	coef, bias, err := model.Train(rows, cfg.TrainCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("train: %w", err)
	}

	weights := &model.Weights{
		FeatureOrder:  names,
		Mean:          mean,
		Std:           std,
		Coef:          coef,
		Bias:          bias,
		BuyThreshold:  cfg.BuyThreshold,
		SellThreshold: cfg.SellThreshold,
		HorizonDays:   cfg.HorizonDays,
		DeadbandPct:   cfg.DeadbandPct,
		TrainedAt:     now().UTC(),
		Training: model.TrainingMetadata{
			TrainFrom:     cfg.From,
			TrainTill:     cfg.Split,
			ValFrom:       cfg.Split,
			ValTill:       cfg.Till,
			Tickers:       append([]string(nil), cfg.Tickers...),
			TrainSamples:  len(trainSamples),
			ValSamples:    len(valSamples),
			TrainAccuracy: classificationAccuracy(rows, coef, bias, mean, std),
			ValAccuracy:   labeledAccuracy(valSamples, coef, bias, mean, std),
		},
	}
	signalSource := &model.SignalSource{Weights: weights, MaxLots: cfg.MaxLots}
	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:        cfg.Tickers,
		From:           cfg.Split,
		Till:           cfg.Till,
		Deposit:        cfg.Deposit,
		MaxLots:        cfg.MaxLots,
		CommissionRate: cfg.CommissionRate,
		WarmupDays:     backtest.DefaultWarmupDays,
		KillSwitch:     true,
		SignalSource:   signalSource,
		Source:         source,
		Logger:         log.Default(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build validation engine: %w", err)
	}

	result, err := engine.Run(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("validation backtest: %w", err)
	}

	weights.Training.ValSharpe = result.Sharpe
	weights.Training.ValHitRate = result.HitRate
	weights.Training.ValMaxDrawdownPct = result.MaxDrawdownPct
	if strings.TrimSpace(cfg.OutPath) != "" {
		if err := weights.Save(cfg.OutPath); err != nil {
			return nil, nil, fmt.Errorf("save trained weights: %w", err)
		}
	}

	if stdout != nil {
		fmt.Fprint(stdout, result.Markdown())
	}
	return weights, result, nil
}

func SplitTrainVal(samples []model.LabeledSample, split time.Time) (train, val []model.LabeledSample) {
	for _, sample := range samples {
		if sample.LabelDate.Before(split) {
			train = append(train, sample)
		} else {
			val = append(val, sample)
		}
	}
	return train, val
}

func validateConfig(cfg Config) error {
	if len(cfg.Tickers) == 0 {
		return errors.New("tickers must not be empty")
	}
	if cfg.HorizonDays <= 0 {
		return errors.New("horizon-days must be positive")
	}
	if cfg.DeadbandPct < 0 {
		return errors.New("deadband-pct must be non-negative")
	}
	if cfg.MaxLots <= 0 {
		return errors.New("max lots must be positive")
	}
	if cfg.Deposit.Sign() <= 0 {
		return errors.New("deposit must be positive")
	}
	if cfg.CommissionRate.IsNegative() {
		return errors.New("commission rate must be non-negative")
	}
	if !cfg.From.Before(cfg.Split) || !cfg.Split.Before(cfg.Till) {
		return errors.New("split date must be after from and before till")
	}
	return nil
}

func classificationAccuracy(samples []model.Sample, coef []float64, bias float64, mean, std []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, sample := range samples {
		normalized := make([]float64, len(sample.X))
		for i, value := range sample.X {
			divisor := std[i]
			if divisor == 0 {
				divisor = 1
			}
			normalized[i] = (value - mean[i]) / divisor
		}
		logit := dotVector(coef, normalized) + bias
		prediction := 0.0
		if sigmoidValue(logit) >= 0.5 {
			prediction = 1
		}
		if prediction == sample.Y {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func labeledAccuracy(samples []model.LabeledSample, coef []float64, bias float64, mean, std []float64) float64 {
	rows := make([]model.Sample, 0, len(samples))
	for _, sample := range samples {
		vector, _ := model.ToVector(sample.Feature)
		rows = append(rows, model.Sample{X: vector, Y: sample.Label})
	}
	return classificationAccuracy(rows, coef, bias, mean, std)
}

func dotVector(a, b []float64) float64 {
	total := 0.0
	for i := range a {
		total += a[i] * b[i]
	}
	return total
}

func sigmoidValue(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	exponential := math.Exp(z)
	return exponential / (1 + exponential)
}
