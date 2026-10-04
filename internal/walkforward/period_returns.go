package walkforward

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

const PeriodReturnsFileName = "period_returns.csv"

type PeriodReturn struct {
	Date        time.Time
	RealizedNet decimal.Decimal
}

func DailyRealizedPnl(result backtest.Result) []PeriodReturn {
	realized := make(map[string]decimal.Decimal)
	for _, trade := range result.Trades {
		if trade.ClosedAt.IsZero() {
			continue
		}
		key := dateKey(trade.ClosedAt)
		realized[key] = realized[key].Add(trade.NetPnl)
	}

	days := make(map[string]time.Time)
	for _, point := range result.EquityCurve {
		key := dateKey(point.Date)
		if _, ok := days[key]; !ok {
			days[key] = point.Date.UTC()
		}
	}
	for _, trade := range result.Trades {
		if trade.ClosedAt.IsZero() {
			continue
		}
		key := dateKey(trade.ClosedAt)
		if _, ok := days[key]; !ok {
			days[key] = trade.ClosedAt.UTC()
		}
	}

	keys := make([]string, 0, len(days))
	for key := range days {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]PeriodReturn, 0, len(keys))
	for _, key := range keys {
		out = append(out, PeriodReturn{Date: days[key], RealizedNet: realized[key]})
	}
	return out
}

func dateKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func MarshalPeriodReturnsCSV(returns []PeriodReturn) ([]byte, error) {
	sorted, err := normalizePeriodReturns(returns)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"date", "realized_net"}); err != nil {
		return nil, fmt.Errorf("walk-forward: write period returns header: %w", err)
	}
	for _, r := range sorted {
		if err := writer.Write([]string{dateKey(r.Date), r.RealizedNet.String()}); err != nil {
			return nil, fmt.Errorf("walk-forward: write period returns row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("walk-forward: flush period returns: %w", err)
	}
	return buf.Bytes(), nil
}

func normalizePeriodReturns(returns []PeriodReturn) ([]PeriodReturn, error) {
	sorted := make([]PeriodReturn, len(returns))
	copy(sorted, returns)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })
	for i, r := range sorted {
		if r.Date.IsZero() {
			return nil, fmt.Errorf("walk-forward: period return %d has a zero date", i)
		}
		if i > 0 && !sorted[i-1].Date.Before(sorted[i].Date) {
			return nil, fmt.Errorf("walk-forward: period returns have duplicate or unsorted date %s", dateKey(sorted[i].Date))
		}
	}
	return sorted, nil
}

func ParsePeriodReturnsCSV(data []byte) ([]PeriodReturn, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("walk-forward: parse period returns: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("walk-forward: period returns file is empty")
	}
	if len(records[0]) != 2 || records[0][0] != "date" || records[0][1] != "realized_net" {
		return nil, fmt.Errorf("walk-forward: period returns header must be date,realized_net")
	}
	out := make([]PeriodReturn, 0, len(records)-1)
	for i, row := range records[1:] {
		if len(row) != 2 {
			return nil, fmt.Errorf("walk-forward: period returns row %d has %d columns, want 2", i+1, len(row))
		}
		date, err := time.Parse("2006-01-02", row[0])
		if err != nil {
			return nil, fmt.Errorf("walk-forward: period returns row %d has invalid date %q: %w", i+1, row[0], err)
		}
		value, err := decimal.NewFromString(row[1])
		if err != nil {
			return nil, fmt.Errorf("walk-forward: period returns row %d has invalid realized_net %q: %w", i+1, row[1], err)
		}
		out = append(out, PeriodReturn{Date: date, RealizedNet: value})
	}
	for i := 1; i < len(out); i++ {
		if !out[i-1].Date.Before(out[i].Date) {
			return nil, fmt.Errorf("walk-forward: period returns are not strictly increasing at row %d", i+1)
		}
	}
	return out, nil
}

func SavePeriodReturns(dir string, returns []PeriodReturn) error {
	payload, err := MarshalPeriodReturnsCSV(returns)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("walk-forward: create window dir: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, PeriodReturnsFileName), payload)
}

func LoadPeriodReturns(dir string) ([]PeriodReturn, error) {
	data, err := os.ReadFile(filepath.Join(dir, PeriodReturnsFileName))
	if err != nil {
		return nil, fmt.Errorf("walk-forward: read period returns %q: %w", filepath.Join(dir, PeriodReturnsFileName), err)
	}
	return ParsePeriodReturnsCSV(data)
}

func (w *Window) SetPeriodReturns(returns []PeriodReturn) error {
	payload, err := MarshalPeriodReturnsCSV(returns)
	if err != nil {
		return err
	}
	w.PeriodReturnsFile = PeriodReturnsFileName
	w.PeriodReturnsSHA256 = HashBytes(payload)
	return nil
}
