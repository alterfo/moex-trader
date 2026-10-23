package main

import (
	"fmt"
	"math"
)

type rowSpec struct {
	id, book, kind, feat string
	hor                  int
	sign                 float64
	group, group2        string
	groups               []string
	inter                [2]string
	desc                 string
}

var tsGroups = []string{"Si", "CNY", "GOLD", "BR", "NG", "MIX", "RTS", "Eu", "ED", "SILV"}
var perpGroups = []string{"USDRUBTOM", "CNYRUBTOM", "IMOEX", "GLDRUBTOM", "EURRUBTOM"}
var fxInst = []string{"USD000UTSTOM", "CNYRUB_TOM", "EUR_RUB__TOM"}
var commodityGroups = []string{"BR", "GOLD", "NG", "SILV", "PLT", "PLD"}
var currencyGroups = []string{"Si", "CNY", "Eu"}

func specs() []rowSpec {
	var r []rowSpec
	n := 0
	add := func(prefix string, s rowSpec) {
		n++
		s.id = fmt.Sprintf("%s%02d", prefix, countPrefix(r, prefix)+1)
		r = append(r, s)
	}
	for _, f := range featNames {
		for _, h := range []int{1, 5} {
			add("A", rowSpec{book: "1", kind: "fm", feat: f, hor: h, sign: featSign[f], desc: fmt.Sprintf("%s -> vol-scaled abnormal fo return h%d, FM over liquid groups", f, h)})
		}
	}
	for _, p := range [][2]string{{"BI", "DG"}, {"BI", "TZ"}, {"TI", "SR"}} {
		add("B", rowSpec{book: "1", kind: "fm", feat: p[0] + "x" + p[1], inter: p, hor: 1, sign: 1, desc: fmt.Sprintf("rank(%s) x (rank(%s)+0.5) -> abnormal fo return h1, FM", p[0], p[1])})
	}
	for _, f := range []string{"BI", "TI"} {
		for _, g := range tsGroups {
			add("C", rowSpec{book: "1", kind: "ts", feat: f, group: g, hor: 1, sign: featSign[f], desc: fmt.Sprintf("%s of %s -> own vol-scaled return h1, time series", f, g)})
		}
	}
	for _, f := range []string{"aBI", "DG", "SR", "CI", "OT", "TZ"} {
		for _, h := range []int{1, 5} {
			sg := 1.0
			add("D", rowSpec{book: "1", kind: "vol", feat: f, hor: h, sign: sg, desc: fmt.Sprintf("%s -> log realised volatility h%d, FM with AR controls (two-sided)", f, h)})
		}
	}
	add("E", rowSpec{book: "1", kind: "basis", group: "Si", group2: "USD000UTSTOM", hor: 1, sign: -1, desc: "Si front vs USDRUB spot basis z20 -> next-day basis change"})
	add("E", rowSpec{book: "1", kind: "basis", group: "CNY", group2: "CNYRUB_TOM", hor: 1, sign: -1, desc: "CNY front vs CNYRUB_TOM basis z20 -> next-day basis change"})
	add("E", rowSpec{book: "1", kind: "basis", group: "USDRUBTOM", group2: "USD000UTSTOM", hor: 1, sign: -1, desc: "USDRUBF perpetual vs spot basis z20 -> next-day basis change"})
	for _, f := range []string{"BI", "TI", "NOF", "CI"} {
		for _, in := range fxInst {
			add("F", rowSpec{book: "1", kind: "fxts", feat: f, group: in, hor: 1, sign: featSign[f], desc: fmt.Sprintf("%s of %s -> own vol-scaled return h1, time series", f, in)})
		}
	}
	for _, in := range fxInst {
		add("F", rowSpec{book: "1", kind: "fxvol", feat: "aBI", group: in, hor: 1, sign: 1, desc: fmt.Sprintf("|BI| of %s -> own log realised volatility h1 with AR controls (two-sided)", in)})
	}
	for _, p := range [][3]string{{"Si", "BI", "USD000UTSTOM"}, {"Si", "TI", "USD000UTSTOM"}, {"CNY", "BI", "CNYRUB_TOM"}, {"CNY", "TI", "CNYRUB_TOM"}, {"Eu", "BI", "EUR_RUB__TOM"}} {
		add("G", rowSpec{book: "1", kind: "fut2spot", feat: p[1], group: p[0], group2: p[2], hor: 1, sign: featSign[p[1]], desc: fmt.Sprintf("%s of %s futures -> %s spot vol-scaled return h1, time series", p[1], p[0], p[2])})
	}
	add("H", rowSpec{book: "1", kind: "agg", feat: "BI", groups: commodityGroups, hor: 1, sign: 1, desc: "mean BI of commodity futures -> mean vol-scaled commodity return h1"})
	add("H", rowSpec{book: "1", kind: "agg", feat: "BI", groups: commodityGroups, hor: 5, sign: 1, desc: "mean BI of commodity futures -> mean vol-scaled commodity return h5"})
	add("H", rowSpec{book: "1", kind: "agg", feat: "BI", groups: currencyGroups, hor: 1, sign: 1, desc: "mean BI of currency futures -> mean vol-scaled currency return h1"})
	add("H", rowSpec{book: "1", kind: "agg", feat: "BI", groups: currencyGroups, hor: 5, sign: 1, desc: "mean BI of currency futures -> mean vol-scaled currency return h5"})
	add("H", rowSpec{book: "1", kind: "disp", feat: "BI", hor: 1, sign: 1, desc: "cross-sectional std of BI over liquid groups -> mean absolute vol-scaled return h1 (two-sided)"})
	add("H", rowSpec{book: "1", kind: "breadth", feat: "BI", hor: 1, sign: 1, desc: "share of liquid groups with BI > 0 -> mean vol-scaled return h1"})
	for _, g := range perpGroups {
		add("M", rowSpec{book: "1", kind: "ts", feat: "BI", group: g, hor: 1, sign: 1, desc: fmt.Sprintf("BI of %s (perpetual contract) -> own vol-scaled return h1, time series", g)})
	}
	return r
}

func countPrefix(r []rowSpec, p string) int {
	c := 0
	for _, x := range r {
		if len(x.id) > 0 && x.id[:1] == p {
			c++
		}
	}
	return c
}

func isNaN(x float64) bool { return math.IsNaN(x) || math.IsInf(x, 0) }
