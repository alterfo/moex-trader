package main

import (
	"flag"
	"log"
)

func main() {
	mode := flag.String("mode", "run", "fetch or run")
	macroDir := flag.String("macro", "data/macro-long", "macro series directory")
	series := flag.String("series", "work/alphafit-series.csv", "alphafit dump")
	matrix := flag.String("matrix", "work/matrix.csv", "flowscan matrix")
	news := flag.String("news", "data/news_history.jsonl", "news history")
	flag.Parse()
	switch *mode {
	case "fetch":
		if err := fetchAll(*macroDir); err != nil {
			log.Fatal(err)
		}
	case "run":
		if err := run(*macroDir, *series, *matrix, *news); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown mode %s", *mode)
	}
}
