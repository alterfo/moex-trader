package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/macro"
)

var energyList = []string{"IRAO", "HYDR", "MSNG", "UPRO", "FEES", "OGKB", "TGKA", "TGKN", "MRKP", "MRKC", "MRKV", "MRKU", "MRKZ", "GAZP", "NVTK"}
var agriList = []string{"PHOR", "RAGR", "NKHP"}

type wxSeries struct {
	byDate map[string]float64
}

func loadWx(dir, name string) (*wxSeries, error) {
	s, err := macro.LoadCSV(dir + "/" + name + ".csv")
	if err != nil {
		return nil, err
	}
	w := &wxSeries{byDate: map[string]float64{}}
	for _, p := range s {
		w.byDate[p.Date.Format(dayFmt)] = p.Value
	}
	return w, nil
}

func (w *wxSeries) window(d time.Time, from, to int, f func(float64) float64) (float64, int) {
	var s float64
	n := 0
	for k := from; k <= to; k++ {
		v, ok := w.byDate[d.AddDate(0, 0, -k).Format(dayFmt)]
		if ok {
			s += f(v)
			n++
		}
	}
	return s, n
}

func ident(x float64) float64 { return x }

func hdd(x float64) float64 { return math.Max(18-x, 0) }

func (w *wxSeries) anomaly(d time.Time, days int, f func(float64) float64, sum bool) float64 {
	cur, n := w.window(d, 1, days, f)
	if n < days-1 {
		return math.NaN()
	}
	var clim []float64
	for y := 1; y <= 5; y++ {
		dd := d.AddDate(-y, 0, 0)
		c, m := w.window(dd, 1, days, f)
		if m >= days-1 {
			if sum {
				clim = append(clim, c)
			} else {
				clim = append(clim, c/float64(m))
			}
		}
	}
	if len(clim) < 4 {
		return math.NaN()
	}
	if sum {
		sort.Float64s(clim)
		med := clim[len(clim)/2]
		return cur - med
	}
	return cur/float64(n) - mean(clim)
}

func flag01(x float64) float64 {
	if math.IsNaN(x) {
		return math.NaN()
	}
	if x > 0 {
		return 1
	}
	return 0
}

type wave4Out struct {
	zHDD, zRostovP func(d time.Time) float64
	blocks         map[string][]string
}

func addWave4States(dir string, dates []time.Time, ss *stateSet, fac map[string]*factor) (*wave4Out, error) {
	out := &wave4Out{blocks: map[string][]string{}}
	add := func(id, block string, f func(d time.Time) float64) {
		ss.ids = append(ss.ids, id)
		v := make([]float64, len(dates))
		for i, d := range dates {
			v[i] = f(d)
		}
		ss.values[id] = v
		out.blocks[block] = append(out.blocks[block], id)
	}
	for i, c := range cities {
		t, err := loadWx(dir, "wx_"+c.name+"_t")
		if err != nil {
			return nil, err
		}
		id := "W" + two(i+1)
		add(id, "weather", func(d time.Time) float64 { return flag01(t.anomaly(d, 3, ident, false)) })
	}
	mosT, err := loadWx(dir, "wx_moscow_t")
	if err != nil {
		return nil, err
	}
	rostP, err := loadWx(dir, "wx_rostov_p")
	if err != nil {
		return nil, err
	}
	add("W11", "weather", func(d time.Time) float64 { return flag01(mosT.anomaly(d, 7, hdd, true)) })
	add("W12", "weather", func(d time.Time) float64 { return flag01(rostP.anomaly(d, 7, ident, true)) })
	zof := func(w *wxSeries, f func(float64) float64) func(d time.Time) float64 {
		return func(d time.Time) float64 {
			a := w.anomaly(d, 7, f, true)
			if math.IsNaN(a) {
				return math.NaN()
			}
			var xs []float64
			for k := 1; k <= 180; k += 3 {
				if v := w.anomaly(d.AddDate(0, 0, -k), 7, f, true); !math.IsNaN(v) {
					xs = append(xs, v)
				}
			}
			if len(xs) < 40 {
				return math.NaN()
			}
			sd := stdev(xs)
			if sd == 0 {
				return math.NaN()
			}
			return a / sd
		}
	}
	out.zHDD = zof(mosT, hdd)
	out.zRostovP = zof(rostP, ident)

	cm := []struct{ id, file string }{
		{"K01", "cm_ng"}, {"K02", "cm_ttf"}, {"K03", "cm_hg"}, {"K04", "cm_ali"}, {"K05", "cm_hrc"}, {"K06", "cm_si"},
		{"K07", "cm_pl"}, {"K08", "cm_pa"}, {"K09", "cm_zw"}, {"K10", "cm_zc"}, {"K11", "cm_zs"}, {"K12", "cm_ura"},
	}
	cseries := map[string]macro.Series{}
	for _, c := range cm {
		s, err := macro.LoadCSV(dir + "/" + c.file + ".csv")
		if err != nil {
			return nil, err
		}
		cseries[c.id] = s
		fac[c.file] = buildFactor(c.file, s)
		sc := s
		add(c.id, "commodity", func(d time.Time) float64 {
			i := lastBefore(sc, d)
			if i < 10 || d.Sub(sc[i].Date) > 7*24*time.Hour || sc[i].Value <= 0 || sc[i-10].Value <= 0 {
				return math.NaN()
			}
			if sc[i].Value > sc[i-10].Value {
				return 1
			}
			return 0
		})
	}
	gd := []struct{ id, file string }{{"N01", "gd_sanc_tone"}, {"N02", "gd_energy_tone"}, {"N03", "gd_econ_tone"}, {"N04", "gd_vol"}}
	for _, g := range gd {
		s, err := macro.LoadCSV(dir + "/" + g.file + ".csv")
		if err != nil {
			return nil, err
		}
		roll := make([]float64, len(s))
		for i := range s {
			roll[i] = math.NaN()
			if i >= 6 {
				var sum float64
				for k := i - 6; k <= i; k++ {
					sum += s[k].Value
				}
				roll[i] = sum / 7
			}
		}
		sc := s
		low := g.id != "N04"
		add(g.id, "gdelt", func(d time.Time) float64 {
			i := lastBefore(sc, d)
			if i < 0 || d.Sub(sc[i].Date) > 7*24*time.Hour {
				return math.NaN()
			}
			h := highFlag(roll, i)
			if math.IsNaN(h) || !low {
				return h
			}
			return 1 - h
		})
	}
	brent, err := macro.LoadCSV(dir + "/brent.csv")
	if err != nil {
		return nil, err
	}
	eqSeries := []macro.Series{brent, cseries["K01"], cseries["K02"], cseries["K03"], cseries["K04"], cseries["K05"], cseries["K07"], cseries["K08"], cseries["K09"]}
	rets := make([][]float64, len(eqSeries))
	for k, s := range eqSeries {
		rets[k] = make([]float64, len(s))
		for i := range s {
			rets[k][i] = math.NaN()
			if i >= 10 && s[i].Value > 0 && s[i-10].Value > 0 {
				rets[k][i] = math.Log(s[i].Value / s[i-10].Value)
			}
		}
	}
	add("E01", "equilibrium", func(d time.Time) float64 {
		var tot float64
		n := 0
		for k, s := range eqSeries {
			i := lastBefore(s, d)
			if i < 270 || d.Sub(s[i].Date) > 7*24*time.Hour || math.IsNaN(rets[k][i]) {
				continue
			}
			var xs []float64
			for q := i - 250; q < i; q++ {
				if !math.IsNaN(rets[k][q]) {
					xs = append(xs, rets[k][q])
				}
			}
			if len(xs) < 150 {
				continue
			}
			sd := stdev(xs)
			if sd == 0 {
				continue
			}
			tot += rets[k][i] / sd
			n++
		}
		if n < 7 {
			return math.NaN()
		}
		if tot > 0 {
			return 1
		}
		return 0
	})
	return out, nil
}

func two(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

var wave4Mode bool

func toSet(a []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range a {
		m[s] = true
	}
	return m
}

func blockReport(ss *stateSet, w4 *wave4Out) {
	names := []string{"weather", "commodity", "gdelt", "equilibrium"}
	for _, b := range names {
		ids := w4.blocks[b]
		if len(ids) < 2 {
			continue
		}
		var ser [][]float64
		for _, id := range ids {
			ser = append(ser, ss.values[id])
		}
		m := len(ser)
		c := make([][]float64, m)
		for i := range c {
			c[i] = make([]float64, m)
			for j := range c[i] {
				if i == j {
					c[i][j] = 1
				} else {
					c[i][j] = pairCorr(ser[i], ser[j])
				}
			}
		}
		e := jacobiEigen(c)
		sort.Float64s(e)
		fmt.Printf("block %-12s %2d states, Nyholt M_eff %.2f, largest eigenvalues %.2f %.2f %.2f\n", b, m, nyholt(ser), e[m-1], e[m-2], e[m-3])
	}
}
