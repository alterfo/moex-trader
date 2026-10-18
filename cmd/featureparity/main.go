package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

type pair struct {
	Live domain.FeatureContext `json:"live"`
	DS   map[string]string     `json:"ds"`
	LLM  float64               `json:"llm_p"`
}

var masked = map[string]bool{"news_count": true, "news_sentiment": true}

func main() {
	modelPath := flag.String("model", "ensemble_model.json", "ensemble model JSON")
	pairsPath := flag.String("pairs", "", "JSONL of {live: FeatureContext, ds: {feature: value}}")
	flag.Parse()
	m, err := model.LoadEnsembleModel(*modelPath)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Open(*pairsPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	order := m.FeatureOrder
	var liveP, dsP, llmP, unmaskedP float64
	n := 0
	swapIntoLive := make(map[string]float64)
	swapIntoDS := make(map[string]float64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		var p pair
		if err := json.Unmarshal(scanner.Bytes(), &p); err != nil {
			log.Fatal(err)
		}
		lv, err := model.ToVectorFor(p.Live, order)
		if err != nil {
			log.Fatal(err)
		}
		unmaskedP += m.Probability(lv)
		llmP += p.LLM
		dv := make([]float64, len(order))
		for i, name := range order {
			if masked[name] {
				lv[i] = 0
				continue
			}
			if v, ok := p.DS[name]; ok {
				x, err := strconv.ParseFloat(v, 64)
				if err == nil {
					dv[i] = x
				}
			}
		}
		for i, name := range order {
			if masked[name] {
				dv[i] = 0
			}
			if _, ok := p.DS[name]; !ok {
				dv[i] = lv[i]
			}
		}
		pl := m.Probability(lv)
		pd := m.Probability(dv)
		liveP += pl
		dsP += pd
		n++
		for i, name := range order {
			if lv[i] == dv[i] {
				continue
			}
			a := append([]float64(nil), lv...)
			a[i] = dv[i]
			swapIntoLive[name] += m.Probability(a) - pl
			b := append([]float64(nil), dv...)
			b[i] = lv[i]
			swapIntoDS[name] += m.Probability(b) - pd
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("mean logged live p=%.4f, model on unmasked ingest vectors=%.4f\n", llmP/float64(n), unmaskedP/float64(n))
	fmt.Printf("pairs=%d mean p on live vectors=%.4f on dataset vectors=%.4f gap=%.4f\n", n, liveP/float64(n), dsP/float64(n), (liveP-dsP)/float64(n))
	names := make([]string, 0, len(swapIntoLive))
	for k := range swapIntoLive {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return swapIntoLive[names[i]] > swapIntoLive[names[j]] })
	fmt.Println("feature | mean change in p when the dataset value replaces the live value (live vector) | when the live value replaces the dataset value (dataset vector)")
	for _, k := range names {
		fmt.Printf("%-34s %+.4f %+.4f\n", k, swapIntoLive[k]/float64(n), swapIntoDS[k]/float64(n))
	}
}
