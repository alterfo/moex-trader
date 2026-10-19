package gapattrib

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MinValue       = 30e6
	minNamesPerDay = 30
	minHistory     = 252
	valueWindow    = 60
	valueMinObs    = 50
)

type Panel struct {
	Dates   []time.Time
	Tickers []string
	Open    [][]float64
	Close   [][]float64
	Value   [][]float64
}

func nan() float64 { return math.NaN() }

func LoadSnapshot(path string) (*Panel, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(bufio.NewReaderSize(file, 1<<20))
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, name := range header {
		col[name] = i
	}
	for _, need := range []string{"date", "ticker", "open", "close", "value"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("snapshot %s: missing column %s", path, need)
		}
	}
	type row struct {
		ticker             string
		open, close, value float64
	}
	byDate := map[string][]row{}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		open, e1 := strconv.ParseFloat(rec[col["open"]], 64)
		closeP, e2 := strconv.ParseFloat(rec[col["close"]], 64)
		if e1 != nil || e2 != nil || open <= 0 || closeP <= 0 {
			continue
		}
		value, e3 := strconv.ParseFloat(rec[col["value"]], 64)
		if e3 != nil {
			value = nan()
		}
		byDate[rec[col["date"]]] = append(byDate[rec[col["date"]]], row{rec[col["ticker"]], open, closeP, value})
	}
	var days []string
	for day, rows := range byDate {
		if len(rows) >= minNamesPerDay {
			days = append(days, day)
		}
	}
	sort.Strings(days)
	tickerSet := map[string]bool{}
	for _, day := range days {
		for _, r := range byDate[day] {
			tickerSet[r.ticker] = true
		}
	}
	panel := &Panel{}
	for t := range tickerSet {
		panel.Tickers = append(panel.Tickers, t)
	}
	sort.Strings(panel.Tickers)
	index := make(map[string]int, len(panel.Tickers))
	for i, t := range panel.Tickers {
		index[t] = i
	}
	for _, day := range days {
		parsed, err := time.Parse("2006-01-02", day)
		if err != nil {
			return nil, err
		}
		panel.Dates = append(panel.Dates, parsed)
		o := make([]float64, len(panel.Tickers))
		c := make([]float64, len(panel.Tickers))
		v := make([]float64, len(panel.Tickers))
		for i := range o {
			o[i], c[i], v[i] = nan(), nan(), nan()
		}
		for _, r := range byDate[day] {
			i := index[r.ticker]
			o[i], c[i], v[i] = r.open, r.close, r.value
		}
		panel.Open = append(panel.Open, o)
		panel.Close = append(panel.Close, c)
		panel.Value = append(panel.Value, v)
	}
	return panel, nil
}

func (p *Panel) DateIndex(day string) int {
	target, err := time.Parse("2006-01-02", day)
	if err != nil {
		return -1
	}
	i := sort.Search(len(p.Dates), func(i int) bool { return !p.Dates[i].Before(target) })
	if i < len(p.Dates) && p.Dates[i].Equal(target) {
		return i
	}
	return -1
}

func (p *Panel) TickerIndex() map[string]int {
	m := make(map[string]int, len(p.Tickers))
	for i, t := range p.Tickers {
		m[t] = i
	}
	return m
}

type dividendRecord struct {
	Ticker      string `json:"ticker"`
	LastBuyDate string `json:"last_buy_date"`
	Net         string `json:"dividend_net"`
}

func LoadDividends(path string, panel *Panel) ([][]float64, error) {
	div := make([][]float64, len(panel.Dates))
	for i := range div {
		div[i] = make([]float64, len(panel.Tickers))
	}
	if path == "" {
		return div, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	index := panel.TickerIndex()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec dividendRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, err
		}
		col, ok := index[rec.Ticker]
		if !ok || len(rec.LastBuyDate) < 10 {
			continue
		}
		net, err := strconv.ParseFloat(rec.Net, 64)
		if err != nil {
			continue
		}
		last, err := time.Parse("2006-01-02", rec.LastBuyDate[:10])
		if err != nil {
			continue
		}
		pos := sort.Search(len(panel.Dates), func(i int) bool { return panel.Dates[i].After(last) })
		if pos < len(panel.Dates) {
			div[pos][col] += net
		}
	}
	return div, scanner.Err()
}

type Adjusted struct {
	Open  [][]float64
	Close [][]float64
	Fill  [][]float64
}

func Adjust(panel *Panel, div [][]float64) *Adjusted {
	rows, cols := len(panel.Dates), len(panel.Tickers)
	adj := &Adjusted{Open: make([][]float64, rows), Close: make([][]float64, rows), Fill: make([][]float64, rows)}
	for i := range adj.Open {
		adj.Open[i] = make([]float64, cols)
		adj.Close[i] = make([]float64, cols)
		adj.Fill[i] = make([]float64, cols)
	}
	for j := 0; j < cols; j++ {
		cum := 1.0
		last := nan()
		for i := 0; i < rows; i++ {
			cl, op := panel.Close[i][j], panel.Open[i][j]
			gap, intra := nan(), nan()
			if i > 0 {
				prev := panel.Close[i-1][j]
				d := div[i][j]
				if !(d <= 0.35*prev) {
					d = 0
				}
				bad := false
				if !math.IsNaN(cl) && !math.IsNaN(prev) {
					ret := cl/prev - 1
					bad = ret < -0.45 || ret > 0.8
				}
				gap = (op + d) / prev
				intra = cl / op
				if bad {
					gap, intra = 1, 1
				}
			}
			factor := gap * intra
			if math.IsNaN(factor) {
				factor = 1
			}
			cum *= factor
			if math.IsNaN(cl) {
				adj.Close[i][j] = nan()
			} else {
				adj.Close[i][j] = cum
				last = cum
			}
			adj.Fill[i][j] = last
			if i > 0 && !math.IsNaN(adj.Close[i-1][j]) && !math.IsNaN(gap) {
				adj.Open[i][j] = adj.Close[i-1][j] * gap
			} else {
				adj.Open[i][j] = nan()
			}
		}
	}
	return adj
}

func (p *Panel) Eligible(i, j int) bool {
	if i < minHistory-1 {
		return false
	}
	for k := i - minHistory + 1; k <= i; k++ {
		if math.IsNaN(p.Close[k][j]) {
			return false
		}
	}
	values := make([]float64, 0, valueWindow)
	for k := i - valueWindow + 1; k <= i; k++ {
		if k >= 0 && !math.IsNaN(p.Value[k][j]) {
			values = append(values, p.Value[k][j])
		}
	}
	if len(values) < valueMinObs {
		return false
	}
	sort.Float64s(values)
	n := len(values)
	median := values[n/2]
	if n%2 == 0 {
		median = (values[n/2-1] + values[n/2]) / 2
	}
	return median >= MinValue
}

type Scores struct {
	Dates   []string
	Tickers []string
	R       [][]float64
}

func LoadScores(path string) (*Scores, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(bufio.NewReaderSize(file, 1<<20))
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	s := &Scores{Tickers: header[1:]}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		row := make([]float64, len(s.Tickers))
		for k := range row {
			v, perr := strconv.ParseFloat(rec[k+1], 64)
			if perr != nil || rec[k+1] == "" {
				v = nan()
			}
			row[k] = v
		}
		s.Dates = append(s.Dates, rec[0])
		s.R = append(s.R, row)
	}
	return s, nil
}

type KeyRate struct {
	days  []time.Time
	rates []float64
}

func LoadKeyRate(path string) (*KeyRate, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	if _, err := reader.Read(); err != nil {
		return nil, err
	}
	kr := &KeyRate{}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		day, err := time.Parse("2006-01-02", rec[0])
		if err != nil {
			return nil, err
		}
		rate, err := strconv.ParseFloat(rec[1], 64)
		if err != nil {
			return nil, err
		}
		kr.days = append(kr.days, day)
		kr.rates = append(kr.rates, rate)
	}
	return kr, nil
}

func (k *KeyRate) Carry(from, to time.Time) float64 {
	if k == nil || len(k.days) == 0 {
		return 0
	}
	total := 0.0
	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		ref := day.AddDate(0, 0, -1)
		i := sort.Search(len(k.days), func(i int) bool { return k.days[i].After(ref) })
		if i == 0 {
			continue
		}
		total += k.rates[i-1] / 100 / 365
	}
	return total
}
