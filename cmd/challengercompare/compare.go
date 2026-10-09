package main

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

type record struct {
	Time       time.Time `json:"ts"`
	Ticker     string    `json:"ticker"`
	Champion   side      `json:"champion"`
	Challenger side      `json:"challenger"`
}

type side struct {
	Action string `json:"action"`
	Error  string `json:"error"`
}

type pair struct {
	date       string
	ticker     string
	champion   float64
	challenger float64
	forward    float64
}

type closeSeries struct {
	dates  []string
	closes []float64
}

type summary struct {
	Pairs            int
	Dates            int
	Skipped          int
	ChallengerErrors int
	Agreement        float64
	ChampionMean     float64
	ChallengerMean   float64
	ChampionHit      float64
	ChallengerHit    float64
	DiffMean         float64
	DiffLo           float64
	DiffHi           float64
}

func direction(action string) float64 {
	switch action {
	case "BUY":
		return 1
	case "SELL":
		return -1
	}
	return 0
}

func dayKey(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01-02")
}

func buildPairs(records []record, series map[string]closeSeries, horizon int, loc *time.Location) ([]pair, int, int) {
	last := make(map[string]record)
	for _, r := range records {
		key := r.Ticker + "|" + dayKey(r.Time, loc)
		if prev, ok := last[key]; !ok || r.Time.After(prev.Time) {
			last[key] = r
		}
	}
	keys := make([]string, 0, len(last))
	for k := range last {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []pair
	skipped, errs := 0, 0
	for _, k := range keys {
		r := last[k]
		if r.Challenger.Error != "" {
			errs++
			continue
		}
		s, ok := series[r.Ticker]
		date := dayKey(r.Time, loc)
		if !ok {
			skipped++
			continue
		}
		i := sort.SearchStrings(s.dates, date)
		if i >= len(s.dates) || s.dates[i] != date || i+horizon >= len(s.closes) || s.closes[i] <= 0 {
			skipped++
			continue
		}
		out = append(out, pair{
			date:       date,
			ticker:     r.Ticker,
			champion:   direction(r.Champion.Action),
			challenger: direction(r.Challenger.Action),
			forward:    s.closes[i+horizon]/s.closes[i] - 1,
		})
	}
	return out, skipped, errs
}

func summarize(pairs []pair, skipped, errs, reps int, rng *rand.Rand) summary {
	out := summary{Pairs: len(pairs), Skipped: skipped, ChallengerErrors: errs}
	if len(pairs) == 0 {
		return out
	}
	byDate := make(map[string][]float64)
	dates := map[string]bool{}
	var agree, cMean, xMean float64
	var cHit, cN, xHit, xN float64
	for _, p := range pairs {
		dates[p.date] = true
		if p.champion == p.challenger {
			agree++
		}
		cMean += p.champion * p.forward
		xMean += p.challenger * p.forward
		if p.champion != 0 {
			cN++
			if p.champion*p.forward > 0 {
				cHit++
			}
		}
		if p.challenger != 0 {
			xN++
			if p.challenger*p.forward > 0 {
				xHit++
			}
		}
		byDate[p.date] = append(byDate[p.date], (p.challenger-p.champion)*p.forward)
	}
	n := float64(len(pairs))
	out.Dates = len(dates)
	out.Agreement = agree / n
	out.ChampionMean = cMean / n
	out.ChallengerMean = xMean / n
	if cN > 0 {
		out.ChampionHit = cHit / cN
	}
	if xN > 0 {
		out.ChallengerHit = xHit / xN
	}
	out.DiffMean = out.ChallengerMean - out.ChampionMean
	out.DiffLo, out.DiffHi = bootstrapByDate(byDate, reps, rng)
	return out
}

func bootstrapByDate(groups map[string][]float64, reps int, rng *rand.Rand) (float64, float64) {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	draws := make([]float64, 0, reps)
	for r := 0; r < reps; r++ {
		sum, cnt := 0.0, 0
		for range keys {
			g := groups[keys[rng.Intn(len(keys))]]
			for _, v := range g {
				sum += v
			}
			cnt += len(g)
		}
		draws = append(draws, sum/float64(cnt))
	}
	sort.Float64s(draws)
	return quantile(draws, 0.025), quantile(draws, 0.975)
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	return sorted[int(q*float64(len(sorted)-1)+0.5)]
}

func verdict(s summary, minPairs, minDates int) string {
	if s.Pairs < minPairs || s.Dates < minDates {
		return "inconclusive: not enough paired decisions"
	}
	switch {
	case s.DiffLo > 0:
		return "challenger better (CI excludes 0); promotion still needs the walk-forward gate and explicit sign-off"
	case s.DiffHi < 0:
		return "challenger worse (CI excludes 0)"
	}
	return "inconclusive: CI includes 0"
}
