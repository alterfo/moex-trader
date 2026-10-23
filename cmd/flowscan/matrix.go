package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
)

var baseNames = []string{
	"ValDay",
	"OpenDay",
	"CloseDay",
	"Open10",
	"Close10",
	"Close1830",
	"High",
	"Low",
	"NJ",
	"A1",
	"A2",
	"ImbVol",
	"ImbBBOVal",
	"ImbBBOVol",
	"Depth",
	"LevAsym",
	"ThinBid",
	"SpLv",
	"SpBBO",
	"Sp1m",
	"SpRatio",
	"Ticket",
	"TicketB",
	"TicketS",
	"Top3",
	"Trades",
	"TrCntImb",
	"LogVolRatio",
	"AggGap",
	"CLV",
	"VwapDev",
	"PutOverVal",
	"CanDiff",
	"PlaceAgg",
	"PutSize",
	"A4",
	"A5",
	"CanVwGap",
	"CanOrdRatio",
	"PutOrders",
	"PutOverBook",
	"A2Late",
	"A2Early",
	"A2H1",
	"A2H2",
	"TicketGapLate",
	"AggGapEarly",
	"A4Late",
	"A4Early",
	"ImbStd",
	"SpStd",
	"SpLast",
	"SpEarlyOverMid",
	"BBOoverStd",
	"DepthOpenClose",
	"PeakMin",
	"ShareMid",
	"ShareLast",
	"A1Trades",
	"TicketMed",
	"TicketStd",
	"Amihud",
	"Idle",
	"ShareNeg",
	"A1Tail",
	"RetLast",
	"RetEarly",
	"RV",
	"RSdn",
	"RSup",
	"MaxJump",
	"Park",
	"PrStd",
	"MornA1",
	"MornShare",
	"EvA1",
	"EvA2",
	"EvSpRatio",
	"EvRet",
	"AucA1",
	"VPIN",
	"Kyle",
	"FlowRet",
	"FlowAC",
	"Skew",
	"ImbAC2",
	"ProfTilt",
	"ImbSlope",
}

func (r *run) exportMatrix(path string) {
	p := r.p
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("flowscan: %v", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	hdr := []string{"date", "secid", "rank", "t18", "div_drop", "rb1", "rb5", "rb20", "f1", "f5", "f10", "u1", "u5", "u10"}
	hdr = append(hdr, baseNames...)
	fmt.Fprintln(w, strings.Join(hdr, ","))
	t18 := map[int]bool{}
	for _, s := range production18 {
		if i, ok := p.tIdx[s]; ok {
			t18[i] = true
		}
	}
	fm := func(x float64) string {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return ""
		}
		return strconv.FormatFloat(x, 'g', 8, 64)
	}
	rows := 0
	for d := range p.dates {
		if d < 61 || p.excluded[d] {
			continue
		}
		adm := p.admittedAt(d)
		var u [3]float64
		var un [3]int
		hs := [3]int{1, 5, 10}
		for _, i := range adm {
			for k, h := range hs {
				if x := p.ret(i, d, d+h); !math.IsNaN(x) {
					u[k] += x
					un[k]++
				}
			}
		}
		for k := range u {
			if un[k] >= 40 {
				u[k] /= float64(un[k])
			} else {
				u[k] = math.NaN()
			}
		}
		for k, i := range adm {
			if k >= 100 {
				break
			}
			b := p.base[i][d]
			if b == nil {
				continue
			}
			dd := 0
			if p.divDrop[i][d] {
				dd = 1
			}
			t := 0
			if t18[i] {
				t = 1
			}
			line := []string{p.dates[d], p.tickers[i], strconv.Itoa(k + 1), strconv.Itoa(t), strconv.Itoa(dd),
				fm(p.ret(i, d-1, d)), fm(p.ret(i, d-5, d)), fm(p.ret(i, d-20, d)),
				fm(p.ret(i, d, d+1)), fm(p.ret(i, d, d+5)), fm(p.ret(i, d, d+10)), fm(u[0]), fm(u[1]), fm(u[2])}
			for c := 0; c < nb; c++ {
				line = append(line, fm(float64(b[c])))
			}
			fmt.Fprintln(w, strings.Join(line, ","))
			rows++
		}
	}
	log.Printf("flowscan: matrix wrote %d rows to %s", rows, path)
}
