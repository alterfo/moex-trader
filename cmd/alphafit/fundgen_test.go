package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestTrendTAndSlope(t *testing.T) {
	y := make([]float64, 50)
	for i := range y {
		y[i] = 2*float64(i) + math.Sin(float64(i))
	}
	if tv := trendT(y); tv < 50 {
		t.Fatalf("trend t %v", tv)
	}
	if b := slopeIdx(y); math.Abs(b-2) > 0.05 {
		t.Fatalf("slope %v", b)
	}
}

func TestWindowAndRsum(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5}
	if w := window(a, 3, 3); len(w) != 3 || w[0] != 2 {
		t.Fatal(w)
	}
	if window(a, 1, 3) != nil {
		t.Fatal("short window must be nil")
	}
	if rsum(a, 1, 3) != 9 || !math.IsNaN(rsum(a, -1, 2)) {
		t.Fatal("rsum")
	}
}

func TestBlockShuffleKeepsDutyCycle(t *testing.T) {
	f := make([]bool, 105)
	for i := 0; i < 30; i++ {
		f[i*3] = true
	}
	g := circBlockPerm(f, 20, 7)
	if len(g) != len(f) {
		t.Fatalf("length %d", len(g))
	}
	a, b := 0, 0
	for i := range f {
		if f[i] {
			a++
		}
		if g[i] {
			b++
		}
	}
	if a != b {
		t.Fatalf("duty %d vs %d", a, b)
	}
}

func TestRankPartNeedsAllComponents(t *testing.T) {
	var recs []*frec
	for i := 0; i < 40; i++ {
		recs = append(recs, &frec{v: map[string]float64{"x": float64(i), "y": float64(40 - i)}})
	}
	recs[0].v["y"] = nan
	m := rankPart(recs, []fcomp{{"x", 1}, {"y", -1}})
	if _, ok := m[recs[0]]; ok {
		t.Fatal("record with a missing component must be dropped")
	}
	if len(m) != 39 {
		t.Fatalf("got %d", len(m))
	}
}

func TestMissingDayMakesWindowNaN(t *testing.T) {
	a := []float64{1, 2, nan, 4, 5, 6}
	if fw(a, 5, 4) != nil {
		t.Fatal("window with a hole must be nil")
	}
	if !math.IsNaN(wsum(a, 5, 4)) {
		t.Fatal("sum with a hole must be NaN")
	}
	if w := fw(a, 5, 3); len(w) != 3 {
		t.Fatal("clean window must pass")
	}
}

func TestSeriesFeaturesNaNAfterGap(t *testing.T) {
	nd := 300
	s := &fser{secid: "X", adm: make([]bool, nd), r: nanSlice(nd), ab: nanSlice(nd), op: nanSlice(nd), hi: nanSlice(nd), lo: nanSlice(nd), cl: nanSlice(nd), val: nanSlice(nd), a2: nanSlice(nd), lev: nanSlice(nd)}
	mk := make([]float64, nd)
	for d := 0; d < nd; d++ {
		mk[d] = 0
		s.adm[d] = true
		s.r[d], s.ab[d] = 0.001*float64(d%7-3), 0.001*float64(d%7-3)
		s.cl[d], s.val[d] = 100, 1e6
		s.hi[d], s.lo[d], s.op[d] = 101, 99, 100
	}
	cal := fcal{wday: make([]int, nd), mday: make([]int, nd), tom: make([]bool, nd), monthKey: make([]string, nd)}
	md := map[string][]float64{}
	for _, nm := range fundMacro {
		md[nm] = nanSlice(nd)
	}
	clean := fundSeries(s, mk, cal, md)
	last := clean[len(clean)-1]
	if !finite(last.v["C04"]) {
		t.Fatal("clean series must give C04")
	}
	s.r[250], s.ab[250] = nan, nan
	gap := fundSeries(s, mk, cal, md)
	if finite(gap[len(gap)-1].v["C04"]) {
		t.Fatal("a missing return inside the 250 window must give NaN")
	}
}

func TestBookFlagsDoNotSeeToday(t *testing.T) {
	net := make([]float64, 200)
	for i := range net {
		net[i] = 0.001 * float64(i%9-4)
	}
	a1, a2 := bookNetFlags(net)
	net[150] = -9
	b1, b2 := bookNetFlags(net)
	for i := 0; i <= 150; i++ {
		if a1[i] != b1[i] || a2[i] != b2[i] {
			t.Fatalf("flag at %d changed by today's return", i)
		}
	}
	changed := false
	for i := 151; i < 200; i++ {
		if a2[i] != b2[i] {
			changed = true
		}
	}
	if !changed {
		t.Fatal("later flags must react to the large loss")
	}
}

func TestTimingSeriesZeroMeanExposureEffect(t *testing.T) {
	flags := []bool{true, false, true, false}
	net := []float64{1, 1, 1, 1}
	ts := timingSeries(flags, net)
	if math.Abs(mean(ts)) > 1e-12 {
		t.Fatalf("timing benefit of an uninformative rule must be zero: %v", mean(ts))
	}
}

func TestPlugInLagGrowsWithPersistence(t *testing.T) {
	a := make([]float64, 400)
	b := make([]float64, 400)
	for i := range a {
		a[i] = math.Sin(float64(i) * 2.3)
		b[i] = math.Sin(float64(i) / 20)
	}
	if plugInLag(b) <= plugInLag(a) {
		t.Fatalf("slow series must get the longer bandwidth: %d vs %d", plugInLag(b), plugInLag(a))
	}
}

func TestPeerLookupUsesSharedGrid(t *testing.T) {
	peers := map[int]map[string][]string{260: {"A": {"B"}}, 280: {"A": {"C"}}}
	if p := peerLookup(peers, 275); p == nil || p["A"][0] != "B" {
		t.Fatal("date 275 must use the refresh at 260")
	}
	if p := peerLookup(peers, 281); p == nil || p["A"][0] != "C" {
		t.Fatal("date 281 must use the refresh at 280")
	}
	if peerLookup(peers, 259) != nil {
		t.Fatal("no peers before the first refresh")
	}
}

func TestMacroDeltaRealPrintsOnly(t *testing.T) {
	dir := t.TempDir()
	body := "date,value\n2026-01-02,100\n2026-01-05,100\n2026-01-06,110\n2026-01-09,121\n"
	for _, nm := range fundMacro {
		if err := os.WriteFile(filepath.Join(dir, nm+".csv"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dates := []string{"2026-01-02", "2026-01-05", "2026-01-06", "2026-01-07", "2026-01-08", "2026-01-09", "2026-01-12"}
	d := readMacroDeltas(dir, dates)["brent"]
	if finite(d[2]) {
		t.Fatal("a repeated value is not a real print")
	}
	if !finite(d[3]) || math.Abs(d[3]-math.Log(1.1)) > 1e-12 {
		t.Fatalf("print dated the previous trading day must reach the next day: %v", d[3])
	}
	if finite(d[4]) {
		t.Fatal("no new print on 2026-01-08")
	}
	if finite(d[5]) {
		t.Fatal("a print dated on the trading day itself is not yet known")
	}
	if !finite(d[6]) || math.Abs(d[6]-math.Log(1.1)) > 1e-12 {
		t.Fatalf("print of 2026-01-09 must reach 2026-01-12: %v", d[6])
	}
}

func TestHoleWeekdaysAreInsertedAndBreakWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "m.csv")
	hdr := "date,secid,div_drop,rb1,OpenDay,High,Low,CloseDay,ValDay,A2,LevAsym\n"
	rows := hdr + "2022-02-24,AAA,0,0.01,10,11,9,10,100,0,0\n2022-03-29,AAA,0,,10,11,9,10,100,0,0\n2022-03-30,AAA,0,0.01,10,11,9,10,100,0,0\n"
	if err := os.WriteFile(path, []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	dates, series, _ := readOHLC(path)
	if len(dates) < 20 {
		t.Fatalf("hole weekdays missing: %d dates", len(dates))
	}
	s := series["AAA"]
	i24 := sortSearch(dates, "2022-02-24")
	i30 := sortSearch(dates, "2022-03-30")
	if fw(s.r, i30, i30-i24+1) != nil {
		t.Fatal("a window across the hole must be NaN")
	}
}

func TestE06OnlyWhenMonthChangesWithinTen(t *testing.T) {
	nd := 700
	s := &fser{secid: "X", adm: make([]bool, nd), r: nanSlice(nd), ab: nanSlice(nd), op: nanSlice(nd), hi: nanSlice(nd), lo: nanSlice(nd), cl: nanSlice(nd), val: nanSlice(nd), a2: nanSlice(nd), lev: nanSlice(nd)}
	mk := make([]float64, nd)
	cal := fcal{wday: make([]int, nd), mday: make([]int, nd), tom: make([]bool, nd), monthKey: make([]string, nd)}
	for d := 0; d < nd; d++ {
		s.adm[d] = true
		s.r[d], s.ab[d] = 0.001*float64(d%5-2), 0.001*float64(d%5-2)
		s.cl[d], s.val[d] = 100, 1e6
		s.hi[d], s.lo[d], s.op[d] = 101, 99, 100
		cal.monthKey[d] = fmt.Sprintf("m%02d", d/21)
		cal.tom[d] = d%21 < 3
		cal.wday[d] = d % 5
		cal.mday[d] = (d % 21) / 7
	}
	md := map[string][]float64{}
	for _, nm := range fundMacro {
		md[nm] = nanSlice(nd)
	}
	recs := fundSeries(s, mk, cal, md)
	var withFlag, without *frec
	for _, r := range recs {
		if r.di == 590 {
			without = r
		}
		if r.di == 605 {
			withFlag = r
		}
	}
	if finite(without.v["E06"]) {
		t.Fatal("day 590 has no month change within ten days")
	}
	if !finite(withFlag.v["E06"]) {
		t.Fatal("day 605 has a month change within ten days (month ends at 608)")
	}
}

func TestRevSignalLongsGapDowns(t *testing.T) {
	ds := &dataset{dates: []string{"2025-01-03"}}
	t0 := &tfmCtx{ds: ds, cv: map[string][]float64{}, col: map[string]int{"B05": 0, "B04": 1, "B12": 2}}
	var rows []*row
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("S%02d", i)
		rows = append(rows, &row{secid: id})
		t0.cv["2025-01-03|"+id] = []float64{float64(i), float64(i), float64(i)}
	}
	sig := revSignal(t0, 0, rows, "gap")
	if !(sig[0] > sig[39]) {
		t.Fatalf("the most negative gap must get the highest signal: %v %v", sig[0], sig[39])
	}
	bl := revSignal(t0, 0, rows, "blend")
	if !(bl[0] > bl[39]) {
		t.Fatal("blend must also long gap-downs")
	}
}

func TestBlendDropsNameWithOneMissingMember(t *testing.T) {
	ds := &dataset{dates: []string{"2025-01-03"}}
	t0 := &tfmCtx{ds: ds, cv: map[string][]float64{}, col: map[string]int{"B05": 0, "B04": 1, "B12": 2}}
	var rows []*row
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("S%02d", i)
		rows = append(rows, &row{secid: id})
		t0.cv["2025-01-03|"+id] = []float64{float64(i), float64(i), float64(i)}
	}
	t0.cv["2025-01-03|S07"][1] = nan
	sig := revSignal(t0, 0, rows, "blend")
	if !math.IsNaN(sig[7]) {
		t.Fatal("a name with one missing member must be excluded from BLEND")
	}
	if g := revSignal(t0, 0, rows, "gap"); math.IsNaN(g[7]) {
		t.Fatal("GAP needs only B05")
	}
}

func TestPermuteSignalKeepsMarginalOnly(t *testing.T) {
	sig := make([]float64, 60)
	for i := range sig {
		sig[i] = float64(i)
	}
	sig[5] = nan
	orig := append([]float64{}, sig...)
	permuteSignal(sig, 7, 3)
	if !math.IsNaN(sig[5]) {
		t.Fatal("missing positions must stay missing")
	}
	var a, b []float64
	moved := 0
	for i := range sig {
		if math.IsNaN(orig[i]) {
			continue
		}
		a, b = append(a, orig[i]), append(b, sig[i])
		if sig[i] != orig[i] {
			moved++
		}
	}
	if moved < 40 {
		t.Fatalf("permutation must scramble the cross-section, moved %d", moved)
	}
	sortFloats := func(v []float64) []float64 { sort.Float64s(v); return v }
	x, y := sortFloats(a), sortFloats(b)
	for i := range x {
		if x[i] != y[i] {
			t.Fatal("marginal distribution must be preserved")
		}
	}
}

func TestRevRateOK(t *testing.T) {
	if !revRateOK(1.0) || !revRateOK(0.5) || !revRateOK(0.3) || revRateOK(0.05) {
		t.Fatal("only the frozen reversal rates are allowed")
	}
}
