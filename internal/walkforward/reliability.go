package walkforward

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ReliabilityRow struct {
	Date        time.Time
	Ticker      string
	Probability float64
	Label       float64
}

func MarshalReliabilityCSV(rows []ReliabilityRow) ([]byte, error) {
	sorted, err := normalizeReliabilityRows(rows)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"date", "ticker", "probability", "label"}); err != nil {
		return nil, fmt.Errorf("walk-forward: write reliability header: %w", err)
	}
	for _, row := range sorted {
		record := []string{
			dateKey(row.Date),
			strings.ToUpper(strings.TrimSpace(row.Ticker)),
			strconv.FormatFloat(row.Probability, 'g', -1, 64),
			strconv.FormatFloat(row.Label, 'g', -1, 64),
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("walk-forward: write reliability row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("walk-forward: flush reliability: %w", err)
	}
	return buf.Bytes(), nil
}

func ParseReliabilityCSV(data []byte) ([]ReliabilityRow, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("walk-forward: parse reliability: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("walk-forward: reliability file is empty")
	}
	if len(records[0]) != 4 || records[0][0] != "date" || records[0][1] != "ticker" || records[0][2] != "probability" || records[0][3] != "label" {
		return nil, fmt.Errorf("walk-forward: reliability header must be date,ticker,probability,label")
	}
	out := make([]ReliabilityRow, 0, len(records)-1)
	for i, record := range records[1:] {
		if len(record) != 4 {
			return nil, fmt.Errorf("walk-forward: reliability row %d has %d columns, want 4", i+1, len(record))
		}
		date, err := time.Parse("2006-01-02", record[0])
		if err != nil {
			return nil, fmt.Errorf("walk-forward: reliability row %d has invalid date %q: %w", i+1, record[0], err)
		}
		probability, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			return nil, fmt.Errorf("walk-forward: reliability row %d has invalid probability %q: %w", i+1, record[2], err)
		}
		label, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			return nil, fmt.Errorf("walk-forward: reliability row %d has invalid label %q: %w", i+1, record[3], err)
		}
		out = append(out, ReliabilityRow{
			Date:        date,
			Ticker:      strings.ToUpper(strings.TrimSpace(record[1])),
			Probability: probability,
			Label:       label,
		})
	}
	sorted, err := normalizeReliabilityRows(out)
	if err != nil {
		return nil, err
	}
	return sorted, nil
}

func SaveReliability(dir string, rows []ReliabilityRow) error {
	payload, err := MarshalReliabilityCSV(rows)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("walk-forward: create window dir: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, ReliabilityFileName), payload)
}

func LoadReliability(dir string) ([]ReliabilityRow, error) {
	data, err := os.ReadFile(filepath.Join(dir, ReliabilityFileName))
	if err != nil {
		return nil, fmt.Errorf("walk-forward: read reliability %q: %w", filepath.Join(dir, ReliabilityFileName), err)
	}
	return ParseReliabilityCSV(data)
}

func (w *Window) SetReliabilityRows(rows []ReliabilityRow) error {
	payload, err := MarshalReliabilityCSV(rows)
	if err != nil {
		return err
	}
	w.ReliabilityFile = ReliabilityFileName
	w.ReliabilitySHA256 = HashBytes(payload)
	return nil
}

func normalizeReliabilityRows(rows []ReliabilityRow) ([]ReliabilityRow, error) {
	sorted := make([]ReliabilityRow, len(rows))
	copy(sorted, rows)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Date.Equal(sorted[j].Date) {
			return sorted[i].Ticker < sorted[j].Ticker
		}
		return sorted[i].Date.Before(sorted[j].Date)
	})
	for i, row := range sorted {
		if row.Date.IsZero() {
			return nil, fmt.Errorf("walk-forward: reliability row %d has a zero date", i)
		}
		ticker := strings.ToUpper(strings.TrimSpace(row.Ticker))
		if ticker == "" {
			return nil, fmt.Errorf("walk-forward: reliability row %d has an empty ticker", i)
		}
		sorted[i].Ticker = ticker
		if math.IsNaN(row.Probability) || math.IsInf(row.Probability, 0) || row.Probability < 0 || row.Probability > 1 {
			return nil, fmt.Errorf("walk-forward: reliability row %d probability out of range: %v", i, row.Probability)
		}
		if row.Label != 0 && row.Label != 1 {
			return nil, fmt.Errorf("walk-forward: reliability row %d label must be 0 or 1, got %v", i, row.Label)
		}
		if i > 0 && sorted[i-1].Date.Equal(sorted[i].Date) && sorted[i-1].Ticker == sorted[i].Ticker {
			return nil, fmt.Errorf("walk-forward: duplicate reliability row for %s on %s", sorted[i].Ticker, dateKey(sorted[i].Date))
		}
	}
	return sorted, nil
}
