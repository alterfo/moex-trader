package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
)

var nan = math.NaN()

type rec struct {
	date, secid, asset                                 string
	valDay, closeMain, openMain                        float64
	bi, ti, nof, ci, depth, relSpread, ticket, otr, rv float64
	nJoined                                            int
	lateShare                                          float64
	lateTS, lateOB, lateOS                             float64
}

type store struct {
	dates    []string
	dIdx     map[string]int
	byGroup  map[string][]*rec
	closes   map[string][]float64
	lastIdx  map[string]int
	groups   []string
	excl     []bool
	frontSec map[string][]string
	rollAge  map[string][]int
	lateTSBy map[string][]float64
	expCache map[string]int
}

func f64(s string) float64 {
	if s == "" {
		return nan
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nan
	}
	return v
}

func loadStore(path string, perSecidGroup bool) (*store, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	ix := map[string]int{}
	for i, h := range hdr {
		ix[h] = i
	}
	var all []*rec
	dset := map[string]bool{}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		g := func(n string) float64 { return f64(row[ix[n]]) }
		x := &rec{date: row[ix["date"]], secid: row[ix["secid"]], asset: row[ix["asset"]],
			valDay: g("val_day"), closeMain: g("close_main"), openMain: g("open_main"),
			bi: g("bi"), ti: g("ti"), nof: g("nof"), ci: g("ci"), depth: g("depth"), relSpread: g("rel_spread"),
			ticket: g("ticket"), otr: g("otr"), rv: g("rv"), lateShare: g("late_share"), lateTS: g("late_ts"), lateOB: g("late_ob"), lateOS: g("late_os")}
		n, _ := strconv.Atoi(row[ix["n_joined"]])
		x.nJoined = n
		if perSecidGroup {
			x.asset = x.secid
		}
		all = append(all, x)
		dset[x.date] = true
	}
	s := &store{dIdx: map[string]int{}, byGroup: map[string][]*rec{}, closes: map[string][]float64{}, lastIdx: map[string]int{}, frontSec: map[string][]string{}, rollAge: map[string][]int{}, lateTSBy: map[string][]float64{}, expCache: map[string]int{}}
	for d := range dset {
		s.dates = append(s.dates, d)
	}
	sort.Strings(s.dates)
	for i, d := range s.dates {
		s.dIdx[d] = i
		s.excl = append(s.excl, d >= "2022-02-25" && d <= "2022-03-28")
	}
	nd := len(s.dates)
	best := map[string][]*rec{}
	for _, x := range all {
		di := s.dIdx[x.date]
		if x.closeMain > 0 {
			c := s.closes[x.secid]
			if c == nil {
				c = make([]float64, nd)
				for i := range c {
					c[i] = nan
				}
				s.closes[x.secid] = c
			}
			c[di] = x.closeMain
			lt := s.lateTSBy[x.secid]
			if lt == nil {
				lt = make([]float64, nd)
				for i := range lt {
					lt[i] = 1
				}
				s.lateTSBy[x.secid] = lt
			}
			lt[di] = x.lateTS
			if di > s.lastIdx[x.secid] {
				s.lastIdx[x.secid] = di
			}
		}
		if x.valDay < 1e6 || !(x.closeMain > 0) {
			continue
		}
		b := best[x.asset]
		if b == nil {
			b = make([]*rec, nd)
			best[x.asset] = b
		}
		if b[di] == nil || x.valDay > b[di].valDay {
			b[di] = x
		}
	}
	for g := range best {
		s.groups = append(s.groups, g)
	}
	sort.Strings(s.groups)
	s.byGroup = best
	for _, g := range s.groups {
		fs := make([]string, nd)
		ra := make([]int, nd)
		prev := ""
		age := 99
		for d := 0; d < nd; d++ {
			x := best[g][d]
			if x == nil {
				continue
			}
			fs[d] = x.secid
			if prev != "" && x.secid != prev {
				age = 0
			} else if prev != "" {
				age++
			}
			ra[d] = age
			prev = x.secid
		}
		s.frontSec[g] = fs
		s.rollAge[g] = ra
	}
	return s, nil
}

func (s *store) expired(secid string, d int) bool { return false }

func (s *store) ret(g string, d, h int) float64 {
	x := s.byGroup[g][d]
	if x == nil || d+h >= len(s.dates) || s.expired(x.secid, d) {
		return nan
	}
	for k := d; k <= d+h; k++ {
		if s.excl[k] {
			return nan
		}
	}
	c := s.closes[x.secid]
	a, b := c[d], c[d+h]
	if !(a > 0) || !(b > 0) {
		return nan
	}
	lt := s.lateTSBy[x.secid]
	if !allowLate && (lt[d] > 0.2 || lt[d+h] > 0.2) {
		return nan
	}
	return b/a - 1
}

func (s *store) ret1Back(g string, d int) float64 {
	if d < 1 {
		return nan
	}
	x := s.byGroup[g][d]
	if x == nil {
		return nan
	}
	c := s.closes[x.secid]
	a, b := c[d-1], c[d]
	if !(a > 0) || !(b > 0) {
		return nan
	}
	return b/a - 1
}

func (s *store) sigma60(g string, d int) float64 {
	var v []float64
	for k := d - 59; k <= d; k++ {
		if k < 1 || s.excl[k] {
			continue
		}
		if r := s.ret1Back(g, k); !math.IsNaN(r) {
			v = append(v, r)
		}
	}
	if len(v) < 40 {
		return nan
	}
	return stdOf(v)
}
