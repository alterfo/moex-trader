package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
)

type dayScore struct {
	score float64
	ret   float64
	n     int
}

type tranche struct {
	date     string
	spread   float64
	topRet   float64
	quintile [5]float64
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("newsportfolio", flag.ContinueOnError)
	scoresPath := fs.String("scores", "", "newsvalidate scores CSV")
	minNames := fs.Int("min-names", 10, "minimum scored tickers per date")
	costPerSide := fs.Float64("cost-pct", 0.15, "cost per fill in percent")
	block := fs.Int("block", 10, "bootstrap block length in dates")
	reps := fs.Int("reps", 5000, "bootstrap repetitions")
	seed := fs.Int64("seed", 20261018, "bootstrap seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *scoresPath == "" {
		return fmt.Errorf("-scores is required")
	}
	byDate, err := load(*scoresPath)
	if err != nil {
		return err
	}
	tranches := build(byDate, *minNames)
	if len(tranches) == 0 {
		return fmt.Errorf("no dates with at least %d scored tickers", *minNames)
	}
	report(tranches, *costPerSide, *block, *reps, *seed)
	return nil
}

func load(path string) (map[string]map[string]*dayScore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]map[string]*dayScore)
	for _, row := range rows[1:] {
		score, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			return nil, err
		}
		ret, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			return nil, err
		}
		day := byDate[row[2]]
		if day == nil {
			day = make(map[string]*dayScore)
			byDate[row[2]] = day
		}
		entry := day[row[1]]
		if entry == nil {
			entry = &dayScore{ret: ret}
			day[row[1]] = entry
		}
		entry.score += score
		entry.n++
	}
	return byDate, nil
}

func build(byDate map[string]map[string]*dayScore, minNames int) []tranche {
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	var out []tranche
	for _, d := range dates {
		day := byDate[d]
		if len(day) < minNames {
			continue
		}
		type item struct {
			ticker string
			score  float64
			ret    float64
		}
		items := make([]item, 0, len(day))
		for t, e := range day {
			items = append(items, item{t, e.score / float64(e.n), e.ret})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].score != items[j].score {
				return items[i].score < items[j].score
			}
			return items[i].ticker < items[j].ticker
		})
		n := len(items)
		k := n / 5
		tr := tranche{date: d}
		for q := 0; q < 5; q++ {
			lo, hi := q*n/5, (q+1)*n/5
			tr.quintile[q] = mean(items, lo, hi, func(i item) float64 { return i.ret })
		}
		bottom := mean(items, 0, k, func(i item) float64 { return i.ret })
		top := mean(items, n-k, n, func(i item) float64 { return i.ret })
		tr.topRet = top
		tr.spread = top - bottom
		out = append(out, tr)
	}
	return out
}

func mean[T any](items []T, lo, hi int, f func(T) float64) float64 {
	if hi <= lo {
		return 0
	}
	var s float64
	for _, it := range items[lo:hi] {
		s += f(it)
	}
	return s / float64(hi-lo)
}

func avg(xs []float64) float64 {
	return mean(xs, 0, len(xs), func(x float64) float64 { return x })
}

func bootstrap(xs []float64, block, reps int, seed int64) (float64, float64) {
	rng := rand.New(rand.NewSource(seed))
	n := len(xs)
	means := make([]float64, reps)
	for r := 0; r < reps; r++ {
		var s float64
		count := 0
		for count < n {
			start := rng.Intn(n)
			for j := 0; j < block && count < n; j++ {
				s += xs[(start+j)%n]
				count++
			}
		}
		means[r] = s / float64(n)
	}
	sort.Float64s(means)
	return means[int(0.025*float64(reps))], means[int(0.975*float64(reps))]
}

func report(tr []tranche, costPerSide float64, block, reps int, seed int64) {
	spreads := make([]float64, len(tr))
	tops := make([]float64, len(tr))
	for i, t := range tr {
		spreads[i] = t.spread
		tops[i] = t.topRet
	}
	fmt.Printf("dates=%d  first=%s last=%s\n", len(tr), tr[0].date, tr[len(tr)-1].date)
	gross := avg(spreads)
	fmt.Printf("gross long-short spread per tranche: %+.3f%%  breakeven cost per side: %.3f%%\n", gross, gross/4)
	for _, c := range []float64{0.10, costPerSide, 0.20} {
		net := make([]float64, len(spreads))
		for i, s := range spreads {
			net[i] = s - 4*c
		}
		lo, hi := bootstrap(net, block, reps, seed)
		tag := ""
		if c == costPerSide {
			tag = "  <- primary"
		}
		fmt.Printf("cost %.2f%%/side: net %+.3f%%  CI95 [%+.3f, %+.3f]  pass=%v%s\n", c, avg(net), lo, hi, lo > 0, tag)
	}
	longNet := make([]float64, len(tops))
	for i, v := range tops {
		longNet[i] = v - 2*costPerSide
	}
	lo, hi := bootstrap(longNet, block, reps, seed)
	fmt.Printf("long-only top quintile excess net of %.2f%% round trip: %+.3f%%  CI95 [%+.3f, %+.3f]\n", 2*costPerSide, avg(longNet), lo, hi)
	fmt.Print("quintile mean excess (low..high score):")
	for q := 0; q < 5; q++ {
		qs := make([]float64, len(tr))
		for i, t := range tr {
			qs[i] = t.quintile[q]
		}
		fmt.Printf(" %+.3f", avg(qs))
	}
	fmt.Println()
	quarters := map[string][]float64{}
	var keys []string
	for _, t := range tr {
		m, _ := strconv.Atoi(t.date[5:7])
		key := fmt.Sprintf("%sQ%d", t.date[:4], (m-1)/3+1)
		if _, ok := quarters[key]; !ok {
			keys = append(keys, key)
		}
		quarters[key] = append(quarters[key], t.spread-4*costPerSide)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s dates=%d net %+.3f%%\n", k, len(quarters[k]), avg(quarters[k]))
	}
}
