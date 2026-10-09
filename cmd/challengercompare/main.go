package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, logPath string
	var horizon, reps, minPairs, minDates int
	var seed int64
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&logPath, "log", "artifacts/challenger/decisions.jsonl", "challenger decision log written by cmd/trader")
	flag.IntVar(&horizon, "horizon", 10, "forward return horizon in trading days")
	flag.IntVar(&reps, "reps", 2000, "bootstrap replicates")
	flag.IntVar(&minPairs, "min-pairs", 200, "minimum paired decisions before a verdict is given")
	flag.IntVar(&minDates, "min-dates", 40, "minimum distinct dates before a verdict is given")
	flag.Int64Var(&seed, "seed", 42, "RNG seed")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	records, err := loadRecords(logPath)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("no records in %s", logPath)
	}
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return err
	}
	from := records[0].Time.AddDate(0, 0, -5)
	series, err := loadSeries(context.Background(), cfg.MOEXISSBaseURL, records, from, time.Now().AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	pairs, skipped, errs := buildPairs(records, series, horizon, loc)
	s := summarize(pairs, skipped, errs, reps, rand.New(rand.NewSource(seed)))
	fmt.Printf("records: %d, paired decisions: %d on %d dates (skipped without a closed %dd forward bar: %d, challenger errors: %d)\n",
		len(records), s.Pairs, s.Dates, horizon, s.Skipped, s.ChallengerErrors)
	fmt.Printf("agreement: %.1f%%\n", 100*s.Agreement)
	fmt.Printf("mean signed %dd return per decision: champion %+.4f, challenger %+.4f\n", horizon, s.ChampionMean, s.ChallengerMean)
	fmt.Printf("hit rate on non-HOLD signals: champion %.1f%%, challenger %.1f%%\n", 100*s.ChampionHit, 100*s.ChallengerHit)
	fmt.Printf("challenger minus champion: %+.4f [%+.4f, %+.4f] (95%% date-bootstrap)\n", s.DiffMean, s.DiffLo, s.DiffHi)
	fmt.Println("verdict:", verdict(s, minPairs, minDates))
	return nil
}

func loadRecords(path string) ([]record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

func loadSeries(ctx context.Context, baseURL string, records []record, from, till time.Time) (map[string]closeSeries, error) {
	client := moex.NewClient(baseURL, nil)
	src := backtest.NewISSSource(baseURL, client)
	tickers := map[string]bool{}
	for _, r := range records {
		tickers[r.Ticker] = true
	}
	out := make(map[string]closeSeries, len(tickers))
	for tk := range tickers {
		candles, err := src.History(ctx, tk, from, till)
		if err != nil {
			return nil, fmt.Errorf("%s history: %w", tk, err)
		}
		var s closeSeries
		for _, c := range candles {
			v, _ := c.Close.Float64()
			s.dates = append(s.dates, c.Begin.Format("2006-01-02"))
			s.closes = append(s.closes, v)
		}
		out[tk] = s
	}
	return out, nil
}
