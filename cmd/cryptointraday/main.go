package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/olegsidorkin/moex-trader/internal/cryptocost"
)

type options struct {
	mode      string
	barDir    string
	dailyDir  string
	session   string
	top       int
	quantile  float64
	out       string
	maxHold   int
	makerFill float64
}

func main() {
	var o options
	flag.StringVar(&o.mode, "mode", "sessions", "sessions | mom | orb")
	flag.StringVar(&o.barDir, "bars", filepath.Join("data", "crypto", "binance-um", "klines5m"), "5m bar directory")
	flag.StringVar(&o.dailyDir, "daily", filepath.Join("data", "crypto", "binance-um"), "daily panel directory")
	flag.StringVar(&o.session, "session", "s0", "s0 (UTC day) | s1 (US cash)")
	flag.IntVar(&o.top, "top", 100, "universe size")
	flag.Float64Var(&o.quantile, "quantile", 0.2, "long and short quantile for O-MOM")
	flag.StringVar(&o.out, "out", "", "sessions output CSV")
	flag.IntVar(&o.maxHold, "max-hold", 360, "maximum hold in minutes for O-ORB")
	flag.Float64Var(&o.makerFill, "maker-fill", 0.55, "fill probability of a limit entry at the range boundary")
	flag.Parse()

	w, ok := windowByName(o.session)
	if !ok {
		log.Fatalf("unknown session %q", o.session)
	}
	contracts, err := listContracts(o.barDir)
	if err != nil {
		log.Fatalf("bars: %v", err)
	}
	if len(contracts) == 0 {
		log.Fatalf("no contracts in %s", o.barDir)
	}
	out := o.out
	if out == "" {
		out = filepath.Join("artifacts", "crypto", fmt.Sprintf("sessions-%s.csv", w.name))
	}
	switch o.mode {
	case "sessions":
		if err := runSessions(o, w, contracts, o.barDir, out); err != nil {
			log.Fatalf("sessions: %v", err)
		}
	case "mom":
		rows, err := readSessions(out)
		if err != nil {
			log.Fatalf("read sessions %s: %v", out, err)
		}
		if err := runMom(o, w, rows); err != nil {
			log.Fatalf("mom: %v", err)
		}
	case "orb":
		rows, err := readSessions(out)
		if err != nil {
			log.Fatalf("read sessions %s: %v", out, err)
		}
		if err := runORB(o, w, contracts, rows); err != nil {
			log.Fatalf("orb: %v", err)
		}
	default:
		log.Fatalf("unknown mode %q", o.mode)
	}
}

func barFilePath(dir, sym string) string {
	return filepath.Join(dir, sym+".csv")
}

var costTiers = []struct {
	minVol     float64
	halfSpread float64
	impact     float64
}{
	{1e9, 0.5, 1.0},
	{1e8, 1.5, 2.0},
	{1e7, 4.0, 4.0},
	{0, 10.0, 10.0},
}

func tierCost(vol float64) cryptocost.Config {
	t := costTiers[len(costTiers)-1]
	for _, c := range costTiers {
		if vol >= c.minVol {
			t = c
			break
		}
	}
	return cryptocost.Config{
		Venue:         cryptocost.Binance(),
		Instrument:    cryptocost.Perp,
		Exec:          cryptocost.Taker,
		HalfSpreadBps: t.halfSpread,
		ImpactBps:     t.impact,
	}
}

func allInCostBps(o options, vol float64) float64 {
	return tierCost(vol).AllInBpsPerSide()
}

func volumeThreshold(vol dailyVol, date string, names int) float64 {
	var vals []float64
	for _, m := range vol {
		if v, ok := m[date]; ok && v > 0 {
			vals = append(vals, v)
		}
	}
	if len(vals) < 10 {
		return math.Inf(1)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(vals)))
	decile := len(vals) / 10
	if decile < 1 {
		decile = 1
	}
	if names > 0 && decile > names {
		decile = names
	}
	return vals[decile-1]
}

type tradeRec struct {
	symbol string
	date   string
	dir    float64
	entry  float64
	exit   float64
	gross  float64
	cost   float64
	fund   float64
	net    float64
	hold   int
}

func runORB(o options, w sessionWindow, contracts []string, sessions []*sessionRec) error {
	vol := loadDailyVolumeOrDie(o.dailyDir, contracts)
	fund := loadDailyFunding(o.dailyDir, contracts)
	rangeQ := map[string]map[string]float64{}
	for _, r := range sessions {
		m, ok := rangeQ[r.symbol]
		if !ok {
			m = map[string]float64{}
			rangeQ[r.symbol] = m
		}
		m[r.date] = r.qvol
	}
	medians := map[string]map[string]float64{}
	for sym, m := range rangeQ {
		var dates []string
		for d := range m {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		byDate := map[string]float64{}
		for i, d := range dates {
			start := i - 20
			if start < 0 {
				start = 0
			}
			vals := make([]float64, 0, i-start+1)
			for j := start; j < i; j++ {
				vals = append(vals, m[dates[j]])
			}
			if len(vals) < 10 {
				continue
			}
			sort.Float64s(vals)
			byDate[d] = vals[len(vals)/2]
		}
		medians[sym] = byDate
	}
	thresholds := map[string]float64{}
	for _, r := range sessions {
		if _, ok := thresholds[r.date]; !ok {
			thresholds[r.date] = volumeThreshold(vol, r.date, o.top)
		}
	}
	var trades []tradeRec
	for _, sym := range contracts {
		path := barFilePath(o.barDir, sym)
		var inRange bool
		var rangeHigh, rangeLow float64
		var posDir float64
		var entry float64
		var entryT int64
		var stop float64
		var day string
		var traded bool
		reset := func(d string) {
			day = d
			inRange = true
			rangeHigh = 0
			rangeLow = 0
			posDir = 0
			entry = 0
			stop = 0
			traded = false
		}
		closePosition := func(price float64, ts int64) {
			if posDir == 0 {
				return
			}
			ret := posDir * (price/entry - 1) * 1e4
			dailyVol := prevVolume(vol, sym, day)
			c := allInCostBps(o, dailyVol)
			f := fundAt(fund, sym, day, w) * posDir
			trades = append(trades, tradeRec{
				symbol: sym, date: day, dir: posDir, entry: entry, exit: price,
				gross: ret, cost: 2 * c, fund: -f, net: ret - 2*c - f,
				hold: int((ts - entryT) / 60000),
			})
			posDir = 0
			entry = 0
			stop = 0
		}
		err := streamBars(path, func(b bar) bool {
			m := minuteOfDay(b.t)
			if m < w.startMin || m > w.endMin {
				if posDir != 0 {
					closePosition(b.open, b.t)
				}
				return true
			}
			d := dateOf(b.t)
			if d != day {
				closePosition(b.open, b.t)
				reset(d)
			}
			if inRange {
				if m < w.startMin+w.lenMin {
					if rangeHigh == 0 {
						rangeHigh = b.high
						rangeLow = b.low
					} else {
						rangeHigh = math.Max(rangeHigh, b.high)
						rangeLow = math.Min(rangeLow, b.low)
					}
					return true
				}
				inRange = false
			}
			if posDir != 0 {
				if posDir > 0 && b.low <= stop {
					price := stop
					if b.open < stop {
						price = b.open
					}
					closePosition(price, b.t)
					return true
				}
				if posDir < 0 && b.high >= stop {
					price := stop
					if b.open > stop {
						price = b.open
					}
					closePosition(price, b.t)
					return true
				}
				if int64((b.t-entryT)/60000) >= int64(o.maxHold) {
					closePosition(b.close, b.t)
					return true
				}
				return true
			}
			if traded {
				return true
			}
			roomsVol := prevVolume(vol, sym, day)
			if thresholds[day] > 0 && roomsVol < thresholds[day] {
				return true
			}
			med, ok := medians[sym][day]
			rq, ok2 := rangeQ[sym][day]
			if !ok || !ok2 || med <= 0 || rq < med {
				return true
			}
			if rangeHigh <= 0 || rangeLow <= 0 || rangeHigh == rangeLow {
				return true
			}
			if b.close > rangeHigh {
				posDir = 1
				entry = b.close
				stop = rangeLow
			} else if b.close < rangeLow {
				posDir = -1
				entry = b.close
				stop = rangeHigh
			} else {
				return true
			}
			entryT = b.t
			traded = true
			return true
		})
		if err != nil {
			continue
		}
	}
	if len(trades) == 0 {
		return fmt.Errorf("orb %s: no trades", w.name)
	}
	reportORB(w, trades)
	return nil
}

func reportORB(w sessionWindow, trades []tradeRec) {
	byDate := map[string][]tradeRec{}
	var dates []string
	for _, t := range trades {
		if _, ok := byDate[t.date]; !ok {
			dates = append(dates, t.date)
		}
		byDate[t.date] = append(byDate[t.date], t)
	}
	sort.Strings(dates)
	var daily []float64
	for _, d := range dates {
		ts := byDate[d]
		s := 0.0
		for _, t := range ts {
			s += t.net
		}
		daily = append(daily, s/float64(len(ts)))
	}
	var nets []float64
	var holds []float64
	wins := 0
	for _, t := range trades {
		nets = append(nets, t.net)
		holds = append(holds, float64(t.hold))
		if t.net > 0 {
			wins++
		}
	}
	fmt.Printf("cell O-ORB %s: trades %d over %d days, mean net %.1f bps/trade, t(trade) %.2f, hit %.1f%%, hold %.0f min\n",
		w.name, len(trades), len(dates), mean(nets), nwT(nets, 5), 100*float64(wins)/float64(len(trades)), mean(holds))
	fmt.Printf("  mean gross %.1f, cost %.1f, funding %.1f bps/trade; daily book mean %.2f bps/day, t %.2f, sharpe %.2f\n",
		meanField(trades, func(t tradeRec) float64 { return t.gross }),
		meanField(trades, func(t tradeRec) float64 { return t.cost }),
		meanField(trades, func(t tradeRec) float64 { return t.fund }),
		mean(daily), nwT(daily, 5), sharpe(daily))
	if len(daily) >= 2 {
		fmt.Printf("  halves %.2f / %.2f bps/day; trades/day %.1f\n", mean(daily[:len(daily)/2]), mean(daily[len(daily)/2:]), float64(len(trades))/float64(len(dates)))
	}
}

func meanField(trades []tradeRec, f func(tradeRec) float64) float64 {
	if len(trades) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, t := range trades {
		s += f(t)
	}
	return s / float64(len(trades))
}

func loadDailyVolumeOrDie(dir string, symbols []string) dailyVol {
	vol, err := loadDailyVolume(filepath.Join(dir, "klines1d"), symbols)
	if err != nil {
		log.Fatalf("daily volume: %v", err)
	}
	return vol
}

type csvWriterT struct {
	w   *csv.Writer
	err error
}

func csvWriter(f *os.File) *csvWriterT {
	return &csvWriterT{w: csv.NewWriter(f)}
}

func (c *csvWriterT) write(row []string) {
	if c.err != nil {
		return
	}
	c.err = c.w.Write(row)
}

func (c *csvWriterT) done() error {
	c.w.Flush()
	if c.err != nil {
		return c.err
	}
	return c.w.Error()
}

func createFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.Create(path)
}

func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	if _, err := r.Read(); err != nil {
		return nil, err
	}
	var out [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func fnum(v float64) string {
	return fmt.Sprintf("%.10g", v)
}
