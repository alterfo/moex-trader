package walkforward

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func TestDailyRealizedPnlSumsByCloseDateAndFillsTradingDays(t *testing.T) {
	d1 := day(2025, 4, 1)
	d2 := day(2025, 4, 2)
	d3 := day(2025, 4, 3)

	result := backtest.Result{
		EquityCurve: []backtest.EquityPoint{
			{Date: d1, Equity: decimal.NewFromInt(100000)},
			{Date: d2, Equity: decimal.NewFromInt(100050)},
			{Date: d3, Equity: decimal.NewFromInt(100120)},
		},
		Trades: []backtest.Trade{
			{Ticker: "SBER", Action: domain.ActionBuy, NetPnl: decimal.NewFromInt(100), ClosedAt: d1},
			{Ticker: "GAZP", Action: domain.ActionSell, NetPnl: decimal.NewFromInt(-30), ClosedAt: d1},
			{Ticker: "SBER", Action: domain.ActionSell, NetPnl: decimal.NewFromInt(50), ClosedAt: d3},
		},
	}

	returns := DailyRealizedPnl(result)
	if len(returns) != 3 {
		t.Fatalf("len(returns) = %d, want 3", len(returns))
	}
	if !returns[0].Date.Equal(d1) || !returns[0].RealizedNet.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("returns[0] = %+v, want %s=70", returns[0], d1.Format("2006-01-02"))
	}
	if !returns[1].Date.Equal(d2) || !returns[1].RealizedNet.IsZero() {
		t.Fatalf("returns[1] = %+v, want %s=0", returns[1], d2.Format("2006-01-02"))
	}
	if !returns[2].Date.Equal(d3) || !returns[2].RealizedNet.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("returns[2] = %+v, want %s=50", returns[2], d3.Format("2006-01-02"))
	}
}

func TestDailyRealizedPnlEmptyResult(t *testing.T) {
	returns := DailyRealizedPnl(backtest.Result{})
	if len(returns) != 0 {
		t.Fatalf("len(returns) = %d, want 0", len(returns))
	}
}

func TestPeriodReturnsCSVRoundTrip(t *testing.T) {
	original := []PeriodReturn{
		{Date: day(2025, 4, 1), RealizedNet: decimal.NewFromFloat(123.45)},
		{Date: day(2025, 4, 2), RealizedNet: decimal.NewFromInt(0)},
		{Date: day(2025, 4, 3), RealizedNet: decimal.NewFromFloat(-987.65)},
	}
	payload, err := MarshalPeriodReturnsCSV(original)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(payload), "date,realized_net\n") {
		t.Fatalf("CSV header missing: %q", payload)
	}
	parsed, err := ParsePeriodReturnsCSV(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != len(original) {
		t.Fatalf("len(parsed) = %d, want %d", len(parsed), len(original))
	}
	for i := range original {
		if !parsed[i].Date.Equal(original[i].Date) || !parsed[i].RealizedNet.Equal(original[i].RealizedNet) {
			t.Fatalf("row %d = %+v, want %+v", i, parsed[i], original[i])
		}
	}
}

func TestMarshalPeriodReturnsCSVSortsAndRejectsDuplicates(t *testing.T) {
	unsorted := []PeriodReturn{
		{Date: day(2025, 4, 3), RealizedNet: decimal.NewFromInt(3)},
		{Date: day(2025, 4, 1), RealizedNet: decimal.NewFromInt(1)},
	}
	payload, err := MarshalPeriodReturnsCSV(unsorted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "2025-04-01,1\n2025-04-03,3\n") {
		t.Fatalf("marshal did not sort rows: %q", payload)
	}

	duplicate := []PeriodReturn{
		{Date: day(2025, 4, 1), RealizedNet: decimal.NewFromInt(1)},
		{Date: day(2025, 4, 1), RealizedNet: decimal.NewFromInt(2)},
	}
	if _, err := MarshalPeriodReturnsCSV(duplicate); err == nil {
		t.Fatal("MarshalPeriodReturnsCSV should reject duplicate dates")
	}
}

func TestParsePeriodReturnsCSVRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"bad_header":    "date,wrong\n2025-04-01,1\n",
		"bad_date":      "date,realized_net\n2025-04-xx,1\n",
		"bad_decimal":   "date,realized_net\n2025-04-01,1.2.3\n",
		"wrong_columns": "date,realized_net\n2025-04-01,1,extra\n",
		"unsorted":      "date,realized_net\n2025-04-03,1\n2025-04-01,2\n",
		"duplicate":     "date,realized_net\n2025-04-01,1\n2025-04-01,2\n",
		"empty":         "",
	}
	for name, content := range cases {
		if _, err := ParsePeriodReturnsCSV([]byte(content)); err == nil {
			t.Fatalf("%s: ParsePeriodReturnsCSV returned no error", name)
		}
	}
}

func TestSaveLoadPeriodReturns(t *testing.T) {
	original := []PeriodReturn{
		{Date: day(2025, 4, 1), RealizedNet: decimal.NewFromFloat(100.25)},
		{Date: day(2025, 4, 2), RealizedNet: decimal.NewFromFloat(-50.75)},
	}
	dir := t.TempDir()
	if err := SavePeriodReturns(dir, original); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPeriodReturns(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(original) {
		t.Fatalf("len(loaded) = %d, want %d", len(loaded), len(original))
	}
	for i := range original {
		if !loaded[i].Date.Equal(original[i].Date) || !loaded[i].RealizedNet.Equal(original[i].RealizedNet) {
			t.Fatalf("row %d = %+v, want %+v", i, loaded[i], original[i])
		}
	}
}

func TestWindowValidatePeriodReturnsConsistency(t *testing.T) {
	base := Window{ID: "w", Config: testConfig(), ConfigHash: ConfigHash(testConfig()), CreatedAt: time.Now().UTC()}

	fileOnly := base
	fileOnly.PeriodReturnsFile = PeriodReturnsFileName
	if err := fileOnly.Validate(); err == nil || !strings.Contains(err.Error(), "SHA256 is required") {
		t.Fatalf("Validate() error = %v, want missing-SHA256 error", err)
	}

	hashOnly := base
	hashOnly.PeriodReturnsSHA256 = HashBytes([]byte("x"))
	if err := hashOnly.Validate(); err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Fatalf("Validate() error = %v, want missing-file error", err)
	}
}

func TestSaveLoadWindowWithPeriodReturns(t *testing.T) {
	returns := []PeriodReturn{
		{Date: day(2025, 4, 1), RealizedNet: decimal.NewFromInt(42)},
		{Date: day(2025, 4, 2), RealizedNet: decimal.NewFromInt(-7)},
	}
	w := Window{
		ID:         NewWindowID(day(2025, 4, 1), day(2025, 6, 30)),
		Config:     testConfig(),
		ConfigHash: ConfigHash(testConfig()),
	}
	if err := w.SetPeriodReturns(returns); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "2025-04-01_2025-06-30")
	if err := SavePeriodReturns(dir, returns); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, w, ""); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PeriodReturnsFile != PeriodReturnsFileName || loaded.PeriodReturnsSHA256 == "" {
		t.Fatalf("period returns metadata did not round-trip: %+v", loaded)
	}

	path := filepath.Join(dir, PeriodReturnsFileName)
	if err := os.WriteFile(path, []byte("date,realized_net\n2025-04-01,999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "period returns SHA256 mismatch") {
		t.Fatalf("Load() error = %v, want period returns SHA256 mismatch", err)
	}
}
