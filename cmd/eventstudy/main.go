package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

const roundTripCost = 0.003

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, newsPath, outPath, arMode, studyStr, histStr, tillStr string
	var reps, placeboReps, dedup int
	var seed int64
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&newsPath, "news", "data/news_history.jsonl", "news archive JSONL")
	flag.StringVar(&outPath, "out", "/tmp/eventstudy.md", "markdown report")
	flag.StringVar(&arMode, "ar-mode", "beta", "abnormal return: beta (market model) or excess (r - r_IMOEX)")
	flag.StringVar(&studyStr, "study-from", "2026-04-01", "study window start")
	flag.StringVar(&histStr, "hist-from", "2025-10-01", "estimation window start")
	flag.StringVar(&tillStr, "till", "", "candle end date (default tomorrow)")
	flag.IntVar(&reps, "reps", 2000, "bootstrap replicates")
	flag.IntVar(&placeboReps, "placebo-reps", 1000, "placebo replicates")
	flag.IntVar(&dedup, "dedup", 5, "trading days between same ticker+topic events")
	flag.Int64Var(&seed, "seed", 42, "RNG seed")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	studyFrom, err := time.Parse("2006-01-02", studyStr)
	if err != nil {
		return err
	}
	histFrom, err := time.Parse("2006-01-02", histStr)
	if err != nil {
		return err
	}
	till := time.Now().AddDate(0, 0, 1)
	if tillStr != "" {
		if till, err = time.Parse("2006-01-02", tillStr); err != nil {
			return err
		}
	}
	tickers := make([]string, 0, len(cfg.Tickers))
	for _, t := range cfg.Tickers {
		tickers = append(tickers, strings.ToUpper(t))
	}

	p, err := loadPanel(context.Background(), cfg.MOEXISSBaseURL, tickers, histFrom, till, studyFrom, arMode)
	if err != nil {
		return err
	}
	records, err := model.LoadFinanalysNewsHistory(newsPath)
	if err != nil {
		return err
	}
	defs := topicDefs()
	topics, err := buildTopics(p, records, defs, dedup)
	if err != nil {
		return err
	}

	rng := rand.New(rand.NewSource(seed))
	var b strings.Builder
	writef(&b, "# Event causal study (%s)\n\n", time.Now().Format("2006-01-02"))
	writef(&b, "AR mode `%s`; tickers %d; study %s..%s (%d trading days); news records %d; reps %d; placebo %d; seed %d.\n\n",
		arMode, len(tickers), p.dates[p.studyStart], p.dates[len(p.dates)-1], len(p.dates)-p.studyStart, len(records), reps, placeboReps, seed)

	primary := window{0, 1}
	drift := window{1, 5}
	pre := window{-5, -1}

	type topicRes struct {
		def     topicDef
		td      topicData
		dates   int
		prim    evStats
		sec     evStats
		pretr   evStats
		aipwP   aipwResult
		aipwOK  bool
		aipwS   aipwResult
		aipwSOK bool
		placebo float64
		placS   float64
		mde     float64
		twfe    twfeResult
		twfeOK  bool
		market  marketResult
	}
	results := make([]topicRes, len(defs))
	for i, def := range defs {
		td := topics[i]
		r := topicRes{def: def, td: td}
		dateSet := map[int]bool{}
		for _, e := range td.events {
			dateSet[e.t0] = true
		}
		r.dates = len(dateSet)
		r.prim = p.eventStats(td, primary, reps, rng)
		r.sec = p.eventStats(td, drift, reps, rng)
		r.pretr = p.eventStats(td, pre, reps, rng)
		r.aipwP, r.aipwOK = p.aipw(td, primary)
		r.aipwS, r.aipwSOK = p.aipw(td, drift)
		r.placebo = p.placeboP(td, primary, r.prim.did.Mean, placeboReps, rng)
		r.placS = p.placeboP(td, drift, r.sec.did.Mean, placeboReps, rng)
		r.mde = 2.8 * r.prim.did.SE
		r.twfe, r.twfeOK = p.twfe(td, []int{-3, -2, -1, 0, 1, 2, 3})
		r.market = p.marketLevel(td, reps, rng)
		results[i] = r
	}

	writef(&b, "## Events\n\n| topic | role | events | distinct dates | raw flagged ticker-days |\n|---|---|---|---|---|\n")
	for _, r := range results {
		raw := 0
		for _, row := range r.td.flagged {
			for _, f := range row {
				if f {
					raw++
				}
			}
		}
		role := "exploratory"
		if r.def.primary {
			role = "primary"
		}
		writef(&b, "| %s | %s | %d | %d | %d |\n", r.def.name, role, len(r.td.events), r.dates, raw)
	}

	writef(&b, "\n## Event-study CAR and stacked DiD (95%% date-bootstrap CI, return units)\n\n")
	writef(&b, "| topic | window | N | CAR [CI] | stacked DiD [CI] (N) | placebo p |\n|---|---|---|---|---|---|\n")
	for _, r := range results {
		writef(&b, "| %s | CAR[0,+1] | %d | %s | %s (%d) | %.3f |\n", r.def.name, r.prim.car.N, fmtIv(r.prim.car), fmtIv(r.prim.did), r.prim.did.N, r.placebo)
		writef(&b, "| %s | CAR[+1,+5] | %d | %s | %s (%d) | %.3f |\n", r.def.name, r.sec.car.N, fmtIv(r.sec.car), fmtIv(r.sec.did), r.sec.did.N, r.placS)
		writef(&b, "| %s | CAR[-5,-1] pre-trend | %d | %s | - | - |\n", r.def.name, r.pretr.car.N, fmtIv(r.pretr.car))
	}

	writef(&b, "\n## Doubly robust AIPW (ATT, date-clustered SE)\n\n")
	writef(&b, "| topic | window | treated | controls | mean propensity | ATT | SE | t |\n|---|---|---|---|---|---|---|---|\n")
	for _, r := range results {
		writeAipw(&b, r.def.name, "CAR[0,+1]", r.aipwP, r.aipwOK)
		writeAipw(&b, r.def.name, "CAR[+1,+5]", r.aipwS, r.aipwSOK)
	}

	writef(&b, "\n## TWFE event-time coefficients (ticker + date FE, SE clustered by date)\n\n")
	writef(&b, "| topic | k=-3 | k=-2 | k=-1 | k=0 | k=+1 | k=+2 | k=+3 |\n|---|---|---|---|---|---|---|---|\n")
	for _, r := range results {
		if !r.twfeOK {
			writef(&b, "| %s | n/a | n/a | n/a | n/a | n/a | n/a | n/a |\n", r.def.name)
			continue
		}
		cells := make([]string, len(r.twfe.ks))
		for j := range r.twfe.ks {
			cells[j] = fmt.Sprintf("%+.4f (%.4f)", r.twfe.coef[j], r.twfe.se[j])
		}
		writef(&b, "| %s | %s |\n", r.def.name, strings.Join(cells, " | "))
	}

	writef(&b, "\n## Market-level IMOEX check (top-decile topic-count days vs others, 2-day return)\n\n")
	writef(&b, "| topic | high days | other days | diff [CI] |\n|---|---|---|---|\n")
	for _, r := range results {
		writef(&b, "| %s | %d | %d | %s |\n", r.def.name, r.market.NHigh, r.market.NOthers, fmtIv(r.market.Diff))
	}

	var primIdx []int
	var ps []float64
	for i, r := range results {
		if r.def.primary {
			primIdx = append(primIdx, i)
			pv := r.placebo
			if math.IsNaN(pv) {
				pv = 1
			}
			ps = append(ps, pv)
		}
	}
	adj := holm(ps)
	writef(&b, "\n## Acceptance gate (protocol section 5, PRIMARY CAR[0,+1])\n\n")
	writef(&b, "| topic | (a) N>=30 & dates>=10 | (b) DiD & AIPW same sign, CIs exclude 0 | (c) Holm p<0.05 | (d) pre-trend CI has 0 | (e) abs effect >0.003 | MDE (80%% power) | verdict |\n|---|---|---|---|---|---|---|---|\n")
	for k, i := range primIdx {
		r := results[i]
		a := r.prim.did.N >= 30 && r.dates >= 10
		aipwLo := r.aipwP.ATT - 1.96*r.aipwP.SE
		aipwHi := r.aipwP.ATT + 1.96*r.aipwP.SE
		bOK := r.aipwOK && r.prim.did.excludesZero() && (aipwLo > 0 || aipwHi < 0) && (r.prim.did.Mean > 0) == (r.aipwP.ATT > 0)
		c := adj[k] < 0.05
		d := !r.pretr.car.excludesZero()
		e := math.Abs(r.prim.did.Mean) > roundTripCost
		verdict := "not supported"
		if !a {
			verdict = "underpowered"
		}
		if a && bOK && c && d && e {
			verdict = "SUPPORTED"
		}
		writef(&b, "| %s | %s (N=%d, dates=%d) | %s | %s (adj p=%.3f) | %s | %s | %.4f | %s |\n",
			r.def.name, yn(a), r.prim.did.N, r.dates, yn(bOK), yn(c), adj[k], yn(d), yn(e), r.mde, verdict)
	}

	if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Print(b.String())
	return nil
}

func fmtIv(iv interval) string {
	if math.IsNaN(iv.Mean) {
		return "n/a"
	}
	return fmt.Sprintf("%+.4f [%+.4f, %+.4f]", iv.Mean, iv.Lo, iv.Hi)
}

func writeAipw(b *strings.Builder, topic, win string, r aipwResult, ok bool) {
	if !ok {
		writef(b, "| %s | %s | %d | %d | - | n/a | n/a | n/a |\n", topic, win, r.NTreated, r.NControl)
		return
	}
	writef(b, "| %s | %s | %d | %d | %.3f | %+.4f | %.4f | %.2f |\n", topic, win, r.NTreated, r.NControl, r.MeanProp, r.ATT, r.SE, r.ATT/r.SE)
}

func yn(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func writef(b *strings.Builder, format string, args ...interface{}) {
	fmt.Fprintf(b, format, args...)
}
