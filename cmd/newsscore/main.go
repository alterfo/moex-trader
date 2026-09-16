package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/shopspring/decimal"
)

type rawRecord struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
}

type finanalysRecord struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
	Sentiment   float64 `json:"sentiment"`
	Confidence  float64 `json:"confidence"`
	Type        string  `json:"type"`
	Reason      string  `json:"reason"`
}

func main() {
	input := flag.String("in", "", "input raw JSONL")
	output := flag.String("out", "", "output finanalys-format JSONL")
	method := flag.String("method", "none", "scoring method: lexicon|regex|none|model")
	modelPath := flag.String("model-path", "news_classifier.json", "trained news classifier path (used when -method=model)")
	flag.Parse()

	if *input == "" || *output == "" {
		log.Fatal("both -in and -out are required")
	}

	var scorer func(string) decimal.Decimal
	switch *method {
	case "lexicon":
		scorer = features.PolarityScore
	case "regex":
		scorer = features.RegexScore
	case "none":
		scorer = features.NoneScore
	case "model":
		weights, err := model.LoadNewsClassifier(*modelPath)
		if err != nil {
			log.Fatalf("load news classifier %q: %v", *modelPath, err)
		}
		scorer = weights.Scorer()
	default:
		log.Fatalf("unknown method %q (want lexicon|regex|none|model)", *method)
	}

	inFile, err := os.Open(*input)
	if err != nil {
		log.Fatal(err)
	}
	defer inFile.Close()

	outFile, err := os.Create(*output)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()
	w := bufio.NewWriter(outFile)

	scanner := bufio.NewScanner(inFile)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	n := 0
	for scanner.Scan() {
		var r rawRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		sent := scorer(r.Title)
		conf := 0.5
		if !sent.IsZero() {
			conf = 0.7
		}
		o := finanalysRecord{
			Ticker:      r.Ticker,
			ArticleID:   r.ArticleID,
			Title:       r.Title,
			Source:      r.Source,
			TrustWeight: r.TrustWeight,
			PublishedTS: r.PublishedTS,
			Sentiment:   sent.InexactFloat64(),
			Confidence:  conf,
			Type:        "news",
			Reason:      "newsscore:" + *method,
		}
		line, _ := json.Marshal(o)
		w.Write(line)
		w.WriteByte('\n')
		n++
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "%d records scored with %s → %s\n", n, *method, *output)
}
