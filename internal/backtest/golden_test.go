package backtest_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

var updateGolden = flag.Bool("update", false, "rewrite internal/backtest/testdata/golden/golden.json")

const (
	goldenFixtureDir = "testdata/golden"
	goldenModelRel   = "../../ensemble_model.json"
	goldenWarmupIdx  = 300
)

var goldenTickers = []string{"SBER", "LKOH", "GAZP"}

type goldenFixtureFile struct {
	Tickers []string                      `json:"tickers"`
	Candles map[string][]goldenCandleFile `json:"candles"`
}

type goldenCandleFile struct {
	Begin  string `json:"begin"`
	End    string `json:"end"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

type goldenResultFile struct {
	RealizedPnl     string `json:"realized_pnl"`
	ClosedTrades    int    `json:"closed_trades"`
	TotalCommission string `json:"total_commission"`
	MaxDrawdownPct  string `json:"max_drawdown_pct"`
	MaxDrawdownRub  string `json:"max_drawdown_rub"`
}

type goldenSource struct {
	candles map[string][]moex.Candle
}

func (s goldenSource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	candles, ok := s.candles[strings.ToUpper(strings.TrimSpace(ticker))]
	if !ok {
		return nil, fmt.Errorf("golden fixture: no candles for %q", ticker)
	}
	return candles, nil
}

func loadGoldenFixture(t *testing.T) map[string][]moex.Candle {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenFixtureDir, "candles.json"))
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	var fixture goldenFixtureFile
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse golden fixture: %v", err)
	}
	out := make(map[string][]moex.Candle, len(fixture.Candles))
	for ticker, rows := range fixture.Candles {
		candles := make([]moex.Candle, 0, len(rows))
		for _, row := range rows {
			candle, err := parseGoldenCandle(row)
			if err != nil {
				t.Fatalf("parse golden candle %s: %v", ticker, err)
			}
			candles = append(candles, candle)
		}
		out[ticker] = candles
	}
	return out
}

func parseGoldenCandle(row goldenCandleFile) (moex.Candle, error) {
	open, err := decimal.NewFromString(row.Open)
	if err != nil {
		return moex.Candle{}, fmt.Errorf("open %q: %w", row.Open, err)
	}
	high, err := decimal.NewFromString(row.High)
	if err != nil {
		return moex.Candle{}, fmt.Errorf("high %q: %w", row.High, err)
	}
	low, err := decimal.NewFromString(row.Low)
	if err != nil {
		return moex.Candle{}, fmt.Errorf("low %q: %w", row.Low, err)
	}
	close, err := decimal.NewFromString(row.Close)
	if err != nil {
		return moex.Candle{}, fmt.Errorf("close %q: %w", row.Close, err)
	}
	volume, err := decimal.NewFromString(row.Volume)
	if err != nil {
		return moex.Candle{}, fmt.Errorf("volume %q: %w", row.Volume, err)
	}
	begin, err := time.Parse("2006-01-02 15:04:05", row.Begin)
	if err != nil {
		return moex.Candle{}, fmt.Errorf("begin %q: %w", row.Begin, err)
	}
	end, err := time.Parse("2006-01-02 15:04:05", row.End)
	if err != nil {
		return moex.Candle{}, fmt.Errorf("end %q: %w", row.End, err)
	}
	return moex.Candle{Open: open, High: high, Low: low, Close: close, Volume: volume, Begin: begin, End: end}, nil
}

func runGoldenBacktest(t *testing.T, candles map[string][]moex.Candle) *backtest.Result {
	t.Helper()
	ensemble, err := model.LoadEnsembleModel(filepath.Join(goldenModelRel))
	if err != nil {
		t.Fatalf("load deployed ensemble model: %v", err)
	}
	first := candles[goldenTickers[0]]
	if len(first) < goldenWarmupIdx+2 {
		t.Fatalf("golden fixture has %d candles, need at least %d", len(first), goldenWarmupIdx+2)
	}
	from := first[goldenWarmupIdx].Begin
	till := first[len(first)-1].End
	engine, err := backtest.NewEngine(backtest.Config{
		Tickers:        goldenTickers,
		From:           from,
		Till:           till,
		Deposit:        decimal.RequireFromString("1000000"),
		MaxLots:        1000,
		CommissionRate: decimal.RequireFromString("0.0005"),
		SpreadPct:      decimal.RequireFromString("0.0005"),
		SlippagePct:    decimal.RequireFromString("0.0005"),
		KillSwitch:     true,
		SignalSource: &model.EnsembleSignalSource{
			Model:          ensemble,
			TargetNotional: decimal.RequireFromString("15000"),
		},
		Source: goldenSource{candles: candles},
	})
	if err != nil {
		t.Fatalf("build golden engine: %v", err)
	}
	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatalf("run golden backtest: %v", err)
	}
	return result
}

func goldenResultToFile(result *backtest.Result) goldenResultFile {
	return goldenResultFile{
		RealizedPnl:     result.RealizedPnl.String(),
		ClosedTrades:    result.ClosedTrades,
		TotalCommission: result.TotalCommission.String(),
		MaxDrawdownPct:  strconv.FormatFloat(result.MaxDrawdownPct, 'g', -1, 64),
		MaxDrawdownRub:  result.MaxDrawdownRub.String(),
	}
}

func loadGoldenResultFile(t *testing.T) goldenResultFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenFixtureDir, "golden.json"))
	if err != nil {
		t.Fatalf("read golden.json: %v", err)
	}
	var golden goldenResultFile
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("parse golden.json: %v", err)
	}
	return golden
}

func goldenMatches(result *backtest.Result, golden goldenResultFile) bool {
	if result.ClosedTrades != golden.ClosedTrades {
		return false
	}
	if !decimal.RequireFromString(result.RealizedPnl.String()).Equal(decimal.RequireFromString(golden.RealizedPnl)) {
		return false
	}
	if !decimal.RequireFromString(result.TotalCommission.String()).Equal(decimal.RequireFromString(golden.TotalCommission)) {
		return false
	}
	if strconv.FormatFloat(result.MaxDrawdownPct, 'g', -1, 64) != golden.MaxDrawdownPct {
		return false
	}
	return decimal.RequireFromString(result.MaxDrawdownRub.String()).Equal(decimal.RequireFromString(golden.MaxDrawdownRub))
}

func TestGoldenBacktest(t *testing.T) {
	result := runGoldenBacktest(t, loadGoldenFixture(t))
	if *updateGolden {
		golden := goldenResultToFile(result)
		raw, err := json.MarshalIndent(golden, "", "  ")
		if err != nil {
			t.Fatalf("marshal golden: %v", err)
		}
		if err := os.WriteFile(filepath.Join(goldenFixtureDir, "golden.json"), append(raw, '\n'), 0o644); err != nil {
			t.Fatalf("write golden.json: %v", err)
		}
		return
	}
	want := loadGoldenResultFile(t)
	if !goldenMatches(result, want) {
		t.Fatalf("golden backtest mismatch:\n got %+v\nwant %+v", goldenResultToFile(result), want)
	}
}

func TestGoldenBacktestPerturbedFixtureFails(t *testing.T) {
	if *updateGolden {
		t.Skip("skipped while updating golden.json")
	}
	want := loadGoldenResultFile(t)
	candles := loadGoldenFixture(t)
	perturbed := make(map[string][]moex.Candle, len(candles))
	for ticker, series := range candles {
		perturbed[ticker] = append([]moex.Candle(nil), series...)
	}
	first := perturbed[goldenTickers[0]]
	idx := goldenWarmupIdx + 20
	if idx >= len(first) {
		t.Fatalf("perturbation index %d out of range for %d candles", idx, len(first))
	}
	first[idx].Close = first[idx].Close.Add(first[idx].Close.Div(decimal.NewFromInt(2)))
	perturbed[goldenTickers[0]] = first

	result := runGoldenBacktest(t, perturbed)
	if goldenMatches(result, want) {
		t.Fatalf("perturbed fixture reproduced the golden result: %+v", goldenResultToFile(result))
	}
}
