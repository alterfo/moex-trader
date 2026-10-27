package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type headline struct {
	id, ticker, source, title string
	ts                        int64
}

func main() {
	mode := flag.String("mode", "pilot", "pilot | score")
	newsPath := flag.String("news", "data/news_history.jsonl", "news history")
	out := flag.String("out", "work/newsllm", "output directory")
	base := flag.String("ollama", "http://127.0.0.1:11434", "ollama base url")
	model := flag.String("model", "huihui_ai/qwen3-abliterated:4b", "pinned model name")
	n := flag.Int("n", 500, "pilot sample size")
	seed := flag.Int64("seed", 20261026, "pilot sample seed")
	from := flag.String("from", "2022-01-03", "first date of the scoring window")
	till := flag.String("till", "2026-10-16", "last date of the scoring window")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	items, err := loadHeadlines(*newsPath, *from, *till)
	if err != nil {
		log.Fatal(err)
	}
	c := newClient(*base, *model)
	ctx := context.Background()
	dg, err := c.digest(ctx)
	if err != nil {
		log.Fatal(err)
	}
	h := sha256.Sum256([]byte(instruction))
	meta, _ := json.MarshalIndent(map[string]any{
		"model": *model, "digest": dg, "instruction_sha256": hex.EncodeToString(h[:]),
		"instruction": instruction, "headlines": len(items), "from": *from, "till": *till,
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(*out, "meta.json"), meta, 0o644); err != nil {
		log.Fatal(err)
	}
	switch *mode {
	case "pilot":
		pilot(ctx, c, items, *n, *seed, *out)
	case "score":
		score(ctx, c, items, *out)
	default:
		log.Fatalf("unknown mode %s", *mode)
	}
}

func loadHeadlines(path, from, till string) ([]headline, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	loc := time.FixedZone("MSK", 3*3600)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	seen := map[string]bool{}
	var out []headline
	for sc.Scan() {
		var r struct {
			ID     string `json:"article_id"`
			Ticker string `json:"ticker"`
			Title  string `json:"title"`
			Source string `json:"source"`
			TS     int64  `json:"published_ts"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.TS == 0 || r.Ticker == "" || r.Title == "" {
			continue
		}
		d := time.Unix(r.TS, 0).In(loc).Format("2006-01-02")
		if d < from || d > till {
			continue
		}
		key := r.ID + "|" + r.Ticker
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, headline{id: r.ID, ticker: r.Ticker, source: r.Source, title: r.Title, ts: r.TS})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ts != out[j].ts {
			return out[i].ts < out[j].ts
		}
		return out[i].id+out[i].ticker < out[j].id+out[j].ticker
	})
	return out, sc.Err()
}

func score(ctx context.Context, c *client, items []headline, dir string) {
	path := filepath.Join(dir, "scores.csv")
	done := map[string]bool{}
	if f, err := os.Open(path); err == nil {
		r := csv.NewReader(f)
		for {
			rec, err := r.Read()
			if err != nil {
				break
			}
			done[rec[0]+"|"+rec[1]] = true
		}
		f.Close()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	start := time.Now()
	var fresh, bad int
	for i, it := range items {
		if done[it.id+"|"+it.ticker] {
			continue
		}
		reply, err := askRetry(ctx, c, buildPrompt(it.ticker, it.source, it.title, false))
		if err != nil {
			log.Fatal(err)
		}
		label, ok := parseLabel(reply)
		sc := ""
		if ok {
			v, _ := labelScore(label)
			sc = strconv.FormatFloat(v, 'f', 0, 64)
		} else {
			bad++
		}
		w.Write([]string{it.id, it.ticker, strconv.FormatInt(it.ts, 10), sc})
		fresh++
		if fresh%200 == 0 {
			w.Flush()
			el := time.Since(start).Seconds()
			log.Printf("score: %d of %d, %.2f s per headline, unparseable %d", i+1, len(items), el/float64(fresh), bad)
		}
	}
	w.Flush()
	log.Printf("score: finished, %d new, unparseable %d", fresh, bad)
}

func askRetry(ctx context.Context, c *client, prompt string) (string, error) {
	var err error
	for a := 0; a < 5; a++ {
		var r string
		r, err = c.ask(ctx, prompt)
		if err == nil {
			return r, nil
		}
		time.Sleep(time.Duration(a+1) * 10 * time.Second)
	}
	return "", err
}

func pilot(ctx context.Context, c *client, items []headline, n int, seed int64, dir string) {
	rng := rand.New(rand.NewSource(seed))
	idx := rng.Perm(len(items))
	if n > len(idx) {
		n = len(idx)
	}
	idx = idx[:n]
	hist := map[string]int{}
	var sc, lex []float64
	var bad int
	full := make([]string, n)
	start := time.Now()
	for k, i := range idx {
		it := items[i]
		reply, err := askRetry(ctx, c, buildPrompt(it.ticker, it.source, it.title, false))
		if err != nil {
			log.Fatal(err)
		}
		label, ok := parseLabel(reply)
		if !ok {
			bad++
			continue
		}
		full[k] = label
		hist[label]++
		v, _ := labelScore(label)
		sc, lex = append(sc, v), append(lex, lexScore(it.title))
	}
	perHeadline := time.Since(start).Seconds() / float64(n)
	var same, both int
	for k, i := range idx {
		it := items[i]
		reply, err := askRetry(ctx, c, buildPrompt(it.ticker, it.source, it.title, true))
		if err != nil {
			log.Fatal(err)
		}
		label, ok := parseLabel(reply)
		if !ok || full[k] == "" {
			continue
		}
		both++
		if label == full[k] {
			same++
		}
	}
	maxShare := 0.0
	for _, l := range labels {
		if s := float64(hist[l]) / float64(n-bad); s > maxShare {
			maxShare = s
		}
	}
	rho := spearman(sc, lex)
	predicted := perHeadline * 145000 * 1.2 / 3600
	rep := fmt.Sprintf("pilot: n %d, seconds per headline %.3f, predicted full run %.1f hours (x1.2 factor), fallback to 2023-2026 %v\n", n, perHeadline, predicted, predicted > 36)
	for _, l := range labels {
		rep += fmt.Sprintf("pilot: label %-16s %4d (%.1f%%)\n", l, hist[l], 100*float64(hist[l])/float64(n-bad))
	}
	rep += fmt.Sprintf("pilot: unparseable %d (%.1f%%, gate at most 5%%), max class share %.1f%% (gate at most 80%%), |Spearman with lexicon| %.3f (gate at least 0.4)\n", bad, 100*float64(bad)/float64(n), 100*maxShare, math.Abs(rho))
	rep += fmt.Sprintf("pilot: ticker-redacted twin, label agreement %d of %d (%.1f%%)\n", same, both, 100*float64(same)/float64(both))
	fmt.Print(rep)
	os.WriteFile(filepath.Join(dir, "pilot.txt"), []byte(rep), 0o644)
}

func spearman(a, b []float64) float64 {
	return pearson(rankAvg(a), rankAvg(b))
}

func rankAvg(v []float64) []float64 {
	n := len(v)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	r := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		for k := i; k <= j; k++ {
			r[idx[k]] = float64(i+j) / 2
		}
		i = j + 1
	}
	return r
}

func pearson(a, b []float64) float64 {
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(len(a))
	mb /= float64(len(b))
	var sab, saa, sbb float64
	for i := range a {
		sab += (a[i] - ma) * (b[i] - mb)
		saa += (a[i] - ma) * (a[i] - ma)
		sbb += (b[i] - mb) * (b[i] - mb)
	}
	if saa == 0 || sbb == 0 {
		return math.NaN()
	}
	return sab / math.Sqrt(saa*sbb)
}
