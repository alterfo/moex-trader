package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/strategyvalidation"
	"github.com/olegsidorkin/moex-trader/internal/tearsheet"
	"github.com/olegsidorkin/moex-trader/internal/trainrun"
	"github.com/olegsidorkin/moex-trader/internal/walkforward"
)

const modelFileName = "model.json"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath string
	var fromStr, tillStr string
	var tickersStr string
	var depositStr string
	var maxLots int
	var commissionStr string
	var spreadStr string
	var slippageStr string
	var borrowPctDayStr string
	var trainDays, testDays, stepDays int
	var anchored bool
	var horizonDays int
	var deadbandPct float64
	var labelCommissionPct float64
	var learningRate, l2Lambda float64
	var epochs int
	var buyPct, sellPct float64
	var labelMode string
	var intervalMin int
	var featureBPD int
	var newsHistory string
	var wfDir string
	var outPath string
	var tearsheetPath string
	var candidatesPath string
	var matrixOutPath string
	var pboKey string
	var pboSplits int

	flag.StringVar(&configPath, "config", "config.yaml", "path to config YAML")
	flag.StringVar(&fromStr, "from", "", "walk-forward window start date YYYY-MM-DD (default: three years ago)")
	flag.StringVar(&tillStr, "till", "", "walk-forward window end date YYYY-MM-DD (default: today)")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&depositStr, "deposit", "100000", "paper deposit per window, in RUB")
	flag.IntVar(&maxLots, "max-lots", 0, "max position in lots (default: config risk.max_lots)")
	flag.StringVar(&commissionStr, "commission-rate", "0.0005", "commission rate applied to notional per fill")
	flag.StringVar(&spreadStr, "spread-pct", "0", "half-spread cost applied against each fill, as a fraction of price")
	flag.StringVar(&slippageStr, "slippage-pct", "0", "additional adverse slippage applied against each fill, as a fraction of price")
	flag.StringVar(&borrowPctDayStr, "borrow-pct-day", "0", "short-borrow cost per day as a fraction of short-leg notional")
	flag.IntVar(&trainDays, "train-days", 365, "training window length in calendar days")
	flag.IntVar(&testDays, "test-days", 63, "out-of-sample test window length in calendar days")
	flag.IntVar(&stepDays, "step-days", 63, "how far the split date advances between windows")
	flag.BoolVar(&anchored, "anchored", false, "use an expanding (anchored) training window instead of a fixed-length rolling one")
	flag.IntVar(&horizonDays, "horizon-days", 5, "forward-return label horizon in trading days")
	flag.Float64Var(&deadbandPct, "deadband-pct", 0.5, "exclude labels with absolute forward return below this percent")
	flag.Float64Var(&labelCommissionPct, "label-commission-pct", 0, "one-way commission rate to bake into the label dead zone")
	flag.Float64Var(&learningRate, "learning-rate", model.DefaultTrainConfig().LearningRate, "gradient descent learning rate")
	flag.Float64Var(&l2Lambda, "l2-lambda", model.DefaultTrainConfig().L2Lambda, "L2 regularization strength")
	flag.IntVar(&epochs, "epochs", model.DefaultTrainConfig().Epochs, "gradient descent epochs")
	flag.Float64Var(&buyPct, "buy-pct", 0.55, "probability at/above which to BUY")
	flag.Float64Var(&sellPct, "sell-pct", 0.45, "probability at/below which to SELL")
	flag.StringVar(&labelMode, "label-mode", "excess", "label target: excess (vs IMOEX) or absolute forward return")
	flag.IntVar(&intervalMin, "interval-min", 0, "candle interval in minutes for intraday bars (24 or 0 = daily)")
	flag.IntVar(&featureBPD, "feature-bars-per-day", 0, "scale day-named feature windows by this many bars/session")
	flag.StringVar(&newsHistory, "news-history", "", "path to a finanalys-format news_history.jsonl")
	flag.StringVar(&wfDir, "wf-dir", "", "directory to persist each window's model, config and metrics (required)")
	flag.StringVar(&outPath, "out", "", "path to write the aggregate markdown report (default: stdout)")
	flag.StringVar(&tearsheetPath, "tearsheet", "", "when set, write an aggregate HTML tearsheet to this path plus a <path>.metrics.json summary")
	flag.StringVar(&candidatesPath, "candidates", "", "path to a JSON array of candidate configs; when set, runs a PBO grid instead of a single walk-forward")
	flag.StringVar(&matrixOutPath, "pbo-matrix-out", "", "candidates mode: path to write the period-return matrix JSON (default: <wf-dir>/pbo_matrix.json)")
	flag.StringVar(&pboKey, "pbo-key", "walkforward_candidates", "candidates mode: key under which the period-return matrix is archived")
	flag.IntVar(&pboSplits, "pbo-splits", 0, "candidates mode: CSCV split count s (must be even; default: largest even value <= min(periods, 16))")
	flag.Parse()

	deposit, err := decimal.NewFromString(depositStr)
	if err != nil {
		return fmt.Errorf("parse -deposit: %w", err)
	}
	commissionRate, err := decimal.NewFromString(commissionStr)
	if err != nil {
		return fmt.Errorf("parse -commission-rate: %w", err)
	}
	spreadPct, err := decimal.NewFromString(spreadStr)
	if err != nil {
		return fmt.Errorf("parse -spread-pct: %w", err)
	}
	slippagePct, err := decimal.NewFromString(slippageStr)
	if err != nil {
		return fmt.Errorf("parse -slippage-pct: %w", err)
	}
	borrowPctPerDay, err := decimal.NewFromString(borrowPctDayStr)
	if err != nil {
		return fmt.Errorf("parse -borrow-pct-day: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if len(tickers) == 0 {
		return errors.New("tickers must not be empty")
	}
	if maxLots <= 0 {
		maxLots = cfg.Risk.MaxLots
	}
	if maxLots <= 0 {
		return errors.New("max lots must be positive")
	}
	if strings.TrimSpace(wfDir) == "" {
		return errors.New("-wf-dir is required")
	}

	now := time.Now()
	var from, till time.Time
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return fmt.Errorf("parse -from: %w", err)
		}
	} else {
		from = now.AddDate(-3, 0, 0)
	}
	if tillStr != "" {
		till, err = time.Parse("2006-01-02", tillStr)
		if err != nil {
			return fmt.Errorf("parse -till: %w", err)
		}
	} else {
		till = now
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	var source backtest.HistoricalSource
	resolvedBPD := featureBPD
	if resolvedBPD < 0 {
		resolvedBPD = features.ConfigForInterval(intervalMin).BarsPerDay
	}
	if intervalMin > 0 && intervalMin != 24 {
		source = backtest.NewISSSourceInterval(cfg.MOEXISSBaseURL, moexClient, intervalMin)
	} else {
		source = backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	specs, err := walkforward.GenerateWindowSpecs(from, till, trainDays, testDays, stepDays, anchored)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return fmt.Errorf("walk-forward: window %s..%s is too short for train-days=%d test-days=%d", from.Format("2006-01-02"), till.Format("2006-01-02"), trainDays, testDays)
	}
	log.Printf("walkforward: %d windows generated (train=%dd test=%dd step=%dd anchored=%v)", len(specs), trainDays, testDays, stepDays, anchored)

	baseTrainCfg := model.TrainConfig{LearningRate: learningRate, L2Lambda: l2Lambda, Epochs: epochs}

	if strings.TrimSpace(candidatesPath) != "" {
		return runCandidates(ctx, candidatesRunConfig{
			candidatesPath:     candidatesPath,
			specs:              specs,
			tickers:            tickers,
			deposit:            deposit,
			commissionRate:     commissionRate,
			maxLots:            maxLots,
			labelCommissionPct: labelCommissionPct,
			labelMode:          model.LabelMode(labelMode),
			intervalMin:        intervalMin,
			featureBPD:         resolvedBPD,
			trainCfg:           baseTrainCfg,
			newsHistory:        newsHistory,
			matrixOutPath:      matrixOutPath,
			wfDir:              wfDir,
			pboKey:             pboKey,
			pboSplits:          pboSplits,
		}, source)
	}

	var results []backtest.Result
	for i, spec := range specs {
		windowID := walkforward.NewWindowID(spec.Split, spec.Till)
		windowDir := filepath.Join(wfDir, windowID)
		modelOut := filepath.Join(windowDir, modelFileName)

		weights, result, err := trainrun.Run(ctx, trainrun.Config{
			Tickers:            tickers,
			From:               spec.From,
			Till:               spec.Till,
			Split:              spec.Split,
			HorizonDays:        horizonDays,
			DeadbandPct:        deadbandPct,
			LabelCommissionPct: labelCommissionPct,
			IntervalMin:        intervalMin,
			FeatureBPD:         resolvedBPD,
			TrainCfg:           baseTrainCfg,
			BuyThreshold:       buyPct,
			SellThreshold:      sellPct,
			MaxLots:            maxLots,
			Deposit:            deposit,
			CommissionRate:     commissionRate,
			OutPath:            modelOut,
			NewsHistory:        newsHistory,
			LabelMode:          model.LabelMode(labelMode),
			Now:                time.Now,
		}, source, io.Discard)
		if err != nil {
			return fmt.Errorf("window %d/%d (%s): %w", i+1, len(specs), windowID, err)
		}

		wfConfig := walkforward.Config{
			WindowStart:     spec.Split,
			WindowEnd:       spec.Till,
			SignalSource:    "trainrun",
			ModelPath:       modelOut,
			Deposit:         deposit.String(),
			MaxLots:         maxLots,
			CommissionRate:  commissionRate.String(),
			SpreadPct:       spreadPct.String(),
			SlippagePct:     slippagePct.String(),
			BorrowPctPerDay: borrowPctPerDay.String(),
			Tickers:         append([]string(nil), tickers...),
			BuyThreshold:    buyPct,
			SellThreshold:   sellPct,
			FeatureOrder:    append([]string(nil), weights.FeatureOrder...),
		}
		window := walkforward.Window{ID: windowID, Config: wfConfig}
		if err := walkforward.Save(windowDir, window, modelOut); err != nil {
			return fmt.Errorf("window %d/%d (%s): persist: %w", i+1, len(specs), windowID, err)
		}
		metrics := walkforward.MetricsFromResult(windowID, spec, *result)
		if err := walkforward.SaveMetrics(windowDir, metrics); err != nil {
			return fmt.Errorf("window %d/%d (%s): persist metrics: %w", i+1, len(specs), windowID, err)
		}

		log.Printf("walkforward: window %d/%d %s OOS %s..%s trades=%d hitrate=%.1f%% sharpe=%.2f netPnl=%s%s",
			i+1, len(specs), windowID, spec.Split.Format("2006-01-02"), spec.Till.Format("2006-01-02"),
			result.ClosedTrades, result.HitRate*100, result.Sharpe, result.NetPnl.StringFixed(2), significanceSuffix(result.StatisticallySignificant()))

		results = append(results, *result)
	}

	agg := walkforward.Aggregate(tickers, deposit, results)
	report := agg.Markdown()
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", outPath, err)
		}
		log.Printf("walkforward: aggregate report written to %s", outPath)
	} else {
		fmt.Print(report)
	}

	if tearsheetPath != "" {
		html, metricsJSON, err := tearsheet.Render(agg)
		if err != nil {
			return fmt.Errorf("render tearsheet: %w", err)
		}
		if err := os.WriteFile(tearsheetPath, html, 0o644); err != nil {
			return fmt.Errorf("write tearsheet %q: %w", tearsheetPath, err)
		}
		metricsPath := tearsheetPath + ".metrics.json"
		if err := os.WriteFile(metricsPath, metricsJSON, 0o644); err != nil {
			return fmt.Errorf("write tearsheet metrics %q: %w", metricsPath, err)
		}
		log.Printf("walkforward: tearsheet written to %s (metrics: %s)", tearsheetPath, metricsPath)
	}
	return nil
}

func significanceSuffix(significant bool) string {
	if significant {
		return ""
	}
	return " [below min-trades]"
}

type candidateSpec struct {
	Name          string  `json:"name"`
	BuyThreshold  float64 `json:"buy_threshold"`
	SellThreshold float64 `json:"sell_threshold"`
	HorizonDays   int     `json:"horizon_days"`
	DeadbandPct   float64 `json:"deadband_pct"`
}

func loadCandidates(path string) ([]candidateSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read candidates %q: %w", path, err)
	}
	var candidates []candidateSpec
	if err := json.Unmarshal(data, &candidates); err != nil {
		return nil, fmt.Errorf("parse candidates %q: %w", path, err)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("candidates %q: must contain at least one candidate", path)
	}
	seen := make(map[string]bool, len(candidates))
	for i, c := range candidates {
		if strings.TrimSpace(c.Name) == "" {
			return nil, fmt.Errorf("candidate %d: name must not be empty", i)
		}
		if seen[c.Name] {
			return nil, fmt.Errorf("candidate %d: duplicate name %q", i, c.Name)
		}
		seen[c.Name] = true
		if c.HorizonDays <= 0 {
			return nil, fmt.Errorf("candidate %q: horizon_days must be positive", c.Name)
		}
		if c.BuyThreshold <= 0 || c.BuyThreshold >= 1 {
			return nil, fmt.Errorf("candidate %q: buy_threshold must be in (0, 1)", c.Name)
		}
		if c.SellThreshold <= 0 || c.SellThreshold >= 1 {
			return nil, fmt.Errorf("candidate %q: sell_threshold must be in (0, 1)", c.Name)
		}
		if c.SellThreshold >= c.BuyThreshold {
			return nil, fmt.Errorf("candidate %q: sell_threshold must be below buy_threshold", c.Name)
		}
		if c.DeadbandPct < 0 {
			return nil, fmt.Errorf("candidate %q: deadband_pct must be non-negative", c.Name)
		}
	}
	return candidates, nil
}

type candidatesRunConfig struct {
	candidatesPath     string
	specs              []walkforward.WindowSpec
	tickers            []string
	deposit            decimal.Decimal
	commissionRate     decimal.Decimal
	maxLots            int
	labelCommissionPct float64
	labelMode          model.LabelMode
	intervalMin        int
	featureBPD         int
	trainCfg           model.TrainConfig
	newsHistory        string
	matrixOutPath      string
	wfDir              string
	pboKey             string
	pboSplits          int
}

func runCandidates(ctx context.Context, cfg candidatesRunConfig, source backtest.HistoricalSource) error {
	candidates, err := loadCandidates(cfg.candidatesPath)
	if err != nil {
		return err
	}
	log.Printf("walkforward: PBO grid over %d candidates x %d periods", len(candidates), len(cfg.specs))

	depositF, _ := cfg.deposit.Float64()
	matrix := make([][]float64, len(cfg.specs))
	for row := range matrix {
		matrix[row] = make([]float64, len(candidates))
	}

	for c, candidate := range candidates {
		for row, spec := range cfg.specs {
			_, result, err := trainrun.Run(ctx, trainrun.Config{
				Tickers:            cfg.tickers,
				From:               spec.From,
				Till:               spec.Till,
				Split:              spec.Split,
				HorizonDays:        candidate.HorizonDays,
				DeadbandPct:        candidate.DeadbandPct,
				LabelCommissionPct: cfg.labelCommissionPct,
				IntervalMin:        cfg.intervalMin,
				FeatureBPD:         cfg.featureBPD,
				TrainCfg:           cfg.trainCfg,
				BuyThreshold:       candidate.BuyThreshold,
				SellThreshold:      candidate.SellThreshold,
				MaxLots:            cfg.maxLots,
				Deposit:            cfg.deposit,
				CommissionRate:     cfg.commissionRate,
				NewsHistory:        cfg.newsHistory,
				LabelMode:          cfg.labelMode,
				Now:                time.Now,
			}, source, io.Discard)
			if err != nil {
				return fmt.Errorf("candidate %q period %d/%d: %w", candidate.Name, row+1, len(cfg.specs), err)
			}
			netPnl, _ := result.NetPnl.Float64()
			periodReturn := 0.0
			if depositF > 0 {
				periodReturn = netPnl / depositF
			}
			matrix[row][c] = periodReturn
		}
		log.Printf("walkforward: candidate %d/%d %q done", c+1, len(candidates), candidate.Name)
	}

	archive := map[string][][]float64{cfg.pboKey: matrix}
	payload, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal PBO matrix: %w", err)
	}
	payload = append(payload, '\n')
	matrixOutPath := cfg.matrixOutPath
	if matrixOutPath == "" {
		matrixOutPath = filepath.Join(cfg.wfDir, "pbo_matrix.json")
	}
	if err := os.MkdirAll(filepath.Dir(matrixOutPath), 0o755); err != nil {
		return fmt.Errorf("create matrix output dir: %w", err)
	}
	if err := os.WriteFile(matrixOutPath, payload, 0o644); err != nil {
		return fmt.Errorf("write PBO matrix %q: %w", matrixOutPath, err)
	}
	log.Printf("walkforward: period-return matrix (%d periods x %d candidates) written to %s", len(cfg.specs), len(candidates), matrixOutPath)

	splits := cfg.pboSplits
	if splits <= 0 {
		splits = strategyvalidation.DefaultSplits(len(cfg.specs))
	}
	if splits < 2 {
		log.Printf("walkforward: not enough periods (%d) to compute PBO; matrix archived for later use", len(cfg.specs))
		return nil
	}
	pboResult := strategyvalidation.ProbabilityOfBacktestOverfitting(matrix, splits)
	log.Printf("walkforward: PBO(%s, s=%d) = %.4f (%d/%d combinations overfit)", cfg.pboKey, splits, pboResult.PBO, pboResult.Overfit, pboResult.Trials)
	return nil
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
