package main

import (
	"math"
	"testing"
)

func near(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestSlotOfCoversMainSession(t *testing.T) {
	cases := map[string]int{"10:05:00": 0, "10:10:00": 1, "13:00:00": 35, "17:00:00": 83, "18:30:00": 101, "18:50:00": 105, "10:00:00": -1, "07:05:00": -1, "18:55:00": -1, "10:07:00": -1}
	for tm, want := range cases {
		if got := slotOf(tm); got != want {
			t.Fatalf("slotOf(%s) = %d, want %d", tm, got, want)
		}
	}
}

func TestOLSRecoversCoefficients(t *testing.T) {
	var x [][]float64
	var y []float64
	for i := 0; i < 50; i++ {
		a, b := float64(i%7), float64((i*3)%11)
		x = append(x, []float64{1, a, b})
		y = append(y, 2+3*a-0.5*b)
	}
	beta, ok := ols(x, y)
	if !ok || !near(beta[0], 2, 1e-5) || !near(beta[1], 3, 1e-5) || !near(beta[2], -0.5, 1e-5) {
		t.Fatalf("beta %v ok %v", beta, ok)
	}
}

func TestNeweyWestDetectsPositiveMean(t *testing.T) {
	x := make([]float64, 200)
	for i := range x {
		x[i] = 0.5 + math.Sin(float64(i)*1.7)
	}
	m, tv := neweyWest(x, 5)
	if !near(m, 0.5, 0.05) || tv < 5 {
		t.Fatalf("mean %f t %f", m, tv)
	}
	if _, tz := neweyWest(make([]float64, 50), 5); !math.IsNaN(tz) {
		t.Fatalf("constant series must give NaN t, got %f", tz)
	}
}

func TestRankScaleHandlesTies(t *testing.T) {
	got := rankScale([]float64{3, 1, 1, 2})
	want := []float64{0.375, -0.25, -0.25, 0.125}
	for i := range want {
		if !near(got[i], want[i], 1e-12) {
			t.Fatalf("rank %v want %v", got, want)
		}
	}
}

func fullWork() *work {
	w := &work{}
	for s := 0; s < nSlots; s++ {
		w.ts[s] = tsBar{open: 100, close: 100, val: 100000, valB: 70000, valS: 30000}
		w.ob[s] = obBar{spread: 2, imb: 0.4, imbBBO: 0.2}
		w.os[s] = osBar{putB: 600, putS: 400, cancB: 100, cancS: 300}
		w.tsOK[s], w.obOK[s], w.osOK[s] = true, true, true
	}
	return w
}

func TestBuildRecFeaturesOnConstantDay(t *testing.T) {
	rec := buildRec(0, "TEST", fullWork())
	if rec.nJoined != slotFeat+1 {
		t.Fatalf("joined %d", rec.nJoined)
	}
	want := [5]float64{0.4, 0.4, 0.2, 0.4, 0.4}
	for i, v := range want {
		if !near(rec.feat[i], v, 1e-6) {
			t.Fatalf("feature %d = %f want %f", i, rec.feat[i], v)
		}
	}
	if rec.bars == nil || !near(rec.mid.tradeImb, 0.4, 1e-6) || rec.late.n != slotLate+1 {
		t.Fatalf("cuts %+v %+v", rec.mid, rec.late)
	}
}

func TestBuildRecRequiresTwentyJoinedBars(t *testing.T) {
	w := fullWork()
	for s := 15; s < nSlots; s++ {
		w.obOK[s] = false
	}
	rec := buildRec(0, "TEST", w)
	if !math.IsNaN(rec.feat[0]) || rec.nJoined != 15 {
		t.Fatalf("joined %d feat %f", rec.nJoined, rec.feat[0])
	}
}

func TestEventsForFindsFirstCrossingAndSignedOutcome(t *testing.T) {
	p := &panel{dates: make([]string, 40), excluded: make([]bool, 40)}
	p.close = [][]float32{make([]float32, 40)}
	for d := 0; d < 40; d++ {
		p.close[0][d] = float32(100 + float64(d%2))
	}
	b := &bars{}
	for s := 0; s < nSlots; s++ {
		b.close[s] = float32(nan)
		b.ts[s] = true
		b.close[s] = 100.5
		b.joined[s] = true
		b.imb[s] = 0.5
		b.putS[s], b.cancS[s] = 100, 30
	}
	for s := 10; s < nSlots; s++ {
		b.close[s] = 102
	}
	rec := &dayRec{bars: b}
	var hour [nSlots]float64
	for s := range hour {
		hour[s] = 0
	}
	evs := p.eventsFor(0, 30, rec, hour)
	var up *event
	for i := range evs {
		if evs[i].real && evs[i].dir > 0 && evs[i].day == 30 {
			up = &evs[i]
			break
		}
	}
	if up == nil {
		t.Fatalf("no real upward event in %+v", evs)
	}
	if !near(up.out, math.Log(102.0/102.0), 1e-9) {
		t.Fatalf("outcome %f", up.out)
	}
	if !near(up.c1, 0.5, 1e-9) || !near(up.c2, 0.3, 1e-6) {
		t.Fatalf("features c1 %f c2 %f", up.c1, up.c2)
	}
}

func TestBootstrapDiffSeparatesGroups(t *testing.T) {
	var top, bot []sample
	for d := 0; d < 60; d++ {
		top = append(top, sample{d, 0.01 + 0.001*float64(d%3)})
		bot = append(bot, sample{d, -0.01 + 0.001*float64(d%3)})
	}
	diff, lo, hi := bootstrapDiff(top, bot, 500, 1, 0.002, 0.998)
	if !near(diff, 0.02, 1e-9) || lo <= 0 || hi < diff {
		t.Fatalf("diff %f ci [%f, %f]", diff, lo, hi)
	}
}

func TestRunDContrarianOnTrendingSeries(t *testing.T) {
	var cal []string
	closes := map[string]float64{}
	hhi := map[string]float64{}
	px := 100.0
	day := 0
	for y := 2021; y <= 2024; y++ {
		for m := 1; m <= 12; m++ {
			for dd := 1; dd <= 20; dd++ {
				d := dateString(y, m, dd)
				cal = append(cal, d)
				px *= 1.0005
				closes[d] = px
				hhi[d] = float64(day%9) + 1
				day++
			}
		}
	}
	o := runD(cal, closes, hhi)
	if o.periods == 0 || math.IsNaN(o.meanDiff) {
		t.Fatalf("no periods: %+v", o)
	}
}

func dateString(y, m, d int) string {
	return string([]byte{byte('0' + y/1000), byte('0' + y/100%10), byte('0' + y/10%10), byte('0' + y%10), '-', byte('0' + m/10), byte('0' + m%10), '-', byte('0' + d/10), byte('0' + d%10)})
}
