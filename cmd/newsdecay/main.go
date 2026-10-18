package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/newsstudy"
)

var horizons = []int{1, 2, 5, 10, 20}

type record struct {
	Ticker      string `json:"ticker"`
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	Source      string `json:"source"`
	PublishedTS int64  `json:"published_ts"`
}

type sample struct {
	ticker  string
	date    string
	score   float64
	excess  map[int]float64
	turnTkr float64
}

func normTitle(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("newsdecay", flag.ContinueOnError)
	configPath := fs.String("config", "config.sandbox.yaml", "config path")
	newsPath := fs.String("news", "data/news_history.jsonl", "news archive")
	classifier := fs.String("classifier", "", "trained classifier JSON (trainnewsmodel output)")
	trainTill := fs.String("train-till", "2025-01-01", "headlines before this date count as seen in training")
	testFrom := fs.String("test-from", "2025-03-01", "first test publication date")
	testTill := fs.String("test-till", "2026-09-30", "test publication date upper bound")
	minTurnover := fs.Float64("min-turnover", 1e7, "median 20-day turnover floor, RUB")
	maxTickers := fs.Int("max-tickers-per-article", 3, "drop pairs of articles mapped to more tickers")
	minTickers := fs.Int("min-tickers", 10, "tickers per date for the pooled portfolio")
	minGroup := fs.Int("min-group", 5, "tickers per date inside a liquidity tercile")
	block := fs.Int("block", 10, "bootstrap block length in dates")
	reps := fs.Int("reps", 5000, "bootstrap repeats")
	seed := fs.Int64("seed", 20261018, "bootstrap seed")
	dumpPath := fs.String("dump", "", "optional CSV of ticker-day samples")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *classifier == "" {
		return fmt.Errorf("-classifier is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	weights, err := model.LoadNewsClassifier(*classifier)
	if err != nil {
		return err
	}
	scorer := weights.Scorer()
	trainEnd, err := time.Parse("2006-01-02", *trainTill)
	if err != nil {
		return err
	}
	from, err := time.Parse("2006-01-02", *testFrom)
	if err != nil {
		return err
	}
	till, err := time.Parse("2006-01-02", *testTill)
	if err != nil {
		return err
	}

	f, err := os.Open(*newsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var recs []record
	tickersPerArticle := make(map[string]map[string]bool)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		var r record
		if json.Unmarshal(scanner.Bytes(), &r) != nil || r.Title == "" || r.PublishedTS <= 0 {
			continue
		}
		if r.Source == "MOEX сайт-новости" {
			continue
		}
		r.Ticker = strings.ToUpper(r.Ticker)
		recs = append(recs, r)
		if tickersPerArticle[r.ArticleID] == nil {
			tickersPerArticle[r.ArticleID] = make(map[string]bool)
		}
		tickersPerArticle[r.ArticleID][r.Ticker] = true
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	seenInTrain := make(map[string]bool)
	for _, r := range recs {
		if time.Unix(r.PublishedTS, 0).UTC().Before(trainEnd) {
			seenInTrain[r.Ticker+"|"+normTitle(r.Title)] = true
		}
	}
	var arts []newsstudy.Article
	score := make(map[string]float64)
	removedSeen := 0
	for _, r := range recs {
		t := time.Unix(r.PublishedTS, 0).UTC()
		if t.Before(from) || !t.Before(till) {
			continue
		}
		if *maxTickers > 0 && len(tickersPerArticle[r.ArticleID]) > *maxTickers {
			continue
		}
		if seenInTrain[r.Ticker+"|"+normTitle(r.Title)] {
			removedSeen++
			continue
		}
		a := newsstudy.Article{Ticker: r.Ticker, ArticleID: r.ArticleID, At: t}
		if _, dup := score[a.Key()]; dup {
			continue
		}
		s, _ := scorer(r.Title).Float64()
		score[a.Key()] = s
		arts = append(arts, a)
	}
	log.Printf("test pairs %d (removed %d seen in train)", len(arts), removedSeen)

	src := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	outcomes, err := newsstudy.BuildOutcomes(context.Background(), src, arts, horizons)
	if err != nil {
		return err
	}

	type dayKey struct{ ticker, date string }
	type agg struct {
		sum  float64
		n    int
		o    newsstudy.Outcome
		turn float64
	}
	days := make(map[dayKey]*agg)
	for _, a := range arts {
		o, ok := outcomes[a.Key()]
		if !ok || o.Turnover20 < *minTurnover {
			continue
		}
		k := dayKey{a.Ticker, a.At.In(newsstudy.Moscow).Format("2006-01-02")}
		g := days[k]
		if g == nil {
			g = &agg{o: o}
			days[k] = g
		}
		g.sum += score[a.Key()]
		g.n++
		g.turn = o.Turnover20
	}
	tickTurn := make(map[string][]float64)
	for k, g := range days {
		tickTurn[k.ticker] = append(tickTurn[k.ticker], g.turn)
	}
	medTurn := make(map[string]float64)
	var order []string
	for t, v := range tickTurn {
		sort.Float64s(v)
		medTurn[t] = newsstudy.Quantile(v, 0.5)
		order = append(order, t)
	}
	sort.Slice(order, func(i, j int) bool { return medTurn[order[i]] < medTurn[order[j]] })
	group := make(map[string]int)
	for i, t := range order {
		group[t] = i * 3 / len(order)
	}
	var samples []sample
	for k, g := range days {
		samples = append(samples, sample{ticker: k.ticker, date: k.date, score: g.sum / float64(g.n), excess: g.o.Excess, turnTkr: medTurn[k.ticker]})
	}
	log.Printf("ticker-days %d, tickers %d", len(samples), len(order))

	if *dumpPath != "" {
		out, err := os.Create(*dumpPath)
		if err != nil {
			return err
		}
		w := bufio.NewWriter(out)
		fmt.Fprintln(w, "date,ticker,score,excess_1,excess_2,excess_5,excess_10,excess_20,group")
		for _, s := range samples {
			fmt.Fprintf(w, "%s,%s,%.6f,%.4f,%.4f,%.4f,%.4f,%.4f,%d\n", s.date, s.ticker, s.score, s.excess[1], s.excess[2], s.excess[5], s.excess[10], s.excess[20], group[s.ticker])
		}
		w.Flush()
		out.Close()
	}

	byDate := make(map[string][]sample)
	for _, s := range samples {
		byDate[s.date] = append(byDate[s.date], s)
	}
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	spreads := func(filter func(sample) bool, minN, h int) []float64 {
		var out []float64
		for _, d := range dates {
			var rows []sample
			for _, s := range byDate[d] {
				if filter == nil || filter(s) {
					rows = append(rows, s)
				}
			}
			if len(rows) < minN {
				continue
			}
			out = append(out, rankSpread(rows, h))
		}
		return out
	}
	rng := func() *rand.Rand { return rand.New(rand.NewSource(*seed)) }

	fmt.Printf("ticker-days=%d tickers=%d dates=%d\n", len(samples), len(order), len(dates))
	fmt.Println("h | dates | gross spread %/tranche | CI95 | per day %")
	pooled := make(map[int][]float64)
	for _, h := range horizons {
		v := spreads(nil, *minTickers, h)
		pooled[h] = v
		lo, hi := newsstudy.BlockBootstrapMean(v, *block, *reps, rng())
		m := newsstudy.Mean(v)
		fmt.Printf("%d | %d | %+.4f | [%+.4f, %+.4f] | %+.4f\n", h, len(v), m, lo, hi, m/float64(h))
	}
	if a, b := pooled[1], pooled[10]; len(a) > 0 && len(b) > 0 {
		lo, hi := newsstudy.BlockBootstrapDiff(scalePerDay(a, 1), scalePerDay(b, 10), *block, *reps, rng())
		fmt.Printf("per-day spread h1 minus h10: %+.4f CI95 [%+.4f, %+.4f]\n", newsstudy.Mean(a)-newsstudy.Mean(b)/10, lo, hi)
	}
	fmt.Println("liquidity terciles (h=10, 0=least liquid):")
	tercile := make([][]float64, 3)
	for g := 0; g < 3; g++ {
		gg := g
		v := spreads(func(s sample) bool { return group[s.ticker] == gg }, *minGroup, 10)
		tercile[g] = v
		lo, hi := newsstudy.BlockBootstrapMean(v, *block, *reps, rng())
		fmt.Printf("tercile %d | dates %d | %+.4f | [%+.4f, %+.4f]\n", g, len(v), newsstudy.Mean(v), lo, hi)
	}
	lo, hi := newsstudy.BlockBootstrapDiff(tercile[0], tercile[2], *block, *reps, rng())
	fmt.Printf("low minus high tercile: %+.4f CI95 [%+.4f, %+.4f]\n", newsstudy.Mean(tercile[0])-newsstudy.Mean(tercile[2]), lo, hi)

	var topLeg, botLeg []float64
	for _, d := range dates {
		rows := byDate[d]
		if len(rows) < *minTickers {
			continue
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].score < rows[j].score })
		k := len(rows) / 5
		var b, t float64
		for i := 0; i < k; i++ {
			b += rows[i].excess[10]
			t += rows[len(rows)-1-i].excess[10]
		}
		botLeg = append(botLeg, b/float64(k))
		topLeg = append(topLeg, t/float64(k))
	}
	blo, bhi := newsstudy.BlockBootstrapMean(botLeg, *block, *reps, rng())
	tlo, thi := newsstudy.BlockBootstrapMean(topLeg, *block, *reps, rng())
	fmt.Printf("h=10 bottom quintile mean excess %+.4f [%+.4f, %+.4f]; top quintile %+.4f [%+.4f, %+.4f]\n", newsstudy.Mean(botLeg), blo, bhi, newsstudy.Mean(topLeg), tlo, thi)
	neg := make([]float64, len(botLeg))
	for i := range botLeg {
		neg[i] = -botLeg[i]
	}
	alo, ahi := newsstudy.BlockBootstrapDiff(neg, topLeg, *block, *reps, rng())
	fmt.Printf("asymmetry |bottom| minus top: %+.4f CI95 [%+.4f, %+.4f]\n", newsstudy.Mean(neg)-newsstudy.Mean(topLeg), alo, ahi)
	return nil
}

func scalePerDay(v []float64, h int) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		out[i] = v[i] / float64(h)
	}
	return out
}

func rankSpread(rows []sample, h int) float64 {
	n := len(rows)
	sorted := append([]sample(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].score < sorted[j].score })
	ranks := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && sorted[j+1].score == sorted[i].score {
			j++
		}
		mid := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			ranks[k] = mid
		}
		i = j + 1
	}
	mean := float64(n+1) / 2
	var absSum float64
	for _, r := range ranks {
		absSum += math.Abs(r - mean)
	}
	if absSum == 0 {
		return 0
	}
	var ret float64
	for i, s := range sorted {
		ret += (ranks[i] - mean) / absSum * s.excess[h]
	}
	return ret
}
