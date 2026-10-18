package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/newsstudy"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("llmdirection", flag.ContinueOnError)
	mode := fs.String("mode", "run", "run | analyze")
	configPath := fs.String("config", "config.sandbox.yaml", "config path")
	newsPath := fs.String("news", "data/news_history.jsonl", "news JSONL")
	fromStr := fs.String("from", "", "first publication date (YYYY-MM-DD)")
	tillStr := fs.String("till", "", "publication date upper bound, exclusive")
	limit := fs.Int("max", 1200, "sample size")
	perDay := fs.Int("per-ticker-day", 5, "max articles per ticker per day")
	seed := fs.Int64("seed", 42, "sampling and bootstrap seed")
	url := fs.String("ollama", "http://192.168.88.193:11434", "Ollama base URL")
	model := fs.String("model", "qwen3.8:latest", "model")
	timeout := fs.Duration("timeout", 90*time.Second, "per-request timeout")
	outPath := fs.String("out", "llm_direction.csv", "output CSV")
	inA := fs.String("a", "", "analyze: CSV of the clean sample")
	inB := fs.String("b", "", "analyze: CSV of the pre-cutoff sample")
	reps := fs.Int("reps", 2000, "bootstrap repeats")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *mode == "analyze" {
		return analyze(*inA, *inB, *reps, *seed)
	}
	if *fromStr == "" || *tillStr == "" {
		return fmt.Errorf("-from and -till are required")
	}
	from, err := time.Parse("2006-01-02", *fromStr)
	if err != nil {
		return err
	}
	till, err := time.Parse("2006-01-02", *tillStr)
	if err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tickers := make(map[string]bool)
	for _, t := range cfg.Tickers {
		tickers[t] = true
	}
	arts, err := loadSample(*newsPath, tickers, from, till, *perDay, *limit, *seed)
	if err != nil {
		return err
	}
	ctx := context.Background()
	src := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	outcomes, err := newsstudy.BuildOutcomes(ctx, src, toStudy(arts), horizons)
	if err != nil {
		return err
	}
	log.Printf("sampled %d articles, %d with complete outcomes", len(arts), len(outcomes))

	done := make(map[string]bool)
	appendMode := false
	if f, err := os.Open(*outPath); err == nil {
		rows, _ := csv.NewReader(f).ReadAll()
		f.Close()
		for i, r := range rows {
			if i > 0 && len(r) > 2 {
				done[r[1]+"|"+r[2]] = true
			}
		}
		appendMode = len(rows) > 0
	}
	flag := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	out, err := os.OpenFile(*outPath, flag, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	if !appendMode {
		w.Write([]string{"date", "ticker", "article_id", "direction", "score", "prior5", "excess_1", "excess_5", "excess_10", "bad"})
	}
	asker := newAsker(*url, *model, *timeout)
	asked, bad := 0, 0
	for _, a := range arts {
		key := a.Ticker + "|" + a.ArticleID
		o, ok := outcomes[key]
		if !ok || done[a.Ticker+"|"+a.ArticleID] {
			continue
		}
		score, dir, _, err := asker.ask(ctx, a.Ticker, a.Title)
		badFlag := "0"
		if err != nil {
			bad++
			badFlag = "1"
			dir = "bad"
			score = 0
		}
		asked++
		w.Write([]string{
			a.at().In(newsstudy.Moscow).Format("2006-01-02"), a.Ticker, a.ArticleID, dir,
			strconv.FormatFloat(score, 'f', 0, 64),
			strconv.FormatFloat(o.Prior5, 'f', 4, 64),
			strconv.FormatFloat(o.Excess[1], 'f', 4, 64),
			strconv.FormatFloat(o.Excess[5], 'f', 4, 64),
			strconv.FormatFloat(o.Excess[10], 'f', 4, 64),
			badFlag,
		})
		w.Flush()
		if asked%50 == 0 {
			log.Printf("asked %d, unparsable %d", asked, bad)
		}
	}
	log.Printf("done: asked %d, unparsable %d -> %s", asked, bad, *outPath)
	return w.Error()
}

func readObs(path string, h int) ([]newsstudy.Obs, []newsstudy.Obs, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, nil, 0, 0, err
	}
	col := map[int]int{1: 6, 5: 7, 10: 8}[h]
	var all, nonNeutral []newsstudy.Obs
	total, bad := 0, 0
	for i, r := range rows {
		if i == 0 || len(r) < 10 {
			continue
		}
		total++
		if r[9] == "1" {
			bad++
			continue
		}
		score, _ := strconv.ParseFloat(r[4], 64)
		prior, _ := strconv.ParseFloat(r[5], 64)
		ex, _ := strconv.ParseFloat(r[col], 64)
		label := 0.0
		if ex > 0 {
			label = 1
		}
		o := newsstudy.Obs{Date: r[0], Score: score, Label: label, Aux: prior}
		all = append(all, o)
		if score != 0 {
			nonNeutral = append(nonNeutral, o)
		}
	}
	return all, nonNeutral, total, bad, nil
}

func deadband(path string, h int, band float64) ([]newsstudy.Obs, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	col := map[int]int{1: 6, 5: 7, 10: 8}[h]
	var out []newsstudy.Obs
	for i, r := range rows {
		if i == 0 || len(r) < 10 || r[9] == "1" {
			continue
		}
		ex, _ := strconv.ParseFloat(r[col], 64)
		if math.Abs(ex) < band {
			continue
		}
		score, _ := strconv.ParseFloat(r[4], 64)
		label := 0.0
		if ex > 0 {
			label = 1
		}
		out = append(out, newsstudy.Obs{Date: r[0], Score: score, Label: label})
	}
	return out, nil
}

func report(name, path string, reps int, seed int64) (map[int][]newsstudy.Obs, error) {
	res := make(map[int][]newsstudy.Obs)
	for _, h := range horizons {
		all, nn, total, bad, err := readObs(path, h)
		if err != nil {
			return nil, err
		}
		res[h] = all
		keys, _ := newsstudy.ByDate(all)
		rng := rand.New(rand.NewSource(seed))
		a := newsstudy.AUC(all)
		lo, hi := newsstudy.AUCCI(all, reps, rng)
		nnAUC := newsstudy.AUC(nn)
		band, _ := deadband(path, h, 0.5)
		prior := make([]float64, len(all))
		score := make([]float64, len(all))
		for i, o := range all {
			prior[i] = o.Aux
			score[i] = o.Score
		}
		fmt.Printf("%s h=%d: n=%d dates=%d unparsable=%d/%d AUC=%.4f CI95=[%.4f,%.4f] nonneutral n=%d AUC=%.4f deadband0.5 n=%d AUC=%.4f rho(prior5)=%.3f tercileAUC=%.4f\n",
			name, h, len(all), len(keys), bad, total, a, lo, hi, len(nn), nnAUC, len(band), newsstudy.AUC(band), newsstudy.Spearman(score, prior), newsstudy.TercileAUC(all))
	}
	return res, nil
}

func analyze(pathA, pathB string, reps int, seed int64) error {
	if pathA == "" {
		return fmt.Errorf("-a is required")
	}
	a, err := report("A", pathA, reps, seed)
	if err != nil {
		return err
	}
	if pathB == "" {
		return nil
	}
	b, err := report("B", pathB, reps, seed)
	if err != nil {
		return err
	}
	for _, h := range horizons {
		lo, hi := newsstudy.AUCDiffCI(a[h], b[h], reps, rand.New(rand.NewSource(seed)))
		fmt.Printf("B-A h=%d: diff=%.4f CI95=[%.4f,%.4f]\n", h, newsstudy.AUC(b[h])-newsstudy.AUC(a[h]), lo, hi)
	}
	return nil
}

var horizons = []int{1, 5, 10}

func toStudy(arts []article) []newsstudy.Article {
	out := make([]newsstudy.Article, len(arts))
	for i, a := range arts {
		out[i] = newsstudy.Article{Ticker: a.Ticker, ArticleID: a.ArticleID, At: a.at()}
	}
	return out
}
