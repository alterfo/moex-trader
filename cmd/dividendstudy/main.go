package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/shopspring/decimal"
)

type dividendRecord struct {
	Ticker       string `json:"ticker"`
	Figi         string `json:"figi"`
	DeclaredDate string `json:"declared_date"`
	LastBuyDate  string `json:"last_buy_date"`
	PaymentDate  string `json:"payment_date"`
	DividendNet  string `json:"dividend_net"`
	Regularity   string `json:"regularity"`
}

type event struct {
	ticker   string
	lastBuy  time.Time
	divNet   float64
	wave     string
	season   int
	year     int
	entryIdx int
	exitIdx  int
}

var lotByTicker = map[string]int{
	"GAZP": 10, "GMKN": 10, "MTSS": 10, "RUAL": 10,
}

const (
	commission = 0.0005
	spread     = 0.0005
	slippage   = 0.0005
	targetNot  = 30000.0
	costPerLeg = commission + spread + slippage
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, calendarPath, outPath, fromStr, tillStr string
	var offsetsStr, lotsStr string
	var seed int64
	var repl int
	var taxRate float64
	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&calendarPath, "calendar", "data/dividends.jsonl", "dividend calendar JSONL")
	flag.StringVar(&outPath, "out", "/tmp/dividendstudy.md", "markdown report")
	flag.StringVar(&fromStr, "from", "2023-10-01", "candle fetch start YYYY-MM-DD")
	flag.StringVar(&tillStr, "till", "2027-03-01", "candle fetch end YYYY-MM-DD")
	flag.StringVar(&offsetsStr, "offsets", "-1,-2,-3,-5,-10", "entry offsets in trading days before LastBuyDate")
	flag.StringVar(&lotsStr, "lots", "", "comma-separated TICKER:LOT overrides (default GAZP/GMKN/MTSS/RUAL=10, rest 1)")
	flag.Float64Var(&taxRate, "tax", 0.13, "withholding tax on dividend received")
	flag.Int64Var(&seed, "seed", 42, "RNG seed for bootstrap")
	flag.IntVar(&repl, "repl", 2000, "bootstrap replicates")
	flag.Parse()

	for _, ov := range splitComma(lotsStr) {
		parts := strings.SplitN(ov, ":", 2)
		if len(parts) != 2 {
			continue
		}
		var lot int
		if _, err := fmt.Sscanf(parts[1], "%d", &lot); err == nil && lot > 0 {
			lotByTicker[strings.ToUpper(parts[0])] = lot
		}
	}

	offsets := make([]int, 0)
	for _, o := range splitComma(offsetsStr) {
		var v int
		if _, err := fmt.Sscanf(o, "%d", &v); err == nil {
			offsets = append(offsets, v)
		}
	}
	sort.Ints(offsets)

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return fmt.Errorf("parse -from: %w", err)
	}
	till, err := time.Parse("2006-01-02", tillStr)
	if err != nil {
		return fmt.Errorf("parse -till: %w", err)
	}

	records, err := loadCalendar(calendarPath)
	if err != nil {
		return err
	}
	studyStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	studyEnd := time.Date(2026, 12, 19, 0, 0, 0, 0, time.UTC)

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	fetchSource := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	ctx := context.Background()

	byTicker := make(map[string]time.Time) // ticker -> LastBuyDate, sanity
	dayIdx := make(map[string][]time.Time) // ticker -> trading-day list
	closeBy := make(map[string]map[time.Time]float64)
	openBy := make(map[string]map[time.Time]float64)

	for _, rec := range records {
		lb, err := time.Parse(time.RFC3339, rec.LastBuyDate)
		if err != nil {
			return fmt.Errorf("%s: parse last_buy_date: %w", rec.Ticker, err)
		}
		lb = lb.UTC()
		if lb.Before(studyStart) || lb.After(studyEnd) {
			continue
		}
		ticker := strings.ToUpper(rec.Ticker)
		byTicker[ticker] = lb
	}

	for _, ticker := range sortedKeys(byTicker) {
		candles, err := fetchSource.History(ctx, ticker, from, till)
		if err != nil {
			return fmt.Errorf("%s: history: %w", ticker, err)
		}
		candles = dropIncompleteTrailing(candles)
		days := make([]time.Time, 0, len(candles))
		closes := make(map[time.Time]float64)
		opens := make(map[time.Time]float64)
		for _, c := range candles {
			d := c.Begin.UTC()
			days = append(days, d)
			closes[d] = toFloat(c.Close)
			opens[d] = toFloat(c.Open)
		}
		sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
		dayIdx[ticker] = days
		closeBy[ticker] = closes
		openBy[ticker] = opens
	}

	imoexCandles, err := fetchSource.History(ctx, "IMOEX", from, till)
	if err != nil {
		return fmt.Errorf("imoex history: %w", err)
	}
	imoexCandles = dropIncompleteTrailing(imoexCandles)
	imoexClose := make(map[time.Time]float64)
	for _, c := range imoexCandles {
		imoexClose[c.Begin.UTC()] = toFloat(c.Close)
	}

	events := make([]event, 0)
	for _, rec := range records {
		lb, err := time.Parse(time.RFC3339, rec.LastBuyDate)
		if err != nil {
			continue
		}
		lb = lb.UTC()
		if lb.Before(studyStart) || lb.After(studyEnd) {
			continue
		}
		ticker := strings.ToUpper(rec.Ticker)
		divNet, err := parseFloat(rec.DividendNet)
		if err != nil || divNet <= 0 {
			continue
		}
		days := dayIdx[ticker]
		idx := -1
		for i, d := range days {
			if sameDay(d, lb) {
				idx = i
				break
			}
		}
		if idx < 0 || idx+1 >= len(days) {
			continue
		}
		events = append(events, event{
			ticker:   ticker,
			lastBuy:  lb,
			divNet:   divNet,
			wave:     waveOf(lb),
			season:   sixWeekOf(lb),
			year:     lb.Year(),
			entryIdx: idx,
			exitIdx:  idx + 1,
		})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].lastBuy.Equal(events[j].lastBuy) {
			return events[i].ticker < events[j].ticker
		}
		return events[i].lastBuy.Before(events[j].lastBuy)
	})

	var report strings.Builder
	writef(&report, "# Dividend drift event-study (2) — %s\n\n", time.Now().Format("2006-01-02"))
	writef(&report, "- Calendar: %s (%d records), study window 2024-01-01 -> 2026-12-19\n", calendarPath, len(records))
	writef(&report, "- Study events: %d (waves: %s)\n", len(events), waveSummary(events))
	writef(&report, "- Position: %0.f RUB notional, lot-quantized %v, tax %.0f%%, costs comm/spread/slip %.4f/%.4f/%.4f per leg\n\n",
		targetNot, lotByTicker, taxRate*100, commission, spread, slippage)

	for _, k := range offsets {
		rets, imoexRets, gaps := eventReturns(events, k, dayIdx, closeBy, openBy, imoexClose, taxRate)
		excess := make([]float64, len(rets))
		for i := range rets {
			excess[i] = rets[i] - imoexRets[i]
		}
		meanRaw := mean(rets)
		meanEx := mean(excess)
		ci := waveClusterCI(events, excess, repl, rand.New(rand.NewSource(seed)))
		looTicker := leaveOneOut(events, excess, func(e event) string { return e.ticker })
		looSeason := leaveOneOut(events, excess, func(e event) string { return fmt.Sprintf("%d-%d", e.year, e.season) })
		looYear := leaveOneOut(events, excess, func(e event) string { return fmt.Sprintf("%d", e.year) })
		looWave := leaveOneOut(events, excess, func(e event) string { return fmt.Sprintf("%d-%s", e.year, e.wave) })

		writef(&report, "## Entry offset %d (entry = close at LastBuyDate %+d TD, exit = ex-date close)\n\n", k, k)
		writef(&report, "- n=%d: mean raw %.6f, mean IMOEX-adjusted (excess) %.6f, mean gap ratio %.4f\n", len(rets), meanRaw, meanEx, mean(gaps))
		writef(&report, "- Wave-cluster bootstrap CI(mean excess): [%.6f, %.6f] %s\n", ci[0], ci[1], ciLabel(ci))
		writef(&report, "- Leave-one-out (mean excess > 0): ticker %v | season %v | year %v | wave %v\n\n",
			looTicker, looSeason, looYear, looWave)
	}

	coverWaves := distinctWaves(events)
	writef(&report, "Coverage: events=%d (>=30 %v), waves=%d (>=2 %v: %s), outside-largest-wave share=%.0f%% (>=20%% %v)\n",
		len(events), len(events) >= 30, len(coverWaves), len(coverWaves) >= 2, strings.Join(coverWaves, ", "),
		outsideLargestWaveShare(events)*100, outsideLargestWaveShare(events) >= 0.2)

	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report.String()), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		log.Printf("dividendstudy: report written to %s", outPath)
	} else {
		fmt.Print(report.String())
	}
	return nil
}

func eventReturns(events []event, k int, dayIdx map[string][]time.Time, closeBy, openBy map[string]map[time.Time]float64, imoexClose map[time.Time]float64, taxRate float64) (rets, imoexRets, gaps []float64) {
	for _, ev := range events {
		days := dayIdx[ev.ticker]
		idx := ev.entryIdx
		entryIdx := idx + k // k negative: k trading days BEFORE LastBuyDate
		exitIdx := ev.exitIdx
		if entryIdx < 0 || entryIdx >= len(days) || exitIdx >= len(days) {
			continue
		}
		entryDay := days[entryIdx]
		exitDay := days[exitIdx]
		entryClose := closeBy[ev.ticker][entryDay]
		exitClose := closeBy[ev.ticker][exitDay]
		if entryClose <= 0 || exitClose <= 0 {
			continue
		}
		lot := 1
		if l, ok := lotByTicker[ev.ticker]; ok && l > 0 {
			lot = l
		}
		numLots := int(targetNot / (entryClose * float64(lot)))
		if numLots < 1 {
			numLots = 1
		}
		buyGross := entryClose * float64(numLots*lot)
		sellGross := exitClose * float64(numLots*lot)
		divRecv := (1 - taxRate) * ev.divNet * float64(numLots*lot)
		signed := (sellGross - buyGross + divRecv - costPerLeg*buyGross - costPerLeg*sellGross) / buyGross
		rets = append(rets, signed)
		ie, ok := imoexClose[entryDay]
		ix, ok2 := imoexClose[exitDay]
		if ok && ok2 && ie > 0 {
			imoexRets = append(imoexRets, ix/ie-1)
		} else {
			imoexRets = append(imoexRets, 0)
		}
		prevClose := closeBy[ev.ticker][days[ev.entryIdx]]
		exOpen := openBy[ev.ticker][days[ev.exitIdx]]
		if exOpen > 0 && ev.divNet > 0 && prevClose > 0 {
			gaps = append(gaps, (exOpen-prevClose)/ev.divNet)
		}
	}
	return rets, imoexRets, gaps
}

// waveClusterCI resamples whole wave groups (cluster bootstrap over the wave
// sequence) — each draw picks waves with replacement; geometric block length
// in waves with mean 2.
func waveClusterCI(events []event, excess []float64, repl int, rng *rand.Rand) [2]float64 {
	byWave := make(map[string][]float64)
	key := make([]string, 0)
	for i, ev := range events {
		w := fmt.Sprintf("%d-%s", ev.year, ev.wave)
		if _, ok := byWave[w]; !ok {
			key = append(key, w)
		}
		byWave[w] = append(byWave[w], excess[i])
	}
	sort.Strings(key)
	means := make([]float64, 0, repl)
	for r := 0; r < repl; r++ {
		var picked []float64
		for total := 0; total < len(excess); {
			L := 1 + int(math.Log(1-rng.Float64())/math.Log(1-0.5)) // geometric mean 2
			for b := 0; b < L && total < len(excess); b++ {
				w := key[rng.Intn(len(key))]
				picked = append(picked, byWave[w]...)
				total += len(byWave[w])
			}
		}
		means = append(means, mean(picked))
	}
	sort.Float64s(means)
	return [2]float64{means[int(0.05*float64(repl))], means[int(0.95*float64(repl))]}
}

func leaveOneOut(events []event, excess []float64, key func(event) string) bool {
	groups := make(map[string][]float64)
	for i, ev := range events {
		g := key(ev)
		groups[g] = append(groups[g], excess[i])
	}
	for _, vals := range groups {
		total := sum(excess) - sum(vals)
		cnt := len(excess) - len(vals)
		if cnt <= 0 {
			continue
		}
		if total/float64(cnt) <= 0 {
			return false
		}
	}
	return true
}

func waveOf(t time.Time) string {
	switch t.Month() {
	case time.February, time.March, time.April, time.May:
		return "SPRING"
	case time.August, time.September, time.October, time.November:
		return "AUTUMN"
	}
	return "OTHER"
}

func sixWeekOf(t time.Time) int {
	return (int(t.YearDay())-1)/42 + 1
}

func loadCalendar(path string) ([]dividendRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read calendar: %w", err)
	}
	var out []dividendRecord
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec dividendRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("calendar line: %w", err)
		}
		out = append(out, rec)
	}
	return out, nil
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if q := strings.TrimSpace(p); q != "" {
			out = append(out, q)
		}
	}
	return out
}

func sortedKeys(m map[string]time.Time) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func parseFloat(s string) (float64, error) {
	var f float64
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%f", &f); err != nil {
		return 0, err
	}
	return f, nil
}

func waveSummary(events []event) string {
	cnt := make(map[string]int)
	for _, e := range events {
		cnt[e.wave]++
	}
	keys := make([]string, 0, len(cnt))
	for k := range cnt {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, cnt[k]))
	}
	return strings.Join(parts, ", ")
}

func distinctWaves(events []event) []string {
	m := make(map[string]bool)
	for _, e := range events {
		m[fmt.Sprintf("%d-%s", e.year, e.wave)] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func outsideLargestWaveShare(events []event) float64 {
	cnt := make(map[string]int)
	for _, e := range events {
		cnt[fmt.Sprintf("%d-%s", e.year, e.wave)]++
	}
	max := 0
	for _, c := range cnt {
		if c > max {
			max = c
		}
	}
	return 1 - float64(max)/float64(len(events))
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return sum(v) / float64(len(v))
}

func sum(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s
}

func ciLabel(ci [2]float64) string {
	if ci[0] > 0 {
		return "*"
	}
	return ""
}

func writef(b *strings.Builder, format string, args ...interface{}) {
	fmt.Fprintf(b, format, args...)
}

func dropIncompleteTrailing(candles []moex.Candle) []moex.Candle {
	if len(candles) == 0 {
		return candles
	}
	last := candles[len(candles)-1]
	now := time.Now().UTC()
	if !last.Begin.IsZero() {
		ly, lm, ld := last.Begin.Date()
		ny, nm, nd := now.Date()
		if ly == ny && lm == nm && ld == nd {
			return candles[:len(candles)-1]
		}
	}
	return candles
}

func toFloat(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}
