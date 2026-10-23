package main

import (
	"flag"
	"log"
)

func main() {
	mode := flag.String("mode", "run", "fetch, fetch2, run or wave2")
	macroDir := flag.String("macro", "data/macro-long", "macro series directory")
	series := flag.String("series", "work/alphafit-series.csv", "alphafit dump")
	exA2 := flag.String("exa2", "work/alphafit-series-exA2.csv", "alphafit dump with the A2 family zeroed")
	matrix := flag.String("matrix", "work/matrix.csv", "flowscan matrix")
	news := flag.String("news", "data/news_history.jsonl", "news history")
	flag.Parse()
	switch *mode {
	case "fetch":
		if err := fetchAll(*macroDir); err != nil {
			log.Fatal(err)
		}
	case "fetch2":
		if err := fetchWave2(*macroDir); err != nil {
			log.Fatal(err)
		}
	case "run":
		if err := run(*macroDir, *series, *matrix, *news, false, *exA2); err != nil {
			log.Fatal(err)
		}
	case "wave2":
		if err := run(*macroDir, *series, *matrix, *news, true, *exA2); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown mode %s", *mode)
	}
}
