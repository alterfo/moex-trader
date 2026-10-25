package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

var vitalFeatures = []string{"LevAsym", "SpLv", "NJ", "A2", "A2H2", "A2Late", "ImbVol", "TicketMed", "UnexpA1", "Idle", "VwapDev", "Trades", "PutOrders", "A5", "Depth", "ImbSlope"}

var transformNames = []string{"mean5", "z20", "chg5", "tercileRank", "lag1"}

type pairSpec struct {
	id, a, b string
	op       string
}

var pairSpecs = []pairSpec{
	{"prod", "LevAsym", "SpLv", "prod"}, {"prod", "LevAsym", "NJ", "prod"}, {"prod", "LevAsym", "A2", "prod"},
	{"prod", "SpLv", "NJ", "prod"}, {"prod", "SpLv", "A2", "prod"}, {"prod", "NJ", "A2", "prod"},
	{"prod", "TicketMed", "A2", "prod"}, {"prod", "Idle", "A2", "prod"}, {"prod", "Trades", "LevAsym", "prod"},
	{"prod", "Idle", "SpLv", "prod"}, {"prod", "TicketMed", "LevAsym", "prod"}, {"prod", "A2H2", "LevAsym", "prod"},
	{"diff", "A2", "A2Late", "diff"}, {"diff", "LevAsym", "SpLv", "diff"}, {"diff", "A2", "ctrl_rb5", "diff"},
	{"prod", "A2", "ctrl_rb1", "prod"}, {"diff", "ImbVol", "A2", "diff"}, {"diff", "Depth", "SpLv", "diff"},
	{"diff", "A5", "A2", "diff"}, {"prod", "NJ", "Idle", "prod"},
}

type cand struct {
	id, desc string
}

func lib2Candidates() []cand {
	var out []cand
	n := 1
	for _, f := range vitalFeatures {
		for _, t := range transformNames {
			out = append(out, cand{fmt.Sprintf("L%03d", n), f + " " + t})
			n++
		}
	}
	for _, p := range pairSpecs {
		out = append(out, cand{fmt.Sprintf("L%03d", n), p.a + " " + p.op + " " + p.b})
		n++
	}
	return out
}

func normalTwoSided(t float64) float64 {
	return math.Erfc(math.Abs(t) / math.Sqrt2)
}

func seqStat(v []float64, lo, hi int) []float64 {
	var o []float64
	for d := lo; d <= hi; d++ {
		if !math.IsNaN(v[d]) {
			o = append(o, v[d])
		}
	}
	return o
}

var libWindow = "realtime"

var libMode = "lib2"

var extraModerators = []string{"OwnVol20", "LiquidityRank", "Ret1", "Ret5", "Ret20"}

func lib2(pp *pipe, frzPath string) {
	ds := pp.ds
	nf := len(ds.names)
	fidx := map[string]int{}
	for i, n := range ds.names {
		fidx[n] = i
	}
	cands := lib2Candidates()
	nc := len(cands)
	nctl := 5
	nex := 0
	if libMode == "lib3" {
		nex = len(extraModerators)
	}
	total := nc + nex + nctl
	ncm := nc + nex
	nd := len(ds.dates)
	vals := make([][]float64, len(ds.rows))
	rowPos := map[*row]int{}
	for i, x := range ds.rows {
		rowPos[x] = i
		vals[i] = make([]float64, total)
		for c := range vals[i] {
			vals[i][c] = nan
		}
	}
	hist := map[string][]*row{}
	for d := 0; d < nd; d++ {
		for _, x := range ds.byDate[d] {
			hist[x.secid] = append(hist[x.secid], x)
		}
	}
	col := 0
	for _, f := range vitalFeatures {
		k := fidx[f]
		for ti := range transformNames {
			for _, seq := range hist {
				for j, x := range seq {
					v := x.raw[k]
					out := nan
					if !math.IsNaN(v) {
						switch ti {
						case 0:
							var a []float64
							for q := j - 4; q <= j; q++ {
								if q >= 0 && !math.IsNaN(seq[q].raw[k]) {
									a = append(a, seq[q].raw[k])
								}
							}
							if len(a) >= 3 {
								out = mean(a)
							}
						case 1:
							var a []float64
							for q := j - 20; q < j; q++ {
								if q >= 0 && !math.IsNaN(seq[q].raw[k]) {
									a = append(a, seq[q].raw[k])
								}
							}
							if len(a) >= 10 {
								m := mean(a)
								var s float64
								for _, u := range a {
									s += (u - m) * (u - m)
								}
								s = math.Sqrt(s / float64(len(a)))
								if s > 0 {
									out = (v - m) / s
								}
							}
						case 2:
							var a []float64
							for q := j - 5; q < j; q++ {
								if q >= 0 && !math.IsNaN(seq[q].raw[k]) {
									a = append(a, seq[q].raw[k])
								}
							}
							if len(a) >= 3 {
								out = v - mean(a)
							}
						case 3:
							out = v
						case 4:
							if j >= 1 && !math.IsNaN(seq[j-1].raw[k]) {
								out = seq[j-1].raw[k]
							}
						}
					}
					vals[rowPos[x]][col] = out
				}
			}
			col++
		}
	}
	for _, p := range pairSpecs {
		for d := 0; d < nd; d++ {
			for _, x := range ds.byDate[d] {
				get := func(name string) float64 {
					switch name {
					case "ctrl_rb1":
						return float64(x.feat[nf])
					case "ctrl_rb5":
						return float64(x.feat[nf+1])
					}
					k := fidx[name]
					if math.IsNaN(x.raw[k]) {
						return nan
					}
					return float64(x.feat[k])
				}
				a, b := get(p.a), get(p.b)
				if math.IsNaN(a) || math.IsNaN(b) {
					continue
				}
				if p.op == "prod" {
					vals[rowPos[x]][col] = a * b
				} else {
					vals[rowPos[x]][col] = a - b
				}
			}
		}
		col++
	}
	if libMode == "lib3" {
		for d := 0; d < nd; d++ {
			for _, x := range ds.byDate[d] {
				v := vals[rowPos[x]]
				v[nc], v[nc+1], v[nc+2], v[nc+3], v[nc+4] = x.sig, x.rank, x.rb1, x.rb5, x.rb20
			}
		}
	}
	ctlSeeds := []int64{20261024, 20261025, 20261026, 20261027, 20261028}
	for d := 0; d < nd; d++ {
		rows := ds.byDate[d]
		var who []int
		for i, x := range rows {
			if !math.IsNaN(x.raw[fidx["LevAsym"]]) {
				who = append(who, i)
			}
		}
		for c, sd := range ctlSeeds {
			rng := rand.New(rand.NewSource(sd*1000003 + int64(d)))
			perm := rng.Perm(len(who))
			for j, i := range who {
				vals[rowPos[rows[i]]][nc+nex+c] = rows[who[perm[j]]].raw[fidx["LevAsym"]]
			}
		}
	}
	tercileFix := func(d int, c int) {
		rows := ds.byDate[d]
		for tc := 0; tc < 3; tc++ {
			lo, hi := float64(tc)*33.34, float64(tc+1)*33.34
			var v []float64
			var who []*row
			for _, x := range rows {
				if x.rank > lo && x.rank <= hi && !math.IsNaN(vals[rowPos[x]][c]) {
					v = append(v, vals[rowPos[x]][c])
					who = append(who, x)
				}
			}
			if len(v) < 10 {
				for _, x := range who {
					vals[rowPos[x]][c] = nan
				}
				continue
			}
			rk := rankScale(v)
			for j, x := range who {
				vals[rowPos[x]][c] = rk[j]
			}
		}
	}
	for fi := range vitalFeatures {
		c := fi*len(transformNames) + 3
		for d := 0; d < nd; d++ {
			tercileFix(d, c)
		}
	}
	crank := make([][]float64, nd)
	for d := 0; d < nd; d++ {
		rows := ds.byDate[d]
		crank[d] = make([]float64, len(rows)*total)
		for i := range crank[d] {
			crank[d][i] = nan
		}
		for c := 0; c < total; c++ {
			var v []float64
			var who []int
			for i, x := range rows {
				if a := vals[rowPos[x]][c]; !math.IsNaN(a) {
					v = append(v, a)
					who = append(who, i)
				}
			}
			if len(v) < 30 {
				continue
			}
			rk := rankScale(v)
			for j, i := range who {
				crank[d][i*total+c] = rk[j]
			}
		}
	}

	raw, err := readFile(frzPath)
	if err != nil {
		fatal(err)
	}
	var fz frozen
	unmarshal(raw, &fz)
	t0 := sortSearch(ds.dates, "2020-04-01")
	r := newRunner(pp, fz, []string{"blend"})
	r.keep = map[int][]float64{}
	blocks := [][2]string{{"2025-01-03", "2025-03-31"}, {"2025-04-01", "2025-06-30"}, {"2025-07-01", "2025-09-30"}, {"2025-10-01", "2025-12-31"}, {"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-06-30"}, {"2026-07-01", "2026-10-16"}}
	wFrom, wTo, wSplit := "2025-01-03", "2026-10-16", "2026-01-05"
	if libWindow == "explore" {
		blocks = [][2]string{{"2022-07-01", "2022-12-30"}, {"2023-01-02", "2023-06-30"}, {"2023-07-03", "2023-12-29"}, {"2024-01-01", "2024-06-28"}, {"2024-07-01", "2024-12-30"}}
		wFrom, wTo, wSplit = "2022-07-01", "2024-12-30", "2023-10-02"
	}
	for _, b := range blocks {
		lo, hi := pp.dateRange(b[0], b[1])
		r.block(lo, hi, t0, nil, false)
	}
	lo, _ := pp.dateRange(wFrom, wTo)
	_, hi := pp.dateRange(wFrom, wTo)
	split := sortSearch(ds.dates, wSplit) - lo

	icS := make([][]float64, total)
	bsS := make([][]float64, total)
	for c := range icS {
		icS[c] = make([]float64, hi-lo+1)
		bsS[c] = make([]float64, hi-lo+1)
		for i := range icS[c] {
			icS[c][i], bsS[c][i] = nan, nan
		}
	}
	covAcc := make([][]float64, total)
	covN := make([][]int, total)
	for c := range covAcc {
		covAcc[c] = make([]float64, total)
		covN[c] = make([]int, total)
	}
	for d := lo; d <= hi; d++ {
		rows := ds.byDate[d]
		pr := r.keep[d]
		if pr == nil {
			continue
		}
		for c := 0; c < total; c++ {
			var y, p, s []float64
			for i, x := range rows {
				sv := crank[d][i*total+c]
				if math.IsNaN(sv) || math.IsNaN(x.y10) || math.IsNaN(pr[i]) {
					continue
				}
				y = append(y, x.y10)
				p = append(p, pr[i])
				s = append(s, sv)
			}
			if len(y) < 30 {
				continue
			}
			p = rankScale(p)
			if libMode == "lib3" {
				cut1, cut2 := thirds(s)
				var pt, yt, pb, yb []float64
				for i := range s {
					if s[i] >= cut2 {
						pt, yt = append(pt, p[i]), append(yt, y[i])
					} else if s[i] <= cut1 {
						pb, yb = append(pb, p[i]), append(yb, y[i])
					}
				}
				if len(pt) >= 15 && len(pb) >= 15 {
					it, ib := pearson(pt, yt), pearson(pb, yb)
					icS[c][d-lo] = it
					bsS[c][d-lo] = it - ib
				}
				continue
			}
			icS[c][d-lo] = pearson(s, y)
			my, mp, ms := mean(y), mean(p), mean(s)
			var spp, sss, sps, spy, ssy float64
			for i := range y {
				a, b, z := p[i]-mp, s[i]-ms, y[i]-my
				spp += a * a
				sss += b * b
				sps += a * b
				spy += a * z
				ssy += b * z
			}
			den := spp*sss - sps*sps
			if den > 1e-12 {
				bsS[c][d-lo] = (spp*ssy - sps*spy) / den
			}
		}
		for a := 0; a < total; a++ {
			for b := a + 1; b < total; b++ {
				var xs, ys []float64
				for i := range rows {
					u, v := crank[d][i*total+a], crank[d][i*total+b]
					if !math.IsNaN(u) && !math.IsNaN(v) {
						xs = append(xs, u)
						ys = append(ys, v)
					}
				}
				if len(xs) >= 30 {
					if cc := pearson(xs, ys); !math.IsNaN(cc) {
						covAcc[a][b] += cc
						covN[a][b]++
					}
				}
			}
		}
	}
	cm := make([][]float64, ncm)
	for a := 0; a < ncm; a++ {
		cm[a] = make([]float64, ncm)
		for b := 0; b < ncm; b++ {
			if a == b {
				cm[a][b] = 1
			} else {
				i, j := a, b
				if i > j {
					i, j = j, i
				}
				if covN[i][j] > 0 {
					cm[a][b] = covAcc[i][j] / float64(covN[i][j])
				}
			}
		}
	}
	e := jacobiEigen(cm)
	mu := mean(e)
	var vv float64
	for _, x := range e {
		vv += (x - mu) * (x - mu)
	}
	vv /= float64(ncm - 1)
	meff := 1 + float64(ncm-1)*(1-vv/float64(ncm))
	thr := 0.05 / meff
	maxE, offSum := 0.0, 0.0
	for _, x := range e {
		maxE = math.Max(maxE, x)
	}
	for a := 0; a < ncm; a++ {
		for b := a + 1; b < ncm; b++ {
			offSum += math.Abs(cm[a][b])
		}
	}
	var wSum, bSum float64
	var wN, bN int
	for a := 0; a < 80; a++ {
		for b := a + 1; b < 80; b++ {
			if a/5 == b/5 {
				wSum += cm[a][b]
				wN++
			} else {
				bSum += math.Abs(cm[a][b])
				bN++
			}
		}
	}
	fmt.Printf("lib2: mean correlation within the 16 feature blocks %.3f (%d pairs), mean absolute correlation between blocks %.3f (%d pairs)\n", wSum/float64(wN), wN, bSum/float64(bN), bN)
	fmt.Printf("lib2: largest eigenvalue %.2f, mean absolute pairwise correlation %.3f\n", maxE, offSum/float64(ncm*(ncm-1)/2))
	fmt.Printf("lib2: %d candidates, %d placebo controls, M_eff %.2f, threshold p <= %.5f on the incremental slope\n", ncm, nctl, meff, thr)
	if libMode == "lib3" {
		fmt.Println("lib3: statistic = mean daily IC of the composite in the top third of the moderator minus its IC in the bottom third; the IC column is the top-third IC")
	}
	fmt.Printf("%-5s %-26s %9s %7s %10s %7s %8s %9s %9s %8s flags\n", "id", "candidate", "IC", "t", "inc.slope", "t", "p", "half1", "half2", "ic_sign")
	type rowOut struct {
		c int
		p float64
	}
	var ps []float64
	hits, ctlHits := 0, 0
	for c := 0; c < total; c++ {
		ic := seqStat(icS[c], 0, hi-lo)
		bs := seqStat(bsS[c], 0, hi-lo)
		mi, _ := neweyWest(ic, 10)
		mb, tb := neweyWest(bs, 10)
		_, ti := neweyWest(ic, 10)
		h1, h2 := halves(bsS[c], split)
		p := normalTwoSided(tb)
		id, desc := fmt.Sprintf("C%02d", c-ncm+1), "placebo LevAsym permuted"
		if c < nc {
			id, desc = cands[c].id, cands[c].desc
			ps = append(ps, p)
		} else if c < ncm {
			id, desc = fmt.Sprintf("X%02d", c-nc+1), extraModerators[c-nc]
			ps = append(ps, p)
		}
		flags := ""
		same := h1*mb > 0 && h2*mb > 0 && (libMode == "lib3" || mi*mb > 0)
		if p <= thr && same {
			flags += " HIT"
			if c < ncm {
				hits++
			} else {
				ctlHits++
			}
		}
		fmt.Printf("%-5s %-26s %+9.5f %+7.2f %+10.5f %+7.2f %8.4f %+9.5f %+9.5f %8v%s\n", id, desc, mi, ti, mb, tb, p, h1, h2, mi*mb > 0, flags)
	}
	if libMode == "lib3" {
		diagnoseModerators(ds, crank, r.keep, total, lo, hi, split, []int{82, 91})
	}
	sort.Float64s(ps)
	bh := 0.0
	for i, p := range ps {
		if p <= 0.10*float64(i+1)/float64(len(ps)) {
			bh = p
		}
	}
	fmt.Printf("lib2: hits %d of %d, control hits %d of %d, BH(0.10) cut %.5f\n", hits, ncm, ctlHits, nctl, bh)
}

func thirds(s []float64) (float64, float64) {
	c := append([]float64(nil), s...)
	sort.Float64s(c)
	n := len(c)
	return c[n/3], c[n-1-n/3]
}

func diagnoseModerators(ds *dataset, crank [][]float64, keep map[int][]float64, total, lo, hi, split int, cols []int) {
	mech := make([]float64, hi-lo+1)
	strat := map[int][]float64{}
	for _, c := range cols {
		strat[c] = make([]float64, hi-lo+1)
		for i := range strat[c] {
			strat[c][i] = nan
		}
	}
	for i := range mech {
		mech[i] = nan
	}
	groupIC := func(p, y []float64, idx []int, s []float64) (float64, bool) {
		if len(idx) < 12 {
			return nan, false
		}
		cut1, cut2 := thirds(pick(s, idx))
		var pt, yt, pb, yb []float64
		for _, i := range idx {
			if s[i] >= cut2 {
				pt, yt = append(pt, p[i]), append(yt, y[i])
			} else if s[i] <= cut1 {
				pb, yb = append(pb, p[i]), append(yb, y[i])
			}
		}
		if len(pt) < 6 || len(pb) < 6 {
			return nan, false
		}
		return pearson(pt, yt) - pearson(pb, yb), true
	}
	for d := lo; d <= hi; d++ {
		rows := ds.byDate[d]
		pr := keep[d]
		if pr == nil {
			continue
		}
		var p, y []float64
		var who []int
		for i, x := range rows {
			if !math.IsNaN(pr[i]) && !math.IsNaN(x.y10) {
				p = append(p, pr[i])
				y = append(y, x.y10)
				who = append(who, i)
			}
		}
		if len(p) < 30 {
			continue
		}
		p = rankScale(p)
		ext := make([]float64, len(p))
		for i := range p {
			ext[i] = math.Abs(p[i])
		}
		all := make([]int, len(p))
		for i := range all {
			all[i] = i
		}
		if v, ok := groupIC(p, y, all, ext); ok {
			mech[d-lo] = v
		}
		order := append([]int(nil), all...)
		sort.Slice(order, func(a, b int) bool { return ext[order[a]] < ext[order[b]] })
		for _, c := range cols {
			s := make([]float64, len(p))
			okAll := true
			for j, i := range who {
				s[j] = crank[d][i*total+c]
				if math.IsNaN(s[j]) {
					okAll = false
				}
			}
			if !okAll {
				continue
			}
			var sum float64
			cnt := 0
			for k := 0; k < 3; k++ {
				a, b := k*len(order)/3, (k+1)*len(order)/3
				if v, ok := groupIC(p, y, order[a:b], s); ok {
					sum += v
					cnt++
				}
			}
			if cnt == 3 {
				strat[c][d-lo] = sum / 3
			}
		}
	}
	rep := func(name string, v []float64) {
		m, t := neweyWest(clean(v), 10)
		h1, h2 := halves(v, split)
		fmt.Printf("diag %-52s mean %+.5f NW t %+.2f halves %+.5f / %+.5f n %d\n", name, m, t, h1, h2, len(clean(v)))
	}
	rep("mechanical control: composite |prediction| as the moderator", mech)
	for _, c := range cols {
		rep(fmt.Sprintf("column %d moderator within |prediction| terciles (stratified)", c+1), strat[c])
	}
}

func pick(s []float64, idx []int) []float64 {
	o := make([]float64, len(idx))
	for k, i := range idx {
		o[k] = s[i]
	}
	return o
}
