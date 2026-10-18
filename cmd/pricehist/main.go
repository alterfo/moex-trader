package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func main() {
	tickers := flag.String("tickers", "YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI,DATA,IMOEX", "comma-separated tickers")
	fromStr := flag.String("from", "2022-01-01", "first day")
	tillStr := flag.String("till", "", "last day (default today)")
	iss := flag.String("iss-base", "https://iss.moex.com/iss", "MOEX ISS base URL")
	out := flag.String("out", "prices.csv", "output CSV")
	flag.Parse()

	from, err := time.Parse("2006-01-02", *fromStr)
	if err != nil {
		log.Fatal(err)
	}
	till := time.Now()
	if *tillStr != "" {
		if till, err = time.Parse("2006-01-02", *tillStr); err != nil {
			log.Fatal(err)
		}
	}
	src := backtest.NewISSSource(*iss, moex.NewClient(*iss, nil))

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"ticker", "date", "open", "high", "low", "close", "volume"}); err != nil {
		log.Fatal(err)
	}
	for _, t := range strings.Split(*tickers, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		candles, err := src.History(context.Background(), t, from, till)
		if err != nil {
			log.Fatalf("%s: %v", t, err)
		}
		for _, c := range candles {
			if err := w.Write([]string{t, c.Begin.Format("2006-01-02"), c.Open.String(), c.High.String(), c.Low.String(), c.Close.String(), c.Volume.String()}); err != nil {
				log.Fatal(err)
			}
		}
		fmt.Fprintf(os.Stderr, "%s: %d bars\n", t, len(candles))
	}
}
