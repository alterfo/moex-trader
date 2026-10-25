package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
)

var nan = math.NaN()

type quote struct {
	close, sp float64
}

type event struct {
	ticker, last string
	yield        float64
	run10, run5  float64
	post5        float64
	spread       float64
	li           int
}

func f64(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nan
	}
	return v
}

func load(path string) ([]string, map[string]map[string]quote) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<22))
	hdr, _ := r.Read()
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	byDate := map[string]map[string]quote{}
	var dates []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		d := rec[ix["date"]]
		if byDate[d] == nil {
			byDate[d] = map[string]quote{}
			dates = append(dates, d)
		}
		byDate[d][rec[ix["secid"]]] = quote{close: f64(rec[ix["CloseDay"]]), sp: f64(rec[ix["SpBBO"]])}
	}
	sort.Strings(dates)
	return dates, byDate
}

func mean(a []float64) float64 {
	var s float64
	for _, x := range a {
		s += x
	}
	return s / float64(len(a))
}

func ret(byDate map[string]map[string]quote, dates []string, sec string, a, b int) float64 {
	qa, okA := byDate[dates[a]][sec]
	qb, okB := byDate[dates[b]][sec]
	if !okA || !okB || !(qa.close > 0) || !(qb.close > 0) {
		return nan
	}
	return math.Log(qb.close / qa.close)
}

func market(byDate map[string]map[string]quote, dates []string, a, b int) float64 {
	var v []float64
	for sec := range byDate[dates[a]] {
		if x := ret(byDate, dates, sec, a, b); !math.IsNaN(x) && math.Abs(x) < 0.5 {
			v = append(v, x)
		}
	}
	if len(v) < 40 {
		return nan
	}
	return mean(v)
}

func rank(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	r := make([]float64, len(v))
	for k, i := range idx {
		r[i] = float64(k)
	}
	return r
}

func corr(a, b []float64) float64 {
	ma, mb := mean(a), mean(b)
	var sab, saa, sbb float64
	for i := range a {
		sab += (a[i] - ma) * (b[i] - mb)
		saa += (a[i] - ma) * (a[i] - ma)
		sbb += (b[i] - mb) * (b[i] - mb)
	}
	return sab / math.Sqrt(saa*sbb)
}

func main() {
	matrix := flag.String("matrix", "work/matrix.csv", "flowscan matrix")
	div := flag.String("div", "data/dividends.jsonl", "dividend calendar")
	flag.Parse()
	dates, byDate := load(*matrix)
	alias := map[string]string{"YNDX": "YDEX", "TCSG": "T"}
	f, err := os.Open(*div)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	seen := map[string]bool{}
	var events []event
	for sc.Scan() {
		var r struct {
			Ticker string `json:"ticker"`
			Last   string `json:"last_buy_date"`
			Net    string `json:"dividend_net"`
		}
		if json.Unmarshal([]byte(sc.Text()), &r) != nil || len(r.Last) < 10 {
			continue
		}
		t := strings.TrimSpace(r.Ticker)
		if a, ok := alias[t]; ok {
			t = a
		}
		l := r.Last[:10]
		if seen[t+"|"+l] {
			continue
		}
		seen[t+"|"+l] = true
		li := sort.SearchStrings(dates, l)
		if li >= len(dates) || li < 12 || li+8 >= len(dates) {
			continue
		}
		if dates[li] != l {
			li--
		}
		amount := f64(r.Net)
		p0, ok0 := byDate[dates[li-11]][t]
		if !ok0 || !(p0.close > 0) || math.IsNaN(amount) || amount <= 0 {
			continue
		}
		e := event{ticker: t, last: l, li: li, yield: amount / p0.close, spread: p0.sp}
		for _, w := range []struct {
			dst  *float64
			a, b int
		}{{&e.run10, li - 11, li - 1}, {&e.run5, li - 6, li - 1}, {&e.post5, li + 2, li + 7}} {
			x := ret(byDate, dates, t, w.a, w.b)
			m := market(byDate, dates, w.a, w.b)
			if math.IsNaN(x) || math.IsNaN(m) {
				*w.dst = nan
			} else {
				*w.dst = x - m
			}
		}
		events = append(events, e)
	}
	fmt.Printf("divrunup: %d dividend events with a top-100 price history around the last-buy date, yields median %.1f%%\n", len(events), median(yields(events))*100)
	rows := []struct {
		id  string
		get func(e event) float64
	}{
		{"DV1 run-up, 10 days to the day before the last-buy day", func(e event) float64 { return e.run10 }},
		{"DV2 run-up, 5 days to the day before the last-buy day", func(e event) float64 { return e.run5 }},
		{"DV3 post ex-date drift, 5 days from the second day after", func(e event) float64 { return e.post5 }},
	}
	for _, r := range rows {
		var y, o []float64
		var cl []string
		for _, e := range events {
			if v := r.get(e); !math.IsNaN(v) {
				y, o, cl = append(y, e.yield), append(o, v), append(cl, e.last[:7])
			}
		}
		if len(y) < 40 {
			fmt.Printf("%-60s not testable (%d events)\n", r.id, len(y))
			continue
		}
		c := corr(rank(y), rank(o))
		lo, hi := bootCorr(y, o, cl)
		hiY := median(y)
		var top, bot []float64
		for i := range y {
			if y[i] >= hiY {
				top = append(top, o[i])
			} else {
				bot = append(bot, o[i])
			}
		}
		fmt.Printf("%-60s n %3d  rank corr(yield, abnormal) %+.3f [%+.3f, %+.3f]  mean abnormal: top-half yield %+.2f%%, bottom half %+.2f%%, all %+.2f%%\n", r.id, len(y), c, lo, hi, mean(top)*100, mean(bot)*100, mean(o)*100)
	}
	economics(events)
}

func yields(es []event) []float64 {
	var o []float64
	for _, e := range es {
		o = append(o, e.yield)
	}
	return o
}

func median(a []float64) float64 {
	c := append([]float64(nil), a...)
	sort.Float64s(c)
	return c[len(c)/2]
}

func bootCorr(y, o []float64, cl []string) (float64, float64) {
	groups := map[string][]int{}
	var keys []string
	for i, c := range cl {
		if _, ok := groups[c]; !ok {
			keys = append(keys, c)
		}
		groups[c] = append(groups[c], i)
	}
	rng := rand.New(rand.NewSource(20261025))
	var cs []float64
	for d := 0; d < 3000; d++ {
		var yy, oo []float64
		for range keys {
			g := groups[keys[rng.Intn(len(keys))]]
			for _, i := range g {
				yy, oo = append(yy, y[i]), append(oo, o[i])
			}
		}
		if len(yy) > 10 {
			cs = append(cs, corr(rank(yy), rank(oo)))
		}
	}
	sort.Float64s(cs)
	return cs[int(0.025*float64(len(cs)))], cs[int(0.975*float64(len(cs)))]
}

func economics(es []event) {
	var cn, cy []string
	var cv []float64
	for _, thr := range []float64{0.0, 0.03, 0.06} {
		var net, gross []float64
		for _, e := range es {
			if e.yield < thr || math.IsNaN(e.run10) {
				continue
			}
			cost := 2 * (0.0005 + 0.5*e.spread/1e4)
			if math.IsNaN(e.spread) {
				cost = 0.003
			}
			gross = append(gross, e.run10)
			net = append(net, e.run10-cost)
			if thr == 0.03 {
				cn, cy, cv = append(cn, e.last[:7]), append(cy, e.last[:4]), append(cv, e.run10-cost)
			}
		}
		if len(net) < 20 {
			continue
		}
		lo, hi := bootMean(net)
		if thr == 0.03 {
			lo2, hi2 := clusterMean(cv, cn)
			fmt.Printf("robustness (added after the first read) for yield >= 3%%: month-cluster bootstrap 95%% interval of the net mean [%+.2f%%, %+.2f%%]\n", lo2*100, hi2*100)
			by := map[string][]float64{}
			for i, y := range cy {
				by[y] = append(by[y], cv[i])
			}
			var ys []string
			for y := range by {
				ys = append(ys, y)
			}
			sort.Strings(ys)
			for _, y := range ys {
				fmt.Printf("  %s: %d events, mean net %+.2f%%\n", y, len(by[y]), mean(by[y])*100)
			}
		}
		fmt.Printf("economics: buy 11 days before the last-buy day, sell the day before it, long only against the market, yield >= %.0f%%: %d events, gross %+.2f%%, net of the quoted spread both ways %+.2f%% [%+.2f%%, %+.2f%%]\n", thr*100, len(net), mean(gross)*100, mean(net)*100, lo*100, hi*100)
	}
}

func bootMean(x []float64) (float64, float64) {
	rng := rand.New(rand.NewSource(20261026))
	ms := make([]float64, 4000)
	for d := range ms {
		var s float64
		for range x {
			s += x[rng.Intn(len(x))]
		}
		ms[d] = s / float64(len(x))
	}
	sort.Float64s(ms)
	return ms[int(0.025*4000)], ms[int(0.975*4000)]
}

func clusterMean(v []float64, cl []string) (float64, float64) {
	groups := map[string][]float64{}
	var keys []string
	for i, c := range cl {
		if _, ok := groups[c]; !ok {
			keys = append(keys, c)
		}
		groups[c] = append(groups[c], v[i])
	}
	rng := rand.New(rand.NewSource(20261027))
	ms := make([]float64, 4000)
	for d := range ms {
		var s float64
		n := 0
		for range keys {
			for _, x := range groups[keys[rng.Intn(len(keys))]] {
				s += x
				n++
			}
		}
		ms[d] = s / float64(n)
	}
	sort.Float64s(ms)
	return ms[int(0.025*4000)], ms[int(0.975*4000)]
}
