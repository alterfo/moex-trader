package main

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/macro"
)

const dayFmt = "2006-01-02"

func lastBefore(s macro.Series, d time.Time) int {
	i := sort.Search(len(s), func(k int) bool { return !s[k].Date.Before(d) })
	return i - 1
}

func logChanges(s macro.Series) []float64 {
	out := make([]float64, len(s))
	for i := range s {
		out[i] = math.NaN()
		if i > 0 && s[i].Value > 0 && s[i-1].Value > 0 {
			out[i] = math.Log(s[i].Value / s[i-1].Value)
		}
	}
	return out
}

func rollVol(ch []float64, w int) []float64 {
	out := make([]float64, len(ch))
	for i := range out {
		out[i] = math.NaN()
		if i < w {
			continue
		}
		var a []float64
		for k := i - w + 1; k <= i; k++ {
			if !math.IsNaN(ch[k]) {
				a = append(a, ch[k])
			}
		}
		if len(a) < w-2 {
			continue
		}
		m := mean(a)
		var ss float64
		for _, x := range a {
			ss += (x - m) * (x - m)
		}
		out[i] = math.Sqrt(ss / float64(len(a)-1))
	}
	return out
}

func highFlag(v []float64, i int) float64 {
	if i < 120 || math.IsNaN(v[i]) {
		return math.NaN()
	}
	lo := i - 250
	if lo < 0 {
		lo = 0
	}
	var a []float64
	for k := lo; k < i; k++ {
		if !math.IsNaN(v[k]) {
			a = append(a, v[k])
		}
	}
	if len(a) < 120 {
		return math.NaN()
	}
	sort.Float64s(a)
	if v[i] > a[len(a)/2] {
		return 1
	}
	return 0
}

func mean(a []float64) float64 {
	var s float64
	for _, x := range a {
		s += x
	}
	return s / float64(len(a))
}

type stateSet struct {
	ids    []string
	values map[string][]float64
}

func vals(s macro.Series) []float64 {
	o := make([]float64, len(s))
	for i, p := range s {
		o[i] = p.Value
	}
	return o
}

func sumWindow(v []float64, w int) []float64 {
	o := make([]float64, len(v))
	for i := range o {
		o[i] = math.NaN()
		if i >= w-1 {
			var s float64
			for k := i - w + 1; k <= i; k++ {
				s += v[k]
			}
			o[i] = s
		}
	}
	return o
}

func newsCounts(path string) (map[string]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	loc := time.FixedZone("MSK", 3*3600)
	seen := map[string]bool{}
	out := map[string]float64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r struct {
			ID    string `json:"article_id"`
			Title string `json:"title"`
			TS    int64  `json:"published_ts"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.TS == 0 {
			continue
		}
		if !strings.Contains(strings.ToLower(r.Title), "санкц") || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out[time.Unix(r.TS, 0).In(loc).Format(dayFmt)]++
	}
	return out, sc.Err()
}

type factor struct {
	name   string
	series macro.Series
	chg    map[string]float64
}

func buildFactor(name string, s macro.Series) *factor {
	f := &factor{name: name, series: s, chg: map[string]float64{}}
	ch := logChanges(s)
	for i, p := range s {
		if !math.IsNaN(ch[i]) {
			f.chg[p.Date.Format(dayFmt)] = ch[i]
		}
	}
	return f
}

func (f *factor) tenDay(d time.Time) float64 {
	i := lastBefore(f.series, d)
	if i >= 0 && d.Sub(f.series[i].Date) > 7*24*time.Hour {
		return math.NaN()
	}
	if i < 10 || f.series[i].Value <= 0 || f.series[i-10].Value <= 0 {
		return math.NaN()
	}
	return math.Log(f.series[i].Value / f.series[i-10].Value)
}

func (f *factor) tenDayZ(d time.Time) float64 {
	i := lastBefore(f.series, d)
	if i < 130 {
		return math.NaN()
	}
	lo := i - 250
	if lo < 10 {
		lo = 10
	}
	var a []float64
	for k := lo; k < i; k++ {
		if f.series[k].Value > 0 && f.series[k-10].Value > 0 {
			a = append(a, math.Log(f.series[k].Value/f.series[k-10].Value))
		}
	}
	if len(a) < 100 {
		return math.NaN()
	}
	m := mean(a)
	var ss float64
	for _, x := range a {
		ss += (x - m) * (x - m)
	}
	sd := math.Sqrt(ss / float64(len(a)-1))
	if sd == 0 {
		return math.NaN()
	}
	return f.tenDay(d) / sd
}

func buildStates(dir, newsPath string, dates []time.Time) (*stateSet, map[string]*factor, error) {
	load := func(n string) (macro.Series, error) { return macro.LoadCSV(dir + "/" + n + ".csv") }
	kr, err := load("keyrate")
	if err != nil {
		return nil, nil, err
	}
	cny, err := load("cnyrub")
	if err != nil {
		return nil, nil, err
	}
	brent, err := load("brent")
	if err != nil {
		return nil, nil, err
	}
	imoex, err := load("imoex")
	if err != nil {
		return nil, nil, err
	}
	gold, err := load("gold")
	if err != nil {
		return nil, nil, err
	}
	rvi, err := load("rvi")
	if err != nil {
		return nil, nil, err
	}
	fund, err := load("fundvalue")
	if err != nil {
		return nil, nil, err
	}
	kp, err := load("kp")
	if err != nil {
		return nil, nil, err
	}
	news, err := newsCounts(newsPath)
	if err != nil {
		return nil, nil, err
	}
	ss := &stateSet{values: map[string][]float64{}}
	add := func(id string, f func(d time.Time) float64) {
		ss.ids = append(ss.ids, id)
		v := make([]float64, len(dates))
		for i, d := range dates {
			v[i] = f(d)
		}
		ss.values[id] = v
	}
	cnyVol := rollVol(logChanges(cny), 10)
	brentVol := rollVol(logChanges(brent), 20)
	imoexVol := rollVol(logChanges(imoex), 10)
	goldVol := rollVol(logChanges(gold), 10)
	rviV := vals(rvi)
	fundSum := sumWindow(vals(fund), 5)
	krCh := make([]float64, len(kr))
	for i := 1; i < len(kr); i++ {
		if kr[i].Value != kr[i-1].Value {
			krCh[i] = 1
		}
	}
	imoexMA := make([]float64, len(imoex))
	for i := range imoex {
		imoexMA[i] = math.NaN()
		if i >= 99 {
			var s float64
			for k := i - 99; k <= i; k++ {
				s += imoex[k].Value
			}
			imoexMA[i] = s / 100
		}
	}
	add("F01", func(d time.Time) float64 {
		i := lastBefore(kr, d)
		if i < 63 {
			return math.NaN()
		}
		if kr[i].Value < kr[i-63].Value {
			return 1
		}
		return 0
	})
	add("F02", func(d time.Time) float64 {
		i := lastBefore(kr, d)
		if i < 5 {
			return math.NaN()
		}
		for k := i - 4; k <= i; k++ {
			if krCh[k] == 1 {
				return 1
			}
		}
		return 0
	})
	add("F03", func(d time.Time) float64 { return highFlag(cnyVol, lastBefore(cny, d)) })
	add("F04", func(d time.Time) float64 { return highFlag(brentVol, lastBefore(brent, d)) })
	add("F05", func(d time.Time) float64 { return highFlag(imoexVol, lastBefore(imoex, d)) })
	add("F06", func(d time.Time) float64 {
		i := lastBefore(imoex, d)
		if i < 0 || math.IsNaN(imoexMA[i]) {
			return math.NaN()
		}
		if imoex[i].Value > imoexMA[i] {
			return 1
		}
		return 0
	})
	add("F07", func(d time.Time) float64 { return highFlag(goldVol, lastBefore(gold, d)) })
	add("F08", func(d time.Time) float64 { return highFlag(rviV, lastBefore(rvi, d)) })
	var cal []time.Time
	for t := dates[0].AddDate(0, 0, -420); !t.After(dates[len(dates)-1]); t = t.AddDate(0, 0, 1) {
		cal = append(cal, t)
	}
	cnt := make([]float64, len(cal))
	for i, t := range cal {
		cnt[i] = news[t.Format(dayFmt)]
	}
	sum5 := sumWindow(cnt, 5)
	calIdx := map[string]int{}
	for i, t := range cal {
		calIdx[t.Format(dayFmt)] = i
	}
	add("F09", func(d time.Time) float64 {
		i, ok := calIdx[d.AddDate(0, 0, -1).Format(dayFmt)]
		if !ok || i < 130 {
			return math.NaN()
		}
		return highFlag(sum5, i)
	})
	tom := make([]float64, len(dates))
	for i, d := range dates {
		first, last := 0, 0
		for k := i; k >= 0 && dates[k].Month() == d.Month() && dates[k].Year() == d.Year(); k-- {
			first++
		}
		for k := i; k < len(dates) && dates[k].Month() == d.Month() && dates[k].Year() == d.Year(); k++ {
			last++
		}
		if first <= 3 || (last <= 2 && i+last < len(dates)) {
			tom[i] = 1
		}
	}
	ss.ids = append(ss.ids, "F10")
	ss.values["F10"] = tom
	add("F11", func(d time.Time) float64 {
		i := lastBefore(brent, d)
		if i < 10 {
			return math.NaN()
		}
		if brent[i].Value > brent[i-10].Value {
			return 1
		}
		return 0
	})
	add("F12", func(d time.Time) float64 {
		i := lastBefore(fund, d)
		if i < 0 {
			return math.NaN()
		}
		return highFlag(fundSum, i)
	})
	add("C01", func(d time.Time) float64 {
		if saturnLongitude(d.AddDate(0, 0, -1)) < 180 {
			return 1
		}
		return 0
	})
	add("C02", func(d time.Time) float64 {
		if moonAge(d.AddDate(0, 0, -1).Add(12*time.Hour)) < 14.765 {
			return 1
		}
		return 0
	})
	add("C03", func(d time.Time) float64 {
		i := lastBefore(kp, d)
		if i < 0 {
			return math.NaN()
		}
		if kp[i].Value >= 5 {
			return 1
		}
		return 0
	})
	fac := map[string]*factor{
		"brent": buildFactor("brent", brent), "cny": buildFactor("cny", cny), "gold": buildFactor("gold", gold),
		"imoex": buildFactor("imoex", imoex), "rvi": buildFactor("rvi", rvi),
	}
	return ss, fac, nil
}

func addWave2States(dir string, dates []time.Time, ss *stateSet, fac map[string]*factor) error {
	load := func(n string) (macro.Series, error) { return macro.LoadCSV(dir + "/" + n + ".csv") }
	long, err := load("rugbitr10y")
	if err != nil {
		return err
	}
	short, err := load("rugbitr1y")
	if err != nil {
		return err
	}
	usd, err := load("usdrub")
	if err != nil {
		return err
	}
	rts, err := load("rtsi")
	if err != nil {
		return err
	}
	btc, err := load("btc")
	if err != nil {
		return err
	}
	add := func(id string, f func(d time.Time) float64) {
		ss.ids = append(ss.ids, id)
		v := make([]float64, len(dates))
		for i, d := range dates {
			v[i] = f(d)
		}
		ss.values[id] = v
	}
	freshIdx := func(s macro.Series, d time.Time) int {
		i := lastBefore(s, d)
		if i >= 0 && d.Sub(s[i].Date) > 7*24*time.Hour {
			return -1
		}
		return i
	}
	usdVol := rollVol(logChanges(usd), 10)
	btcVol := rollVol(logChanges(btc), 10)
	rtsMA := make([]float64, len(rts))
	for i := range rts {
		rtsMA[i] = math.NaN()
		if i >= 99 {
			var s float64
			for k := i - 99; k <= i; k++ {
				s += rts[k].Value
			}
			rtsMA[i] = s / 100
		}
	}
	add("G01", func(d time.Time) float64 {
		i, j := freshIdx(long, d), freshIdx(short, d)
		if i < 10 || j < 10 || long[i].Value <= 0 || long[i-10].Value <= 0 || short[j].Value <= 0 || short[j-10].Value <= 0 {
			return math.NaN()
		}
		if math.Log(long[i].Value/long[i-10].Value)-math.Log(short[j].Value/short[j-10].Value) > 0 {
			return 1
		}
		return 0
	})
	add("G02", func(d time.Time) float64 { return highFlag(usdVol, freshIdx(usd, d)) })
	add("G03", func(d time.Time) float64 {
		i := freshIdx(rts, d)
		if i < 0 || math.IsNaN(rtsMA[i]) {
			return math.NaN()
		}
		if rts[i].Value > rtsMA[i] {
			return 1
		}
		return 0
	})
	add("C04", func(d time.Time) float64 {
		if mercuryRetrograde(d.AddDate(0, 0, -1).Add(12 * time.Hour)) {
			return 1
		}
		return 0
	})
	add("C05", func(d time.Time) float64 {
		if d.Day()%2 == 0 {
			return 1
		}
		return 0
	})
	add("C06", func(d time.Time) float64 { return highFlag(btcVol, freshIdx(btc, d)) })
	for _, n := range []string{"moexog", "moexfn", "moexmm", "moexch", "moexeu", "moextl"} {
		s, err := load(n)
		if err != nil {
			return err
		}
		fac[n] = buildFactor(n, s)
	}
	return nil
}
