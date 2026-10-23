package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/placebo"
)

const warmupDays = 100

type window struct {
	Name string
	From time.Time
	Till time.Time
}

type memSource struct {
	candles map[string][]moex.Candle
}

func (m *memSource) History(_ context.Context, ticker string, from, till time.Time) ([]moex.Candle, error) {
	var out []moex.Candle
	for _, c := range m.candles[ticker] {
		if !from.IsZero() && c.Begin.Before(from) {
			continue
		}
		if !till.IsZero() && c.Begin.After(till.AddDate(0, 0, 1)) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

type runParams struct {
	cfg      *config.Config
	tickers  []string
	source   backtest.HistoricalSource
	notional decimal.Decimal
	deposit  decimal.Decimal
	cap      decimal.Decimal
	borrow   decimal.Decimal
}

type outcome struct {
	Realized float64
	Closed   int
}

type cellReport struct {
	Window        string    `json:"window"`
	Placebo       string    `json:"placebo"`
	Borrow        string    `json:"borrow_per_day"`
	Real          float64   `json:"real_realized_net_borrow"`
	RealClosed    int       `json:"real_closed_trades"`
	N             int       `json:"n"`
	Mean          float64   `json:"placebo_mean"`
	Sd            float64   `json:"placebo_sd"`
	Q05           float64   `json:"q05"`
	Q50           float64   `json:"q50"`
	Q95           float64   `json:"q95"`
	P             float64   `json:"p"`
	MeanClosed    float64   `json:"placebo_mean_closed_trades"`
	Placebos      []float64 `json:"placebos"`
	RealPercent   float64   `json:"real_percentile_in_placebo"`
	SecondsPerRun float64   `json:"seconds_per_run"`
}

type report struct {
	Seed    int64        `json:"seed"`
	Cells   []cellReport `json:"cells"`
	Verdict []string     `json:"verdict"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tablePath, outPath, borrowList, tickersStr string
	var w2From, w2Till, w1From, w1Till, only string
	var n, workers int
	var seed int64
	var notionalStr, depositStr, capStr string
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tablePath, "score-table", "data/ridge_scores.csv", "ticker,date,p score table")
	flag.StringVar(&outPath, "out", "", "output path prefix for .json and .md (default: stdout markdown)")
	flag.StringVar(&borrowList, "borrow", "0.00005,0.00047", "comma-separated daily borrow rates")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (default: config tickers)")
	flag.StringVar(&w2From, "w2-from", "2025-03-05", "primary window start")
	flag.StringVar(&w2Till, "w2-till", "2026-10-18", "primary window end")
	flag.StringVar(&w1From, "w1-from", "2026-07-20", "descriptive window start")
	flag.StringVar(&w1Till, "w1-till", "2026-10-18", "descriptive window end")
	flag.StringVar(&only, "only", "", "run one window only: w1 or w2 (default both)")
	flag.IntVar(&n, "n", 200, "placebo runs per (placebo, rate, window)")
	flag.IntVar(&workers, "workers", 4, "parallel engine runs")
	flag.Int64Var(&seed, "seed", 20261019, "base seed")
	flag.StringVar(&notionalStr, "target-notional", "15000", "target notional per position")
	flag.StringVar(&depositStr, "deposit", "1000000", "starting deposit")
	flag.StringVar(&capStr, "cap", "90000", "max net long and max net short")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	notional, err := decimal.NewFromString(notionalStr)
	if err != nil {
		return err
	}
	deposit, err := decimal.NewFromString(depositStr)
	if err != nil {
		return err
	}
	capRub, err := decimal.NewFromString(capStr)
	if err != nil {
		return err
	}
	var borrows []decimal.Decimal
	for _, part := range splitComma(borrowList) {
		b, err := decimal.NewFromString(part)
		if err != nil {
			return fmt.Errorf("parse -borrow: %w", err)
		}
		borrows = append(borrows, b)
	}
	windows, err := parseWindows(w2From, w2Till, w1From, w1Till)
	if err != nil {
		return err
	}
	windows, err = selectWindows(windows, only)
	if err != nil {
		return err
	}

	base, err := model.LoadScoreTable(tablePath, 1000, notional)
	if err != nil {
		return err
	}
	table := placebo.Table(base.Series())

	earliest := windows[0].From
	latest := windows[0].Till
	for _, w := range windows {
		if w.From.Before(earliest) {
			earliest = w.From
		}
		if w.Till.After(latest) {
			latest = w.Till
		}
	}
	ctx := context.Background()
	log.SetOutput(io.Discard)
	issClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	iss := backtest.NewISSSource(cfg.MOEXISSBaseURL, issClient)
	mem := &memSource{candles: map[string][]moex.Candle{}}
	fetchFrom := earliest.AddDate(0, 0, -warmupDays-30)
	for _, ticker := range tickers {
		candles, err := iss.History(ctx, ticker, fetchFrom, latest.AddDate(0, 0, 1))
		if err != nil {
			return fmt.Errorf("history %s: %w", ticker, err)
		}
		mem.candles[ticker] = candles
	}

	rep := report{Seed: seed}
	for _, w := range windows {
		for _, b := range borrows {
			params := runParams{cfg: cfg, tickers: tickers, source: mem, notional: notional, deposit: deposit, cap: capRub, borrow: b}
			started := time.Now()
			real, err := runOnce(ctx, params, w, model.NewScoreTableSource(table, 1000, notional))
			if err != nil {
				return err
			}
			perRun := time.Since(started).Seconds()
			for _, kind := range []string{"shift", "permute"} {
				placebos, closedMean, err := runPlacebos(ctx, params, w, table, kind, n, workers, seed)
				if err != nil {
					return err
				}
				rep.Cells = append(rep.Cells, summarize(w.Name, kind, b.String(), real, placebos, closedMean, perRun))
			}
		}
	}
	rep.Verdict = verdict(rep.Cells)
	md := markdown(rep)
	if outPath == "" {
		fmt.Print(md)
		return nil
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath+".json", data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(outPath+".md", []byte(md), 0o644)
}
