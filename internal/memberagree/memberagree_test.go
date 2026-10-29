package memberagree

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

func day(year int, month time.Month, dayOfMonth int) time.Time {
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
}

func TestJoinAttachesDecisionByDay(t *testing.T) {
	trades := []backtest.Trade{
		{Ticker: "SBER", Action: domain.ActionBuy, Lots: 1, OpenedAt: day(2026, 1, 5), ClosedAt: day(2026, 1, 9), NetPnl: decimal.NewFromInt(100)},
		{Ticker: "GAZP", Action: domain.ActionSell, Lots: 2, OpenedAt: day(2026, 1, 6), ClosedAt: day(2026, 1, 12), NetPnl: decimal.NewFromInt(-50)},
		{Ticker: "LKOH", Action: domain.ActionBuy, Lots: 1, OpenedAt: day(2026, 1, 7), ClosedAt: day(2026, 1, 8), NetPnl: decimal.NewFromInt(10)},
	}
	decisions := []Decision{
		{Ticker: "SBER", Date: day(2026, 1, 5).Add(7 * time.Hour), LGB: 0.80, XGB: 0.70, Logistic: 0.65},
		{Ticker: "GAZP", Date: day(2026, 1, 6), LGB: 0.20, XGB: 0.30, Logistic: 0.35},
	}
	rows, unmatched := Join(trades, decisions, 0.60, 0.40)
	if unmatched != 1 {
		t.Fatalf("unmatched = %d, want 1", unmatched)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if !rows[0].Unanimous || !rows[0].SignAgreement {
		t.Fatalf("SBER row must be unanimous on both definitions: %+v", rows[0])
	}
	if !rows[1].Unanimous || !rows[1].SignAgreement {
		t.Fatalf("GAZP row must be unanimous on both definitions: %+v", rows[1])
	}
	if math.Abs(rows[0].Ensemble-(0.80+0.70+0.65)/3) > 1e-12 {
		t.Fatalf("ensemble probability = %v", rows[0].Ensemble)
	}
	if rows[0].Ticker != "SBER" || rows[0].Action != string(domain.ActionBuy) {
		t.Fatalf("joined row lost trade fields: %+v", rows[0])
	}
}

func TestUnanimousNeedsAllMembersPastThreshold(t *testing.T) {
	base := []backtest.Trade{{Ticker: "SBER", Action: domain.ActionBuy, OpenedAt: day(2026, 1, 5), ClosedAt: day(2026, 1, 6), NetPnl: decimal.NewFromInt(1)}}
	cases := []struct {
		name     string
		decision Decision
		want     bool
	}{
		{"all above buy", Decision{Ticker: "SBER", Date: day(2026, 1, 5), LGB: 0.61, XGB: 0.70, Logistic: 0.90}, true},
		{"one below buy", Decision{Ticker: "SBER", Date: day(2026, 1, 5), LGB: 0.61, XGB: 0.59, Logistic: 0.90}, false},
		{"one at threshold", Decision{Ticker: "SBER", Date: day(2026, 1, 5), LGB: 0.60, XGB: 0.60, Logistic: 0.60}, true},
		{"all below buy", Decision{Ticker: "SBER", Date: day(2026, 1, 5), LGB: 0.30, XGB: 0.30, Logistic: 0.30}, false},
	}
	for _, testCase := range cases {
		rows, _ := Join(base, []Decision{testCase.decision}, 0.60, 0.40)
		if len(rows) != 1 {
			t.Fatalf("%s: rows = %d", testCase.name, len(rows))
		}
		if rows[0].Unanimous != testCase.want {
			t.Fatalf("%s: unanimous = %v, want %v", testCase.name, rows[0].Unanimous, testCase.want)
		}
	}

	sell := []backtest.Trade{{Ticker: "SBER", Action: domain.ActionSell, OpenedAt: day(2026, 1, 5), ClosedAt: day(2026, 1, 6), NetPnl: decimal.NewFromInt(1)}}
	rows, _ := Join(sell, []Decision{{Ticker: "SBER", Date: day(2026, 1, 5), LGB: 0.40, XGB: 0.35, Logistic: 0.20}}, 0.60, 0.40)
	if len(rows) != 1 || !rows[0].Unanimous {
		t.Fatalf("sell unanimous = %+v", rows)
	}
	rows, _ = Join(sell, []Decision{{Ticker: "SBER", Date: day(2026, 1, 5), LGB: 0.40, XGB: 0.45, Logistic: 0.20}}, 0.60, 0.40)
	if len(rows) != 1 || rows[0].Unanimous {
		t.Fatalf("sell with one member above 0.40 must be split: %+v", rows)
	}
}

func TestSignAgreementIgnoresThreshold(t *testing.T) {
	decision := Decision{Ticker: "SBER", Date: day(2026, 1, 5), LGB: 0.55, XGB: 0.54, Logistic: 0.53}
	if !signAgreement(decision) {
		t.Fatal("three members above 0.5 must count as sign agreement")
	}
	decision.Logistic = 0.49
	if signAgreement(decision) {
		t.Fatal("a member below 0.5 must break sign agreement")
	}
}

func syntheticRows(trades int, start time.Time, unanimous, split float64, conviction []float64) []Row {
	rows := make([]Row, 0, trades)
	for i := 0; i < trades; i++ {
		row := Row{
			Window:    "q1",
			Seed:      42,
			Ticker:    "SBER",
			Action:    "BUY",
			OpenedAt:  start.AddDate(0, 0, 3*i),
			ClosedAt:  start.AddDate(0, 0, 3*i+1),
			Lots:      1,
			Unanimous: i%2 == 0,
		}
		value := split
		if row.Unanimous {
			value = unanimous
		}
		row.NetPnl = decimal.NewFromFloat(value)
		level := conviction[i%len(conviction)]
		if i%4 < 2 {
			row.Ensemble = 0.5 + level
		} else {
			row.Ensemble = 0.5 - level
		}
		rows = append(rows, row)
	}
	return rows
}

func seedRows(rows []Row, seeds ...int) []Row {
	out := append([]Row(nil), rows...)
	for _, seed := range seeds {
		for _, row := range rows {
			copied := row
			copied.Seed = seed
			out = append(out, copied)
		}
	}
	return out
}

func testOptions() Options {
	opts := DefaultOptions()
	opts.MinTrades = 20
	opts.MinDates = 10
	opts.MinSegment = 5
	opts.Bootstrap = 500
	opts.Placebos = 200
	return opts
}

func TestAnalyzeConfirmsWhenSplitTradesAreWorse(t *testing.T) {
	rows := seedRows(syntheticRows(60, day(2026, 1, 5), 100, -100, []float64{0.02, 0.10, 0.30}), 0, 1, 2, 3, 4)
	report := Analyze(rows, 0, testOptions())
	if report.Primary.Primary.Diff <= 0 {
		t.Fatalf("difference = %v, want positive", report.Primary.Primary.Diff)
	}
	if report.Primary.Primary.Low <= 0 {
		t.Fatalf("lower bound = %v, want above zero", report.Primary.Primary.Low)
	}
	if !report.Primary.PassSize || !report.Primary.PassCI || !report.Primary.PassConv {
		t.Fatalf("gates: size=%v ci=%v conviction=%v", report.Primary.PassSize, report.Primary.PassCI, report.Primary.PassConv)
	}
	if !report.PassSeed {
		t.Fatal("seed gate must hold when every seed agrees")
	}
	if !strings.Contains(report.Verdict, "confirmed") {
		t.Fatalf("verdict = %q", report.Verdict)
	}
	if len(report.BySeed) != len(testOptions().Seeds) {
		t.Fatalf("by-seed rows = %d", len(report.BySeed))
	}
	if !strings.Contains(report.Markdown(), "Verdict") {
		t.Fatal("markdown report must state a verdict")
	}
	if !strings.Contains(report.Markdown(), "member spread") {
		t.Fatal("markdown report must carry the descriptive spread terciles")
	}
}

func TestAnalyzeNullWhenSegmentsAreEqual(t *testing.T) {
	rows := syntheticRows(60, day(2026, 1, 5), 100, 100, []float64{0.02, 0.10, 0.30})
	report := Analyze(rows, 0, testOptions())
	if report.Primary.PassCI {
		t.Fatalf("interval gate must fail when the difference is zero: %+v", report.Primary.Primary)
	}
	if !strings.Contains(report.Verdict, "null") {
		t.Fatalf("verdict = %q", report.Verdict)
	}
}

func TestAnalyzeFlagsConvictionConfound(t *testing.T) {
	rows := syntheticRows(60, day(2026, 1, 5), 0, 0, []float64{0.02, 0.10, 0.30})
	for i := range rows {
		if rows[i].Unanimous {
			rows[i].NetPnl = decimal.NewFromFloat(200)
		} else {
			rows[i].NetPnl = decimal.NewFromFloat(100)
		}
	}
	for i := range rows {
		if i%3 == 2 {
			if rows[i].Unanimous {
				rows[i].NetPnl = decimal.NewFromFloat(-50)
			} else {
				rows[i].NetPnl = decimal.NewFromFloat(50)
			}
		}
	}
	report := Analyze(rows, 0, testOptions())
	if report.Primary.PassCI && !report.Primary.PassConv && !strings.Contains(report.Verdict, "conviction") {
		t.Fatalf("verdict must name the conviction confound, got %q", report.Verdict)
	}
}

func TestAnalyzeIsDeterministic(t *testing.T) {
	rows := syntheticRows(60, day(2026, 1, 5), 100, -100, []float64{0.02, 0.10, 0.30})
	first := Analyze(rows, 0, testOptions())
	second := Analyze(rows, 0, testOptions())
	if first.Primary.Primary.Low != second.Primary.Primary.Low || first.Primary.Primary.High != second.Primary.Primary.High {
		t.Fatalf("bootstrap is not reproducible: %v/%v vs %v/%v", first.Primary.Primary.Low, first.Primary.Primary.High, second.Primary.Primary.Low, second.Primary.Primary.High)
	}
}

func TestAnalyzeReportsUnmatchedAndEmptyPrimarySeed(t *testing.T) {
	rows := syntheticRows(30, day(2026, 1, 5), 100, -100, []float64{0.02, 0.10, 0.30})
	for i := range rows {
		rows[i].Seed = 0
	}
	opts := testOptions()
	report := Analyze(rows, 4, opts)
	if report.Overall.Unmatched != 4 {
		t.Fatalf("unmatched = %d, want 4", report.Overall.Unmatched)
	}
	if report.Primary.Rows != 0 {
		t.Fatalf("primary rows = %d, want 0 without the primary seed", report.Primary.Rows)
	}
	if !strings.Contains(report.Verdict, "null") {
		t.Fatalf("verdict = %q", report.Verdict)
	}
}

func TestCSVRoundTrip(t *testing.T) {
	rows := syntheticRows(6, day(2026, 1, 5), 100, -100, []float64{0.02})
	rows[0].Ticker = "GAZP"
	rows[0].Action = "SELL"
	rows[0].NetPnl = decimal.RequireFromString("-123.4567")
	path := t.TempDir() + "/rows.csv"
	if err := WriteCSV(path, rows); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	loaded, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("ReadCSV: %v", err)
	}
	if len(loaded) != len(rows) {
		t.Fatalf("loaded %d rows, want %d", len(loaded), len(rows))
	}
	if loaded[0].Ticker != "GAZP" || loaded[0].Action != "SELL" || loaded[0].NetPnl.String() != "-123.4567" {
		t.Fatalf("first row round-trip = %+v", loaded[0])
	}
	if loaded[1].Unanimous != rows[1].Unanimous || loaded[1].SignAgreement != rows[1].SignAgreement {
		t.Fatalf("flags round-trip = %+v", loaded[1])
	}
	if !loaded[0].OpenedAt.Equal(rows[0].OpenedAt) {
		t.Fatalf("opened_at round-trip = %v", loaded[0].OpenedAt)
	}
}

type stubSource struct {
	vol bool
}

func (s stubSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	return domain.TradeSignal{Ticker: feature.Ticker, Action: domain.ActionHold, GeneratedAt: feature.GeneratedAt}, nil
}

func (s stubSource) VolScaleEnabled() bool { return s.vol }

func TestRecorderForwardsVolScale(t *testing.T) {
	recorder := NewRecorder(stubSource{vol: true}, nil)
	if !recorder.VolScaleEnabled() {
		t.Fatal("recorder must forward VolScaleEnabled")
	}
	plain := NewRecorder(stubSource{vol: false}, nil)
	if plain.VolScaleEnabled() {
		t.Fatal("recorder must not invent vol scaling")
	}
}

func TestRecorderCapturesMemberProbabilitiesPerDecisionBar(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"feature_order":  []string{"return_pct"},
		"buy_threshold":  0.60,
		"sell_threshold": 0.40,
		"logistic":       map[string]any{"mean": []float64{0}, "std": []float64{1}, "coef": []float64{0}, "bias": 0},
	})
	if err != nil {
		t.Fatalf("marshal model: %v", err)
	}
	path := filepath.Join(t.TempDir(), "model.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	ensemble, err := model.LoadEnsembleModel(path)
	if err != nil {
		t.Fatalf("load model: %v", err)
	}

	recorder := NewRecorder(stubSource{}, ensemble)
	feature := domain.FeatureContext{
		Ticker:      "sber",
		GeneratedAt: day(2026, 1, 5),
		LastPrice:   decimal.NewFromInt(101),
		PrevClose:   decimal.NewFromInt(100),
	}
	if _, err := recorder.Generate(context.Background(), feature); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	decisions := recorder.Decisions()
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if decisions[0].Ticker != "SBER" || !decisions[0].Date.Equal(day(2026, 1, 5)) {
		t.Fatalf("decision key = %+v", decisions[0])
	}

	trades := []backtest.Trade{{
		Ticker:   "SBER",
		Action:   domain.ActionBuy,
		Lots:     1,
		OpenedAt: day(2026, 1, 5),
		ClosedAt: day(2026, 1, 9),
		NetPnl:   decimal.NewFromInt(10),
	}}
	rows, unmatched := Join(trades, decisions, ensemble.BuyThreshold, ensemble.SellThreshold)
	if unmatched != 0 || len(rows) != 1 {
		t.Fatalf("joined rows = %d, unmatched = %d", len(rows), unmatched)
	}
	if rows[0].Unanimous {
		t.Fatalf("a flat member set at 0.5 must not count as a unanimous BUY: %+v", rows[0])
	}
}

func TestLoadPathSumsUnmatchedMeta(t *testing.T) {
	dir := t.TempDir()
	rows := syntheticRows(4, day(2026, 1, 5), 100, -100, []float64{0.02})
	for _, name := range []string{"q1.csv", "q2.csv"} {
		path := filepath.Join(dir, name)
		if err := WriteCSV(path, rows); err != nil {
			t.Fatalf("WriteCSV: %v", err)
		}
		if err := WriteMeta(path, Meta{Trades: len(rows) + 2, Unmatched: 2}); err != nil {
			t.Fatalf("WriteMeta: %v", err)
		}
	}
	loaded, unmatched, err := LoadPath(dir)
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if len(loaded) != 8 {
		t.Fatalf("loaded %d rows, want 8", len(loaded))
	}
	if unmatched != 4 {
		t.Fatalf("unmatched = %d, want 4", unmatched)
	}
	single, singleUnmatched, err := LoadPath(filepath.Join(dir, "q1.csv"))
	if err != nil {
		t.Fatalf("LoadPath file: %v", err)
	}
	if len(single) != 4 || singleUnmatched != 2 {
		t.Fatalf("single file: rows=%d unmatched=%d", len(single), singleUnmatched)
	}
}
