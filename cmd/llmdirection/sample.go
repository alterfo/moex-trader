package main

import (
	"bufio"
	"encoding/json"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"
)

type article struct {
	Ticker      string `json:"ticker"`
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	PublishedTS int64  `json:"published_ts"`
}

func (a article) at() time.Time { return time.Unix(a.PublishedTS, 0).UTC() }

func loadSample(path string, tickers map[string]bool, from, till time.Time, perTickerDay, limit int, seed int64) ([]article, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	seen := make(map[string]bool)
	var all []article
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		var a article
		if json.Unmarshal(scanner.Bytes(), &a) != nil {
			continue
		}
		a.Ticker = strings.ToUpper(a.Ticker)
		a.Title = strings.TrimSpace(a.Title)
		if a.Title == "" || a.PublishedTS <= 0 || !tickers[a.Ticker] {
			continue
		}
		t := a.at()
		if t.Before(from) || !t.Before(till) {
			continue
		}
		key := a.Ticker + "|" + a.ArticleID
		if seen[key] {
			continue
		}
		seen[key] = true
		all = append(all, a)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].PublishedTS != all[j].PublishedTS {
			return all[i].PublishedTS < all[j].PublishedTS
		}
		return all[i].ArticleID < all[j].ArticleID
	})
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	perDay := make(map[string]int)
	var out []article
	for _, a := range all {
		k := a.Ticker + "|" + a.at().Format("2006-01-02")
		if perDay[k] >= perTickerDay {
			continue
		}
		perDay[k]++
		out = append(out, a)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedTS < out[j].PublishedTS })
	return out, nil
}
