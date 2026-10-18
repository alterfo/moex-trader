package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/newsstudy"
)

var horizons = []int{1, 5, 10}

type decision struct {
	Ticker      string  `json:"ticker"`
	Date        string  `json:"date"`
	Probability float64 `json:"probability"`
}

type window struct {
	Decisions []decision `json:"decisions"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rankspread", flag.ContinueOnError)
	configPath := fs.String("config", "config.sandbox.yaml", "config path")
	wfDir := fs.String("wf-dir", "", "walk-forward directory with <window>/window.json")
	minTickers := fs.Int("min-tickers", 10, "tickers per date")
	cost := fs.Float64("cost-side", 0.15, "cost per side in percent")
	block := fs.Int("block", 10, "bootstrap block length in dates")
	reps := fs.Int("reps", 5000, "bootstrap repeats")
	seed := fs.Int64("seed", 20261018, "bootstrap seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(*wfDir, "*", "window.json"))
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no windows under %q", *wfDir)
	}
	var arts []newsstudy.Article
	prob := make(map[string]float64)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var w window
		if err := json.Unmarshal(data, &w); err != nil {
			return err
		}
		for _, d := range w.Decisions {
			day := strings.SplitN(d.Date, "T", 2)[0]
			at, err := time.ParseInLocation("2006-01-02", day, newsstudy.Moscow)
			if err != nil {
				return err
			}
			a := newsstudy.Article{Ticker: d.Ticker, ArticleID: day, At: at.Add(12 * time.Hour)}
			if _, dup := prob[a.Key()]; dup {
				continue
			}
			prob[a.Key()] = d.Probability
			arts = append(arts, a)
		}
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	src := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	outcomes, err := newsstudy.BuildOutcomes(context.Background(), src, arts, horizons)
	if err != nil {
		return err
	}
	type row struct {
		score  float64
		excess map[int]float64
	}
	byDate := make(map[string][]row)
	for _, a := range arts {
		o, ok := outcomes[a.Key()]
		if !ok {
			continue
		}
		byDate[a.ArticleID] = append(byDate[a.ArticleID], row{prob[a.Key()], o.Excess})
	}
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	rng := func() *rand.Rand { return rand.New(rand.NewSource(*seed)) }
	fmt.Printf("decisions=%d with outcomes dates=%d\n", len(arts), len(dates))
	fmt.Println("h | dates | gross spread % | CI95 | net of tranche cost | net CI95")
	for _, h := range horizons {
		var v []float64
		for _, d := range dates {
			rows := byDate[d]
			if len(rows) < *minTickers {
				continue
			}
			s := make([]float64, len(rows))
			r := make([]float64, len(rows))
			for i, x := range rows {
				s[i] = x.score
				r[i] = x.excess[h]
			}
			v = append(v, newsstudy.RankSpread(s, r))
		}
		lo, hi := newsstudy.BlockBootstrapMean(v, *block, *reps, rng())
		m := newsstudy.Mean(v)
		tranche := 4 * (*cost)
		fmt.Printf("%d | %d | %+.4f | [%+.4f, %+.4f] | %+.4f | [%+.4f, %+.4f]\n", h, len(v), m, lo, hi, m-tranche, lo-tranche, hi-tranche)
	}
	var top, bot []float64
	for _, d := range dates {
		rows := byDate[d]
		if len(rows) < *minTickers {
			continue
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].score < rows[j].score })
		k := len(rows) / 5
		if k == 0 {
			continue
		}
		var b, t float64
		for i := 0; i < k; i++ {
			b += rows[i].excess[10]
			t += rows[len(rows)-1-i].excess[10]
		}
		bot = append(bot, b/float64(k))
		top = append(top, t/float64(k))
	}
	blo, bhi := newsstudy.BlockBootstrapMean(bot, *block, *reps, rng())
	tlo, thi := newsstudy.BlockBootstrapMean(top, *block, *reps, rng())
	fmt.Printf("h=10 bottom quintile mean excess %+.4f [%+.4f, %+.4f]; top quintile %+.4f [%+.4f, %+.4f]; long-only top net of %.2f%% round trip: %+.4f\n",
		newsstudy.Mean(bot), blo, bhi, newsstudy.Mean(top), tlo, thi, 2*(*cost), newsstudy.Mean(top)-2*(*cost))
	return nil
}
