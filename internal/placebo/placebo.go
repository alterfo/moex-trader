package placebo

import (
	"math/rand"
	"sort"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/model"
)

type Table map[string][]model.ScorePoint

func axis(table Table) []time.Time {
	seen := map[time.Time]bool{}
	for _, series := range table {
		for _, point := range series {
			seen[point.Date] = true
		}
	}
	dates := make([]time.Time, 0, len(seen))
	for date := range seen {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	return dates
}

func Shift(table Table, k int) Table {
	dates := axis(table)
	n := len(dates)
	out := make(Table, len(table))
	if n == 0 {
		return out
	}
	offset := ((k % n) + n) % n
	for ticker, series := range table {
		byDate := make(map[time.Time]float64, len(series))
		for _, point := range series {
			byDate[point.Date] = point.P
		}
		shifted := make([]model.ScorePoint, 0, len(series))
		for i, date := range dates {
			if _, ok := byDate[date]; !ok {
				continue
			}
			source := dates[(i+offset)%n]
			if p, ok := byDate[source]; ok {
				shifted = append(shifted, model.ScorePoint{Date: date, P: p})
			}
		}
		out[ticker] = shifted
	}
	return out
}

func PermuteByDate(table Table, seed int64) Table {
	tickers := make([]string, 0, len(table))
	for ticker := range table {
		tickers = append(tickers, ticker)
	}
	sort.Strings(tickers)
	type cell struct {
		ticker string
		index  int
	}
	byDate := map[time.Time][]cell{}
	for _, ticker := range tickers {
		for i, point := range table[ticker] {
			byDate[point.Date] = append(byDate[point.Date], cell{ticker, i})
		}
	}
	out := make(Table, len(table))
	for _, ticker := range tickers {
		out[ticker] = append([]model.ScorePoint(nil), table[ticker]...)
	}
	for date, cells := range byDate {
		values := make([]float64, len(cells))
		for i, c := range cells {
			values[i] = table[c.ticker][c.index].P
		}
		rng := rand.New(rand.NewSource(seed ^ date.Unix()))
		rng.Shuffle(len(values), func(a, b int) { values[a], values[b] = values[b], values[a] })
		for i, c := range cells {
			out[c.ticker][c.index].P = values[i]
		}
	}
	return out
}

func Offsets(table Table, minAbs, n int) []int {
	total := len(axis(table))
	var all []int
	for k := minAbs; k <= total-minAbs; k++ {
		all = append(all, k)
	}
	if len(all) <= n {
		return all
	}
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, all[i*len(all)/n])
	}
	return out
}

func PValue(real float64, placebos []float64) float64 {
	atLeast := 0
	for _, v := range placebos {
		if v >= real {
			atLeast++
		}
	}
	return float64(1+atLeast) / float64(len(placebos)+1)
}
