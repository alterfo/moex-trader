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

type pRow struct {
	date, ticker string
	labelled     bool
	y            bool
	abn          float64
	fold, di     int
	vec          map[string]float64
}

func loadPanelRows(path string, foldFrom int) ([]*pRow, []string, error) {
	hdr, recs, err := readCSV(path)
	if err != nil {
		return nil, nil, err
	}
	ix := colIndex(hdr)
	minFold = foldFrom - 1
	var out []*pRow
	for _, rec := range recs {
		d := trimDate(rec[ix["date"]])
		k := foldOf(d)
		if k < 0 {
			continue
		}
		abn, err := strconv.ParseFloat(rec[ix["abn_ret"]], 64)
		if err != nil {
			continue
		}
		r := &pRow{date: d, ticker: rec[ix["ticker"]], labelled: rec[ix["label"]] != "", y: rec[ix["label"]] == "1", abn: abn, fold: k, vec: map[string]float64{}}
		for _, n := range apNames {
			v, _ := strconv.ParseFloat(rec[ix[n]], 64)
			r.vec[n] = v
		}
		out = append(out, r)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].date != out[b].date {
			return out[a].date < out[b].date
		}
		return out[a].ticker < out[b].ticker
	})
	idx := map[string]int{}
	var dates []string
	for _, r := range out {
		if _, ok := idx[r.date]; !ok {
			idx[r.date] = len(dates)
			dates = append(dates, r.date)
		}
		r.di = idx[r.date]
	}
	return out, dates, nil
}

func scorePanelArm(modelsDir, arm string, seeds []int, rows []*pRow) ([][]float64, error) {
	var out [][]float64
	for _, sd := range seeds {
		p := make([]float64, len(rows))
		cache := map[int]*model.EnsembleModel{}
		for i, r := range rows {
			m := cache[r.fold]
			if m == nil {
				mm, err := model.LoadEnsembleModel(filepath.Join(modelsDir, arm, fmt.Sprintf("s%d", sd), fmt.Sprintf("f%d.json", r.fold+1)))
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
					return nil, fmt.Errorf("feature %q missing", n)
				}
				vec[j] = v
			}
			p[i] = m.Probability(vec)
		}
		out = append(out, p)
	}
	return out, nil
}

func gateSeries(rows []*pRow, pred []float64, nDates int, cost float64) (gate, longOnly []float64) {
	gate, longOnly = make([]float64, nDates), make([]float64, nDates)
	for i := range gate {
		gate[i], longOnly[i] = math.NaN(), math.NaN()
	}
	i := 0
	for i < len(rows) {
		j := i
		for j < len(rows) && rows[j].di == rows[i].di {
			j++
		}
		n := j - i
		if n >= 30 {
			idx := make([]int, n)
			for k := range idx {
				idx[k] = i + k
			}
			sort.SliceStable(idx, func(a, b int) bool { return pred[idx[a]] > pred[idx[b]] })
			q := n / 10
			var top, bot float64
			for k := 0; k < q; k++ {
				top += rows[idx[k]].abn
				bot += rows[idx[n-1-k]].abn
			}
			top /= float64(q)
			bot /= float64(q)
			gate[rows[i].di] = top - bot - cost
			longOnly[rows[i].di] = top - cost
		}
		i = j
	}
	return
}

func bootMean(x []float64, draws, block int, rng *rand.Rand) (float64, float64) {
	var v []float64
	for _, a := range x {
		if !math.IsNaN(a) {
			v = append(v, a)
		}
	}
	n := len(v)
	if n < block+2 {
		return math.NaN(), math.NaN()
	}
	var ms []float64
	for b := 0; b < draws; b++ {
		s, c := 0.0, 0
		need := (n + block - 1) / block
		for q := 0; q < need; q++ {
			st := rng.Intn(n - block + 1)
			for k := 0; k < block; k++ {
				s += v[st+k]
				c++
			}
		}
		ms = append(ms, s/float64(c))
	}
	sort.Float64s(ms)
	return percentile(ms, 0.025), percentile(ms, 0.975)
}

func runPScore(w io.Writer, dataPath, shufPath, modelsDir string, arms []string, seeds []int, foldFrom int, draws int, bootSeed int64) error {
	rows, dates, err := loadPanelRows(dataPath, foldFrom)
	if err != nil {
		return err
	}
	srows, _, err := loadPanelRows(shufPath, foldFrom)
	if err != nil {
		return err
	}
	if len(rows) != len(srows) {
		return fmt.Errorf("row counts differ: %d vs %d", len(rows), len(srows))
	}
	fmt.Fprintf(w, "pscore: %d rows, %d dates, folds from %d, seeds %v\n", len(rows), len(dates), foldFrom, seeds)
	hi := 0
	for i, d := range dates {
		if d >= halfSplit {
			hi = i
			break
		}
	}
	const cost = 0.003
	rng := rand.New(rand.NewSource(bootSeed))
	preds := map[string][][]float64{}
	for _, a := range arms {
		src := rows
		if a == "S" {
			src = srows
		}
		p, err := scorePanelArm(modelsDir, a, seeds, src)
		if err != nil {
			return err
		}
		preds[a] = p
	}
	y := make([]bool, 0)
	var labIdx []int
	for i, r := range rows {
		if r.labelled {
			labIdx = append(labIdx, i)
			y = append(y, r.y)
		}
	}
	dateOfLab := make([]int, len(labIdx))
	for k, i := range labIdx {
		dateOfLab[k] = rows[i].di
	}
	aucOf := map[string][]float64{}
	ranks := map[string][]ranked{}
	for _, a := range arms {
		for s := range seeds {
			sc := make([]float64, len(labIdx))
			for k, i := range labIdx {
				sc[k] = preds[a][s][i]
			}
			ranks[a] = append(ranks[a], rankOrder(sc))
		}
		ones := make([]float64, len(labIdx))
		for i := range ones {
			ones[i] = 1
		}
		for s := range seeds {
			aucOf[a] = append(aucOf[a], weightedAUC(ranks[a][s], y, ones))
		}
		fmt.Fprintf(w, "AUC %-3s seeds %s mean %.4f (context only, labelled rows %d)\n", a, fmtVec(aucOf[a]), mean(aucOf[a]), len(labIdx))
	}
	if _, okA := preds["A"]; okA {
		if _, okS := preds["S"]; okS {
			var diffs []float64
			for b := 0; b < draws; b++ {
				bw := blockWeights(dateOfLab, len(dates), 10, rng)
				sa, ss := 0.0, 0.0
				for s := range seeds {
					sa += weightedAUC(ranks["A"][s], y, bw)
					ss += weightedAUC(ranks["S"][s], y, bw)
				}
				diffs = append(diffs, (sa-ss)/float64(len(seeds)))
			}
			sort.Float64s(diffs)
			fmt.Fprintf(w, "AUC corrected A-S mean %+.4f bootstrap 95%% [%+.4f, %+.4f] (context)\n", mean(aucOf["A"])-mean(aucOf["S"]), percentile(diffs, 0.025), percentile(diffs, 0.975))
		}
	}
	for _, a := range arms {
		avg := make([]float64, len(rows))
		for i := range avg {
			for s := range seeds {
				avg[i] += preds[a][s][i]
			}
			avg[i] /= float64(len(seeds))
		}
		gate, lo := gateSeries(rows, avg, len(dates), cost)
		gateRng := rand.New(rand.NewSource(bootSeed + 1))
		report := func(label string, x []float64) {
			var all, h0, h1 []float64
			for d, v := range x {
				if math.IsNaN(v) {
					continue
				}
				all = append(all, v)
				if d < hi {
					h0 = append(h0, v)
				} else {
					h1 = append(h1, v)
				}
			}
			m, t := neweyWest(all, 10)
			l, u := bootMean(x, draws, 10, gateRng)
			fmt.Fprintf(w, "%s %-8s mean %+.4f%% NW10 t %+.2f boot95 [%+.4f%%, %+.4f%%] halves %+.4f%% / %+.4f%% dates %d\n", a, label, m*100, t, l*100, u*100, mean(h0)*100, mean(h1)*100, len(all))
		}
		report("gate", gate)
		report("longonly", lo)
		var legT, legB []float64
		{
			i := 0
			for i < len(rows) {
				j := i
				for j < len(rows) && rows[j].di == rows[i].di {
					j++
				}
				if n := j - i; n >= 30 {
					idx := make([]int, n)
					for k := range idx {
						idx[k] = i + k
					}
					sort.SliceStable(idx, func(x, y int) bool { return avg[idx[x]] > avg[idx[y]] })
					q := n / 10
					var t, b float64
					for k := 0; k < q; k++ {
						t += rows[idx[k]].abn
						b += rows[idx[n-1-k]].abn
					}
					legT, legB = append(legT, t/float64(q)), append(legB, b/float64(q))
				}
				i = j
			}
		}
		fmt.Fprintf(w, "%s legs (descriptive, gross abnormal 10d): top decile %+.4f%%, bottom decile %+.4f%%; gate if the short leg also pays borrow 17%%/yr (0.47%% per 10 days): %+.4f%%, 25%%/yr (0.69%%): %+.4f%%\n", a, mean(legT)*100, mean(legB)*100, (mean(legT)-mean(legB)-cost-0.0047)*100, (mean(legT)-mean(legB)-cost-0.0069)*100)
		pos := 0
		var per []float64
		for s := range seeds {
			g, _ := gateSeries(rows, preds[a][s], len(dates), cost)
			var v []float64
			for _, x := range g {
				if !math.IsNaN(x) {
					v = append(v, x)
				}
			}
			per = append(per, mean(v))
			if mean(v) > 0 {
				pos++
			}
		}
		fmt.Fprintf(w, "%s seeds gate means %s positive %d/%d\n", a, fmtVec(per), pos, len(seeds))
	}
	return nil
}
