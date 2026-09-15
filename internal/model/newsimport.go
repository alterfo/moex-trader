package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type HistoricalNewsRecord struct {
	Ticker      string
	ArticleID   string
	PublishedAt time.Time
	Sentiment   float64
	TrustWeight float64
}

type finanalysNewsLine struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
	Sentiment   float64 `json:"sentiment"`
}

func LoadFinanalysNewsHistory(path string) ([]HistoricalNewsRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("model: open news history %q: %w", path, err)
	}
	defer f.Close()

	var records []HistoricalNewsRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw finanalysNewsLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("model: parse news history %q line %d: %w", path, lineNo, err)
		}
		ticker := strings.ToUpper(strings.TrimSpace(raw.Ticker))
		if ticker == "" {
			continue
		}
		trustWeight := raw.TrustWeight
		if trustWeight <= 0 {
			trustWeight = 1
		}
		records = append(records, HistoricalNewsRecord{
			Ticker:      ticker,
			ArticleID:   raw.ArticleID,
			PublishedAt: time.Unix(raw.PublishedTS, 0).UTC(),
			Sentiment:   raw.Sentiment,
			TrustWeight: trustWeight,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("model: read news history %q: %w", path, err)
	}
	return records, nil
}

type NewsAggregate struct {
	Sentiment float64
	Count     int
}

func AggregateDailySentiment(records []HistoricalNewsRecord) map[string]map[string]NewsAggregate {
	type accum struct {
		weightedSum float64
		totalWeight float64
		count       int
	}
	acc := make(map[string]map[string]*accum)
	for _, r := range records {
		if r.TrustWeight <= 0 {
			continue
		}
		byDate, ok := acc[r.Ticker]
		if !ok {
			byDate = make(map[string]*accum)
			acc[r.Ticker] = byDate
		}
		key := dateKey(r.PublishedAt)
		a, ok := byDate[key]
		if !ok {
			a = &accum{}
			byDate[key] = a
		}
		a.weightedSum += r.Sentiment * r.TrustWeight
		a.totalWeight += r.TrustWeight
		a.count++
	}

	out := make(map[string]map[string]NewsAggregate, len(acc))
	for ticker, byDate := range acc {
		out[ticker] = make(map[string]NewsAggregate, len(byDate))
		for date, a := range byDate {
			sentiment := 0.0
			if a.totalWeight > 0 {
				sentiment = a.weightedSum / a.totalWeight
			}
			out[ticker][date] = NewsAggregate{Sentiment: sentiment, Count: a.count}
		}
	}
	return out
}

func ApplyNewsOverride(samples []LabeledSample, news map[string]map[string]NewsAggregate) int {
	applied := 0
	for i := range samples {
		byDate, ok := news[samples[i].Feature.Ticker]
		if !ok {
			continue
		}
		agg, ok := byDate[dateKey(samples[i].Feature.GeneratedAt)]
		if !ok {
			continue
		}
		samples[i].Feature.NewsSentiment = decimal.NewFromFloat(agg.Sentiment)
		samples[i].Feature.NewsCount = agg.Count
		applied++
	}
	return applied
}

func ApplyNewsOverrideToCalibration(samples []CalibrationSample, news map[string]map[string]NewsAggregate) int {
	if len(samples) == 0 {
		return 0
	}
	sentIdx, sentOK := indexOfName(samples[0].Names, "news_sentiment")
	countIdx, countOK := indexOfName(samples[0].Names, "news_count")
	if !sentOK || !countOK {
		return 0
	}

	applied := 0
	for i := range samples {
		byDate, ok := news[samples[i].Ticker]
		if !ok {
			continue
		}
		agg, ok := byDate[dateKey(samples[i].Date)]
		if !ok {
			continue
		}
		samples[i].Vector[sentIdx] = agg.Sentiment
		samples[i].Vector[countIdx] = float64(agg.Count)
		applied++
	}
	return applied
}

func indexOfName(names []string, target string) (int, bool) {
	for i, name := range names {
		if name == target {
			return i, true
		}
	}
	return 0, false
}
