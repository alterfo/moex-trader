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

type blockCol struct {
	name string
	fn   featFn
}

func blockCols() []blockCol {
	return []blockCol{
		{"ap_a2", col(bA2)},
		{"ap_a2_m5", m5(bA2)},
		{"ap_a1", col(bA1)},
		{"ap_a4", col(bA4)},
		{"ap_a5", col(bA5)},
		{"ap_lev_asym", col(bLevAsym)},
		{"ap_spread_ratio", logRatioM(bSpLv, 20, 10)},
		{"ap_depth_growth", logRatioM(bDepth, 20, 10)},
		{"ap_ticket_z", zPrev(bTicket, lg)},
		{"ap_otr_z", zPrev(bPutOverVal, ident)},
		{"ap_morn_a1", col(bMornA1)},
		{"ap_auc_a1", col(bAucA1)},
	}
}

var production18 = strings.Split("YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI,DATA", ",")

func (r *run) exportBlock(path string, realtimeFrom string) {
	p := r.p
	cols := blockCols()
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("flowscan: %v", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	hdr := []string{"date", "secid", "t18", "adm100", "realtime", "div_drop", "close"}
	for _, c := range cols {
		hdr = append(hdr, c.name)
	}
	fmt.Fprintln(w, strings.Join(hdr, ","))
	t18 := map[int]bool{}
	for _, s := range production18 {
		if i, ok := p.tIdx[s]; ok {
			t18[i] = true
		}
	}
	rows := 0
	for d := range p.dates {
		if d < 61 || p.excluded[d] {
			continue
		}
		adm := p.admittedAt(d)
		in100 := map[int]bool{}
		for k, i := range adm {
			if k < 100 {
				in100[i] = true
			}
		}
		set := map[int]bool{}
		var ids []int
		for _, i := range adm {
			set[i] = true
			ids = append(ids, i)
		}
		for i := range t18 {
			if !set[i] && p.base[i][d] != nil {
				ids = append(ids, i)
			}
		}
		vals := make([][]float64, len(cols))
		for c := range cols {
			vals[c] = cols[c].fn(p, d, ids)
		}
		rt := 0
		if p.dates[d] >= realtimeFrom {
			rt = 1
		}
		for k, i := range ids {
			a := p.base[i][d]
			if a == nil {
				continue
			}
			b2i := func(b bool) int {
				if b {
					return 1
				}
				return 0
			}
			line := []string{p.dates[d], p.tickers[i], strconv.Itoa(b2i(t18[i])), strconv.Itoa(b2i(in100[i])), strconv.Itoa(rt), strconv.Itoa(b2i(p.divDrop[i][d])), strconv.FormatFloat(float64(a[bCloseDay]), 'g', 10, 64)}
			for c := range cols {
				v := vals[c][k]
				if math.IsNaN(v) || math.IsInf(v, 0) {
					line = append(line, "")
				} else {
					line = append(line, strconv.FormatFloat(v, 'g', 8, 64))
				}
			}
			fmt.Fprintln(w, strings.Join(line, ","))
			rows++
		}
	}
	log.Printf("flowscan: export wrote %d rows to %s", rows, path)
}
