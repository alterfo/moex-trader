package model

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const usdRubTicker = "USD000UTSTOM"

func LogReturnSeries(candles []moex.Candle) map[string]float64 {
	closesByDate := make(map[string]float64, len(candles))
	dates := make([]string, 0, len(candles))
	for _, c := range candles {
		if c.Close.Sign() <= 0 {
			continue
		}
		key := dateKey(c.Begin)
		if _, exists := closesByDate[key]; !exists {
			dates = append(dates, key)
		}
		closePrice, _ := c.Close.Float64()
		closesByDate[key] = closePrice
	}
	sort.Strings(dates)

	returns := make(map[string]float64, len(dates))
	for i := 1; i < len(dates); i++ {
		prev, cur := closesByDate[dates[i-1]], closesByDate[dates[i]]
		if prev <= 0 || cur <= 0 {
			continue
		}
		returns[dates[i]] = math.Log(cur / prev)
	}
	return returns
}

func BuildReturnUniverse(ctx context.Context, source backtest.HistoricalSource, tickers []string, from, till time.Time) (map[string]map[string]float64, error) {
	returns := make(map[string]map[string]float64, len(tickers)+2)
	fetch := func(ticker string) error {
		candles, err := source.History(ctx, ticker, from, till)
		if err != nil {
			return err
		}
		if len(candles) < 90 {
			return fmt.Errorf("insufficient history: %d candles", len(candles))
		}
		returns[ticker] = LogReturnSeries(candles)
		return nil
	}

	for _, ticker := range normalizeTickers(tickers) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		_ = fetch(ticker)
	}
	if err := fetch(benchmarkTicker); err != nil {
		return nil, fmt.Errorf("model: benchmark %s: %w", benchmarkTicker, err)
	}
	_ = fetch(usdRubTicker)

	return returns, nil
}

func commonDates(x, y map[string]float64) []string {
	dates := make([]string, 0, len(x))
	for d := range x {
		if _, ok := y[d]; ok {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	return dates
}

func alignSeries(x, y map[string]float64) (xs, ys []float64) {
	dates := commonDates(x, y)
	xs = make([]float64, len(dates))
	ys = make([]float64, len(dates))
	for i, d := range dates {
		xs[i] = x[d]
		ys[i] = y[d]
	}
	return xs, ys
}

type CrossCorrelationResult struct {
	N               int
	Contemporaneous float64
	Lead            map[int]float64
	Lag             map[int]float64
}

func CrossCorrelation(returnsX, returnsY map[string]float64, maxLag, minOverlap int) CrossCorrelationResult {
	x, y := alignSeries(returnsX, returnsY)
	n := len(x)
	result := CrossCorrelationResult{N: n, Lead: map[int]float64{}, Lag: map[int]float64{}}
	result.Contemporaneous, _ = pearsonCorrelation(x, y)

	for k := 1; k <= maxLag; k++ {
		if n-k < minOverlap {
			continue
		}
		leadCorr, _ := pearsonCorrelation(x[:n-k], y[k:])
		result.Lead[k] = leadCorr
		lagCorr, _ := pearsonCorrelation(y[:n-k], x[k:])
		result.Lag[k] = lagCorr
	}
	return result
}

type GrangerResult struct {
	FStat  float64
	PValue float64
	N      int
}

func GrangerFTest(returnsX, returnsY map[string]float64, lag, minOverlap int) (*GrangerResult, bool) {
	x, y := alignSeries(returnsX, returnsY)
	n := len(x)
	if n-lag < minOverlap {
		return nil, false
	}

	rows := n - lag
	restricted := make([][]float64, rows)
	unrestricted := make([][]float64, rows)
	target := make([]float64, rows)
	for i, t := 0, lag; t < n; i, t = i+1, t+1 {
		row := make([]float64, 0, 1+2*lag)
		row = append(row, 1.0)
		for l := 1; l <= lag; l++ {
			row = append(row, y[t-l])
		}
		restricted[i] = append([]float64(nil), row...)
		for l := 1; l <= lag; l++ {
			row = append(row, x[t-l])
		}
		unrestricted[i] = row
		target[i] = y[t]
	}

	betaR, ok := solveLeastSquares(restricted, target)
	if !ok {
		return nil, false
	}
	betaU, ok := solveLeastSquares(unrestricted, target)
	if !ok {
		return nil, false
	}

	rssR := residualSumSquares(restricted, betaR, target)
	rssU := residualSumSquares(unrestricted, betaU, target)
	dfDen := len(target) - len(betaU)
	if rssU <= 0 || dfDen <= 0 {
		return nil, false
	}

	fStat := ((rssR - rssU) / float64(lag)) / (rssU / float64(dfDen))
	if fStat < 0 {
		fStat = 0
	}
	return &GrangerResult{FStat: fStat, PValue: fSurvival(fStat, float64(lag), float64(dfDen)), N: len(target)}, true
}

func residualSumSquares(x [][]float64, beta []float64, y []float64) float64 {
	var rss float64
	for i, row := range x {
		pred := dot(beta, row)
		resid := y[i] - pred
		rss += resid * resid
	}
	return rss
}

func solveLeastSquares(x [][]float64, y []float64) ([]float64, bool) {
	n := len(x)
	if n == 0 {
		return nil, false
	}
	p := len(x[0])
	xtx := make([][]float64, p)
	for i := range xtx {
		xtx[i] = make([]float64, p)
	}
	xty := make([]float64, p)
	for r := range n {
		for i := range p {
			xty[i] += x[r][i] * y[r]
			for j := range p {
				xtx[i][j] += x[r][i] * x[r][j]
			}
		}
	}
	return gaussianSolve(xtx, xty)
}

func gaussianSolve(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	aug := make([][]float64, n)
	for i := range aug {
		aug[i] = make([]float64, n+1)
		copy(aug[i], a[i])
		aug[i][n] = b[i]
	}
	for col := range n {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(aug[r][col]) > math.Abs(aug[pivot][col]) {
				pivot = r
			}
		}
		aug[col], aug[pivot] = aug[pivot], aug[col]
		if math.Abs(aug[col][col]) < 1e-12 {
			return nil, false
		}
		for r := col + 1; r < n; r++ {
			factor := aug[r][col] / aug[col][col]
			for c := col; c <= n; c++ {
				aug[r][c] -= factor * aug[col][c]
			}
		}
	}
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := aug[i][n]
		for j := i + 1; j < n; j++ {
			sum -= aug[i][j] * x[j]
		}
		x[i] = sum / aug[i][i]
	}
	return x, true
}

func fSurvival(f, d1, d2 float64) float64 {
	if f <= 0 {
		return 1
	}
	x := d1 * f / (d1*f + d2)
	return 1 - regularizedIncompleteBeta(x, d1/2, d2/2)
}

func regularizedIncompleteBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	lbetaA, _ := math.Lgamma(a)
	lbetaB, _ := math.Lgamma(b)
	lbetaAB, _ := math.Lgamma(a + b)
	front := math.Exp(lbetaAB - lbetaA - lbetaB + a*math.Log(x) + b*math.Log(1-x))
	if x < (a+1)/(a+b+2) {
		return front * betaContinuedFraction(x, a, b) / a
	}
	return 1 - front*betaContinuedFraction(1-x, b, a)/b
}

func betaContinuedFraction(x, a, b float64) float64 {
	const maxIter = 200
	const eps = 3e-14
	const fpmin = 1e-300

	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < fpmin {
		d = fpmin
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		m2 := float64(2 * m)
		aa := float64(m) * (b - float64(m)) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		h *= d * c

		aa = -(a + float64(m)) * (qab + float64(m)) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

type LeadLagResult struct {
	Candidate             string
	Target                string
	LeadDays              int
	LeadCorr              float64
	ReverseLeadDays       int
	ReverseCorr           float64
	ContemporaneousCorr   float64
	Asymmetry             float64
	N                     int
	HasGrangerP           bool
	GrangerP              float64
	BonferroniSignificant bool
	FDRSignificant        bool
}

func FindLeaders(returns map[string]map[string]float64, targets, candidates []string, maxLag, grangerLag, minOverlap int, minAsymmetry float64) (results []LeadLagResult, bonferroniAlpha float64, nTests int) {
	for _, target := range targets {
		targetReturns, ok := returns[target]
		if !ok {
			continue
		}
		for _, candidate := range candidates {
			if candidate == target {
				continue
			}
			candidateReturns, ok := returns[candidate]
			if !ok {
				continue
			}
			cc := CrossCorrelation(candidateReturns, targetReturns, maxLag, minOverlap)
			if len(cc.Lead) == 0 || len(cc.Lag) == 0 {
				continue
			}
			leadK, leadV := maxAbsEntry(cc.Lead)
			lagK, lagV := maxAbsEntry(cc.Lag)

			r := LeadLagResult{
				Candidate: candidate, Target: target,
				LeadDays: leadK, LeadCorr: leadV,
				ReverseLeadDays: lagK, ReverseCorr: lagV,
				ContemporaneousCorr: cc.Contemporaneous,
				Asymmetry:           math.Abs(leadV) - math.Abs(lagV),
				N:                   cc.N,
			}
			if granger, ok := GrangerFTest(candidateReturns, targetReturns, grangerLag, minOverlap); ok {
				r.HasGrangerP = true
				r.GrangerP = granger.PValue
			}
			results = append(results, r)
		}
	}

	nTests = len(results)
	bonferroniAlpha = 0.05
	if nTests > 0 {
		bonferroniAlpha = 0.05 / float64(nTests)
	}
	for i := range results {
		if results[i].HasGrangerP && results[i].GrangerP < bonferroniAlpha {
			results[i].BonferroniSignificant = true
		}
	}
	applyBenjaminiHochberg(results, 0.05)

	filtered := make([]LeadLagResult, 0, len(results))
	for _, r := range results {
		if r.Asymmetry >= minAsymmetry {
			filtered = append(filtered, r)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		pi, pj := grangerPOrDefault(filtered[i]), grangerPOrDefault(filtered[j])
		if pi != pj {
			return pi < pj
		}
		return filtered[i].Asymmetry > filtered[j].Asymmetry
	})
	return filtered, bonferroniAlpha, nTests
}

func grangerPOrDefault(r LeadLagResult) float64 {
	if r.HasGrangerP {
		return r.GrangerP
	}
	return 1.0
}

func maxAbsEntry(m map[int]float64) (int, float64) {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	bestKey, bestVal := keys[0], m[keys[0]]
	for _, k := range keys[1:] {
		if math.Abs(m[k]) > math.Abs(bestVal) {
			bestKey, bestVal = k, m[k]
		}
	}
	return bestKey, bestVal
}

func applyBenjaminiHochberg(results []LeadLagResult, alpha float64) {
	type indexed struct {
		idx int
		p   float64
	}
	withP := make([]indexed, 0, len(results))
	for i, r := range results {
		if r.HasGrangerP {
			withP = append(withP, indexed{idx: i, p: r.GrangerP})
		}
	}
	sort.Slice(withP, func(i, j int) bool { return withP[i].p < withP[j].p })
	total := len(withP)
	thresholdRank := 0
	for i, item := range withP {
		rank := i + 1
		if item.p <= (float64(rank)/float64(total))*alpha {
			thresholdRank = rank
		}
	}
	for i := 0; i < thresholdRank; i++ {
		results[withP[i].idx].FDRSignificant = true
	}
}

func BuildLeadLagReport(results []LeadLagResult, bonferroniAlpha float64, nTests, topPerTarget int) string {
	var b strings.Builder
	writeLine := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	writeLine("# Асимметричные лид-лаг связи между инструментами MOEX")
	writeLine("")
	writeLine(fmt.Sprintf("Проверено пар: %d. Bonferroni-порог: p < %.6f. FDR = проходит Benjamini-Hochberg (5%%).", nTests, bonferroniAlpha))
	writeLine("")
	writeLine("| target | leader | lead_days | lead_corr | rev_corr | granger_p | sig |")
	writeLine("|---|---|---:|---:|---:|---:|---|")

	shown := make(map[string]int)
	for _, r := range results {
		if shown[r.Target] >= topPerTarget {
			continue
		}
		shown[r.Target]++
		sig := ""
		switch {
		case r.BonferroniSignificant:
			sig = "BONF"
		case r.FDRSignificant:
			sig = "fdr"
		}
		pStr := "—"
		if r.HasGrangerP {
			pStr = fmt.Sprintf("%.4f", r.GrangerP)
		}
		writeLine(fmt.Sprintf("| %s | %s | %d | %+.3f | %+.3f | %s | %s |",
			r.Target, r.Candidate, r.LeadDays, r.LeadCorr, r.ReverseCorr, pStr, sig))
	}
	return b.String()
}

type RobustnessWindow struct {
	Start          string
	End            string
	N              int
	LeadCorr       float64
	HasLeadCorr    bool
	ReverseCorr    float64
	HasReverseCorr bool
	GrangerP       float64
	HasGrangerP    bool
}

func WindowedRobustness(returnsX, returnsY map[string]float64, leadDays, windowDays, grangerLag, minOverlap int) []RobustnessWindow {
	dates := commonDates(returnsX, returnsY)
	var windows []RobustnessWindow
	for start := 0; start+windowDays <= len(dates); start += windowDays {
		windowDates := dates[start : start+windowDays]
		rx := make(map[string]float64, len(windowDates))
		ry := make(map[string]float64, len(windowDates))
		for _, d := range windowDates {
			rx[d] = returnsX[d]
			ry[d] = returnsY[d]
		}

		cc := CrossCorrelation(rx, ry, leadDays, minOverlap)
		w := RobustnessWindow{Start: windowDates[0], End: windowDates[len(windowDates)-1], N: cc.N}
		if v, ok := cc.Lead[leadDays]; ok {
			w.LeadCorr, w.HasLeadCorr = v, true
		}
		if v, ok := cc.Lag[leadDays]; ok {
			w.ReverseCorr, w.HasReverseCorr = v, true
		}
		if granger, ok := GrangerFTest(rx, ry, grangerLag, minOverlap); ok {
			w.GrangerP, w.HasGrangerP = granger.PValue, true
		}
		windows = append(windows, w)
	}
	return windows
}

func BuildRobustnessReport(candidate, target string, leadDays int, windows []RobustnessWindow) string {
	var b strings.Builder
	writeLine := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	writeLine(fmt.Sprintf("# Устойчивость лид-лаг связи %s -> %s (лаг %d дн.) на непересекающихся окнах", candidate, target, leadDays))
	writeLine("")
	writeLine("| window | n | lead_corr | reverse_corr | granger_p |")
	writeLine("|---|---:|---:|---:|---:|")
	for _, w := range windows {
		leadStr, revStr, pStr := "—", "—", "—"
		if w.HasLeadCorr {
			leadStr = fmt.Sprintf("%+.3f", w.LeadCorr)
		}
		if w.HasReverseCorr {
			revStr = fmt.Sprintf("%+.3f", w.ReverseCorr)
		}
		if w.HasGrangerP {
			pStr = fmt.Sprintf("%.4f", w.GrangerP)
		}
		writeLine(fmt.Sprintf("| %s..%s | %d | %s | %s | %s |", w.Start, w.End, w.N, leadStr, revStr, pStr))
	}
	return b.String()
}
