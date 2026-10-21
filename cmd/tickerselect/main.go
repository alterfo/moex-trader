package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/placebo"
	"github.com/olegsidorkin/moex-trader/internal/tickersel"
)

const warmupDays = 100

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
	source   backtest.HistoricalSource
	table    placebo.Table
	notional decimal.Decimal
	deposit  decimal.Decimal
	cap      decimal.Decimal
	borrow   decimal.Decimal
}

type outcome struct {
	Total  float64
	Ticker map[string]float64
}

func runBlock(ctx context.Context, p runParams, tickers []string, b tickersel.Block) (outcome, error) {
	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:         tickers,
		From:            b.From,
		Till:            b.Till,
		Deposit:         p.deposit,
		MaxLots:         1000,
		MaxNetShort:     p.cap,
		MaxNetLong:      p.cap,
		SectorCaps:      p.cfg.Risk.SectorCaps,
		Sectors:         p.cfg.Risk.Sectors,
		CommissionRate:  decimal.RequireFromString("0.0005"),
		SpreadPct:       decimal.RequireFromString("0.0005"),
		SlippagePct:     decimal.RequireFromString("0.0005"),
		BorrowPctPerDay: p.borrow,
		WarmupDays:      warmupDays,
		KillSwitch:      true,
		SignalSource:    model.NewScoreTableSource(p.table, 1000, p.notional),
		Source:          p.source,
		FeatureConfig:   features.PriceFeatureConfig{},
	})
	if err != nil {
		return outcome{}, err
	}
	result, err := engine.Run(ctx)
	if err != nil {
		return outcome{}, err
	}
	total, _ := result.RealizedPnlNetBorrow.Float64()
	per := make(map[string]float64, len(tickers))
	for _, tr := range result.Trades {
		v, _ := tr.NetPnl.Float64()
		per[tr.Ticker] += v
	}
	return outcome{Total: total, Ticker: per}, nil
}

type rateReport struct {
	Borrow       string     `json:"borrow_per_day"`
	Blocks       int        `json:"evaluated_blocks"`
	Full         float64    `json:"full"`
	WalkForward  float64    `json:"walk_forward"`
	InSample     float64    `json:"in_sample_leak"`
	RandomMean   float64    `json:"random_mean"`
	RandomSd     float64    `json:"random_sd"`
	RandomQ05    float64    `json:"random_q05"`
	RandomQ50    float64    `json:"random_q50"`
	RandomQ95    float64    `json:"random_q95"`
	P            float64    `json:"p_walk_forward_vs_random"`
	PInSample    float64    `json:"p_in_sample_vs_random"`
	MeanSpearman float64    `json:"mean_spearman_prev_next_block"`
	MeanOverlap  float64    `json:"mean_dropped_in_next_bottom_k"`
	Random       []float64  `json:"random"`
	WalkDropped  [][]string `json:"walk_forward_dropped"`
	FullPerBlock []float64  `json:"full_per_block"`
	WalkPerBlock []float64  `json:"walk_forward_per_block"`
}

type report struct {
	Seed    int64        `json:"seed"`
	K       int          `json:"k"`
	Block   int          `json:"block_days"`
	Rates   []rateReport `json:"rates"`
	Verdict []string     `json:"verdict"`
}

func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parallel(n, workers int, fn func(i int) error) error {
	errs := make([]error, n)
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			errs[i] = fn(i)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func evaluate(ctx context.Context, p runParams, tickers []string, blocks []tickersel.Block, k, n, workers int, seed int64) (rateReport, error) {
	rep := rateReport{Borrow: p.borrow.String(), Blocks: len(blocks) - 1}
	full := make([]outcome, len(blocks))
	if err := parallel(len(blocks), workers, func(i int) error {
		o, err := runBlock(ctx, p, tickers, blocks[i])
		full[i] = o
		return err
	}); err != nil {
		return rep, err
	}
	rep.Random = make([]float64, n)
	var spearman, overlap float64
	for j := 1; j < len(blocks); j++ {
		rep.Full += full[j].Total
		rep.FullPerBlock = append(rep.FullPerBlock, full[j].Total)
		dropWF := tickersel.WorstK(tickers, full[j-1].Ticker, k)
		dropIS := tickersel.WorstK(tickers, full[j].Ticker, k)
		rep.WalkDropped = append(rep.WalkDropped, dropWF)
		spearman += tickersel.Spearman(tickers, full[j-1].Ticker, full[j].Ticker)
		overlap += tickersel.Overlap(dropWF, tickers, full[j].Ticker)
		wf, err := runBlock(ctx, p, tickersel.Keep(tickers, dropWF), blocks[j])
		if err != nil {
			return rep, err
		}
		is, err := runBlock(ctx, p, tickersel.Keep(tickers, dropIS), blocks[j])
		if err != nil {
			return rep, err
		}
		rep.WalkForward += wf.Total
		rep.WalkPerBlock = append(rep.WalkPerBlock, wf.Total)
		rep.InSample += is.Total
		draws := make([]float64, n)
		if err := parallel(n, workers, func(i int) error {
			rng := rand.New(rand.NewSource(seed + int64(i)*1000 + int64(j)))
			o, err := runBlock(ctx, p, tickersel.Keep(tickers, tickersel.RandomDrop(tickers, k, rng)), blocks[j])
			draws[i] = o.Total
			return err
		}); err != nil {
			return rep, err
		}
		for i := range draws {
			rep.Random[i] += draws[i]
		}
	}
	m := float64(len(blocks) - 1)
	rep.MeanSpearman = spearman / m
	rep.MeanOverlap = overlap / m
	sorted := append([]float64(nil), rep.Random...)
	sort.Float64s(sorted)
	for _, v := range rep.Random {
		rep.RandomMean += v
	}
	rep.RandomMean /= float64(n)
	for _, v := range rep.Random {
		rep.RandomSd += (v - rep.RandomMean) * (v - rep.RandomMean)
	}
	if n > 1 {
		rep.RandomSd = sqrt(rep.RandomSd / float64(n-1))
	}
	rep.RandomQ05, rep.RandomQ50, rep.RandomQ95 = quantile(sorted, 0.05), quantile(sorted, 0.5), quantile(sorted, 0.95)
	rep.P = placebo.PValue(rep.WalkForward, rep.Random)
	rep.PInSample = placebo.PValue(rep.InSample, rep.Random)
	return rep, nil
}

func verdict(rates []rateReport) []string {
	if len(rates) == 0 {
		return []string{"no rates evaluated"}
	}
	sort.Slice(rates, func(i, j int) bool {
		a, _ := decimal.NewFromString(rates[i].Borrow)
		b, _ := decimal.NewFromString(rates[j].Borrow)
		return a.LessThan(b)
	})
	ok := func(r rateReport) bool { return r.WalkForward > r.Full && r.P <= 0.05 }
	high, low := rates[len(rates)-1], rates[0]
	switch {
	case ok(high):
		return []string{"ticker filter works: walk-forward beats full and random drops at borrow " + high.Borrow}
	case ok(low):
		return []string{"limited by the rate: works at borrow " + low.Borrow + " but not at " + high.Borrow}
	default:
		return []string{"dropping losers is indistinguishable from dropping random tickers; the filter is not adopted"}
	}
}

func markdown(r report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Ticker selection check\n\nSeed base: %d, K=%d, block %d days. Statistic: sum over evaluated blocks of realized P&L net of borrow, RUB.\n\n", r.Seed, r.K, r.Block)
	b.WriteString("| borrow/day | blocks | full | walk-forward | in-sample (leak) | random mean | random sd | q05 | q50 | q95 | p wf | p in-sample | mean Spearman | mean overlap |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, c := range r.Rates {
		fmt.Fprintf(&b, "| %s | %d | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.4f | %.4f | %.3f | %.3f |\n",
			c.Borrow, c.Blocks, c.Full, c.WalkForward, c.InSample, c.RandomMean, c.RandomSd, c.RandomQ05, c.RandomQ50, c.RandomQ95, c.P, c.PInSample, c.MeanSpearman, c.MeanOverlap)
	}
	b.WriteString("\nVerdict:\n\n")
	for _, v := range r.Verdict {
		fmt.Fprintf(&b, "- %s\n", v)
	}
	return b.String()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var configPath, tablePath, outPath, borrowList, tickersStr, from, till string
	var n, workers, k, blockDays int
	var seed int64
	var notionalStr, depositStr, capStr string
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tablePath, "score-table", "data/ridge_scores.csv", "ticker,date,p score table")
	flag.StringVar(&outPath, "out", "", "output path prefix for .json and .md (default: stdout markdown)")
	flag.StringVar(&borrowList, "borrow", "0.00005,0.00047", "comma-separated daily borrow rates")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (default: config tickers)")
	flag.StringVar(&from, "from", "2025-03-05", "window start")
	flag.StringVar(&till, "till", "2026-10-18", "window end")
	flag.IntVar(&n, "n", 200, "random-drop draws")
	flag.IntVar(&k, "k", 4, "tickers dropped per block")
	flag.IntVar(&blockDays, "block-days", 91, "calendar days per block")
	flag.IntVar(&workers, "workers", 4, "parallel engine runs")
	flag.Int64Var(&seed, "seed", 20261022, "base seed")
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
	fromT, err := time.Parse("2006-01-02", from)
	if err != nil {
		return fmt.Errorf("parse -from: %w", err)
	}
	tillT, err := time.Parse("2006-01-02", till)
	if err != nil {
		return fmt.Errorf("parse -till: %w", err)
	}
	blocks := tickersel.Blocks(fromT, tillT, blockDays)
	if len(blocks) < 2 {
		return fmt.Errorf("need at least 2 blocks, got %d", len(blocks))
	}
	if k <= 0 || k >= len(tickers) {
		return fmt.Errorf("-k must be in 1..%d", len(tickers)-1)
	}

	base, err := model.LoadScoreTable(tablePath, 1000, notional)
	if err != nil {
		return err
	}
	table := placebo.Table(base.Series())

	ctx := context.Background()
	log.SetOutput(io.Discard)
	issClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	iss := backtest.NewISSSource(cfg.MOEXISSBaseURL, issClient)
	mem := &memSource{candles: map[string][]moex.Candle{}}
	fetchFrom := fromT.AddDate(0, 0, -warmupDays-30)
	for _, ticker := range tickers {
		candles, err := iss.History(ctx, ticker, fetchFrom, tillT.AddDate(0, 0, 1))
		if err != nil {
			return fmt.Errorf("history %s: %w", ticker, err)
		}
		mem.candles[ticker] = candles
	}

	rep := report{Seed: seed, K: k, Block: blockDays}
	for _, b := range borrows {
		params := runParams{cfg: cfg, source: mem, table: table, notional: notional, deposit: deposit, cap: capRub, borrow: b}
		cell, err := evaluate(ctx, params, tickers, blocks, k, n, workers, seed)
		if err != nil {
			return err
		}
		rep.Rates = append(rep.Rates, cell)
	}
	rep.Verdict = verdict(rep.Rates)
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
