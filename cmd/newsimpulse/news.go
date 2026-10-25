package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

var positiveStems = []string{"рост", "вырос", "выросл", "прибыл", "рекорд", "дивиденд", "повыш", "увелич", "выкуп", "контракт", "одобр", "расшир", "улучш", "покупк", "сильн"}
var negativeStems = []string{"пад", "снизил", "снижен", "убыт", "санкц", "штраф", "иск ", "отмен", "дефолт", "банкрот", "расследован", "приостанов", "сократ", "понижен", "риск", "авария", "обвал", "потер"}

type item struct {
	ticker string
	ts     time.Time
	trust  float64
	score  float64
}

func lexScore(title string) float64 {
	t := strings.ToLower(title)
	pos, neg := 0, 0
	for _, s := range positiveStems {
		if strings.Contains(t, s) {
			pos++
		}
	}
	for _, s := range negativeStems {
		if strings.Contains(t, s) {
			neg++
		}
	}
	return float64(pos - neg)
}

func loadNews(path string) ([]item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	loc := time.FixedZone("MSK", 3*3600)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	seen := map[string]bool{}
	var out []item
	for sc.Scan() {
		var r struct {
			ID     string  `json:"article_id"`
			Ticker string  `json:"ticker"`
			Title  string  `json:"title"`
			TS     int64   `json:"published_ts"`
			Trust  float64 `json:"trust_weight"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.TS == 0 || r.Ticker == "" {
			continue
		}
		key := r.ID + "|" + r.Ticker
		if seen[key] {
			continue
		}
		seen[key] = true
		w := r.Trust
		if w <= 0 {
			w = 1
		}
		out = append(out, item{ticker: r.Ticker, ts: time.Unix(r.TS, 0).In(loc), trust: w, score: lexScore(r.Title)})
	}
	return out, sc.Err()
}
