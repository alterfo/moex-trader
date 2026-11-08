package main

import (
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type futureSpec struct {
	SecID     string `json:"secid"`
	Lotsize   string `json:"lotsize"`
	Unit      string `json:"unit"`
	FaceUnit  string `json:"faceunit"`
	AssetCode string `json:"assetcode"`
}

type future struct {
	id     string
	mult   float64
	dates  []string
	at     map[string]int
	settle map[string]float64
	lst    string
}

type option struct {
	id     string
	strike float64
	typ    string
	expiry string
	wap    map[string]float64
	settle map[string]float64
	vol    map[string]float64
}

type trade struct {
	family   string
	strategy string
	dte      int
	expiry   string
	entry    string
	days     int
	pnlRUB   float64
	capital  float64
	iv       float64
	rv       float64
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func parseOptionID(id, prefix string) (float64, string, bool) {
	rest := strings.TrimPrefix(id, prefix)
	if rest == id {
		return 0, "", false
	}
	i := 0
	for i < len(rest) && (rest[i] >= '0' && rest[i] <= '9') {
		i++
	}
	if i < len(rest) && rest[i] == '.' {
		i++
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
	}
	if i == 0 || i >= len(rest) {
		return 0, "", false
	}
	strike, err := strconv.ParseFloat(rest[:i], 64)
	if err != nil {
		return 0, "", false
	}
	if rest[i] != 'B' || i+1 >= len(rest) {
		return 0, "", false
	}
	code := rest[i+1]
	switch {
	case code >= 'A' && code <= 'L':
		return strike, "C", true
	case code >= 'M' && code <= 'X':
		return strike, "P", true
	}
	return 0, "", false
}

func readCSV(path string) ([][]string, error) {
	if strings.HasSuffix(path, ".gz") {
		return readGzCSV(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}

func readGzCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return csv.NewReader(zr).ReadAll()
}

func loadSpecs(dir string) map[string]futureSpec {
	out := map[string]futureSpec{}
	b, err := os.ReadFile(filepath.Join(dir, "futures-meta.jsonl"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var s futureSpec
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			continue
		}
		out[s.SecID] = s
	}
	return out
}

func multiplier(s futureSpec, usdrub float64) float64 {
	l := parseFloat(s.Lotsize)
	if l == 0 {
		l = 1
	}
	lower := strings.ToLower(s.Unit)
	if strings.Contains(lower, "лот") || lower == "котировка" {
		l = 1
	}
	if strings.EqualFold(s.FaceUnit, "USD") || strings.Contains(lower, "доллар") || strings.Contains(lower, "usd") {
		return l * usdrub
	}
	return l
}

func loadUSDRUB(dir string) map[string]float64 {
	out := map[string]float64{}
	rows, err := readCSV(filepath.Join(dir, "usdrub.csv"))
	if err != nil {
		return out
	}
	for _, r := range rows[1:] {
		if len(r) < 2 {
			continue
		}
		out[r[0]] = parseFloat(r[1])
	}
	return out
}

func rateAt(rates map[string]float64, day string) float64 {
	if v, ok := rates[day]; ok && v > 0 {
		return v
	}
	best := ""
	for d, v := range rates {
		if d <= day && d > best && v > 0 {
			best = d
		}
	}
	if best == "" {
		return 0
	}
	return rates[best]
}

func loadFutures(dir, family string, specs map[string]futureSpec, rates map[string]float64) []*future {
	files, err := filepath.Glob(filepath.Join(dir, "futures", family+"*.csv.gz"))
	if err != nil {
		return nil
	}
	var out []*future
	for _, path := range files {
		id := strings.TrimSuffix(filepath.Base(path), ".csv.gz")
		if _, _, ok := parseOptionID(id, family); ok {
			continue
		}
		rows, err := readGzCSV(path)
		if err != nil || len(rows) < 2 {
			continue
		}
		f := &future{id: id, mult: 1, at: map[string]int{}, settle: map[string]float64{}}
		if spec, ok := specs[id]; ok {
			f.mult = multiplier(spec, rateAt(rates, rows[len(rows)-1][0]))
		}
		for _, r := range rows[1:] {
			if len(r) < 3 {
				continue
			}
			v := parseFloat(r[2])
			if v <= 0 {
				continue
			}
			f.settle[r[0]] = v
			f.dates = append(f.dates, r[0])
		}
		if len(f.dates) == 0 {
			continue
		}
		sort.Strings(f.dates)
		for i, d := range f.dates {
			f.at[d] = i
		}
		f.lst = f.dates[len(f.dates)-1]
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].lst < out[j].lst })
	return out
}

func loadOptions(dir, family string) map[string][]*option {
	files, err := filepath.Glob(filepath.Join(dir, "series", family+"*.csv.gz"))
	if err != nil {
		return nil
	}
	byExpiry := map[string][]*option{}
	for _, path := range files {
		id := strings.TrimSuffix(filepath.Base(path), ".csv.gz")
		strike, typ, ok := parseOptionID(id, family)
		if !ok {
			continue
		}
		rows, err := readGzCSV(path)
		if err != nil || len(rows) < 2 {
			continue
		}
		o := &option{id: id, strike: strike, typ: typ, wap: map[string]float64{}, settle: map[string]float64{}, vol: map[string]float64{}}
		var last string
		for _, r := range rows[1:] {
			if len(r) < 3 {
				continue
			}
			last = r[0]
			if v := parseFloat(r[1]); v > 0 {
				o.wap[r[0]] = v
			}
			if v := parseFloat(r[2]); v > 0 {
				o.settle[r[0]] = v
			}
			if v := parseFloat(r[3]); v > 0 {
				o.vol[r[0]] = v
			}
		}
		if last == "" {
			continue
		}
		o.expiry = last
		byExpiry[last] = append(byExpiry[last], o)
	}
	return byExpiry
}

func norm(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

func black76(f, k, sigma, t float64, call bool) float64 {
	if sigma <= 0 || t <= 0 {
		if call {
			return math.Max(f-k, 0)
		}
		return math.Max(k-f, 0)
	}
	d1 := (math.Log(f/k) + 0.5*sigma*sigma*t) / (sigma * math.Sqrt(t))
	d2 := d1 - sigma*math.Sqrt(t)
	if call {
		return f*norm(d1) - k*norm(d2)
	}
	return k*norm(-d2) - f*norm(-d1)
}

func delta(f, k, sigma, t float64, call bool) float64 {
	if sigma <= 0 || t <= 0 {
		if call {
			if f > k {
				return 1
			}
			return 0
		}
		if f < k {
			return -1
		}
		return 0
	}
	d1 := (math.Log(f/k) + 0.5*sigma*sigma*t) / (sigma * math.Sqrt(t))
	if call {
		return norm(d1)
	}
	return norm(d1) - 1
}

func impliedVol(price, f, k, t float64, call bool) (float64, bool) {
	intrinsic := math.Max(f-k, 0)
	if !call {
		intrinsic = math.Max(k-f, 0)
	}
	if price <= intrinsic*1.0001 || t <= 0 {
		return 0, false
	}
	lo, hi := 0.005, 5.0
	if black76(f, k, hi, t, call) < price {
		return 0, false
	}
	for i := 0; i < 100; i++ {
		mid := 0.5 * (lo + hi)
		if black76(f, k, mid, t, call) > price {
			hi = mid
		} else {
			lo = mid
		}
	}
	return 0.5 * (lo + hi), true
}

func neweyWest(xs []float64, lag int) (mean, t float64) {
	n := len(xs)
	if n < 3 {
		return 0, 0
	}
	mean = 0
	for _, x := range xs {
		mean += x
	}
	mean /= float64(n)
	dev := make([]float64, n)
	for i, x := range xs {
		dev[i] = x - mean
	}
	var v float64
	for _, d := range dev {
		v += d * d
	}
	v /= float64(n)
	for k := 1; k <= lag && k < n; k++ {
		var cov float64
		for i := k; i < n; i++ {
			cov += dev[i] * dev[i-k]
		}
		cov /= float64(n)
		v += 2 * (1 - float64(k)/float64(lag+1)) * cov
	}
	if v <= 0 {
		return mean, 0
	}
	return mean, mean / math.Sqrt(v/float64(n))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64{}, xs...)
	sort.Float64s(c)
	if len(c)%2 == 1 {
		return c[len(c)/2]
	}
	return 0.5 * (c[len(c)/2-1] + c[len(c)/2])
}

func cvar(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64{}, xs...)
	sort.Float64s(c)
	n := int(math.Ceil(q * float64(len(c))))
	if n < 1 {
		n = 1
	}
	var s float64
	for _, x := range c[:n] {
		s += x
	}
	return s / float64(n)
}

func stdev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var m float64
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	var v float64
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return math.Sqrt(v / float64(len(xs)-1))
}

func indexOf(dates []string, d string) int {
	for i, x := range dates {
		if x == d {
			return i
		}
	}
	return -1
}

type params struct {
	dir         string
	maxMoney    float64
	families    []string
	dtes        []int
	feeRUB      float64
	spreadFrac  float64
	futCostFrac float64
	marginFrac  float64
	usdrub      map[string]float64
	minVol      float64
	useSettle   bool
}

func straddlePremium(call, put *option, day string) (float64, bool) {
	cv, ok1 := call.wap[day]
	pv, ok2 := put.wap[day]
	if !ok1 || !ok2 {
		return 0, false
	}
	return cv + pv, true
}

func costsRUB(feeRUB, spreadFrac, premium, mult float64, legs int) float64 {
	return float64(legs)*feeRUB + float64(legs)*spreadFrac*premium*mult
}

func runVRP(p params) error {
	var rows [][]string
	rows = append(rows, []string{"family", "dte", "expiries", "iv_mean", "rv_mean", "vrp", "straddle_pct_f", "abs_move_pct_f", "skew25", "moneyness_pct", "dte_real", "net_pct_f"})
	fmt.Printf("\n%-6s %4s %8s %8s %8s %8s %10s %10s %8s %8s %8s %8s\n", "family", "dte", "expiries", "iv", "rv", "vrp", "straddle%", "move%", "skew25", "money%", "dte", "net%")
	for _, family := range p.families {
		specs := loadSpecs(p.dir)
		futures := loadFutures(p.dir, family, specs, p.usdrub)
		byExpiry := loadOptions(p.dir, family)
		if len(futures) == 0 || len(byExpiry) == 0 {
			fmt.Printf("%-6s no data (%d futures, %d expiries)\n", family, len(futures), len(byExpiry))
			continue
		}
		for _, dte := range p.dtes {
			var ivs, rvs, straddles, moves, skews, moneys, nets, dtesReal []float64
			for expiry, opts := range byExpiry {
				f := pickFuture(futures, expiry)
				if f == nil {
					continue
				}
				entry, atmCall, atmPut, ok := findEntry(f, opts, expiry, dte, 3, p.minVol, p.useSettle)
				if !ok {
					continue
				}
				fs := f.settle[entry]
				fe := f.settle[expiry]
				t := float64(daysBetween(entry, expiry)) / 365
				if t <= 0 {
					continue
				}
				money := math.Abs(atmCall.strike-fs) / fs
				if p.maxMoney > 0 && money > p.maxMoney {
					continue
				}
				civ, ok1 := impliedVol(atmCall.settle[entry], fs, atmCall.strike, t, true)
				piv, ok2 := impliedVol(atmPut.settle[entry], fs, atmPut.strike, t, false)
				if !ok1 || !ok2 {
					continue
				}
				ivs = append(ivs, 0.5*(civ+piv))
				rvs = append(rvs, realizedVol(f, entry, expiry))
				prem, ok := premiumAt(atmCall, atmPut, entry, p.useSettle)
				if !ok {
					continue
				}
				cost := costsRUB(p.feeRUB, p.spreadFrac, prem, f.mult, 2)
				net := (prem-math.Abs(fe-atmCall.strike))*f.mult - cost
				straddles = append(straddles, prem/fs*100)
				moves = append(moves, math.Abs(fe-fs)/fs*100)
				moneys = append(moneys, money*100)
				nets = append(nets, net/(fs*f.mult)*100)
				dtesReal = append(dtesReal, float64(daysBetween(entry, expiry)))
				sk := skew25(opts, entry, fs, t)
				if !math.IsNaN(sk) {
					skews = append(skews, sk)
				}
			}
			if len(ivs) == 0 {
				fmt.Printf("%-6s %4d no observations\n", family, dte)
				continue
			}
			mIV, mRV := mean(ivs), mean(rvs)
			mS, mM := mean(straddles), mean(moves)
			mK, mMon, mNet, mDte := mean(skews), mean(moneys), mean(nets), mean(dtesReal)
			fmt.Printf("%-6s %4d %8d %8.3f %8.3f %+8.3f %10.2f %10.2f %8.3f %8.2f %8.1f %+8.2f\n", family, dte, len(ivs), mIV, mRV, mIV-mRV, mS, mM, mK, mMon, mDte, mNet)
			rows = append(rows, []string{family, strconv.Itoa(dte), strconv.Itoa(len(ivs)),
				fmt.Sprintf("%.4f", mIV), fmt.Sprintf("%.4f", mRV), fmt.Sprintf("%+.4f", mIV-mRV),
				fmt.Sprintf("%.4f", mS), fmt.Sprintf("%.4f", mM), fmt.Sprintf("%.4f", mK),
				fmt.Sprintf("%.2f", mMon), fmt.Sprintf("%.1f", mDte), fmt.Sprintf("%+.3f", mNet)})
		}
	}
	return writeCSV(filepath.Join("artifacts", "options", "vrp.csv"), rows)
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func daysBetween(a, b string) int {
	ta, err1 := parseDay(a)
	tb, err2 := parseDay(b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

func pickFuture(futures []*future, expiry string) *future {
	for _, f := range futures {
		if f.lst >= expiry {
			if _, ok := f.settle[expiry]; ok {
				return f
			}
		}
	}
	return nil
}

func realizedVol(f *future, entry, expiry string) float64 {
	ei := f.at[entry]
	xi := f.at[expiry]
	if ei < 0 || xi <= ei+1 {
		return 0
	}
	var rets []float64
	prev := f.settle[f.dates[ei]]
	for i := ei + 1; i <= xi; i++ {
		cur := f.settle[f.dates[i]]
		if prev > 0 && cur > 0 {
			rets = append(rets, math.Log(cur/prev))
		}
		prev = cur
	}
	return stdev(rets) * math.Sqrt(252)
}

func atmPair(opts []*option, day string, forward float64) (*option, *option) {
	var bestCall, bestPut *option
	bestDist := math.Inf(1)
	calls := map[float64]*option{}
	puts := map[float64]*option{}
	for _, o := range opts {
		if _, ok := o.wap[day]; !ok {
			continue
		}
		if o.typ == "C" {
			calls[o.strike] = o
		} else {
			puts[o.strike] = o
		}
	}
	for k, c := range calls {
		if _, ok := puts[k]; !ok {
			continue
		}
		d := math.Abs(k - forward)
		if d < bestDist {
			bestDist = d
			bestCall = c
			bestPut = puts[k]
		}
	}
	return bestCall, bestPut
}

func skew25(opts []*option, entry string, forward, t float64) float64 {
	bestPut, bestCall := math.Inf(1), math.Inf(1)
	putIV, callIV := math.NaN(), math.NaN()
	for _, o := range opts {
		price, ok := o.settle[entry]
		if !ok {
			continue
		}
		iv, ok := impliedVol(price, forward, o.strike, t, o.typ == "C")
		if !ok {
			continue
		}
		d := delta(forward, o.strike, iv, t, o.typ == "C")
		if o.typ == "P" {
			if math.Abs(d+0.25) < bestPut {
				bestPut = math.Abs(d + 0.25)
				putIV = iv
			}
		} else if math.Abs(d-0.25) < bestCall {
			bestCall = math.Abs(d - 0.25)
			callIV = iv
		}
	}
	if math.IsNaN(putIV) || math.IsNaN(callIV) {
		return math.NaN()
	}
	return putIV - callIV
}

func writeCSV(path string, rows [][]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			return err
		}
	}
	return w.Error()
}

func main() {
	mode := flag.String("mode", "vrp", "vrp|battery|fwd|fwdsettle")
	dir := flag.String("dir", "data/options", "data directory")
	families := flag.String("families", "BR,GD,CR,GZ,LK,Eu", "option secid prefixes")
	dtes := flag.String("dte", "5,20", "days to expiry at entry")
	fee := flag.Float64("fee", 4.0, "exchange fee RUB per option contract per side")
	spread := flag.Float64("spread", 0.01, "half-spread as a fraction of premium per side")
	futCost := flag.Float64("futcost", 0.0002, "futures cost per side as a fraction of notional")
	margin := flag.Float64("margin", 0.15, "margin as a fraction of underlying notional")
	usdrub := flag.Float64("usdrub", 0, "fallback USD/RUB when the CBR file has no rate for a date")
	maxMoney := flag.Float64("maxmoney", 0.03, "skip an expiry when the nearest traded strike is further than this from the forward")
	minVol := flag.Float64("minvol", 5, "minimum contracts traded on each leg on the entry day")
	entry := flag.String("entry", "wap", "wap (require trades) or settle (read the settlement price, descriptive only)")
	split := flag.String("split", "2025-07-01", "date that splits the window into halves")
	fwdDate := flag.String("date", "", "snapshot date for fwd mode")
	fwdMin := flag.Int("fwdmin", 1, "minimum days to expiry for fwd mode")
	fwdMax := flag.Int("fwdmax", 10, "maximum days to expiry for fwd mode")
	fwdTarget := flag.Int("target", 5, "days to expiry that defines the forward entry")
	flag.Parse()
	p := params{
		dir:         *dir,
		families:    strings.Split(*families, ","),
		maxMoney:    *maxMoney,
		minVol:      *minVol,
		useSettle:   *entry == "settle",
		feeRUB:      *fee,
		spreadFrac:  *spread,
		futCostFrac: *futCost,
		marginFrac:  *margin,
		usdrub:      loadUSDRUB(*dir),
	}
	if *usdrub > 0 && len(p.usdrub) == 0 {
		p.usdrub["0000-00-00"] = *usdrub
	}
	for _, d := range strings.Split(*dtes, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(d))
		if err != nil {
			log.Fatal(err)
		}
		p.dtes = append(p.dtes, v)
	}
	splitDate = *split
	switch *mode {
	case "vrp":
		if err := runVRP(p); err != nil {
			log.Fatal(err)
		}
	case "fwd":
		date := *fwdDate
		if date == "" {
			date = timeNowDate()
		}
		if err := runForward(p, date, *fwdMin, *fwdMax); err != nil {
			log.Fatal(err)
		}
	case "fwdsettle":
		if err := runForwardSettle(p, *fwdTarget, p); err != nil {
			log.Fatal(err)
		}
	case "battery":
		if err := runBattery(p); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown mode %q", *mode)
	}
}

func timeNowDate() string {
	return time.Now().Format("2006-01-02")
}
