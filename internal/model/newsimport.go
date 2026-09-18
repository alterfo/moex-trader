package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/features"
)

type HistoricalNewsRecord struct {
	Ticker      string
	ArticleID   string
	PublishedAt time.Time
	Sentiment   float64
	TrustWeight float64
	Title       string
}

type finanalysNewsLine struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
	Sentiment   float64 `json:"sentiment"`
	Title       string  `json:"title"`
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
			Title:       raw.Title,
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

func AggregateDailyTopicSignals(records []HistoricalNewsRecord) map[string]map[string]features.TopicSignalAggregate {
	type accum struct {
		negWeighted float64
		negWeight   float64
		sanWeighted float64
		sanWeight   float64
	}
	acc := make(map[string]map[string]*accum)
	for _, r := range records {
		if r.TrustWeight <= 0 || r.Title == "" {
			continue
		}
		events := features.DetectEvents(r.Title)
		if events.Negotiations == 0 && events.Sanctions == 0 {
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
		if events.Negotiations != 0 {
			a.negWeighted += r.Sentiment * r.TrustWeight
			a.negWeight += r.TrustWeight
		}
		if events.Sanctions != 0 {
			a.sanWeighted += r.Sentiment * r.TrustWeight
			a.sanWeight += r.TrustWeight
		}
	}

	out := make(map[string]map[string]features.TopicSignalAggregate, len(acc))
	for ticker, byDate := range acc {
		out[ticker] = make(map[string]features.TopicSignalAggregate, len(byDate))
		for date, a := range byDate {
			negotiations := 0.0
			if a.negWeight > 0 {
				negotiations = a.negWeighted / a.negWeight
			}
			sanctions := 0.0
			if a.sanWeight > 0 {
				sanctions = a.sanWeighted / a.sanWeight
			}
			out[ticker][date] = features.TopicSignalAggregate{Negotiations: negotiations, Sanctions: sanctions}
		}
	}
	return out
}

func ApplyTopicSignalOverrides(samples []LabeledSample, topics map[string]map[string]features.TopicSignalAggregate) int {
	applied := 0
	for i := range samples {
		byDate, ok := topics[samples[i].Feature.Ticker]
		if !ok {
			continue
		}
		agg, ok := byDate[dateKey(samples[i].Feature.GeneratedAt)]
		if !ok {
			continue
		}
		samples[i].Feature.NegotiationsSignal = decimal.NewFromFloat(agg.Negotiations)
		samples[i].Feature.SanctionsSignal = decimal.NewFromFloat(agg.Sanctions)
		applied++
	}
	return applied
}

func ApplyTopicSignalOverridesToCalibration(samples []CalibrationSample, topics map[string]map[string]features.TopicSignalAggregate) int {
	if len(samples) == 0 {
		return 0
	}
	negIdx, negOK := indexOfName(samples[0].Names, "negotiations_signal")
	sanIdx, sanOK := indexOfName(samples[0].Names, "sanctions_signal")
	if !negOK || !sanOK {
		return 0
	}

	applied := 0
	for i := range samples {
		byDate, ok := topics[samples[i].Ticker]
		if !ok {
			continue
		}
		agg, ok := byDate[dateKey(samples[i].Date)]
		if !ok {
			continue
		}
		samples[i].Vector[negIdx] = agg.Negotiations
		samples[i].Vector[sanIdx] = agg.Sanctions
		applied++
	}
	return applied
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

type EventAggregate struct {
	Dividend  int
	Buyback   int
	Sanctions int
	IPO       int
	Report    int
	Delisting int
	MNA       int
	Default   int
}

func AggregateDailyEvents(records []HistoricalNewsRecord) map[string]map[string]EventAggregate {
	acc := make(map[string]map[string]*EventAggregate)
	for _, r := range records {
		if r.TrustWeight <= 0 || r.Title == "" {
			continue
		}
		events := features.DetectEvents(r.Title)
		if events == (features.EventFlags{}) {
			continue
		}
		date := dateKey(r.PublishedAt)
		byDate, ok := acc[r.Ticker]
		if !ok {
			byDate = make(map[string]*EventAggregate)
			acc[r.Ticker] = byDate
		}
		a, ok := byDate[date]
		if !ok {
			a = &EventAggregate{}
			byDate[date] = a
		}
		a.Dividend += events.Dividend
		a.Buyback += events.Buyback
		a.Sanctions += events.Sanctions
		a.IPO += events.IPO
		a.Report += events.Report
		a.Delisting += events.Delisting
		a.MNA += events.MNA
		a.Default += events.Default
	}
	out := make(map[string]map[string]EventAggregate, len(acc))
	for ticker, byDate := range acc {
		out[ticker] = make(map[string]EventAggregate, len(byDate))
		for d, a := range byDate {
			out[ticker][d] = EventAggregate{
				Dividend: a.Dividend, Buyback: a.Buyback, Sanctions: a.Sanctions,
				IPO: a.IPO, Report: a.Report, Delisting: a.Delisting,
				MNA: a.MNA, Default: a.Default,
			}
		}
	}
	return out
}

func ApplyEventOverrides(samples []LabeledSample, events map[string]map[string]EventAggregate) int {
	applied := 0
	for i := range samples {
		byDate, ok := events[samples[i].Feature.Ticker]
		if !ok {
			continue
		}
		agg, ok := byDate[dateKey(samples[i].Feature.GeneratedAt)]
		if !ok {
			continue
		}
		samples[i].Feature.EventDividend = agg.Dividend
		samples[i].Feature.EventBuyback = agg.Buyback
		samples[i].Feature.EventSanctions = agg.Sanctions
		samples[i].Feature.EventIPO = agg.IPO
		samples[i].Feature.EventReport = agg.Report
		samples[i].Feature.EventDelisting = agg.Delisting
		samples[i].Feature.EventMNA = agg.MNA
		samples[i].Feature.EventDefault = agg.Default
		applied++
	}
	return applied
}
