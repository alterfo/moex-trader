package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
)

const llmSplit = "2025-01-03"

func attachLLM(items []item, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	r.FieldsPerRecord = -1
	scores := map[string]float64{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(rec) < 4 || rec[3] == "" {
			continue
		}
		scores[rec[0]+"|"+rec[1]] = f64(rec[3])
	}
	hit := 0
	for i := range items {
		if v, ok := scores[items[i].id+"|"+items[i].ticker]; ok {
			items[i].llm, items[i].hasLLM = v, true
			hit++
		}
	}
	fmt.Printf("newsllm: %d of %d headlines carry an LLM score\n", hit, len(items))
	return nil
}

type llmRow struct {
	id      string
	sig     string
	ctl     []string
	out     func(c cell) float64
	h       int
	primary bool
	perm    int64
	from    string
}

type llmStat struct {
	id        string
	h         int
	primary   bool
	control   bool
	mean, t   float64
	pre, post float64
	preT      float64
	postT     float64
	nPre      int
	nPost     int
}

func olsSlope(y []float64, x [][]float64, j int) float64 {
	k := len(x) + 1
	a := make([][]float64, k)
	for r := range a {
		a[r] = make([]float64, k+1)
	}
	col := func(c, i int) float64 {
		if c == 0 {
			return 1
		}
		return x[c-1][i]
	}
	for i := range y {
		for r := 0; r < k; r++ {
			for c := 0; c < k; c++ {
				a[r][c] += col(r, i) * col(c, i)
			}
			a[r][k] += col(r, i) * y[i]
		}
	}
	for p := 0; p < k; p++ {
		best := p
		for r := p + 1; r < k; r++ {
			if math.Abs(a[r][p]) > math.Abs(a[best][p]) {
				best = r
			}
		}
		if math.Abs(a[best][p]) < 1e-12 {
			return nan
		}
		a[p], a[best] = a[best], a[p]
		for r := 0; r < k; r++ {
			if r == p {
				continue
			}
			f := a[r][p] / a[p][p]
			for c := p; c <= k; c++ {
				a[r][c] -= f * a[p][c]
			}
		}
	}
	return a[j+1][k] / a[j+1][j+1]
}

func centredRank(v []float64) []float64 {
	r := rankAvg(v)
	n := float64(len(v))
	out := make([]float64, len(v))
	for i := range r {
		out[i] = r[i]/n - 0.5
	}
	return out
}

func runLLMRows(dates []string, byDate map[string][]cell, signals map[string]map[string]*agg) {
	nd := len(dates)
	secSet := map[string]bool{}
	for _, d := range dates {
		for _, c := range byDate[d] {
			secSet[c.secid] = true
		}
	}
	arr := map[string]map[string][]float64{}
	mk := func() []float64 {
		v := make([]float64, nd)
		for i := range v {
			v[i] = nan
		}
		return v
	}
	cntA := map[string][]float64{}
	eveA := map[string][]float64{}
	for s := range secSet {
		cntA[s], eveA[s] = make([]float64, nd), make([]float64, nd)
	}
	for i, d := range dates {
		for s, a := range signals[d] {
			if _, ok := cntA[s]; ok {
				cntA[s][i], eveA[s][i] = a.cnt, a.eveCnt
			}
		}
	}
	for _, k := range []string{"llmS", "shock", "sent", "zc", "eveS", "eveSent", "eveZ"} {
		arr[k] = map[string][]float64{}
	}
	for s := range secSet {
		for _, k := range []string{"llmS", "shock", "sent", "zc", "eveS", "eveSent", "eveZ"} {
			arr[k][s] = mk()
		}
		var hist []float64
		for i, d := range dates {
			a := signals[d][s]
			arr["zc"][s][i] = trailingZ(cntA[s], i)
			arr["eveZ"][s][i] = trailingZ(eveA[s], i)
			if a == nil {
				arr["sent"][s][i] = 0
				arr["eveSent"][s][i] = 0
				continue
			}
			arr["sent"][s][i] = a.sent / (a.cnt + 1)
			arr["eveSent"][s][i] = a.eveSent / (a.eveCnt + 1)
			if a.elc > 0 {
				arr["eveS"][s][i] = a.els / (a.elc + 1)
			}
			if a.lc > 0 {
				v := a.ls / (a.lc + 1)
				arr["llmS"][s][i] = v
				if len(hist) >= 20 {
					arr["shock"][s][i] = v - mean(hist[len(hist)-20:])
				}
				hist = append(hist, v)
			}
		}
	}
	abn := map[int]func(c cell) float64{
		1:  func(c cell) float64 { return c.f1 - c.u1 },
		5:  func(c cell) float64 { return c.f5 - c.u5 },
		10: func(c cell) float64 { return c.f10 - c.u10 },
	}
	idy := func(c cell) float64 { return c.idy1 }
	var rows []llmRow
	for _, h := range []int{1, 5, 10} {
		rows = append(rows, llmRow{id: fmt.Sprintf("L1 llm sentiment h%d", h), sig: "llmS", ctl: []string{"zc", "sent"}, out: abn[h], h: h, primary: h == 1})
		rows = append(rows, llmRow{id: fmt.Sprintf("L2 llm sentiment shock h%d", h), sig: "shock", ctl: []string{"zc", "sent"}, out: abn[h], h: h, primary: h == 1})
	}
	rows = append(rows, llmRow{id: "L3 evening llm sentiment -> next open-to-close", sig: "eveS", ctl: []string{"eveZ", "eveSent"}, out: idy, h: 1, primary: true})
	for k := int64(0); k < 5; k++ {
		rows = append(rows, llmRow{id: fmt.Sprintf("C%d permuted llm sentiment h1", k+1), sig: "llmS", out: abn[1], h: 1, primary: true, perm: 20261027 + k})
	}
	var stats, incs []llmStat
	for _, r := range rows {
		var ic, bs []float64
		var lab []string
		for i, d := range dates {
			var sv, ov []float64
			ctl := make([][]float64, len(r.ctl))
			for _, c := range byDate[d] {
				s := arr[r.sig][c.secid][i]
				o := r.out(c)
				if math.IsNaN(s) || math.IsNaN(o) {
					continue
				}
				ok := true
				cv := make([]float64, len(r.ctl))
				for j, ck := range r.ctl {
					cv[j] = arr[ck][c.secid][i]
					if math.IsNaN(cv[j]) {
						ok = false
					}
				}
				if !ok && r.perm == 0 {
					continue
				}
				sv, ov = append(sv, s), append(ov, o)
				for j := range ctl {
					ctl[j] = append(ctl[j], cv[j])
				}
			}
			if len(sv) < 30 {
				continue
			}
			if r.perm != 0 {
				rng := rand.New(rand.NewSource(r.perm*1000003 + int64(i)))
				rng.Shuffle(len(sv), func(a, b int) { sv[a], sv[b] = sv[b], sv[a] })
			}
			v := pearson(rankAvg(sv), rankAvg(ov))
			if math.IsNaN(v) {
				continue
			}
			ic = append(ic, v)
			lab = append(lab, d)
			if r.perm == 0 {
				x := [][]float64{centredRank(sv)}
				for j := range ctl {
					x = append(x, centredRank(ctl[j]))
				}
				bs = append(bs, olsSlope(ov, x, 0))
			} else {
				bs = append(bs, nan)
			}
		}
		stats = append(stats, summarise(r, ic, lab, false))
		if r.perm == 0 {
			var cb []float64
			var cl []string
			for i, v := range bs {
				if !math.IsNaN(v) {
					cb = append(cb, v)
					cl = append(cl, lab[i])
				}
			}
			incs = append(incs, summarise(llmRow{id: r.id + " | incremental slope over " + fmt.Sprint(r.ctl), h: r.h, primary: r.primary}, cb, cl, false))
		}
	}
	fmt.Printf("%-66s %5s %9s %7s %9s %7s %9s %7s %s\n", "row", "dates", "mean IC", "NW t", "2022-24", "t", "2025+", "t", "dates per half")
	for _, s := range stats {
		printStat(s)
	}
	fmt.Println("\nL4 incremental slopes (per-date OLS of the outcome on the centred ranks of the signal and of its controls, slope of the signal):")
	for _, s := range incs {
		printStat(s)
	}
	l5(dates, byDate, arr, abn[1])
	verdict(stats, incs)
}

func summarise(r llmRow, v []float64, lab []string, _ bool) llmStat {
	st := llmStat{id: r.id, h: r.h, primary: r.primary, control: r.perm != 0, mean: nan, t: nan, pre: nan, post: nan, preT: nan, postT: nan}
	if len(v) < 30 {
		return st
	}
	st.mean, st.t = neweyWest(v, r.h+5)
	var pre, post []float64
	for i, d := range lab {
		if d < llmSplit {
			pre = append(pre, v[i])
		} else {
			post = append(post, v[i])
		}
	}
	st.nPre, st.nPost = len(pre), len(post)
	if len(pre) > 0 {
		st.pre = mean(pre)
		_, st.preT = neweyWest(pre, r.h+5)
	}
	if len(post) > 0 {
		st.post = mean(post)
		_, st.postT = neweyWest(post, r.h+5)
	}
	return st
}

func printStat(s llmStat) {
	under := ""
	if s.nPre < 100 || s.nPost < 100 {
		under = " UNDERPOWERED (a half has under 100 dates)"
	}
	fmt.Printf("%-66s %5d %+9.4f %+7.2f %+9.4f %+7.2f %+9.4f %+7.2f  %d / %d%s\n", s.id, s.nPre+s.nPost, s.mean, s.t, s.pre, s.preT, s.post, s.postT, s.nPre, s.nPost, under)
}

func l5(dates []string, byDate map[string][]cell, arr map[string]map[string][]float64, out func(c cell) float64) {
	var ic, rho []float64
	var lab []string
	for i, d := range dates {
		var a, b, o []float64
		for _, c := range byDate[d] {
			s, l, y := arr["llmS"][c.secid][i], arr["sent"][c.secid][i], out(c)
			if math.IsNaN(s) || math.IsNaN(l) || math.IsNaN(y) {
				continue
			}
			a, b, o = append(a, s), append(b, l), append(o, y)
		}
		if len(a) < 30 {
			continue
		}
		ra, rb := rankAvg(a), rankAvg(b)
		diff := make([]float64, len(a))
		for j := range diff {
			diff[j] = ra[j] - rb[j]
		}
		v := pearson(rankAvg(diff), rankAvg(o))
		if math.IsNaN(v) {
			continue
		}
		ic = append(ic, v)
		rho = append(rho, pearson(ra, rb))
		lab = append(lab, d)
	}
	s := summarise(llmRow{id: "L5 IC of rank(llmS) - rank(sent) vs h1 abnormal", h: 1}, ic, lab, false)
	fmt.Println()
	printStat(s)
	fmt.Printf("L5 mean per-date rank correlation of llmS and sent: %+.3f (%d dates)\n", mean(clean(rho)), len(rho))
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

func verdict(stats, incs []llmStat) {
	const T = 3.0
	isHit := func(s llmStat) bool {
		return s.primary && !math.IsNaN(s.t) && math.Abs(s.t) >= T && s.pre*s.post > 0 && s.pre*s.mean > 0
	}
	ctl := 0
	for _, s := range stats {
		if s.control && isHit(s) {
			ctl++
		}
	}
	var hits []string
	byID := map[string]llmStat{}
	for _, s := range incs {
		byID[s.id] = s
	}
	for _, s := range stats {
		if s.control || !isHit(s) {
			continue
		}
		note := "no control hit"
		if ctl > 0 {
			note = "VOID: a control hit"
		}
		for _, inc := range incs {
			if len(inc.id) > len(s.id) && inc.id[:len(s.id)] == s.id {
				same := inc.mean*s.mean > 0
				note += fmt.Sprintf("; incremental slope t %+.2f (same sign %v); halves t %+.2f / %+.2f (both at least 2.0 with the sign: %v)", inc.t, same, s.preT, s.postT, math.Abs(s.preT) >= 2 && math.Abs(s.postT) >= 2 && s.preT*s.postT > 0)
			}
		}
		hits = append(hits, s.id+" ["+note+"]")
	}
	sort.Strings(hits)
	fmt.Printf("\nrule: primary-horizon row with |t| >= %.1f, same sign in both halves and as the mean; control hits %d of 5; hits: %v\n", T, ctl, hits)
}
