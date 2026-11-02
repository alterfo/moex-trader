package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
)

type lvlState struct {
	day        string
	sessionHi  float64
	sessionLo  float64
	prevHi     []float64
	prevLo     []float64
	atr        float64
	volWindow  []float64
	acted      map[float64]bool
	posDir     float64
	entry      float64
	entryT     int64
	stop       float64
	stopAtr    float64
	entriesDay int
}

func runLVL(o options, w sessionWindow, contracts []string) error {
	vol := loadDailyVolumeOrDie(o.dailyDir, contracts)
	fundEv := loadFundingEvents(filepath.Join(o.dailyDir, "funding"), contracts)
	var trades []tradeRec
	for _, sym := range contracts {
		st := lvlState{acted: map[float64]bool{}, prevHi: nil, prevLo: nil}
		var prevClose float64
		closePos := func(price float64, ts int64) {
			if st.posDir == 0 {
				return
			}
			ret := st.posDir * (price/st.entry - 1) * 1e4
			c := allInCostBps(o, prevVolume(vol, sym, st.day))
			f := fundingBetween(fundEv[sym], st.entryT, ts) * st.posDir
			trades = append(trades, tradeRec{
				symbol: sym, date: st.day, dir: st.posDir, entry: st.entry, exit: price,
				gross: ret, cost: 2 * c, fund: -f, net: ret - 2*c - f,
				hold: int((ts - st.entryT) / 60000),
			})
			st.posDir = 0
		}
		err := streamBars(barFilePath(o.barDir, sym), func(b bar) bool {
			m := minuteOfDay(b.t)
			if m < w.startMin || m > w.endMin {
				if st.posDir != 0 {
					closePos(b.open, b.t)
				}
				return true
			}
			d := dateOf(b.t)
			if d != st.day {
				closePos(b.open, b.t)
				if st.sessionHi > 0 {
					st.prevHi = append(st.prevHi, st.sessionHi)
					st.prevLo = append(st.prevLo, st.sessionLo)
					if len(st.prevHi) > 7 {
						st.prevHi = st.prevHi[1:]
						st.prevLo = st.prevLo[1:]
					}
				}
				st.day = d
				st.sessionHi = 0
				st.sessionLo = 0
				st.atr = 0
				st.volWindow = nil
				st.acted = map[float64]bool{}
				st.entriesDay = 0
			}
			if st.sessionHi == 0 {
				st.sessionHi = b.high
				st.sessionLo = b.low
			} else {
				st.sessionHi = math.Max(st.sessionHi, b.high)
				st.sessionLo = math.Min(st.sessionLo, b.low)
			}
			tr := b.high - b.low
			if prevClose > 0 {
				tr = math.Max(tr, math.Max(math.Abs(b.high-prevClose), math.Abs(b.low-prevClose)))
			}
			prevClose = b.close
			if st.atr == 0 {
				st.atr = tr
			} else {
				st.atr = 0.93*st.atr + 0.07*tr
			}
			st.volWindow = append(st.volWindow, b.quoteVol)
			if len(st.volWindow) > 20 {
				st.volWindow = st.volWindow[1:]
			}
			if st.posDir != 0 {
				if st.posDir > 0 && b.low <= st.stop {
					price := st.stop
					if b.open < st.stop {
						price = b.open
					}
					closePos(price, b.t)
					return true
				}
				if st.posDir < 0 && b.high >= st.stop {
					price := st.stop
					if b.open > st.stop {
						price = b.open
					}
					closePos(price, b.t)
					return true
				}
				return true
			}
			if m < w.startMin+w.lenMin || st.entriesDay >= 3 || st.atr <= 0 {
				return true
			}
			levels := levelSet(b.close, st, o)
			for _, lv := range levels {
				if st.acted[lv.level] {
					continue
				}
				band := 0.25 * st.atr
				touched := b.low <= lv.level+band && b.high >= lv.level-band
				if !touched {
					continue
				}
				volMed := median(st.volWindow)
				strong := volMed > 0 && b.quoteVol > 1.5*volMed
				dir := 0.0
				broke := (lv.above && b.close > lv.level) || (!lv.above && b.close < lv.level)
				if broke && strong {
					if lv.above {
						dir = 1
					} else {
						dir = -1
					}
				} else {
					if lv.above {
						dir = -1
					} else {
						dir = 1
					}
				}
				st.acted[lv.level] = true
				st.posDir = dir
				st.entry = b.close
				st.entryT = b.t
				st.stop = b.close - dir*st.atr
				st.entriesDay++
				return true
			}
			return true
		})
		if err != nil {
			continue
		}
	}
	if len(trades) == 0 {
		return fmt.Errorf("lvl %s: no trades", w.name)
	}
	reportCell(w, "O-LVL", trades)
	return nil
}

type level struct {
	level float64
	above bool
}

func levelSet(price float64, st lvlState, o options) []level {
	var out []level
	for _, h := range st.prevHi {
		out = append(out, level{level: h, above: price < h})
	}
	for _, l := range st.prevLo {
		out = append(out, level{level: l, above: price > l})
	}
	if price < 100 {
		step := math.Pow(10, math.Floor(math.Log10(price))-2)
		if step > 0 {
			grid := math.Round(price/step) * step
			out = append(out, level{level: grid, above: price < grid})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		return math.Abs(out[a].level-price) < math.Abs(out[b].level-price)
	})
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	cp := append([]float64(nil), v...)
	sort.Float64s(cp)
	return cp[len(cp)/2]
}
