package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"

	"github.com/olegsidorkin/moex-trader/internal/model"
)

const (
	topK         = 3
	randomDraws  = 20
	sideZoneLow  = 0.40
	sideZoneHigh = 0.60
	minRatio     = 1.5
	minSignAgree = 0.70
	defaultSeed  = 42
	defaultModel = "ensemble_model.json"
	defaultFrom  = "2025-04-01"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("explaincheck", flag.ContinueOnError)
	modelPath := fs.String("model", defaultModel, "ensemble model JSON")
	datasetPath := fs.String("dataset", "", "exportdataset CSV")
	from := fs.String("from", defaultFrom, "first decision date YYYY-MM-DD (inclusive)")
	seed := fs.Int64("seed", defaultSeed, "random seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *datasetPath == "" {
		return fmt.Errorf("-dataset is required")
	}
	m, err := model.LoadEnsembleModel(*modelPath)
	if err != nil {
		return err
	}
	f, err := os.Open(*datasetPath)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return err
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	dateCol, ok := col["date"]
	if !ok {
		return fmt.Errorf("dataset has no date column")
	}
	idx := make([]int, len(m.FeatureOrder))
	for i, name := range m.FeatureOrder {
		c, ok := col[name]
		if !ok {
			return fmt.Errorf("dataset has no column %q", name)
		}
		idx[i] = c
	}
	trained := trainedFeatureIndexes(m.FeatureOrder)
	rng := rand.New(rand.NewSource(*seed))

	var rows, zoneRows, agree int
	var sumTop, sumRand float64
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if rec[dateCol] < *from {
			continue
		}
		vector := make([]float64, len(idx))
		valid := true
		for i, c := range idx {
			v, err := strconv.ParseFloat(rec[c], 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				valid = false
				break
			}
			vector[i] = v
		}
		if !valid {
			continue
		}
		for i, name := range m.FeatureOrder {
			if name == "news_sentiment" || name == "news_count" {
				vector[i] = 0
			}
		}
		base := m.Probability(vector)
		drivers := m.Drivers(model.EnsembleMemberAll, vector, topK)
		if len(drivers) < topK {
			continue
		}
		topIdx := make([]int, 0, topK)
		for _, d := range drivers {
			for i, name := range m.FeatureOrder {
				if name == d.Feature {
					topIdx = append(topIdx, i)
				}
			}
		}
		sumTop += math.Abs(base - m.Probability(replaceWithMean(m, vector, topIdx)))
		var rowRand float64
		for k := 0; k < randomDraws; k++ {
			pick := rng.Perm(len(trained))[:topK]
			chosen := make([]int, topK)
			for j, p := range pick {
				chosen[j] = trained[p]
			}
			rowRand += math.Abs(base - m.Probability(replaceWithMean(m, vector, chosen)))
		}
		sumRand += rowRand / randomDraws
		rows++
		if base <= sideZoneLow || base >= sideZoneHigh {
			zoneRows++
			if (drivers[0].Delta > 0) == (base > 0.5) {
				agree++
			}
		}
	}
	if rows == 0 || zoneRows == 0 {
		return fmt.Errorf("no usable rows (rows=%d, decision-zone rows=%d)", rows, zoneRows)
	}
	ratio := sumTop / sumRand
	signShare := float64(agree) / float64(zoneRows)
	fmt.Printf("rows=%d mean_abs_dp_top%d=%.5f mean_abs_dp_random%d=%.5f ratio=%.3f (need >= %.1f)\n",
		rows, topK, sumTop/float64(rows), topK, sumRand/float64(rows), ratio, minRatio)
	fmt.Printf("decision_zone_rows=%d sign_agreement=%.3f (need >= %.2f)\n", zoneRows, signShare, minSignAgree)
	if ratio >= minRatio && signShare >= minSignAgree {
		fmt.Println("verdict: PASS")
	} else {
		fmt.Println("verdict: FAIL")
	}
	return nil
}

func trainedFeatureIndexes(order []string) []int {
	var out []int
	for i, name := range order {
		if name == "news_sentiment" || name == "news_count" {
			continue
		}
		out = append(out, i)
	}
	return out
}

func replaceWithMean(m *model.EnsembleModel, vector []float64, positions []int) []float64 {
	probe := append([]float64(nil), vector...)
	for _, p := range positions {
		probe[p] = m.Logistic.Mean[p]
	}
	return probe
}
