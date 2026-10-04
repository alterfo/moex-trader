package strategyvalidation

import (
	"fmt"
	"sort"
	"time"
)

type DailyReturn struct {
	Date  time.Time
	Value float64
}

func JoinDailyReturns(variants [][]DailyReturn) ([]time.Time, [][]float64, error) {
	if len(variants) < 2 {
		return nil, nil, fmt.Errorf("strategyvalidation: need at least 2 variants to join, got %d", len(variants))
	}
	values := make([]map[time.Time]float64, len(variants))
	dateSet := make(map[time.Time]struct{})
	for i, series := range variants {
		if len(series) == 0 {
			return nil, nil, fmt.Errorf("strategyvalidation: variant %d has no daily returns", i)
		}
		byDate := make(map[time.Time]float64, len(series))
		for _, r := range series {
			if r.Date.IsZero() {
				return nil, nil, fmt.Errorf("strategyvalidation: variant %d has a zero date", i)
			}
			if _, ok := byDate[r.Date]; ok {
				return nil, nil, fmt.Errorf("strategyvalidation: variant %d has duplicate date %s", i, r.Date.Format("2006-01-02"))
			}
			byDate[r.Date] = r.Value
			dateSet[r.Date] = struct{}{}
		}
		values[i] = byDate
	}

	dates := make([]time.Time, 0, len(dateSet))
	for date := range dateSet {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	common := make(map[time.Time]struct{}, len(dates))
	for _, date := range dates {
		common[date] = struct{}{}
	}
	for _, byDate := range values {
		for date := range common {
			if _, ok := byDate[date]; !ok {
				delete(common, date)
			}
		}
	}
	if len(common) == 0 {
		return nil, nil, fmt.Errorf("strategyvalidation: mismatched calendars: variants share no common trading date")
	}

	matrix := make([][]float64, len(dates))
	for row, date := range dates {
		matrix[row] = make([]float64, len(variants))
		for col, byDate := range values {
			matrix[row][col] = byDate[date]
		}
	}
	return dates, matrix, nil
}
