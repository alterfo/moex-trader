package main

import (
	"math"
	"sort"
)

const (
	bValDay = iota
	bOpenDay
	bCloseDay
	bOpen10
	bClose10
	bClose1830
	bHigh
	bLow
	bNJ
	bA1
	bA2
	bImbVol
	bImbBBOVal
	bImbBBOVol
	bDepth
	bLevAsym
	bThinBid
	bSpLv
	bSpBBO
	bSp1m
	bSpRatio
	bTicket
	bTicketB
	bTicketS
	bTop3
	bTrades
	bTrCntImb
	bLogVolRatio
	bAggGap
	bCLV
	bVwapDev
	bPutOverVal
	bCanDiff
	bPlaceAgg
	bPutSize
	bA4
	bA5
	bCanVwGap
	bCanOrdRatio
	bPutOrders
	bPutOverBook
	bA2Late
	bA2Early
	bA2H1
	bA2H2
	bTicketGapLate
	bAggGapEarly
	bA4Late
	bA4Early
	bImbStd
	bSpStd
	bSpLast
	bSpEarlyOverMid
	bBBOoverStd
	bDepthOpenClose
	bPeakMin
	bShareMid
	bShareLast
	bA1Trades
	bTicketMed
	bTicketStd
	bAmihud
	bIdle
	bShareNeg
	bA1Tail
	bRetLast
	bRetEarly
	bRV
	bRSdn
	bRSup
	bMaxJump
	bPark
	bPrStd
	bMornA1
	bMornShare
	bEvA1
	bEvA2
	bEvSpRatio
	bEvRet
	bAucA1
	bVPIN
	bKyle
	bFlowRet
	bFlowAC
	bSkew
	bImbAC2
	bProfTilt
	bImbSlope
	bProfP2
	bProfP3
	bVolEnt
	bDepEnt
	nb
)

func newBase() *[nb]float32 {
	r := &[nb]float32{}
	for i := range r {
		r[i] = float32(nan)
	}
	return r
}

type jset struct {
	idx []int
}

func joined(m *[nMain]bar, lo, hi int, needOS bool) []int {
	var out []int
	for s := lo; s <= hi && s < nMain; s++ {
		b := &m[s]
		if b.has[hasTS] && b.has[hasOB] && (!needOS || b.has[hasOS]) {
			out = append(out, s)
		}
	}
	return out
}

func sumF(m *[nMain]bar, idx []int, f int) float64 {
	s := 0.0
	for _, i := range idx {
		s += float64(m[i].f[f])
	}
	return s
}

func safeDiv(a, b float64) float64 {
	if b == 0 || math.IsNaN(a) || math.IsNaN(b) {
		return nan
	}
	return a / b
}

func a1Of(m *[nMain]bar, idx []int) float64 {
	v := sumF(m, idx, tVal)
	if v <= 0 {
		return nan
	}
	return (sumF(m, idx, tValB) - sumF(m, idx, tValS)) / v
}

func wMean(m *[nMain]bar, idx []int, f int) float64 {
	v := sumF(m, idx, tVal)
	if v <= 0 {
		return nan
	}
	s := 0.0
	for _, i := range idx {
		s += float64(m[i].f[f]) * float64(m[i].f[tVal])
	}
	return s / v
}

func meanPos(m *[nMain]bar, idx []int, f int) float64 {
	s, n := 0.0, 0
	for _, i := range idx {
		if v := float64(m[i].f[f]); v > 0 {
			s += v
			n++
		}
	}
	if n == 0 {
		return nan
	}
	return s / float64(n)
}

func a4Of(m *[nMain]bar, idx []int) float64 {
	pb, ps := sumF(m, idx, sPutValB), sumF(m, idx, sPutValS)
	cb, cs := sumF(m, idx, sCanValB), sumF(m, idx, sCanValS)
	return safeDiv((pb-cb)-(ps-cs), pb+ps)
}

func aggGapOf(m *[nMain]bar, idx []int) float64 {
	num, den := 0.0, 0.0
	for _, i := range idx {
		b := &m[i].f
		if b[tVwB] > 0 && b[tVwS] > 0 && b[tVwap] > 0 && b[tVal] > 0 {
			num += float64(b[tVal]) * float64(b[tVwB]-b[tVwS]) / float64(b[tVwap])
			den += float64(b[tVal])
		}
	}
	return safeDiv(num, den)
}

func a2Window(m *[nMain]bar, lo, hi, minBars int) float64 {
	idx := joined(m, lo, hi, true)
	if len(idx) < minBars {
		return nan
	}
	return wMean(m, idx, oImbVal)
}

func lastClose(m *[nMain]bar, hi int) (float64, int) {
	for s := hi; s >= 0; s-- {
		if m[s].has[hasTS] && m[s].f[tClose] > 0 {
			return float64(m[s].f[tClose]), s
		}
	}
	return nan, -1
}

func stdOf(v []float64) float64 {
	if len(v) < 3 {
		return nan
	}
	m := mean(v)
	s := 0.0
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return math.Sqrt(s / float64(len(v)-1))
}

func computeBase(w *sessions) *[nb]float32 {
	m := &w.main
	b := newBase()
	set := func(k int, v float64) {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			b[k] = float32(v)
		}
	}
	valDay := 0.0
	for s := 0; s < nMain; s++ {
		if m[s].has[hasTS] {
			valDay += float64(m[s].f[tVal])
			if m[s].f[tClose] > 0 {
				set(bCloseDay, float64(m[s].f[tClose]))
			}
		}
	}
	set(bValDay, valDay)
	if m[0].has[hasTS] {
		if m[0].f[tOpen] > 0 {
			set(bOpen10, float64(m[0].f[tOpen]))
		}
		if m[0].f[tClose] > 0 {
			set(bClose10, float64(m[0].f[tClose]))
		}
	}
	for s := 0; s < nMain; s++ {
		if m[s].has[hasTS] && m[s].f[tOpen] > 0 {
			set(bOpenDay, float64(m[s].f[tOpen]))
			break
		}
	}
	c1830, cs := lastClose(m, slotFeat)
	if cs >= slotNear {
		set(bClose1830, c1830)
	}
	var tsIdx []int
	hi, lo := math.Inf(-1), math.Inf(1)
	for s := 0; s <= slotFeat; s++ {
		if m[s].has[hasTS] {
			tsIdx = append(tsIdx, s)
			if m[s].f[tHigh] > 0 {
				hi = math.Max(hi, float64(m[s].f[tHigh]))
			}
			if m[s].f[tLow] > 0 {
				lo = math.Min(lo, float64(m[s].f[tLow]))
			}
		}
	}
	if !math.IsInf(hi, 0) && !math.IsInf(lo, 0) {
		set(bHigh, hi)
		set(bLow, lo)
	}
	allVal := sumF(m, tsIdx, tVal)
	set(bIdle, 1-float64(countPos(m, tsIdx, tVal))/float64(slotFeat+1))
	if allVal > 0 {
		set(bShareMid, sumRange(m, 35, 59, tVal)/allVal)
		set(bShareLast, sumRange(m, 89, slotFeat, tVal)/allVal)
		maxV, maxS := 0.0, -1
		for _, s := range tsIdx {
			if float64(m[s].f[tVal]) > maxV {
				maxV, maxS = float64(m[s].f[tVal]), s
			}
		}
		if maxS >= 0 {
			set(bPeakMin, float64((maxS+1)*5))
		}
	}
	vol := sumF(m, tsIdx, tVol)
	if vol > 0 && allVal > 0 && !math.IsNaN(c1830) && cs >= slotNear {
		set(bVwapDev, c1830/(allVal/vol)-1)
	}
	if hi > lo && cs >= slotNear {
		set(bCLV, (c1830-lo)/(hi-lo))
	}
	j := joined(m, 0, slotFeat, true)
	set(bNJ, float64(len(j)))
	if len(tsIdx) >= 20 {
		var rets []float64
		prev := nan
		for _, s := range tsIdx {
			c := float64(m[s].f[tClose])
			if c > 0 {
				if !math.IsNaN(prev) {
					rets = append(rets, math.Log(c/prev))
				}
				prev = c
			}
		}
		if len(rets) >= 15 {
			sq, up, dn, mx := 0.0, 0.0, 0.0, 0.0
			for _, r := range rets {
				sq += r * r
				if r > 0 {
					up += r * r
				} else {
					dn += r * r
				}
				mx = math.Max(mx, math.Abs(r))
			}
			if sq > 0 {
				set(bRV, math.Sqrt(sq))
				set(bRSup, up)
				set(bRSdn, dn)
				set(bMaxJump, mx/math.Sqrt(sq))
				if hi > lo {
					set(bPark, math.Log(hi/lo)/(2*math.Sqrt(math.Ln2))/math.Sqrt(sq))
				}
			}
		}
		if p, ps := priceAt(m, 89); ps >= 0 && cs >= slotNear {
			set(bRetLast, math.Log(c1830/p))
		}
		if p, ps := priceAt(m, 17); ps >= 0 && b[bClose10] > 0 {
			set(bRetEarly, math.Log(p/float64(b[bClose10])))
		}
	}
	if len(j) >= 20 {
		fillMain(m, j, b, set)
	}
	fillMorning(w, b, set)
	fillEvening(w, b, set)
	fillAuction(m, set)
	fillMicro(m, set)
	return b
}

func fillMicro(m *[nMain]bar, set func(int, float64)) {
	var rets, imb, vals, idx []float64
	var imbBook, depth []float64
	prev := nan
	var absVol, totVol float64
	for s := 0; s <= slotFeat; s++ {
		if !m[s].has[hasTS] {
			continue
		}
		c := float64(m[s].f[tClose])
		v := float64(m[s].f[tVal])
		vb, vs := float64(m[s].f[tVolB]), float64(m[s].f[tVolS])
		absVol += math.Abs(vb - vs)
		totVol += vb + vs
		if c > 0 && !math.IsNaN(prev) && v > 0 {
			rets = append(rets, math.Log(c/prev))
			imb = append(imb, float64(m[s].f[tValB]-m[s].f[tValS])/v)
			vals = append(vals, v)
			idx = append(idx, float64(s))
			if m[s].has[hasOB] {
				imbBook = append(imbBook, float64(m[s].f[oImbVal]))
			} else {
				imbBook = append(imbBook, nan)
			}
			if m[s].has[hasOB] {
				depth = append(depth, float64(m[s].f[oVolB])+float64(m[s].f[oVolS]))
			} else {
				depth = append(depth, nan)
			}
		}
		if c > 0 {
			prev = c
		}
	}
	if totVol > 0 {
		set(bVPIN, absVol/totVol)
	}
	n := len(rets)
	if n < 20 {
		return
	}
	mi, mr := 0.0, 0.0
	for i := range imb {
		mi += imb[i]
		mr += rets[i]
	}
	mi /= float64(n)
	mr /= float64(n)
	var cov, vi, vr float64
	for i := range imb {
		cov += (imb[i] - mi) * (rets[i] - mr)
		vi += (imb[i] - mi) * (imb[i] - mi)
		vr += (rets[i] - mr) * (rets[i] - mr)
	}
	if vi > 0 {
		set(bKyle, cov/vi)
	}
	if vi > 0 && vr > 0 {
		set(bFlowRet, cov/math.Sqrt(vi*vr))
	}
	ac := func(x []float64) float64 {
		var mx float64
		k := 0
		for _, v := range x {
			if !math.IsNaN(v) {
				mx += v
				k++
			}
		}
		if k < 20 {
			return nan
		}
		mx /= float64(k)
		var num, den float64
		for i := 0; i < len(x); i++ {
			if math.IsNaN(x[i]) {
				continue
			}
			den += (x[i] - mx) * (x[i] - mx)
			if i > 0 && !math.IsNaN(x[i-1]) {
				num += (x[i] - mx) * (x[i-1] - mx)
			}
		}
		if den == 0 {
			return nan
		}
		return num / den
	}
	set(bFlowAC, ac(imb))
	set(bImbAC2, ac(imbBook))
	var s2, s3 float64
	for _, r := range rets {
		s2 += r * r
		s3 += r * r * r
	}
	if s2 > 0 {
		set(bSkew, s3/math.Pow(s2, 1.5)*math.Sqrt(float64(n)))
	}
	var mxIdx, mxv float64
	for i := range idx {
		mxIdx += idx[i]
		mxv += vals[i]
	}
	mxIdx /= float64(n)
	mxv /= float64(n)
	var cv, vx float64
	for i := range idx {
		cv += (idx[i] - mxIdx) * (vals[i] - mxv)
		vx += (idx[i] - mxIdx) * (idx[i] - mxIdx)
	}
	if vx > 0 && mxv > 0 {
		set(bProfTilt, cv/vx/mxv*float64(slotFeat))
	}
	cum := 0.0
	var cs []float64
	for _, v := range imb {
		cum += v
		cs = append(cs, cum)
	}
	var mc float64
	for _, v := range cs {
		mc += v
	}
	mc /= float64(n)
	var cc float64
	for i := range cs {
		cc += (idx[i] - mxIdx) * (cs[i] - mc)
	}
	if vx > 0 {
		set(bImbSlope, cc/vx)
	}
	if n >= 20 {
		u0, u1 := idx[0], idx[n-1]
		if u1 > u0 {
			var s2, s3, n2, n3 float64
			for i := range cs {
				u := 2*(idx[i]-u0)/(u1-u0) - 1
				p2 := 0.5 * (3*u*u - 1)
				p3 := 0.5 * (5*u*u*u - 3*u)
				s2 += cs[i] * p2
				n2 += p2 * p2
				s3 += cs[i] * p3
				n3 += p3 * p3
			}
			set(bProfP2, s2/n2)
			set(bProfP3, s3/n3)
		}
	}
	entropy := func(x []float64) float64 {
		var t float64
		k := 0
		for _, v := range x {
			if !math.IsNaN(v) && v > 0 {
				t += v
				k++
			}
		}
		if k < 20 || t <= 0 {
			return nan
		}
		h := 0.0
		for _, v := range x {
			if !math.IsNaN(v) && v > 0 {
				p := v / t
				h -= p * math.Log(p)
			}
		}
		return h / math.Log(float64(k))
	}
	if e := entropy(vals); !math.IsNaN(e) {
		set(bVolEnt, e)
	}
	if e := entropy(depth); !math.IsNaN(e) {
		set(bDepEnt, e)
	}
}

func fillAuction(m *[nMain]bar, set func(int, float64)) {
	val, a := 0.0, 0.0
	for s := 103; s < nMain; s++ {
		if m[s].has[hasTS] {
			val += float64(m[s].f[tVal])
			a += float64(m[s].f[tValB] - m[s].f[tValS])
		}
	}
	if val > 0 {
		set(bAucA1, a/val)
	}
}

func priceAt(m *[nMain]bar, hi int) (float64, int) {
	return lastClose(m, hi)
}

func countPos(m *[nMain]bar, idx []int, f int) int {
	n := 0
	for _, i := range idx {
		if m[i].f[f] > 0 {
			n++
		}
	}
	return n
}

func sumRange(m *[nMain]bar, lo, hi, f int) float64 {
	s := 0.0
	for k := lo; k <= hi && k < nMain; k++ {
		if m[k].has[hasTS] {
			s += float64(m[k].f[f])
		}
	}
	return s
}

func fillMain(m *[nMain]bar, j []int, b *[nb]float32, set func(int, float64)) {
	val := sumF(m, j, tVal)
	if val <= 0 {
		return
	}
	set(bA1, a1Of(m, j))
	set(bA2, wMean(m, j, oImbVal))
	set(bImbVol, wMean(m, j, oImbVol))
	set(bImbBBOVal, wMean(m, j, oImbValBBO))
	set(bImbBBOVol, wMean(m, j, oImbVolBBO))
	depth := 0.0
	var lev, thin, imbs, sps []float64
	var spRatio, bbo []float64
	for _, i := range j {
		f := &m[i].f
		depth += float64(f[oValB] + f[oValS])
		if f[oLvB]+f[oLvS] > 0 {
			lev = append(lev, float64(f[oLvB]-f[oLvS])/float64(f[oLvB]+f[oLvS]))
		}
		if f[oVwB1] > 0 && f[oVwS1] > 0 {
			mid := float64(f[oVwB1]+f[oVwS1]) / 2
			thin = append(thin, (1-float64(f[oVwB1])/mid)-(float64(f[oVwS1])/mid-1))
		}
		imbs = append(imbs, float64(f[oImbVal]))
		if f[oSpLv] > 0 {
			sps = append(sps, float64(f[oSpLv]))
			if f[oSpBBO] > 0 {
				spRatio = append(spRatio, float64(f[oSpBBO]/f[oSpLv]))
			}
		}
		if f[oSpBBO] > 0 && f[tStd] > 0 {
			bbo = append(bbo, float64(f[oSpBBO]/f[tStd]))
		}
	}
	set(bDepth, depth/float64(len(j)))
	set(bLevAsym, mean(lev))
	set(bThinBid, mean(thin))
	set(bSpLv, meanPos(m, j, oSpLv))
	set(bSpBBO, meanPos(m, j, oSpBBO))
	set(bSp1m, meanPos(m, j, oSp1m))
	set(bSpRatio, mean(spRatio))
	set(bBBOoverStd, mean(bbo))
	set(bImbStd, stdOf(imbs))
	if len(sps) >= 10 {
		set(bSpStd, stdOf(sps))
		set(bSpLast, math.Log(sps[len(sps)-1]/mean(sps)))
	}
	trades := sumF(m, j, tTrades)
	trB, trS := sumF(m, j, tTrB), sumF(m, j, tTrS)
	set(bTrades, trades)
	if trades > 0 {
		set(bTicket, val/trades)
		set(bTrCntImb, (trB-trS)/trades)
	}
	if trB > 0 && trS > 0 {
		set(bTicketB, sumF(m, j, tValB)/trB)
		set(bTicketS, sumF(m, j, tValS)/trS)
	}
	vb, vs := sumF(m, j, tVolB), sumF(m, j, tVolS)
	if vb > 0 && vs > 0 {
		set(bLogVolRatio, math.Log(vb/vs))
	}
	set(bAggGap, aggGapOf(m, j))
	vals := make([]float64, 0, len(j))
	for _, i := range j {
		vals = append(vals, float64(m[i].f[tVal]))
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(vals)))
	top := 0.0
	for k := 0; k < 3 && k < len(vals); k++ {
		top += vals[k]
	}
	set(bTop3, top/val)
	var tick, ticksd, amih []float64
	var a1tr, trw float64
	var negVal, negSell float64
	var negN int
	for _, i := range j {
		f := &m[i].f
		if f[tTrades] > 0 && f[tVal] > 0 {
			t := float64(f[tVal] / f[tTrades])
			tick = append(tick, t)
			if f[tStd] > 0 {
				ticksd = append(ticksd, t/float64(f[tStd]))
			}
			a1tr += float64(f[tTrades]) * float64(f[tValB]-f[tValS]) / float64(f[tVal])
			trw += float64(f[tTrades])
		}
		if f[tVal] > 0 {
			amih = append(amih, math.Abs(float64(f[tChg]))/float64(f[tVal]))
		}
		if f[tChg] < 0 {
			negVal += float64(f[tVal])
			negSell += float64(f[tValS])
			negN++
		}
	}
	if len(tick) > 0 {
		sort.Float64s(tick)
		set(bTicketMed, tick[len(tick)/2])
	}
	set(bTicketStd, mean(ticksd))
	set(bAmihud, mean(amih))
	if trw > 0 {
		set(bA1Trades, a1tr/trw)
	}
	if negN >= 5 && negVal > 0 {
		set(bShareNeg, negSell/negVal)
	}
	var absChg []float64
	for _, i := range j {
		absChg = append(absChg, math.Abs(float64(m[i].f[tChg])))
	}
	sorted := append([]float64(nil), absChg...)
	sort.Float64s(sorted)
	thr := quantile(sorted, 0.9)
	var tail []int
	for k, i := range j {
		if absChg[k] >= thr {
			tail = append(tail, i)
		}
	}
	if len(tail) >= 3 {
		set(bA1Tail, a1Of(m, tail))
	}
	pv, cv := sumF(m, j, sPutValB)+sumF(m, j, sPutValS), sumF(m, j, sCanValB)+sumF(m, j, sCanValS)
	set(bPutOverVal, pv/val)
	pb, ps := sumF(m, j, sPutValB), sumF(m, j, sPutValS)
	cb, cs := sumF(m, j, sCanValB), sumF(m, j, sCanValS)
	if pb > 0 && ps > 0 {
		set(bCanDiff, cb/pb-cs/ps)
	}
	pob, pos := sumF(m, j, sPutOrB), sumF(m, j, sPutOrS)
	if pb > 0 && ps > 0 && pob > 0 && pos > 0 {
		set(bPutSize, math.Log((pb/pob)/(ps/pos)))
	}
	set(bA4, a4Of(m, j))
	set(bA5, safeDiv(cv, pv))
	set(bPutOrders, pob+pos)
	cob, cos := sumF(m, j, sCanOrB), sumF(m, j, sCanOrS)
	set(bCanOrdRatio, safeDiv(cob+cos, pob+pos))
	vol := sumF(m, j, tVol)
	if vol > 0 {
		dv := val / vol
		pvb, pvs := vwapOf(m, j, sPutVwB, sPutVolB), vwapOf(m, j, sPutVwS, sPutVolS)
		set(bPlaceAgg, (pvb-pvs)/dv)
		cvb, cvs := vwapOf(m, j, sCanVwB, sCanVolB), vwapOf(m, j, sCanVwS, sCanVolS)
		set(bCanVwGap, (cvb-cvs)/dv)
	}
	set(bPutOverBook, safeDiv(pv/float64(len(j)), depth/float64(len(j))))
	set(bA2Late, a2Window(m, 83, slotFeat, 8))
	set(bA2Early, a2Window(m, 0, 17, 5))
	set(bA2H1, a2Window(m, 0, 50, 10))
	set(bA2H2, a2Window(m, 51, slotFeat, 10))
	if idx := joined(m, 83, slotFeat, true); len(idx) >= 8 {
		tb, ts := sumF(m, idx, tTrB), sumF(m, idx, tTrS)
		if tb > 0 && ts > 0 {
			set(bTicketGapLate, math.Log((sumF(m, idx, tValB)/tb)/(sumF(m, idx, tValS)/ts)))
		}
	}
	if idx := joined(m, 0, 17, true); len(idx) >= 5 {
		set(bAggGapEarly, aggGapOf(m, idx))
	}
	if idx := joined(m, 95, slotFeat, true); len(idx) >= 3 {
		set(bA4Late, a4Of(m, idx))
	}
	if idx := joined(m, 0, 6, true); len(idx) >= 3 {
		set(bA4Early, a4Of(m, idx))
	}
	e, l := joined(m, 0, 5, true), joined(m, 11, 17, true)
	se, sl := meanPos(m, e, oSpLv), meanPos(m, l, oSpLv)
	if se > 0 && sl > 0 {
		set(bSpEarlyOverMid, math.Log(se/sl))
	}
	o, c := joined(m, 0, 6, true), joined(m, 95, slotFeat, true)
	if len(o) >= 2 && len(c) >= 2 {
		do, dc := depthMean(m, o), depthMean(m, c)
		if do > 0 && dc > 0 {
			set(bDepthOpenClose, math.Log(do/dc))
		}
	}
	var pr []float64
	for _, i := range j {
		if m[i].f[tStd] > 0 {
			pr = append(pr, float64(m[i].f[tStd]))
		}
	}
	set(bPrStd, mean(pr))
}

func depthMean(m *[nMain]bar, idx []int) float64 {
	s := 0.0
	for _, i := range idx {
		s += float64(m[i].f[oValB] + m[i].f[oValS])
	}
	return s / float64(len(idx))
}

func vwapOf(m *[nMain]bar, idx []int, vw, vol int) float64 {
	num, den := 0.0, 0.0
	for _, i := range idx {
		v := float64(m[i].f[vol])
		if m[i].f[vw] > 0 && v > 0 {
			num += float64(m[i].f[vw]) * v
			den += v
		}
	}
	if den == 0 {
		return nan
	}
	return num / den
}

func fillMorning(w *sessions, b *[nb]float32, set func(int, float64)) {
	var idx []int
	val, a, bsum := 0.0, 0.0, 0.0
	for s := 0; s < nMorn; s++ {
		x := &w.morn[s]
		if x.has[hasTS] && x.has[hasOB] {
			idx = append(idx, s)
			val += float64(x.f[tVal])
			a += float64(x.f[tValB] - x.f[tValS])
		}
		if x.has[hasTS] {
			bsum += float64(x.f[tVal])
		}
	}
	if len(idx) >= 10 && val > 0 {
		set(bMornA1, a/val)
	}
	main := float64(b[bValDay])
	if !math.IsNaN(main) && bsum+main > 0 {
		set(bMornShare, bsum/(bsum+main))
	}
}

func fillEvening(w *sessions, b *[nb]float32, set func(int, float64)) {
	var idx []int
	val, a, imb := 0.0, 0.0, 0.0
	sp, spn := 0.0, 0
	last := nan
	for s := 0; s < nEve; s++ {
		x := &w.eve[s]
		if x.has[hasTS] && x.f[tClose] > 0 {
			last = float64(x.f[tClose])
		}
		if x.has[hasTS] && x.has[hasOB] {
			idx = append(idx, s)
			v := float64(x.f[tVal])
			val += v
			a += float64(x.f[tValB] - x.f[tValS])
			imb += float64(x.f[oImbVal]) * v
			if x.f[oSpLv] > 0 {
				sp += float64(x.f[oSpLv])
				spn++
			}
		}
	}
	if len(idx) < 20 || val <= 0 {
		return
	}
	set(bEvA1, a/val)
	set(bEvA2, imb/val)
	if spn >= 20 && b[bSpLv] > 0 {
		set(bEvSpRatio, math.Log(sp/float64(spn)/float64(b[bSpLv])))
	}
	if b[bClose1830] > 0 && !math.IsNaN(last) {
		set(bEvRet, math.Log(last/float64(b[bClose1830])))
	}
}
