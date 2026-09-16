package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultFeatures = "rsi_14,dist_ma20_pct,dist_ma50_pct,mom_21d,mom_63d"
	defaultOutCol   = "p_xsec"
	dateLayout      = "2006-01-02"
)

type options struct {
	inPath   string
	outPath  string
	features string
	outCol   string
	exclude  string
}

// row keeps only what ranking needs: identity plus each requested feature.
type row struct {
	ticker string
	date   string
	values map[string]float64
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	records, err := readDataset(opts.inPath, strings.Split(opts.features, ","), splitComma(opts.exclude))
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("xsecrank: no usable rows in %q", opts.inPath)
	}

	byDate := groupByDate(records)
	ranks := make(map[string]map[string]float64, len(records))
	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	features := splitComma(opts.features)
	for _, date := range dates {
		group := byDate[date]
		if len(group) < 2 {
			continue
		}
		scores := compositePercentile(group, features)
		for _, r := range group {
			if ranks[r.ticker] == nil {
				ranks[r.ticker] = make(map[string]float64)
			}
			ranks[r.ticker][r.date] = scores[r.ticker]
		}
	}

	out, err := os.Create(opts.outPath)
	if err != nil {
		return fmt.Errorf("xsecrank: create %q: %w", opts.outPath, err)
	}
	defer out.Close()
	writer := csv.NewWriter(out)
	defer writer.Flush()
	if err := writer.Write([]string{"ticker", "date", opts.outCol}); err != nil {
		return fmt.Errorf("xsecrank: write header: %w", err)
	}
	written := 0
	for _, date := range dates {
		for _, r := range byDate[date] {
			rank, ok := ranks[r.ticker][r.date]
			if !ok {
				continue
			}
			if err := writer.Write([]string{r.ticker, r.date, strconv.FormatFloat(rank, 'f', 6, 64)}); err != nil {
				return fmt.Errorf("xsecrank: write row: %w", err)
			}
			written++
		}
	}
	log.Printf("xsecrank: wrote %d rows to %s over %d dates", written, opts.outPath, len(dates))
	return nil
}

// compositePercentile ranks each feature across the date's cross-section
// (0..1, average-rank percentile) and averages the per-feature ranks into one
// score per ticker. Averaging ranks rather than raw values keeps the score
// insensitive to the wildly different scales of the features involved.
func compositePercentile(group []row, features []string) map[string]float64 {
	n := len(group)
	composite := make(map[string]float64, n)
	counts := make(map[string]int, n)
	for _, feat := range features {
		values := make([]float64, n)
		for i, r := range group {
			values[i] = r.values[feat]
		}
		ranks := percentileRanks(values)
		for i, r := range group {
			composite[r.ticker] += ranks[i]
			counts[r.ticker]++
		}
	}
	for ticker := range composite {
		if counts[ticker] > 0 {
			composite[ticker] /= float64(counts[ticker])
		}
	}
	return composite
}

// percentileRanks returns the fraction of values strictly below each value,
// with ties sharing the midpoint of their span. Range is [0,1].
func percentileRanks(values []float64) []float64 {
	n := len(values)
	out := make([]float64, n)
	if n <= 1 {
		for i := range out {
			out[i] = 0.5
		}
		return out
	}
	for i := range values {
		less, equal := 0, 0
		for j := range values {
			switch {
			case values[j] < values[i]:
				less++
			case values[j] == values[i]:
				equal++
			}
		}
		out[i] = (float64(less) + float64(equal-1)/2) / float64(n-1)
	}
	return out
}

func readDataset(path string, features, exclude []string) ([]row, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("xsecrank: open %q: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("xsecrank: read header %q: %w", path, err)
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	tickerIdx, ok := index["ticker"]
	if !ok {
		return nil, fmt.Errorf("xsecrank: %q has no ticker column", path)
	}
	dateIdx, ok := index["date"]
	if !ok {
		return nil, fmt.Errorf("xsecrank: %q has no date column", path)
	}
	for _, feat := range features {
		if _, ok := index[feat]; !ok {
			return nil, fmt.Errorf("xsecrank: %q has no feature column %q", path, feat)
		}
	}
	excluded := make(map[string]bool, len(exclude))
	for _, t := range exclude {
		excluded[strings.ToUpper(t)] = true
	}

	var records []row
	for {
		cells, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xsecrank: read %q: %w", path, err)
		}
		ticker := strings.ToUpper(strings.TrimSpace(cells[tickerIdx]))
		if excluded[ticker] || ticker == "" {
			continue
		}
		date := strings.TrimSpace(cells[dateIdx])
		if _, err := parseDate(date); err != nil {
			continue
		}
		values := make(map[string]float64, len(features))
		valid := true
		for _, feat := range features {
			raw := strings.TrimSpace(cells[index[feat]])
			if raw == "" {
				valid = false
				break
			}
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				valid = false
				break
			}
			values[feat] = v
		}
		if !valid {
			continue
		}
		records = append(records, row{ticker: ticker, date: date, values: values})
	}
	return records, nil
}

func groupByDate(records []row) map[string][]row {
	byDate := make(map[string][]row)
	for _, r := range records {
		byDate[r.date] = append(byDate[r.date], r)
	}
	return byDate
}

func parseDate(value string) (string, error) {
	// Dates in the dataset are already YYYY-MM-DD; validate shape only.
	if len(value) != len("2006-01-02") {
		return "", fmt.Errorf("bad date %q", value)
	}
	return value, nil
}

func parseOptions(args []string) (options, error) {
	opts := options{features: defaultFeatures, outCol: defaultOutCol}
	fs := flag.NewFlagSet("xsecrank", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.inPath, "in", "", "dataset CSV from exportdataset (must have ticker,date and the score features)")
	fs.StringVar(&opts.outPath, "out", "", "output ranks CSV (ticker,date,<col>) for backtest -signal-source csvprob")
	fs.StringVar(&opts.features, "features", opts.features, "comma-separated feature columns averaged into the cross-sectional score")
	fs.StringVar(&opts.outCol, "col", opts.outCol, "name of the emitted rank column")
	fs.StringVar(&opts.exclude, "exclude", "", "comma-separated tickers to drop (e.g. FX *_TOM)")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.inPath == "" || opts.outPath == "" {
		return options{}, fmt.Errorf("xsecrank: -in and -out are required")
	}
	if len(splitComma(opts.features)) == 0 {
		return options{}, fmt.Errorf("xsecrank: -features must list at least one column")
	}
	return opts, nil
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
