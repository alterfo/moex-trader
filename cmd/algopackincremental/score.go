package main

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/olegsidorkin/moex-trader/internal/model"
)

type foldSpan struct{ lo, hi string }

var folds = []foldSpan{
	{"2025-01-03", "2025-03-31"},
	{"2025-04-01", "2025-06-30"},
	{"2025-07-01", "2025-09-30"},
	{"2025-10-01", "2025-12-31"},
	{"2026-01-01", "2026-03-31"},
	{"2026-04-01", "2026-06-30"},
	{"2026-07-01", "2026-10-16"},
	{"2026-10-21", "2027-12-31"},
}

const halfSplit = "2026-01-05"

type testRow struct {
	date, ticker string
	y            bool
	fold         int
	vec          map[string]float64
	fwd          float64
	dateIdx      int
}

var minFold int

func foldOf(date string) int {
	for k, f := range folds {
		if k >= minFold && date >= f.lo && date <= f.hi {
			return k
		}
	}
	return -1
}

func loadTestRows(dataPath string) ([]*testRow, []string, error) {
	hdr, rows, err := readCSV(dataPath)
	if err != nil {
		return nil, nil, err
	}
	ix := colIndex(hdr)
	var out []*testRow
	for _, rec := range rows {
		d := trimDate(rec[ix["date"]])
		k := foldOf(d)
		if k < 0 || rec[ix["label"]] == "" {
			continue
		}
		tr := &testRow{date: d, ticker: rec[ix["ticker"]], y: rec[ix["label"]] == "1", fold: k, vec: make(map[string]float64, len(hdr))}
		for j, h := range hdr {
			if j == ix["ticker"] || j == ix["date"] || h == "label" || h == "label_date" || h == "split" {
				continue
			}
			v, err := strconv.ParseFloat(rec[j], 64)
			if err != nil {
				v = 0
			}
			tr.vec[h] = v
		}
		out = append(out, tr)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].date != out[b].date {
			return out[a].date < out[b].date
		}
		return out[a].ticker < out[b].ticker
	})
	dateSet := map[string]int{}
	var dates []string
	for _, r := range out {
		if _, ok := dateSet[r.date]; !ok {
			dateSet[r.date] = len(dates)
			dates = append(dates, r.date)
		}
		r.dateIdx = dateSet[r.date]
	}
	return out, dates, nil
}

func attachForward(rows []*testRow, blockPath string, horizon int) error {
	blk, err := loadBlock(blockPath, true)
	if err != nil {
		return err
	}
	byT := map[string][]string{}
	for k := range blk {
		byT[k.secid] = append(byT[k.secid], k.date)
	}
	for t := range byT {
		sort.Strings(byT[t])
	}
	pos := map[string]map[string]int{}
	for t, ds := range byT {
		m := map[string]int{}
		for i, d := range ds {
			m[d] = i
		}
		pos[t] = m
	}
	for _, r := range rows {
		r.fwd = math.NaN()
		ds := byT[r.ticker]
		i, ok := pos[r.ticker][r.date]
		if !ok || i+horizon >= len(ds) {
			continue
		}
		a, b := blk[blockKey{r.date, r.ticker}], blk[blockKey{ds[i+horizon], r.ticker}]
		if a == nil || b == nil || !(a.close > 0) || !(b.close > 0) {
			continue
		}
		x := b.close/a.close - 1
		if x <= -0.45 || x >= 0.80 {
			continue
		}
		r.fwd = x
	}
	return nil
}

type series struct {
	name string
	pred [][]float64
}

func scoreArm(arm string, modelsDir string, seeds []int, rows []*testRow, lin bool) (*series, error) {
	s := &series{name: arm}
	if lin {
		s.name += "-lin"
	}
	for _, sd := range seeds {
		p := make([]float64, len(rows))
		cache := map[int]*model.EnsembleModel{}
		for i, r := range rows {
			m := cache[r.fold]
			if m == nil {
				path := filepath.Join(modelsDir, arm, fmt.Sprintf("s%d", sd), fmt.Sprintf("f%d.json", r.fold+1))
				mm, err := model.LoadEnsembleModel(path)
				if err != nil {
					return nil, err
				}
				m = mm
				cache[r.fold] = m
			}
			vec := make([]float64, len(m.FeatureOrder))
			for j, n := range m.FeatureOrder {
				v, ok := r.vec[n]
				if !ok {
					return nil, fmt.Errorf("feature %q missing in data for %s %s", n, r.date, r.ticker)
				}
				vec[j] = v
			}
			if lin {
				_, _, l := m.MemberProbabilities(vec)
				p[i] = l
			} else {
				p[i] = m.Probability(vec)
			}
		}
		s.pred = append(s.pred, p)
	}
	return s, nil
}

type icSeries struct {
	dates []int
	vals  [][]float64
}

func dailyIC(rows []*testRow, pred []float64, nDates int) []float64 {
	out := make([]float64, nDates)
	for i := range out {
		out[i] = math.NaN()
	}
	i := 0
	for i < len(rows) {
		j := i
		for j < len(rows) && rows[j].dateIdx == rows[i].dateIdx {
			j++
		}
		var a, b []float64
		for k := i; k < j; k++ {
			if !math.IsNaN(rows[k].fwd) {
				a = append(a, pred[k])
				b = append(b, rows[k].fwd)
			}
		}
		if len(a) >= 10 {
			out[rows[i].dateIdx] = spearman(a, b)
		}
		i = j
	}
	return out
}

func seedMean(vs [][]float64) []float64 {
	out := make([]float64, len(vs[0]))
	for i := range out {
		s, n := 0.0, 0
		for _, v := range vs {
			if !math.IsNaN(v[i]) {
				s += v[i]
				n++
			}
		}
		if n == 0 {
			out[i] = math.NaN()
		} else {
			out[i] = s / float64(n)
		}
	}
	return out
}

func clean(v []float64) []float64 {
	var o []float64
	for _, x := range v {
		if !math.IsNaN(x) {
			o = append(o, x)
		}
	}
	return o
}

func runScore(w io.Writer, dataPath, shufPath, blockPath, modelsDir string, seeds []int, draws int, bootSeed int64, foldFrom int) error {
	minFold = foldFrom - 1
	rows, dates, err := loadTestRows(dataPath)
	if err != nil {
		return err
	}
	if err := attachForward(rows, blockPath, 10); err != nil {
		return err
	}
	srows, _, err := loadTestRows(shufPath)
	if err != nil {
		return err
	}
	if len(srows) != len(rows) {
		return fmt.Errorf("row count differs between data (%d) and shuffled data (%d)", len(rows), len(srows))
	}
	for i := range rows {
		if rows[i].date != srows[i].date || rows[i].ticker != srows[i].ticker || rows[i].y != srows[i].y {
			return fmt.Errorf("row %d differs between data and shuffled data", i)
		}
	}
	fmt.Fprintf(w, "score: %d test rows, %d test dates, seeds %v, positives %.3f\n", len(rows), len(dates), seeds, posRate(rows))
	y := make([]bool, len(rows))
	dateOf := make([]int, len(rows))
	half := make([]int, len(rows))
	for i, r := range rows {
		y[i] = r.y
		dateOf[i] = r.dateIdx
		if r.date >= halfSplit {
			half[i] = 1
		}
	}
	type armSpec struct {
		arm  string
		data []*testRow
		lin  bool
	}
	specs := []armSpec{{"B", rows, false}, {"A", rows, false}, {"S", srows, false}, {"B", rows, true}, {"A", rows, true}, {"S", srows, true}}
	ser := map[string]*series{}
	for _, sp := range specs {
		s, err := scoreArm(sp.arm, modelsDir, seeds, sp.data, sp.lin)
		if err != nil {
			return err
		}
		ser[s.name] = s
	}
	ones := make([]float64, len(rows))
	for i := range ones {
		ones[i] = 1
	}
	hw := [2][]float64{make([]float64, len(rows)), make([]float64, len(rows))}
	for i := range rows {
		hw[half[i]][i] = 1
	}
	ranks := map[string][]ranked{}
	for n, s := range ser {
		for _, p := range s.pred {
			ranks[n] = append(ranks[n], rankOrder(p))
		}
	}
	auc := func(name string, w []float64) []float64 {
		out := make([]float64, len(seeds))
		for k := range seeds {
			out[k] = weightedAUC(ranks[name][k], y, w)
		}
		return out
	}
	for _, n := range []string{"B", "A", "S", "B-lin", "A-lin", "S-lin"} {
		a := auc(n, ones)
		fmt.Fprintf(w, "AUC %-6s seeds %s mean %.4f\n", n, fmtVec(a), mean(a))
	}
	delta := func(a, b string, wt []float64) []float64 {
		x, z := auc(a, wt), auc(b, wt)
		o := make([]float64, len(x))
		for i := range x {
			o[i] = x[i] - z[i]
		}
		return o
	}
	rep := func(label, a, b string) {
		full := delta(a, b, ones)
		h0 := delta(a, b, hw[0])
		h1 := delta(a, b, hw[1])
		pos := 0
		for _, v := range full {
			if v > 0 {
				pos++
			}
		}
		fmt.Fprintf(w, "dAUC %-12s full %+.4f (seeds %s, positive %d/%d) halves %+.4f / %+.4f\n", label, mean(full), fmtVec(full), pos, len(full), mean(h0), mean(h1))
	}
	rep("A-B", "A", "B")
	rep("S-B", "S", "B")
	rep("A-lin - B-lin", "A-lin", "B-lin")
	rep("S-lin - B-lin", "S-lin", "B-lin")
	rep("A-S corrected", "A", "S")
	rng := rand.New(rand.NewSource(bootSeed))
	var dA, dS, dC, dLA []float64
	for d := 0; d < draws; d++ {
		bw := blockWeights(dateOf, len(dates), 10, rng)
		a := func(n string) float64 {
			s := 0.0
			for k := range seeds {
				s += weightedAUC(ranks[n][k], y, bw)
			}
			return s / float64(len(seeds))
		}
		b, aa, ss := a("B"), a("A"), a("S")
		dA = append(dA, aa-b)
		dS = append(dS, ss-b)
		dC = append(dC, (aa-b)-(ss-b))
		dLA = append(dLA, a("A-lin")-a("B-lin"))
	}
	for _, x := range []struct {
		n string
		v []float64
	}{{"A-B", dA}, {"S-B", dS}, {"(A-B)-(S-B) placebo-corrected", dC}, {"A-lin - B-lin", dLA}} {
		sort.Float64s(x.v)
		fmt.Fprintf(w, "bootstrap dAUC %-32s 95%% [%+.4f, %+.4f] (%d draws, block 10 dates)\n", x.n, percentile(x.v, 0.025), percentile(x.v, 0.975), len(x.v))
	}
	icBy := map[string][][]float64{}
	for n, s := range ser {
		for _, p := range s.pred {
			icBy[n] = append(icBy[n], dailyIC(rows, p, len(dates)))
		}
	}
	diff := func(a, b string) []float64 {
		x, z := seedMean(icBy[a]), seedMean(icBy[b])
		o := make([]float64, len(x))
		for i := range x {
			o[i] = x[i] - z[i]
		}
		return o
	}
	hi := 0
	for i, d := range dates {
		if d >= halfSplit {
			hi = i
			break
		}
	}
	perSeedIC := func(a, b string) string {
		out := make([]float64, len(seeds))
		for k := range seeds {
			var v []float64
			for i := range icBy[a][k] {
				if !math.IsNaN(icBy[a][k][i]) && !math.IsNaN(icBy[b][k][i]) {
					v = append(v, icBy[a][k][i]-icBy[b][k][i])
				}
			}
			out[k] = mean(v)
		}
		pos := 0
		for _, x := range out {
			if x > 0 {
				pos++
			}
		}
		return fmt.Sprintf("seeds %s positive %d/%d", fmtVec(out), pos, len(out))
	}
	for _, c := range [][2]string{{"A", "B"}, {"S", "B"}, {"A", "S"}, {"A-lin", "B-lin"}} {
		dd := diff(c[0], c[1])
		all := clean(dd)
		m, t := neweyWest(all, 10)
		m0 := mean(clean(dd[:hi]))
		m1 := mean(clean(dd[hi:]))
		fmt.Fprintf(w, "dIC %s-%s mean %+.4f NW10 t %+.2f over %d dates; halves %+.4f / %+.4f; %s\n", c[0], c[1], m, t, len(all), m0, m1, perSeedIC(c[0], c[1]))
	}
	behaviour(w, rows, ser["B"], ser["A"], dates)
	for _, n := range []string{"B", "A", "S"} {
		m := mean(clean(seedMean(icBy[n])))
		fmt.Fprintf(w, "IC %s mean %+.4f\n", n, m)
	}
	return nil
}

func posRate(rows []*testRow) float64 {
	n := 0
	for _, r := range rows {
		if r.y {
			n++
		}
	}
	return float64(n) / float64(len(rows))
}

func fmtVec(v []float64) string {
	s := ""
	for i, x := range v {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%.4f", x)
	}
	return s
}

func behaviour(w io.Writer, rows []*testRow, b, a *series, dates []string) {
	const buy, sell = 0.60, 0.40
	side := func(p float64) int {
		switch {
		case p > buy:
			return 1
		case p < sell:
			return -1
		}
		return 0
	}
	var buys, sells [2]float64
	var gross [2]float64
	var diffDates float64
	for k := range b.pred {
		diff := map[int]bool{}
		var sum, cnt [2]float64
		for i, r := range rows {
			sb, sa := side(b.pred[k][i]), side(a.pred[k][i])
			if sb != sa {
				diff[r.dateIdx] = true
			}
			for arm, s := range []int{sb, sa} {
				if s == 0 {
					continue
				}
				if s > 0 {
					buys[arm]++
				} else {
					sells[arm]++
				}
				if !math.IsNaN(r.fwd) {
					sum[arm] += float64(s) * r.fwd
					cnt[arm]++
				}
			}
		}
		for arm := 0; arm < 2; arm++ {
			if cnt[arm] > 0 {
				gross[arm] += sum[arm] / cnt[arm]
			}
		}
		diffDates += float64(len(diff))
	}
	n := float64(len(b.pred))
	fmt.Fprintf(w, "behaviour at %.2f/%.2f (seed mean): B buys %.0f sells %.0f gross 10d signed %+.3f%%; A buys %.0f sells %.0f gross 10d signed %+.3f%%; dates with a different trade set %.0f of %d\n", buy, sell, buys[0]/n, sells[0]/n, gross[0]/n*100, buys[1]/n, sells[1]/n, gross[1]/n*100, diffDates/n, len(dates))
}
