package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

var eqNames = []string{"E01", "E02", "E03", "E04", "E05", "E06", "E07", "E08", "E09", "E10", "E11", "E12", "E13"}

var trio = []string{"SBER", "GAZP", "LKOH"}
var oilSet = []string{"ROSN", "LKOH", "TATN"}
var metalSet = []string{"GMKN", "PLZL", "CHMF"}
var bankSet = []string{"SBER", "VTBR", "T"}

type eqBlock struct {
	eq     *eqData
	series map[string][]float64
	band2  map[string][]bool
	a2     map[string][]float64
	a2m5   map[string][]float64
	tz     map[string][]float64
	sr     map[string][]float64
	exWk   map[string][]bool
}

func loadEqBlock(path, divPath string, e *eqData) (*eqBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	nd := len(e.dates)
	b := &eqBlock{eq: e, series: map[string][]float64{}, band2: map[string][]bool{}, a2: map[string][]float64{}, a2m5: map[string][]float64{}, tz: map[string][]float64{}, sr: map[string][]float64{}, exWk: map[string][]bool{}}
	type cell struct{ a2, a5, tz, dg, sr, morn, auc float64 }
	byDate := make([]map[string]cell, nd)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		d, ok := e.dIdx[rec[ix["date"]]]
		if !ok {
			continue
		}
		t := rec[ix["secid"]]
		g := func(n string) float64 { return f64(rec[ix[n]]) }
		if rec[ix["adm100"]] == "1" {
			if byDate[d] == nil {
				byDate[d] = map[string]cell{}
			}
			byDate[d][t] = cell{g("ap_a2"), g("ap_a5"), g("ap_ticket_z"), g("ap_depth_growth"), g("ap_spread_ratio"), g("ap_morn_a1"), g("ap_auc_a1")}
		} else {
			if rec[ix["adm100"]] == "0" && rec[ix["t18"]] == "0" {
				if b.band2[t] == nil {
					b.band2[t] = make([]bool, nd)
					for _, m := range []map[string][]float64{b.a2, b.a2m5, b.tz, b.sr} {
						v := make([]float64, nd)
						for i := range v {
							v[i] = nan
						}
						m[t] = v
					}
				}
				b.band2[t][d] = true
				b.a2[t][d], b.a2m5[t][d], b.tz[t][d], b.sr[t][d] = g("ap_a2"), g("ap_a2_m5"), g("ap_ticket_z"), g("ap_spread_ratio")
			}
		}
	}
	for _, n := range eqNames {
		v := make([]float64, nd)
		for i := range v {
			v[i] = nan
		}
		b.series[n] = v
	}
	meanOf := func(m map[string]cell, names []string, pick func(cell) float64, need int) float64 {
		var v []float64
		for _, t := range names {
			if c, ok := m[t]; ok {
				if x := pick(c); !isNaN(x) {
					v = append(v, x)
				}
			}
		}
		if len(v) < need {
			return nan
		}
		return mean(v)
	}
	var all []string
	for d := 0; d < nd; d++ {
		m := byDate[d]
		if len(m) < 40 {
			continue
		}
		all = all[:0]
		for t := range m {
			all = append(all, t)
		}
		sort.Strings(all)
		var a2s []float64
		pos := 0
		for _, t := range all {
			if x := m[t].a2; !isNaN(x) {
				a2s = append(a2s, x)
				if x > 0 {
					pos++
				}
			}
		}
		if len(a2s) >= 40 {
			b.series["E01"][d] = mean(a2s)
			b.series["E02"][d] = float64(pos) / float64(len(a2s))
			b.series["E04"][d] = stdOf(a2s)
		}
		b.series["E03"][d] = meanOf(m, trio, func(c cell) float64 { return c.a2 }, 3)
		b.series["E05"][d] = meanOf(m, oilSet, func(c cell) float64 { return c.a2 }, 2)
		b.series["E06"][d] = meanOf(m, metalSet, func(c cell) float64 { return c.a2 }, 2)
		b.series["E07"][d] = meanOf(m, bankSet, func(c cell) float64 { return c.a2 }, 2)
		b.series["E08"][d] = meanOf(m, all, func(c cell) float64 { return c.tz }, 40)
		b.series["E09"][d] = meanOf(m, all, func(c cell) float64 { return c.a5 }, 40)
		b.series["E10"][d] = meanOf(m, all, func(c cell) float64 { return c.dg }, 40)
		b.series["E11"][d] = meanOf(m, all, func(c cell) float64 { return c.sr }, 40)
		b.series["E12"][d] = meanOf(m, all, func(c cell) float64 { return c.morn }, 40)
		b.series["E13"][d] = meanOf(m, all, func(c cell) float64 { return c.auc }, 40)
	}
	df, err := os.Open(divPath)
	if err == nil {
		defer df.Close()
		sc := bufio.NewScanner(df)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var rec struct {
				Ticker   string `json:"ticker"`
				LastBuy  string `json:"last_buy_date"`
				Declared string `json:"declared_date"`
			}
			if json.Unmarshal(sc.Bytes(), &rec) != nil || len(rec.LastBuy) < 10 || len(rec.Declared) < 10 {
				continue
			}
			if b.exWk[rec.Ticker] == nil {
				b.exWk[rec.Ticker] = make([]bool, nd)
			}
			lbDate := rec.LastBuy[:10]
			lb := e.dateIdxAtOrAfter(lbDate)
			if lb < nd && e.dates[lb] == lbDate {
				lb++
			}
			dec := rec.Declared[:10]
			for d := lb - 5; d < lb; d++ {
				if d >= 0 && d < nd && e.dates[d] >= dec {
					b.exWk[rec.Ticker][d] = true
				}
			}
		}
	}
	return b, nil
}

func (e *eqData) dateIdxAtOrAfter(date string) int {
	return sort.SearchStrings(e.dates, date)
}

func expiryWeek(dates []string) []float64 {
	out := make([]float64, len(dates))
	for i, ds := range dates {
		t, err := time.Parse("2006-01-02", ds)
		if err != nil {
			continue
		}
		m := t.Month()
		if m != 3 && m != 6 && m != 9 && m != 12 {
			continue
		}
		first := time.Date(t.Year(), m, 1, 0, 0, 0, 0, time.UTC)
		off := (4 - int(first.Weekday()) + 7) % 7
		third := first.AddDate(0, 0, off+14)
		monday := third.AddDate(0, 0, -3)
		if !t.Before(monday) && !t.After(third) {
			out[i] = 1
		}
	}
	return out
}

func alignToFo(c *ctx, e *eqData, v []float64) []float64 {
	out := make([]float64, len(c.fo.dates))
	for i := range out {
		out[i] = nan
		if j, ok := e.dIdx[c.fo.dates[i]]; ok {
			out[i] = v[j]
		}
	}
	return out
}

func specs3() []rowSpec {
	var r []rowSpec
	add := func(prefix string, s rowSpec) {
		s.id = prefix + twoDigits(countPrefix(r, prefix)+1)
		r = append(r, s)
	}
	desc := map[string]string{
		"E01": "universe-mean A2", "E02": "breadth of positive A2", "E03": "mean A2 of SBER, GAZP, LKOH", "E04": "cross-sectional std of A2",
		"E05": "mean A2 of ROSN, LKOH, TATN", "E06": "mean A2 of GMKN, PLZL, CHMF", "E07": "mean A2 of SBER, VTBR, T",
		"E08": "mean ticket-size z", "E09": "mean cancellation intensity A5", "E10": "mean depth growth", "E11": "mean spread ratio",
		"E12": "mean morning-session trade imbalance (07:00-09:55)", "E13": "mean closing-auction trade imbalance (18:40-18:50)",
	}
	ts := func(feat, grp string, hor int, kind string, sign float64) {
		oc := ""
		if kind == "oc1" {
			oc = ", outcome next-day open-to-close of the same contract"
		}
		add("P", rowSpec{book: "1", kind: "e2f", feat: feat, group: grp, hor: hor, sign: sign, group2: kind, desc: fmt.Sprintf("%s (z60) -> %s futures vol-scaled return h%d%s", desc[feat], grp, hor, oc)})
	}
	for _, f := range []string{"E01", "E02", "E03", "E04", "E08", "E09", "E10", "E11", "E12"} {
		ts(f, "MIX", 1, "ret", 1)
	}
	ts("E13", "MIX", 1, "oc1", 1)
	ts("E01", "MIX", 5, "ret", 1)
	ts("E11", "MIX", 5, "ret", 1)
	ts("E01", "RTS", 1, "ret", 1)
	ts("E01", "Si", 1, "ret", 1)
	ts("E07", "Si", 1, "ret", 1)
	ts("E01", "CNY", 1, "ret", 1)
	ts("E01", "BR", 1, "ret", 1)
	ts("E05", "BR", 1, "ret", 1)
	ts("E01", "GOLD", 1, "ret", 1)
	ts("E06", "GOLD", 1, "ret", 1)
	ts("E01", "NG", 1, "ret", 1)
	add("P", rowSpec{book: "1", kind: "e2f", feat: "E01", group: "MIX", hor: 1, sign: 1, group2: "ret", inter: [2]string{"expiryweek", ""}, desc: "universe-mean A2 (z60) x quarterly-expiry-week indicator -> MIX futures vol-scaled return h1 (interaction coefficient)"})
	for _, f := range []string{"E01", "E02", "E07", "E09"} {
		add("P", rowSpec{book: "1", kind: "e2fx", feat: f, group: "CNYRUB_TOM", hor: 1, sign: 1, desc: fmt.Sprintf("%s (z60) -> CNYRUB_TOM spot vol-scaled return h1", desc[f])})
	}
	return r
}

func specs4() []rowSpec {
	var r []rowSpec
	add := func(s rowSpec) {
		s.id = "Q" + twoDigits(len(r)+1)
		s.book = "1"
		r = append(r, s)
	}
	for _, p := range [][3]string{{"MIX", "hhi_agressive", "-"}, {"MIX", "hhi_passive", "-"}, {"MIX", "agr_diff", "+"}, {"MIX", "net_diff", "+"}, {"Si", "hhi_agressive", "-"}, {"Si", "agr_diff", "+"}} {
		sg := 1.0
		if p[2] == "-" {
			sg = -1
		}
		add(rowSpec{kind: "hi2ts", feat: p[1], group: p[0], hor: 1, sign: sg, desc: fmt.Sprintf("fo hi2 %s of %s (z60) -> %s next-day open-to-close, vol-scaled", p[1], p[0], p[0])})
	}
	for _, m := range []string{"hhi_agressive", "hhi_passive"} {
		add(rowSpec{kind: "hi2fm", feat: m, hor: 1, sign: -1, desc: fmt.Sprintf("fo hi2 %s (rank) -> next-day open-to-close abnormal return, FM over liquid groups", m)})
	}
	add(rowSpec{kind: "deferred", feat: "hhi_agressive", group: "CNYRUB_TOM", hor: 1, sign: -1, desc: "fx hi2 hhi_agressive of CNYRUB_TOM -> own next-day open-to-close (deferred: fx tradestats vintage)"})
	add(rowSpec{kind: "deferred", feat: "agr_diff", group: "CNYRUB_TOM", hor: 1, sign: 1, desc: "fx hi2 aggressive buy-minus-sell concentration of CNYRUB_TOM -> own next-day open-to-close (deferred: fx tradestats vintage)"})
	return r
}

func specs5() []rowSpec {
	var r []rowSpec
	add := func(s rowSpec) {
		s.id = "T" + twoDigits(len(r)+1)
		s.book = "2"
		r = append(r, s)
	}
	add(rowSpec{kind: "thin", feat: "a2", hor: 1, sign: 1, desc: "A2 -> thin-name (rank 101-300) vol-scaled abnormal return h1, FM"})
	add(rowSpec{kind: "thin", feat: "a2", hor: 5, sign: 1, desc: "A2 -> thin-name abnormal return h5, FM"})
	add(rowSpec{kind: "thin", feat: "a2m5", hor: 5, sign: 1, desc: "A2 5-day mean -> thin-name abnormal return h5, FM"})
	add(rowSpec{kind: "thin", feat: "a2", hor: 1, sign: 1, inter: [2]string{"exweek", ""}, desc: "A2 x ex-date-week indicator -> thin-name abnormal return h1, FM interaction coefficient"})
	add(rowSpec{kind: "thin", feat: "tz", hor: 1, sign: 1, desc: "ticket-size z20 -> thin-name abnormal return h1, FM"})
	add(rowSpec{kind: "thin", feat: "sr", hor: 5, sign: 1, desc: "spread ratio -> thin-name abnormal return h5, FM"})
	return r
}

var _ = strings.TrimSpace
var _ = math.Abs
