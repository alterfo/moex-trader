package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/spread"
	"github.com/olegsidorkin/moex-trader/internal/storage"
	"github.com/olegsidorkin/moex-trader/internal/tearsheet"
	"github.com/olegsidorkin/moex-trader/internal/walkforward"
)

const (
	signalSourceModel    = "model"
	signalSourceRule     = "rule"
	signalSourceCSVProb  = "csvprob"
	signalSourceEnsemble = "ensemble"
)

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
	var spreadDBPath string
	var spreadMinObs int
	var borrowPctDayStr string
	var maxNetExposureStr string
	var lookbackDays int
	var maxHoldBars int
	var cachePath string
	var outPath string
	var tearsheetPath string
	var killSwitch bool
	var signalSourceName string
	var modelPath string
	var reversalThresholdPct float64
	var minConfidence float64
	var csvProbPath string
	var csvProbCol string
	var csvBuyPct, csvSellPct float64
	var ensemblePath string
	var newsHistory string
	var intervalMin int
	var featureBPD int
	var targetNotionalStr string
	var volScale bool
	var volScaleMinStr string
	var volScaleMaxStr string
	var wfDir string
	var embargoBars int
	var ensembleMember string
	var labelMode string
	var labelHorizonBars int
	var labelDeadbandPct float64

	flag.StringVar(&configPath, "config", "config.yaml", "path to config YAML")
	flag.StringVar(&fromStr, "from", "", "backtest start date YYYY-MM-DD (default: one year ago)")
	flag.StringVar(&tillStr, "till", "", "backtest end date YYYY-MM-DD (default: today)")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&depositStr, "deposit", "100000", "starting paper deposit in RUB")
	flag.IntVar(&maxLots, "max-lots", 1, "max position in lots")
	flag.StringVar(&commissionStr, "commission-rate", "0.0005", "commission rate applied to notional per fill")
	flag.StringVar(&spreadStr, "spread-pct", "0", "half-spread cost applied against each fill, as a fraction of price (e.g. 0.0005 = 0.05%)")
	flag.StringVar(&spreadDBPath, "spread-db", "", "path to trader audit SQLite DB from which per-ticker half-spreads are derived")
	flag.IntVar(&spreadMinObs, "spread-min-obs", 1, "minimum ingest observations required before a per-ticker spread overrides -spread-pct")
	flag.StringVar(&slippageStr, "slippage-pct", "0", "additional adverse slippage applied against each fill, as a fraction of price (e.g. 0.0005 = 0.05%)")
	flag.StringVar(&borrowPctDayStr, "borrow-pct-day", "0", "short-borrow cost per day as a fraction of short-leg notional (e.g. 0.00005 = 0.005%)")
	flag.StringVar(&maxNetExposureStr, "max-net-exposure", "0", "cap on aggregate signed net position notional in RUB (0 = disabled)")
	flag.IntVar(&lookbackDays, "lookback-days", 30, "max decision points per ticker (0 = unlimited)")
	flag.IntVar(&maxHoldBars, "max-hold-bars", 0, "force-close a position after this many decision bars (0 = hold until the signal changes)")
	flag.StringVar(&cachePath, "cache", "", "path to persistent decision cache (e.g. .backtest-cache.json)")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: -)")
	flag.StringVar(&tearsheetPath, "tearsheet", "", "when set, write an HTML tearsheet to this path plus a <path>.metrics.json summary")
	flag.BoolVar(&killSwitch, "kill-switch", true, "enable drawdown kill switch")
	flag.StringVar(&signalSourceName, "signal-source", signalSourceModel, "signal source: model, rule, csvprob or ensemble")
	flag.StringVar(&modelPath, "model-path", "", "path to trained model JSON (default: model.path from config)")
	flag.Float64Var(&reversalThresholdPct, "reversal-threshold", 0.5, "rule-only: |reversal_1d| %% required to trade")
	flag.Float64Var(&minConfidence, "min-confidence", 0, "demote BUY/SELL to HOLD when confidence below threshold (0 = off)")
	flag.StringVar(&csvProbPath, "csv-prob-path", "", "csvprob: path to ticker,date,p_<col> probability CSV")
	flag.StringVar(&csvProbCol, "csv-prob-col", "p_logreg", "csvprob: probability column name")
	flag.Float64Var(&csvBuyPct, "csv-buy-pct", 0.55, "csvprob: probability at/above which to BUY")
	flag.Float64Var(&csvSellPct, "csv-sell-pct", 0.45, "csvprob: probability at/below which to SELL")
	flag.StringVar(&ensemblePath, "ensemble-path", "", "ensemble: path to exported LGBM+XGB+LogReg model JSON")
	flag.StringVar(&newsHistory, "news-history", "", "load historical news sentiment/count overrides from finanalys-format JSONL")
	flag.IntVar(&intervalMin, "interval-min", 0, "candle interval in minutes for intraday bars (24 or 0 = daily; ISS supports 1/10/60)")
	flag.IntVar(&featureBPD, "feature-bars-per-day", 0, "scale day-named feature windows by this many bars/session (0 = keep raw bar-count windows; -1 = auto/calendar from -interval-min)")
	flag.StringVar(&targetNotionalStr, "target-notional", "", "ensemble: target ruble notional per position (when set, TargetLots=max(1, round(notional/price)); override MaxLots)")
	flag.BoolVar(&volScale, "vol-scale", false, "scale target notional by clamp(median realized_volatility / ticker realized_volatility, min, max)")
	flag.StringVar(&volScaleMinStr, "vol-scale-min", "", "vol-scale minimum multiplier (required when -vol-scale is set)")
	flag.StringVar(&volScaleMaxStr, "vol-scale-max", "", "vol-scale maximum multiplier (required when -vol-scale is set)")
	flag.StringVar(&wfDir, "wf-dir", "", "when set, persist the per-window model and decision log under <wf-dir>/<from>_<till>/ for walk-forward reproducibility")
	flag.IntVar(&embargoBars, "embargo-bars", 0, "embargo bars recorded in the persisted walk-forward window (no effect on a standalone backtest)")
	flag.StringVar(&ensembleMember, "ensemble-member", "", "ensemble member to trade: empty (all), lgb, xgb or logistic")
	flag.StringVar(&labelMode, "label-mode", "absolute", "label mode used for persisted reliability labels: absolute or excess")
	flag.IntVar(&labelHorizonBars, "label-horizon-bars", 10, "forward-return horizon in bars used for persisted reliability labels")
	flag.Float64Var(&labelDeadbandPct, "label-deadband-pct", 0.5, "deadband percent used for persisted reliability labels")
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
	maxNetExposure, err := decimal.NewFromString(maxNetExposureStr)
	if err != nil {
		return fmt.Errorf("parse -max-net-exposure: %w", err)
	}
	var targetNotional decimal.Decimal
	if targetNotionalStr != "" {
		targetNotional, err = decimal.NewFromString(targetNotionalStr)
		if err != nil {
			return fmt.Errorf("parse -target-notional: %w", err)
		}
	}
	var volScaleCfg model.VolScale
	if volScale {
		if strings.TrimSpace(volScaleMinStr) == "" || strings.TrimSpace(volScaleMaxStr) == "" {
			return fmt.Errorf("-vol-scale-min and -vol-scale-max are required when -vol-scale is set")
		}
		volScaleCfg.Enabled = true
		volScaleCfg.MinMult, err = decimal.NewFromString(volScaleMinStr)
		if err != nil {
			return fmt.Errorf("parse -vol-scale-min: %w", err)
		}
		volScaleCfg.MaxMult, err = decimal.NewFromString(volScaleMaxStr)
		if err != nil {
			return fmt.Errorf("parse -vol-scale-max: %w", err)
		}
		if !volScaleCfg.MinMult.IsPositive() {
			return fmt.Errorf("-vol-scale-min must be positive")
		}
		if volScaleCfg.MaxMult.LessThan(volScaleCfg.MinMult) {
			return fmt.Errorf("-vol-scale-max must not be below -vol-scale-min")
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if modelPath == "" {
		modelPath = cfg.Model.Path
	}

	var spreadPcts map[string]decimal.Decimal
	if strings.TrimSpace(spreadDBPath) != "" {
		spreadPcts, err = loadPerTickerSpreads(spreadDBPath, spreadMinObs)
		if err != nil {
			return err
		}
	}

	var from, till time.Time
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return fmt.Errorf("parse -from: %w", err)
		}
	}
	if tillStr != "" {
		till, err = time.Parse("2006-01-02", tillStr)
		if err != nil {
			return fmt.Errorf("parse -till: %w", err)
		}
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	var source backtest.HistoricalSource
	resolvedBPD := featureBPD
	if resolvedBPD < 0 {
		resolvedBPD = features.ConfigForInterval(intervalMin).BarsPerDay
	}
	if intervalMin > 0 && intervalMin != 24 {
		source = backtest.NewISSSourceInterval(cfg.MOEXISSBaseURL, moexClient, intervalMin)
		log.Printf("backtest: using intraday interval %d min; feature barsPerDay=%d (0 = raw bar-count windows)", intervalMin, resolvedBPD)
	} else {
		source = backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	}

	var newsOverrides map[string]map[string]backtest.NewsAggregate
	var eventOverrides map[string]map[string]features.EventFlags
	var topicSignalOverrides map[string]map[string]features.TopicSignalAggregate
	if newsHistory != "" {
		overrides, err := backtest.LoadNewsOverrides(newsHistory)
		if err != nil {
			return err
		}
		newsOverrides = overrides.News
		eventOverrides = overrides.Events
		topicSignalOverrides = overrides.Topics
		log.Printf("backtest: loaded %d tickers of news + event + topic-signal overrides from %s", len(newsOverrides), newsHistory)
	}

	signalSource, saveCache, err := buildSignalSource(cfg, signalSourceOptions{
		Mode:                 signalSourceName,
		ModelPath:            modelPath,
		MaxLots:              maxLots,
		CachePath:            cachePath,
		ReversalThresholdPct: reversalThresholdPct,
		MinConfidence:        minConfidence,
		CSVProbPath:          csvProbPath,
		CSVProbCol:           csvProbCol,
		CSVBuyPct:            csvBuyPct,
		CSVSellPct:           csvSellPct,
		EnsemblePath:         ensemblePath,
		TargetNotional:       targetNotional,
		EnsembleMember:       model.EnsembleMember(ensembleMember),
		VolScale:             volScaleCfg,
	})
	if err != nil {
		return err
	}
	if saveCache != nil {
		defer func() {
			if err := saveCache(); err != nil {
				log.Printf("save decision cache: %v", err)
			}
		}()
	}

	var recorder *walkforward.Recorder
	var wfConfig walkforward.Config
	var wfConfigHash string
	var wfModelFile string
	var wfStart, wfEnd time.Time
	if strings.TrimSpace(wfDir) != "" {
		wfStart = from
		if wfStart.IsZero() {
			wfStart = time.Now().AddDate(0, 0, -365)
		}
		wfEnd = till
		if wfEnd.IsZero() {
			wfEnd = time.Now()
		}

		_, canonicalOrder := model.ToVector(domain.FeatureContext{})
		wfConfig = walkforward.Config{
			WindowStart:      wfStart,
			WindowEnd:        wfEnd,
			SignalSource:     signalSourceName,
			Deposit:          deposit.String(),
			MaxLots:          maxLots,
			CommissionRate:   commissionRate.String(),
			SpreadPct:        spreadPct.String(),
			SlippagePct:      slippagePct.String(),
			BorrowPctPerDay:  borrowPctPerDay.String(),
			TargetNotional:   targetNotional.String(),
			Tickers:          append([]string(nil), tickers...),
			FeatureOrder:     append([]string(nil), canonicalOrder...),
			SpreadMinObs:     spreadMinObs,
			SpreadDBPath:     spreadDBPath,
			EmbargoBars:      embargoBars,
			EnsembleMember:   ensembleMember,
			LabelMode:        labelMode,
			LabelHorizonBars: labelHorizonBars,
			LabelDeadbandPct: labelDeadbandPct,
		}
		if volScaleCfg.Enabled {
			wfConfig.VolScaleEnabled = true
			wfConfig.VolScaleMinMult = volScaleCfg.MinMult.String()
			wfConfig.VolScaleMaxMult = volScaleCfg.MaxMult.String()
		}

		var probabilityProvider walkforward.ProbabilityProvider
		switch signalSourceName {
		case signalSourceEnsemble:
			ens, err := model.LoadEnsembleModel(ensemblePath)
			if err != nil {
				return err
			}
			wfConfig.ModelPath = ensemblePath
			wfConfig.FeatureOrder = append([]string(nil), ens.FeatureOrder...)
			wfConfig.BuyThreshold = ens.BuyThreshold
			wfConfig.SellThreshold = ens.SellThreshold
			wfModelFile = ensemblePath
			probabilityProvider = &model.EnsembleSignalSource{Model: ens, Member: model.EnsembleMember(ensembleMember)}
		case signalSourceModel:
			weights, err := model.LoadWeights(modelPath)
			if err != nil {
				return err
			}
			wfConfig.ModelPath = modelPath
			wfConfig.FeatureOrder = append([]string(nil), weights.FeatureOrder...)
			wfConfig.BuyThreshold = weights.BuyThreshold
			wfConfig.SellThreshold = weights.SellThreshold
			wfModelFile = modelPath
			probabilityProvider = &model.SignalSource{Weights: weights}
		case signalSourceCSVProb:
			wfConfig.ModelPath = csvProbPath
			wfConfig.BuyThreshold = csvBuyPct
			wfConfig.SellThreshold = csvSellPct
			wfModelFile = csvProbPath
		}
		wfConfigHash = walkforward.ConfigHash(wfConfig)
		recorder = walkforward.NewRecorder(signalSource, probabilityProvider, wfConfigHash)
		signalSource = recorder
	}

	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:               tickers,
		From:                  from,
		Till:                  till,
		Deposit:               deposit,
		MaxLots:               maxLots,
		MaxNetExposure:        maxNetExposure,
		CommissionRate:        commissionRate,
		SpreadPct:             spreadPct,
		SpreadPcts:            spreadPcts,
		SlippagePct:           slippagePct,
		BorrowPctPerDay:       borrowPctPerDay,
		WarmupDays:            100,
		MaxDecisionsPerTicker: lookbackDays,
		MaxHoldBars:           maxHoldBars,
		KillSwitch:            killSwitch,
		SignalSource:          signalSource,
		Source:                source,
		FeatureConfig:         features.PriceFeatureConfig{BarsPerDay: resolvedBPD},
		NewsOverrides:         newsOverrides,
		EventOverrides:        eventOverrides,
		TopicSignalOverrides:  topicSignalOverrides,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("backtest: window=%s..%s tickers=%d deposit=%s lots=%d commission=%s spread=%s per_ticker_spreads=%d slippage=%s borrow_per_day=%s signal_source=%s lookback=%d kill_switch=%v min_confidence=%s",
		formatFlag(from), formatFlag(till), len(tickers), deposit.String(), maxLots, commissionRate.String(), spreadPct.String(), len(spreadPcts), slippagePct.String(), borrowPctPerDay.String(), signalSourceName, lookbackDays, killSwitch, decimal.NewFromFloat(minConfidence).String())

	result, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("backtest run: %w", err)
	}

	if recorder != nil {
		window := walkforward.Window{
			ID:         walkforward.NewWindowID(wfStart, wfEnd),
			Config:     wfConfig,
			ConfigHash: wfConfigHash,
			Decisions:  recorder.Decisions(),
		}
		returns := walkforward.DailyRealizedPnl(*result)
		if err := window.SetPeriodReturns(returns); err != nil {
			return fmt.Errorf("persist period returns: %w", err)
		}
		windowDir := filepath.Join(wfDir, window.ID)
		if err := walkforward.SavePeriodReturns(windowDir, returns); err != nil {
			return fmt.Errorf("persist walk-forward period returns: %w", err)
		}
		requests := make([]model.DecisionLabelRequest, 0, len(window.Decisions))
		for _, decision := range window.Decisions {
			requests = append(requests, model.DecisionLabelRequest{
				Ticker:      decision.Ticker,
				Date:        decision.Date,
				Probability: decision.Probability,
			})
		}
		labeled, err := model.RealizedReliabilityPairs(ctx, source, requests, labelHorizonBars, labelDeadbandPct, model.LabelMode(labelMode))
		if err != nil {
			return fmt.Errorf("persist reliability labels: %w", err)
		}
		reliabilityRows := make([]walkforward.ReliabilityRow, 0, len(labeled))
		for _, row := range labeled {
			reliabilityRows = append(reliabilityRows, walkforward.ReliabilityRow{
				Date:        row.Date,
				Ticker:      row.Ticker,
				Probability: row.Probability,
				Label:       row.Label,
			})
		}
		if err := window.SetReliabilityRows(reliabilityRows); err != nil {
			return fmt.Errorf("persist reliability metadata: %w", err)
		}
		if err := walkforward.SaveReliability(windowDir, reliabilityRows); err != nil {
			return fmt.Errorf("persist walk-forward reliability: %w", err)
		}
		if err := walkforward.Save(windowDir, window, wfModelFile); err != nil {
			return fmt.Errorf("persist walk-forward window: %w", err)
		}
		log.Printf("backtest: persisted walk-forward window %s with %d decisions and %d reliability rows", windowDir, len(window.Decisions), len(reliabilityRows))
	}

	report := result.Markdown()
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", outPath, err)
		}
		log.Printf("backtest: report written to %s", outPath)
	} else {
		fmt.Print(report)
	}

	if tearsheetPath != "" {
		html, metricsJSON, err := tearsheet.Render(*result)
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
		log.Printf("backtest: tearsheet written to %s (metrics: %s)", tearsheetPath, metricsPath)
	}
	return nil
}

type signalSourceOptions struct {
	Mode                 string
	ModelPath            string
	MaxLots              int
	CachePath            string
	ReversalThresholdPct float64
	MinConfidence        float64
	CSVProbPath          string
	CSVProbCol           string
	CSVBuyPct            float64
	CSVSellPct           float64
	EnsemblePath         string
	TargetNotional       decimal.Decimal
	EnsembleMember       model.EnsembleMember
	VolScale             model.VolScale
}

func buildSignalSource(cfg *config.Config, opts signalSourceOptions) (backtest.SignalSource, func() error, error) {
	var source backtest.SignalSource
	var saveCache func() error
	switch opts.Mode {
	case signalSourceModel:
		weights, err := model.LoadWeights(opts.ModelPath)
		if err != nil {
			return nil, nil, fmt.Errorf("load model: %w", err)
		}
		source = &model.SignalSource{Weights: weights, MaxLots: opts.MaxLots, TargetNotional: opts.TargetNotional, VolScale: opts.VolScale}
	case signalSourceRule:
		threshold := backtestDecimal(opts.ReversalThresholdPct)
		source = &backtest.ReversalRuleSource{Threshold: threshold, MaxLots: opts.MaxLots}
	case signalSourceCSVProb:
		if strings.TrimSpace(opts.CSVProbPath) == "" {
			return nil, nil, fmt.Errorf("csvprob: -csv-prob-path is required")
		}
		src, err := newCSVProbSource(opts.CSVProbPath, opts.CSVProbCol, opts.CSVBuyPct, opts.CSVSellPct, opts.MaxLots, opts.TargetNotional)
		if err != nil {
			return nil, nil, err
		}
		source = src
	case signalSourceEnsemble:
		if strings.TrimSpace(opts.EnsemblePath) == "" {
			return nil, nil, fmt.Errorf("ensemble: -ensemble-path is required")
		}
		m, err := model.LoadEnsembleModel(opts.EnsemblePath)
		if err != nil {
			return nil, nil, err
		}
		source = &model.EnsembleSignalSource{Model: m, MaxLots: opts.MaxLots, TargetNotional: opts.TargetNotional, Member: opts.EnsembleMember, VolScale: opts.VolScale}
	default:
		return nil, nil, fmt.Errorf("unknown signal source %q: want %q, %q, %q or %q", opts.Mode, signalSourceModel, signalSourceRule, signalSourceCSVProb, signalSourceEnsemble)
	}

	if opts.CachePath != "" {
		cache, err := backtest.NewCachedSignalSource(source, opts.CachePath)
		if err != nil {
			return nil, nil, err
		}
		source = cache
		saveCache = cache.Save
	}

	if opts.MinConfidence > 0 {
		source = &backtest.ConfidenceGateSource{
			Inner:         source,
			MinConfidence: decimal.NewFromFloat(opts.MinConfidence),
		}
	}
	return source, saveCache, nil
}

func backtestDecimal(v float64) decimal.Decimal {
	return decimal.NewFromFloat(v)
}

type csvProbSource struct {
	probs          map[string]float64
	buyPct         float64
	sellPct        float64
	maxLots        int
	targetNotional decimal.Decimal
}

func newCSVProbSource(path, col string, buyPct, sellPct float64, maxLots int, targetNotional decimal.Decimal) (*csvProbSource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("csvprob: open %q: %w", path, err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csvprob: read %q: %w", path, err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("csvprob: %q has no data rows", path)
	}
	header := rows[0]
	colIdx := -1
	for i, name := range header {
		if name == col {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		return nil, fmt.Errorf("csvprob: column %q not found in %q (have: %s)", col, path, strings.Join(header, ", "))
	}
	tickerIdx, dateIdx := -1, -1
	for i, name := range header {
		switch name {
		case "ticker":
			tickerIdx = i
		case "date":
			dateIdx = i
		}
	}
	if tickerIdx < 0 || dateIdx < 0 {
		return nil, fmt.Errorf("csvprob: %q must have ticker and date columns", path)
	}
	probs := make(map[string]float64, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) <= colIdx || len(row) <= tickerIdx || len(row) <= dateIdx {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(row[colIdx]), 64)
		if err != nil {
			return nil, fmt.Errorf("csvprob: row with date %q: parse %q: %w", row[dateIdx], row[colIdx], err)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(row[tickerIdx])) + "|" + strings.TrimSpace(row[dateIdx])
		probs[key] = value
	}
	return &csvProbSource{probs: probs, buyPct: buyPct, sellPct: sellPct, maxLots: maxLots, targetNotional: targetNotional}, nil
}

func (s *csvProbSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	key := strings.ToUpper(strings.TrimSpace(feature.Ticker)) + "|" + feature.GeneratedAt.Format("2006-01-02")
	probability, ok := s.probs[key]
	if !ok {
		return domain.TradeSignal{
			Ticker:      feature.Ticker,
			Action:      domain.ActionHold,
			Confidence:  decimal.Zero,
			TargetLots:  0,
			GeneratedAt: feature.GeneratedAt,
			HoldReason:  domain.HoldReasonModel,
		}, nil
	}
	signal := domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		Confidence:  decimal.NewFromFloat(math.Abs(probability-0.5) * 2),
		TargetLots:  0,
		GeneratedAt: feature.GeneratedAt,
		Reasoning:   fmt.Sprintf("csvprob:p=%.4f", probability),
	}
	switch {
	case probability >= s.buyPct:
		signal.Action = domain.ActionBuy
		signal.TargetLots = s.lotsFor(feature)
	case probability <= s.sellPct:
		signal.Action = domain.ActionSell
		signal.TargetLots = s.lotsFor(feature)
	default:
		signal.HoldReason = domain.HoldReasonModel
	}
	return signal, nil
}

// lotsFor sizes a position either from the fixed maxLots or, when a target
// notional is set, from that notional divided by the per-lot price so a
// cross-sectional book gets roughly equal ruble exposure per name.
func (s *csvProbSource) lotsFor(feature domain.FeatureContext) int {
	if !s.targetNotional.IsPositive() || !feature.LastPrice.IsPositive() {
		return s.maxLots
	}
	perUnit := feature.LastPrice
	if feature.LotSize.IsPositive() {
		perUnit = feature.LastPrice.Mul(feature.LotSize)
	}
	lots := s.targetNotional.Div(perUnit).Round(0).IntPart()
	if lots < 1 {
		return 1
	}
	return int(lots)
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

func formatFlag(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func loadPerTickerSpreads(path string, minObservations int) (map[string]decimal.Decimal, error) {
	store, err := storage.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open spread audit db: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("close spread audit db: %v", closeErr)
		}
	}()

	events, err := store.ListAllAuditEvents(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list spread audit events: %w", err)
	}
	table, err := spread.FromAuditEvents(events, spread.Options{MinObservations: minObservations})
	if err != nil {
		return nil, fmt.Errorf("derive per-ticker spreads: %w", err)
	}
	if len(table) == 0 {
		return nil, fmt.Errorf("no per-ticker spreads derivable from %s", path)
	}
	log.Printf("backtest: derived %d per-ticker half-spreads from %s (min observations=%d)", len(table), path, minObservations)
	return map[string]decimal.Decimal(table), nil
}
