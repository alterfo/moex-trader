package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
)

type frozenModels struct {
	Names        []string  `json:"names"`
	Ridge        []float64 `json:"ridge"`
	EN           []float64 `json:"en"`
	A2           []float64 `json:"a2"`
	Hyper        frozen    `json:"hyper"`
	TrainedFrom  string    `json:"trained_from"`
	TrainedUntil string    `json:"trained_until"`
	Rows         float64   `json:"rows"`
}

func freezeModels(pp *pipe, frzPath, out string) {
	raw, err := os.ReadFile(frzPath)
	if err != nil {
		log.Fatal(err)
	}
	var fz frozen
	json.Unmarshal(raw, &fz)
	t0 := sort.SearchStrings(pp.ds.dates, "2020-04-01")
	last := len(pp.ds.dates) - 1
	for last > 0 {
		ok := false
		for _, x := range pp.ds.byDate[last] {
			if !math.IsNaN(x.y10) {
				ok = true
				break
			}
		}
		if ok {
			break
		}
		last--
	}
	m := pp.fit(pp.gr, t0, last, fz)
	names := append([]string{}, pp.ds.names...)
	names = append(names, "ctrl_rb1", "ctrl_rb5", "ctrl_rb20")
	fm := frozenModels{Names: names, Ridge: m.ridge, EN: m.en, A2: m.a2, Hyper: fz, TrainedFrom: pp.ds.dates[t0], TrainedUntil: pp.ds.dates[last], Rows: sumGram(pp.gr, t0, last).n}
	b, _ := json.MarshalIndent(fm, "", "  ")
	if err := os.WriteFile(out, b, 0o644); err != nil {
		log.Fatal(err)
	}
	h := sha256.Sum256(b)
	fmt.Printf("frozen models written to %s: trained %s..%s on %.0f rows, %d inputs, SHA-256 %s\n", out, fm.TrainedFrom, fm.TrainedUntil, fm.Rows, len(names), hex.EncodeToString(h[:]))
}

func forward(pp *pipe, modelsPath, fromDate string) {
	raw, err := os.ReadFile(modelsPath)
	if err != nil {
		log.Fatal(err)
	}
	var fm frozenModels
	if err := json.Unmarshal(raw, &fm); err != nil {
		log.Fatal(err)
	}
	if len(fm.Names) != pp.p {
		log.Fatalf("alphafit: model has %d inputs, matrix gives %d", len(fm.Names), pp.p)
	}
	for i := 0; i < pp.nf; i++ {
		if fm.Names[i] != pp.ds.names[i] {
			log.Fatalf("alphafit: input %d is %q in the model and %q in the matrix", i, fm.Names[i], pp.ds.names[i])
		}
	}
	lo := sort.SearchStrings(pp.ds.dates, fromDate)
	hi := len(pp.ds.dates) - 1
	r := newRunner(pp, fm.Hyper, []string{"blend", "a2"})
	for d := lo; d <= hi; d++ {
		rows := pp.ds.byDate[d]
		n := len(rows)
		preds := map[string][]float64{"blend": make([]float64, n), "a2": make([]float64, n)}
		for i, x := range rows {
			preds["blend"][i] = 0.5 * (predict(fm.Ridge, x.feat) + predict(fm.EN, x.feat))
			preds["a2"][i] = predict(fm.A2, x.feat)
		}
		r.day(d, preds)
	}
	split := lo + (hi-lo)/2
	const draws = 5000
	fmt.Printf("forward: %s..%s (%d dates), models trained through %s\n", pp.ds.dates[lo], pp.ds.dates[hi], hi-lo+1, fm.TrainedUntil)
	report("IC blend", r.out.ic["blend"], split, draws)
	report("IC a2", r.out.ic["a2"], split, draws)
	report("IC blend minus a2", diff(r.out.ic["blend"], r.out.ic["a2"]), split, draws)
	report("net/day GP blend (17%)", r.out.netGP["blend"], split, draws)
	report("net/day GP blend (25%)", r.out.netGP25["blend"], split, draws)
	report("net/day daily blend (17%)", r.out.netD["blend"], split, draws)
}
