package model

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const (
	scoreTableBuy      = 0.60
	scoreTableSell     = 0.40
	scoreTableMaxStale = 7 * 24 * time.Hour
)

type scoreRow struct {
	date time.Time
	p    float64
}

type ScoreTableSource struct {
	rows           map[string][]scoreRow
	MaxLots        int
	TargetNotional decimal.Decimal
}

func LoadScoreTable(path string, maxLots int, targetNotional decimal.Decimal) (*ScoreTableSource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("score table: open %q: %w", path, err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("score table: read %q: %w", path, err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("score table: %q has no data rows", path)
	}
	tickerIdx, dateIdx, pIdx := -1, -1, -1
	for i, name := range records[0] {
		switch strings.TrimSpace(name) {
		case "ticker":
			tickerIdx = i
		case "date":
			dateIdx = i
		case "p":
			pIdx = i
		}
	}
	if tickerIdx < 0 || dateIdx < 0 || pIdx < 0 {
		return nil, fmt.Errorf("score table: %q must have ticker,date,p columns", path)
	}
	rows := make(map[string][]scoreRow)
	for _, record := range records[1:] {
		if len(record) <= tickerIdx || len(record) <= dateIdx || len(record) <= pIdx {
			continue
		}
		date, err := time.Parse("2006-01-02", strings.TrimSpace(record[dateIdx]))
		if err != nil {
			return nil, fmt.Errorf("score table: parse date %q: %w", record[dateIdx], err)
		}
		p, err := strconv.ParseFloat(strings.TrimSpace(record[pIdx]), 64)
		if err != nil {
			return nil, fmt.Errorf("score table: parse p %q: %w", record[pIdx], err)
		}
		if math.IsNaN(p) || math.IsInf(p, 0) {
			continue
		}
		ticker := strings.ToUpper(strings.TrimSpace(record[tickerIdx]))
		rows[ticker] = append(rows[ticker], scoreRow{date: date, p: p})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("score table: %q has no usable rows", path)
	}
	for ticker := range rows {
		series := rows[ticker]
		sort.Slice(series, func(i, j int) bool { return series[i].date.Before(series[j].date) })
	}
	return &ScoreTableSource{rows: rows, MaxLots: maxLots, TargetNotional: targetNotional}, nil
}

func (s *ScoreTableSource) lookup(ticker string, at time.Time) (float64, bool) {
	series := s.rows[strings.ToUpper(strings.TrimSpace(ticker))]
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	i := sort.Search(len(series), func(i int) bool { return series[i].date.After(day) })
	if i == 0 {
		return 0, false
	}
	row := series[i-1]
	if day.Sub(row.date) > scoreTableMaxStale {
		return 0, false
	}
	return row.p, true
}

func (s *ScoreTableSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	probability, ok := s.lookup(feature.Ticker, feature.GeneratedAt)
	if !ok {
		return domain.TradeSignal{
			Ticker:      feature.Ticker,
			Action:      domain.ActionHold,
			Confidence:  decimal.Zero,
			GeneratedAt: feature.GeneratedAt,
			Reasoning:   "scoretable:missing_or_stale",
			HoldReason:  domain.HoldReasonModel,
		}, nil
	}
	signal := domain.TradeSignal{
		Ticker:      feature.Ticker,
		Action:      domain.ActionHold,
		Confidence:  decimal.NewFromFloat(math.Abs(probability-0.5) * 2),
		Probability: decimal.NewFromFloat(probability),
		GeneratedAt: feature.GeneratedAt,
		Reasoning:   fmt.Sprintf("scoretable:p=%.4f", probability),
	}
	switch {
	case probability >= scoreTableBuy:
		signal.Action = domain.ActionBuy
		signal.TargetLots = s.lotsFor(feature)
	case probability <= scoreTableSell:
		signal.Action = domain.ActionSell
		signal.TargetLots = s.lotsFor(feature)
	default:
		signal.HoldReason = domain.HoldReasonModel
	}
	return signal, nil
}

func (s *ScoreTableSource) lotsFor(feature domain.FeatureContext) int {
	if !s.TargetNotional.IsPositive() || !feature.LastPrice.IsPositive() {
		return s.MaxLots
	}
	perUnit := feature.LastPrice
	if feature.LotSize.IsPositive() {
		perUnit = feature.LastPrice.Mul(feature.LotSize)
	}
	lots := s.TargetNotional.Div(perUnit).Round(0).IntPart()
	if lots < 1 {
		return 1
	}
	return int(lots)
}
