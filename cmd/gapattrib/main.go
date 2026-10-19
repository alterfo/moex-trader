package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/gapattrib"
)

var live17 = []string{"YDEX", "OZON", "SBER", "LKOH", "GAZP", "GMKN", "ROSN", "NVTK", "TATN", "MTSS", "MGNT", "PLZL", "CHMF", "T", "VTBR", "RUAL", "POSI"}

func main() {
	snapshot := flag.String("snapshot", "", "tqbr_snap.csv")
	dividends := flag.String("dividends", "", "dividends_pit.jsonl")
	scores := flag.String("scores", "", "full-universe ridge percentiles, date x ticker")
	keyrate := flag.String("keyrate", "data/macro/keyrate.csv", "key rate series")
	from := flag.String("from", "2019-01-01", "first rebalance date")
	till := flag.String("till", "2026-12-31", "last rebalance date")
	flag.Parse()
	if *snapshot == "" || *scores == "" {
		log.Fatal("-snapshot and -scores are required")
	}
	panel, err := gapattrib.LoadSnapshot(*snapshot)
	if err != nil {
		log.Fatal(err)
	}
	div, err := gapattrib.LoadDividends(*dividends, panel)
	if err != nil {
		log.Fatal(err)
	}
	sc, err := gapattrib.LoadScores(*scores)
	if err != nil {
		log.Fatal(err)
	}
	rate, err := gapattrib.LoadKeyRate(*keyrate)
	if err != nil {
		log.Fatal(err)
	}
	live := map[string]bool{}
	for _, t := range live17 {
		live[t] = true
	}
	in := &gapattrib.Inputs{Panel: panel, Adj: gapattrib.Adjust(panel, div), Scores: sc, Live: live, Rate: rate}
	fromT, err := time.Parse("2006-01-02", *from)
	if err != nil {
		log.Fatal(err)
	}
	tillT, err := time.Parse("2006-01-02", *till)
	if err != nil {
		log.Fatal(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "| rung | periods | names | longs | shorts | mean net | NW t | net per deployed | carry per deployed | excess per deployed | mean deployed | sum net | gross per deployed | long leg per capital | short leg per capital |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, rung := range []gapattrib.Rung{gapattrib.RungResearch, gapattrib.RungLiveNames, gapattrib.RungLiveLegMean, gapattrib.RungLiveRule, gapattrib.RungLiveNotional} {
		s := gapattrib.Summarize(gapattrib.Run(in, rung, fromT, tillT))
		fmt.Fprintf(&b, "| %s | %d | %.1f | %.1f | %.1f | %.5f | %.2f | %.4f | %.4f | %.4f | %.1f | %.1f | %.4f | %.4f | %.4f |\n",
			rung, s.Periods, s.MeanNames, s.MeanLongs, s.MeanShorts, s.MeanNet, s.TStat, s.MeanNetPerCap, s.MeanCarryPerCap, s.MeanExcessPerCap, s.MeanCapital, s.SumNet, s.GrossPerCap, s.LongPerCap, s.ShortPerCap)
	}
	fmt.Fprint(os.Stdout, b.String())
	for _, rung := range []gapattrib.Rung{gapattrib.RungLiveNames, gapattrib.RungLiveLegMean, gapattrib.RungLiveRule} {
		s := gapattrib.Summarize(gapattrib.Run(in, rung, fromT, tillT))
		fmt.Fprintf(os.Stdout, "%s: gross per capital %.5f, cost per capital %.5f, NW t of net per capital %.2f\n", rung, s.GrossPerCap, s.CostPerCap, s.TPerCap)
	}
}
