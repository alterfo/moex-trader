package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/betaregime"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/equalweight"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/momentum"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tickersStr, fromStr, tillStr, ensemblePath, outPath string
	var depositStr, commissionStr, spreadStr, slippageStr, borrowPctDayStr, targetNotionalStr, sharesStr string
	var blocksStr string
	var k, rebalanceEvery, maxLots, bootstrapRepl int
	var blockSeed int64

	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&fromStr, "from", "2025-04-01", "window start YYYY-MM-DD")
	flag.StringVar(&tillStr, "till", "2026-09-17", "window end YYYY-MM-DD")
	flag.StringVar(&ensemblePath, "ensemble-path", "", "ensemble model JSON (default: model.ensemble_path from config)")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: stdout)")
	flag.StringVar(&depositStr, "deposit", "1000000", "starting deposit in RUB")
	flag.StringVar(&commissionStr, "commission-rate", "0.0005", "commission rate per fill")
	flag.StringVar(&spreadStr, "spread-pct", "0.0005", "half-spread cost per fill as fraction of price")
	flag.StringVar(&slippageStr, "slippage-pct", "0.0005", "slippage cost per fill as fraction of price")
	flag.StringVar(&borrowPctDayStr, "borrow-pct-day", "0", "short-borrow cost per day as fraction of short-leg notional (e.g. 0.00005 = 0.005%)")
	flag.StringVar(&targetNotionalStr, "target-notional", "15000", "target ruble notional per position")
	flag.StringVar(&sharesStr, "shares", "0,0.5,1", "overlay shares of net exposure to test")
	flag.StringVar(&blocksStr, "blocks", "5,10,20,40,60", "mean geometric block lengths (trading days)")
	flag.IntVar(&k, "k", 5, "top-k (and bottom-k for long+short) momentum selection")
	flag.IntVar(&rebalanceEvery, "rebalance-every", 10, "rebalance every N trading days")
	flag.IntVar(&maxLots, "max-lots", 1000, "risk-gate max lots ceiling")
	flag.IntVar(&bootstrapRepl, "bootstrap-repl", 2000, "bootstrap replicates")
	flag.Int64Var(&blockSeed, "block-seed", 42, "RNG seed for the stationary bootstrap")
	flag.Parse()

	if bootstrapRepl <= 0 {
		return fmt.Errorf("-bootstrap-repl must be positive")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if len(tickers) == 0 {
		return fmt.Errorf("no tickers configured")
	}
	if ensemblePath == "" {
		ensemblePath = cfg.Model.EnsemblePath
	}
	if ensemblePath == "" {
		ensemblePath = "ensemble_model.json"
	}

	deposit, err := decimal.NewFromString(depositStr)
	if err != nil {
		return fmt.Errorf("parse -deposit: %w", err)
	}
	commissionRate, err := decimal.NewFromString(commissionStr)
	if err != nil {
		return fmt.Errorf("parse -commission-rate: %w", err)
	}
	spreadPct, err := decimal.NewFromString(spreadStr)
	if err != nil {
		return fmt.Errorf("parse -spread-pct: %w", err)
	}
	slippagePct, err := decimal.NewFromString(slippageStr)
	if err != nil {
		return fmt.Errorf("parse -slippage-pct: %w", err)
	}
	borrowPctPerDay, err := decimal.NewFromString(borrowPctDayStr)
	if err != nil {
		return fmt.Errorf("parse -borrow-pct-day: %w", err)
	}
	if borrowPctPerDay.IsNegative() {
		return fmt.Errorf("-borrow-pct-day must be non-negative")
	}
	targetNotional, err := decimal.NewFromString(targetNotionalStr)
	if err != nil {
		return fmt.Errorf("parse -target-notional: %w", err)
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return fmt.Errorf("parse -from: %w", err)
	}
	till, err := time.Parse("2006-01-02", tillStr)
	if err != nil {
		return fmt.Errorf("parse -till: %w", err)
	}
	windows := defaultWindows()

	shares, err := parseFloats(sharesStr)
	if err != nil {
		return fmt.Errorf("parse -shares: %w", err)
	}
	blocks, err := parseFloats(blocksStr)
	if err != nil {
		return fmt.Errorf("parse -blocks: %w", err)
	}

	ens, err := model.LoadEnsembleModel(ensemblePath)
	if err != nil {
		return fmt.Errorf("load ensemble: %w", err)
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	fetchSource := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)
	warmupDays := features.PriceFeatureConfig{}.FetchCalendarDays()
	const featureWarmup = 64

	ctx := context.Background()
	mem := &memorySource{candles: make(map[string][]moex.Candle)}
	values := make(map[string]map[time.Time]float64)
	dateSet := make(map[time.Time]struct{})
	fetchFrom := from.AddDate(0, 0, -warmupDays)

	for _, raw := range tickers {
		ticker := strings.ToUpper(strings.TrimSpace(raw))
		if ticker == "" {
			continue
		}
		candles, err := fetchSource.History(ctx, ticker, fetchFrom, till)
		if err != nil {
			return fmt.Errorf("%s: history: %w", ticker, err)
		}
		candles = dropIncompleteTrailing(candles)
		if len(candles) < featureWarmup+1 {
			return fmt.Errorf("%s: only %d candles, need %d", ticker, len(candles), featureWarmup+1)
		}
		mem.candles[ticker] = candles

		builder := features.NewBuilderWithConfig(time.Now, features.PriceFeatureConfig{})
		byDate := make(map[time.Time]float64)
		for d := featureWarmup; d < len(candles); d++ {
			day := candles[d].Begin
			if day.Before(from) {
				continue
			}
			input := features.Input{
				Ticker: ticker,
				Price: features.PriceSnapshot{
					LastPrice: candles[d-1].Close,
					PrevClose: candles[d-2].Close,
					AsOf:      day,
				},
				Candles: candles[:d],
			}
			feature, err := builder.Build(input)
			if err != nil {
				continue
			}
			value, _ := feature.Mom21d.Float64()
			byDate[day] = value
			dateSet[day] = struct{}{}
		}
		values[ticker] = byDate
	}

	imoexCandles, err := fetchSource.History(ctx, "IMOEX", fetchFrom, till)
	if err != nil {
		return fmt.Errorf("imoex history: %w", err)
	}
	imoexCandles = dropIncompleteTrailing(imoexCandles)

	dates := make([]time.Time, 0, len(dateSet))
	for date := range dateSet {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	engineConfig := func(signalSource backtest.SignalSource) backtest.Config {
		return backtest.Config{
			Tickers:        tickers,
			From:           from,
			Till:           till,
			Deposit:        deposit,
			MaxLots:        maxLots,
			CommissionRate: commissionRate,
			SpreadPct:      spreadPct,
			SlippagePct:    slippagePct,
			BorrowPctPerDay: borrowPctPerDay,
			WarmupDays:     warmupDays,
			KillSwitch:     true,
			SignalSource:   signalSource,
			Source:         mem,
			FeatureConfig:  features.PriceFeatureConfig{},
		}
	}

	ensResult, err := runEngine(ctx, engineConfig(&model.EnsembleSignalSource{
		Model:          ens,
		MaxLots:        maxLots,
		TargetNotional: targetNotional,
	}))
	if err != nil {
		return fmt.Errorf("ensemble backtest: %w", err)
	}

	equalResult, err := runEngine(ctx, engineConfig(&equalweight.SignalSource{
		TargetNotional: targetNotional,
		MaxLots:        maxLots,
	}))
	if err != nil {
		return fmt.Errorf("equal-weight backtest: %w", err)
	}

	momentumPlan, err := momentum.BuildPlan(values, dates, k, rebalanceEvery, momentum.VariantLongShort)
	if err != nil {
		return fmt.Errorf("build momentum plan: %w", err)
	}
	momentumResult, err := runEngine(ctx, engineConfig(&momentum.SignalSource{
		Plan:          momentumPlan,
		TargetNotional: targetNotional,
		MaxLots:       maxLots,
	}))
	if err != nil {
		return fmt.Errorf("momentum backtest: %w", err)
	}

	benchReturns := betaregime.DailyReturnsFromCurve(equalResult.EquityCurve)
	momentumReturns := betaregime.DailyReturnsFromCurve(momentumResult.EquityCurve)

	var report strings.Builder
	writef(&report, "# Alpha-without-beta hedge test (0)\n\n")
	writef(&report, "- Window: %s -> %s\n", from.Format("2006-01-02"), till.Format("2006-01-02"))
	writef(&report, "- Tickers: %s\n", strings.Join(tickers, ", "))
	writef(&report, "- Deposit %s, target notional %s, costs comm/spread/slip %s/%s/%s, borrow %s/day\n",
		deposit.String(), targetNotional.String(), commissionRate.String(), spreadPct.String(), slippagePct.String(), borrowPctPerDay.String())
	writef(&report, "- Ensemble realized / unrealized: %.2f / %.2f RUB, closed trades %d, total borrow %.2f RUB, realized net of borrow %.2f RUB\n",
		toFloat(ensResult.RealizedPnl), toFloat(ensResult.UnrealizedPnl), ensResult.ClosedTrades, toFloat(ensResult.TotalBorrow), toFloat(ensResult.RealizedPnlNetBorrow))

	beta, err := perTickerBeta(mem, tickers, from, till, imoexCandles)
	if err != nil {
		return fmt.Errorf("per-ticker beta: %w", err)
	}
	writef(&report, "- Per-name beta (log-return OLS vs IMOEX, window): %s\n\n", formatBeta(beta))

	exposureByDay := dailyExposure(mem, ensResult, tickers, beta, till)
	imoexDaily := betaregime.DailyReturnsFromCandles(imoexCandles)

	for _, share := range shares {
		adj, legTotal, legCost := overlayCurve(ensResult.EquityCurve, exposureByDay, imoexDaily, share, commissionRate, spreadPct)
		adjReturns := betaregime.DailyReturnsFromCurve(adj)
		dailyY, dailyX, dailyClusters, _, skipped := betaregime.AlignDaily(adjReturns, benchReturns, momentumReturns, windows)
		reg, err := betaregime.FitClusterOLS(dailyY, dailyX, dailyClusters)
		if err != nil {
			return fmt.Errorf("share %v: daily regression: %w", share, err)
		}
		residual := make([]float64, len(dailyY))
		for i := range dailyY {
			residual[i] = dailyY[i] - reg.Coef[1]*dailyX[i][0] - reg.Coef[2]*dailyX[i][1]
		}

		boot := bootstrapAlpha(residual, blocks, bootstrapRepl, blockSeed)

		writef(&report, "## Overlay share %.2f\n\n", share)
		writef(&report, "- Leg P&L: %.2f RUB, leg costs: %.2f RUB\n", legTotal, legCost)
		writef(&report, "- Daily regression (cluster by quarter): N=%d, skipped=%d, R2=%.4f\n", reg.Observations, skipped, reg.R2)
		writef(&report, "- alpha=%.6f (cluster t=%.3f, p=%.4f), beta1(eqw)=%.4f, beta2(mom)=%.4f\n",
			reg.Coef[0], reg.TStat[0], reg.PValue[0], reg.Coef[1], reg.Coef[2])
		writef(&report, "- Block bootstrap CI(alpha): %s\n\n", formatBootstrap(boot))

		writef(&report, "| quarter | n | mean daily alpha |\n|---|---|---|\n")
		for clusterIdx, w := range windows {
			vals := make([]float64, 0)
			for i, c := range dailyClusters {
				if c == clusterIdx {
					vals = append(vals, residual[i])
				}
			}
			writef(&report, "| %s -> %s | %d | %.6f |\n",
				w.From.Format("2006-01-02"), w.Till.Format("2006-01-02"), len(vals), mean(vals))
		}
		writef(&report, "\n| quarter | max DD %% |\n|---|---|\n")
		for _, w := range windows {
			writef(&report, "| %s -> %s | %.4f |\n",
				w.From.Format("2006-01-02"), w.Till.Format("2006-01-02"), maxDrawdownPctWithin(adj, w.From, w.Till))
		}
		writef(&report, "\nLeave-one-quarter-out alpha: %s\n\n", formatLeaveOneOut(dailyY, dailyX, dailyClusters, windows))
	}

	rep := report.String()
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(rep), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		log.Printf("alphahedge: report written to %s", outPath)
	} else {
		fmt.Print(rep)
	}
	return nil
}

func formatLeaveOneOut(y []float64, x [][]float64, clusters []int, windows []betaregime.Window) string {
	out := make([]string, 0, len(windows))
	for windowIdx := range windows {
		yy := make([]float64, 0, len(y))
		xx := make([][]float64, 0, len(y))
		cc := make([]int, 0, len(y))
		for i, c := range clusters {
			if c == windowIdx {
				continue
			}
			yy = append(yy, y[i])
			xx = append(xx, x[i])
			cc = append(cc, c)
		}
		reg, err := betaregime.FitClusterOLS(yy, xx, cc)
		if err != nil {
			out = append(out, fmt.Sprintf("drop %s: err", windows[windowIdx].From.Format("2006-01-02")))
			continue
		}
		out = append(out, fmt.Sprintf("drop %s: %.6f", windows[windowIdx].From.Format("2006-01-02"), reg.Coef[0]))
	}
	return strings.Join(out, "; ")
}

type seg struct {
	signedLots int
	from       time.Time
	till       time.Time
	beta       float64
}

func maxDrawdownPctWithin(points []backtest.EquityPoint, from, till time.Time) float64 {
	pts := append([]backtest.EquityPoint(nil), points...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date.Before(pts[j].Date) })
	var peak decimal.Decimal
	var worst float64
	for i := range pts {
		if pts[i].Date.Before(from) || pts[i].Date.After(till) {
			continue
		}
		if peak.IsZero() {
			peak = pts[i].Equity
		}
		if pts[i].Equity.GreaterThan(peak) {
			peak = pts[i].Equity
		}
		if !peak.IsZero() {
			dd := peak.Sub(pts[i].Equity).Div(peak).Mul(decimal.NewFromInt(100))
			if v, _ := dd.Float64(); v > worst {
				worst = v
			}
		}
	}
	return worst
}

func dailyExposure(mem *memorySource, res *backtest.Result, tickers []string, beta map[string]float64, till time.Time) map[time.Time]float64 {
	segs := make(map[string][]seg)
	for _, t := range res.Trades {
		sign := 1
		if t.Action == domain.ActionSell {
			sign = -1
		}
		key := strings.ToUpper(strings.TrimSpace(t.Ticker))
		segs[key] = append(segs[key], seg{signedLots: sign * t.Lots, from: t.OpenedAt, till: t.ClosedAt, beta: beta[key]})
	}
	for _, p := range res.OpenPositions {
		sign := 1
		if p.Side == domain.ActionSell {
			sign = -1
		}
		key := strings.ToUpper(strings.TrimSpace(p.Ticker))
		segs[key] = append(segs[key], seg{signedLots: sign * p.Lots, from: p.OpenedAt, till: till, beta: beta[key]})
	}

	out := make(map[time.Time]float64)
	for _, ticker := range tickers {
		key := strings.ToUpper(strings.TrimSpace(ticker))
		candles := mem.candles[key]
		for _, s := range segs[key] {
			for _, c := range candles {
				if c.Begin.Before(s.from) {
					continue
				}
				if c.Begin.After(s.till) {
					break
				}
				out[c.Begin] += float64(s.signedLots) * toFloat(c.Close) * s.beta
			}
		}
	}
	return out
}

func exposureOn(exposure map[time.Time]float64, day time.Time, prev float64) float64 {
	if v, ok := exposure[day]; ok {
		return v
	}
	return prev
}

func overlayCurve(curve []backtest.EquityPoint, exposure map[time.Time]float64, imoexDaily map[time.Time]float64, share float64, comm, spread decimal.Decimal) ([]backtest.EquityPoint, float64, float64) {
	if share == 0 {
		return curve, 0, 0
	}
	sorted := append([]backtest.EquityPoint(nil), curve...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })
	cost := comm.Add(spread).Add(decimal.Zero)
	out := make([]backtest.EquityPoint, len(sorted))
	cumLeg := 0.0
	cumCost := 0.0
	prevExposure := 0.0
	for i, p := range sorted {
		if i > 0 {
			exp := exposureOn(exposure, sorted[i-1].Date, prevExposure)
			legReturn := imoexDaily[p.Date]
			avgDelta := exposureOn(exposure, p.Date, exp) - exp
			if avgDelta < 0 {
				avgDelta = -avgDelta
			}
			cumLeg += -share * exp * legReturn
			cumCost += share * avgDelta * toFloat(cost)
		}
		out[i] = backtest.EquityPoint{Date: p.Date, Equity: p.Equity.Add(decimal.NewFromFloat(cumLeg - cumCost))}
		prevExposure = exposureOn(exposure, p.Date, prevExposure)
	}
	return out, cumLeg - cumCost, cumCost
}

func perTickerBeta(mem *memorySource, tickers []string, from, till time.Time, imoexCandles []moex.Candle) (map[string]float64, error) {
	imoexLog := make(map[time.Time]float64)
	imoexDays := make([]time.Time, 0)
	var prevClose float64
	for _, c := range imoexCandles {
		cl := toFloat(c.Close)
		if prevClose > 0 && !c.Begin.Before(from) && !c.Begin.After(till) {
			imoexLog[c.Begin] = math.Log(cl / prevClose)
			imoexDays = append(imoexDays, c.Begin)
		}
		prevClose = cl
	}

	out := make(map[string]float64)
	for _, ticker := range tickers {
		key := strings.ToUpper(strings.TrimSpace(ticker))
		var x, y []float64
		var prevClose float64
		for _, c := range mem.candles[key] {
			cl := toFloat(c.Close)
			if prevClose > 0 {
				if imoexVal, ok := imoexLog[c.Begin]; ok {
					x = append(x, imoexVal)
					y = append(y, math.Log(cl/prevClose))
				}
			}
			prevClose = cl
		}
		out[key] = olsSlope(x, y)
	}
	return out, nil
}

func olsSlope(x, y []float64) float64 {
	n := len(x)
	if n < 2 {
		return 0
	}
	xBar, yBar := 0.0, 0.0
	for i := 0; i < n; i++ {
		xBar += x[i]
		yBar += y[i]
	}
	xBar /= float64(n)
	yBar /= float64(n)
	var cov, vx float64
	for i := 0; i < n; i++ {
		cov += (x[i] - xBar) * (y[i] - yBar)
		vx += (x[i] - xBar) * (x[i] - xBar)
	}
	if vx == 0 {
		return 0
	}
	return cov / vx
}

func bootstrapAlpha(residual []float64, blocks []float64, repl int, seed int64) []bootstrapCI {
	out := make([]bootstrapCI, 0, len(blocks))
	rng := rand.New(rand.NewSource(seed))
	n := len(residual)
	for _, m := range blocks {
		if n == 0 {
			out = append(out, bootstrapCI{Block: m})
			continue
		}
		p := 1.0 / m
		means := make([]float64, repl)
		for r := 0; r < repl; r++ {
			idx := rng.Intn(n)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += residual[idx]
				idx = (idx + 1) % n
				if rng.Float64() < p {
					idx = rng.Intn(n)
				}
			}
			means[r] = acc / float64(n)
		}
		sort.Float64s(means)
		out = append(out, bootstrapCI{
			Block: m,
			Lo:    means[int(float64(repl)*0.025)],
			Hi:    means[int(float64(repl)*0.975)],
		})
	}
	return out
}

type bootstrapCI struct {
	Block float64
	Lo    float64
	Hi    float64
}

func formatBootstrap(boot []bootstrapCI) string {
	out := make([]string, 0, len(boot))
	has20, has40 := false, false
	for _, ci := range boot {
		b := 0
		if ci.Hi < 0 || ci.Lo > 0 {
			b = 1
		}
		out = append(out, fmt.Sprintf("L=%g: [%.6f, %.6f]%s", ci.Block, ci.Lo, ci.Hi, map[int]string{1: "*"} [b]))
		if ci.Block == 20 && b == 1 {
			has20 = true
		}
		if ci.Block == 40 && b == 1 {
			has40 = true
		}
	}
	verdict := "NO"
	if has20 && has40 {
		verdict = "YES"
	}
	return strings.Join(out, ", ") + " | CI excludes 0 on contiguous {20,40}: " + verdict
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, f := range v {
		s += f
	}
	return s / float64(len(v))
}

func toFloat(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}

func formatBeta(beta map[string]float64) string {
	keys := make([]string, 0, len(beta))
	for k := range beta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%.3f", k, beta[k]))
	}
	return strings.Join(out, ", ")
}

func parseFloats(s string) ([]float64, error) {
	var out []float64
	for _, raw := range strings.Split(s, ",") {
		var v float64
		if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%g", &v); err != nil {
			return nil, fmt.Errorf("parse %q: %w", raw, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func writef(b *strings.Builder, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(b, format, args...)
}

func defaultWindows() []betaregime.Window {
	return []betaregime.Window{
		{From: day(2025, 4, 1), Till: day(2025, 6, 30)},
		{From: day(2025, 7, 1), Till: day(2025, 9, 30)},
		{From: day(2025, 10, 1), Till: day(2025, 12, 30)},
		{From: day(2026, 1, 5), Till: day(2026, 3, 31)},
		{From: day(2026, 4, 1), Till: day(2026, 6, 30)},
		{From: day(2026, 7, 1), Till: day(2026, 9, 17)},
	}
}

func day(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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

type memorySource struct {
	candles map[string][]moex.Candle
}

func (m memorySource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	candles, ok := m.candles[strings.ToUpper(strings.TrimSpace(ticker))]
	if !ok {
		return nil, fmt.Errorf("alphahedge: no history for %s", ticker)
	}
	return candles, nil
}

func runEngine(ctx context.Context, cfg backtest.Config) (*backtest.Result, error) {
	engine, err := backtest.NewEngine(cfg)
	if err != nil {
		return nil, err
	}
	return engine.Run(ctx)
}