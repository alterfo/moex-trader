package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("algopackincremental: subcommand merge or score")
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	base := fs.String("base", "", "exportdataset CSV")
	block := fs.String("block", "", "flowscan export CSV")
	out := fs.String("out", "", "merged CSV")
	shuffle := fs.Int64("shuffle-seed", 0, "permute the block across tickers within each date")
	data := fs.String("data", "", "merged CSV")
	shuf := fs.String("shuf-data", "", "merged shuffled CSV")
	models := fs.String("models", "models", "directory with arm/seed/fold model JSONs")
	seedList := fs.String("seeds", "20261023,20261024,20261025,20261026,20261027", "model seeds")
	demean := fs.Bool("demean", false, "merge: subtract the date mean from every block column")
	abnormal := fs.Bool("abnormal-label", false, "merge: replace the label by the abnormal 10-day label")
	from := fs.String("from", "", "merge: keep base rows from this date")
	foldFrom := fs.Int("fold-from", 1, "score: first fold (1-based)")
	armList := fs.String("arms", "A,S,A2", "pscore: arms")
	draws := fs.Int("draws", 5000, "bootstrap draws")
	bootSeed := fs.Int64("boot-seed", 20261022, "bootstrap seed")
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}
	switch cmd {
	case "merge":
		if err := merge(*base, *block, *out, mergeOpts{shuffleSeed: *shuffle, fromDate: *from, demean: *demean, abnormal: *abnormal}); err != nil {
			log.Fatalf("algopackincremental: %v", err)
		}
	case "score":
		var seeds []int
		for _, s := range strings.Split(*seedList, ",") {
			v, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				log.Fatalf("algopackincremental: seed %q: %v", s, err)
			}
			seeds = append(seeds, v)
		}
		if err := runScore(os.Stdout, *data, *shuf, *block, *models, seeds, *draws, *bootSeed, *foldFrom); err != nil {
			log.Fatalf("algopackincremental: %v", err)
		}
	case "panel":
		if err := buildPanel(*block, *out, *from, *shuffle); err != nil {
			log.Fatalf("algopackincremental: %v", err)
		}
	case "pscore":
		var seeds []int
		for _, sd := range strings.Split(*seedList, ",") {
			v, err := strconv.Atoi(strings.TrimSpace(sd))
			if err != nil {
				log.Fatalf("algopackincremental: seed %q: %v", sd, err)
			}
			seeds = append(seeds, v)
		}
		if err := runPScore(os.Stdout, *data, *shuf, *models, strings.Split(*armList, ","), seeds, *foldFrom, *draws, *bootSeed); err != nil {
			log.Fatalf("algopackincremental: %v", err)
		}
	case "ks":
		if err := runKS(os.Stdout, *block, "2024-07-01", "2024-12-31", "2025-01-03", "2025-06-30", 0.10); err != nil {
			log.Fatalf("algopackincremental: %v", err)
		}
	default:
		log.Fatalf("algopackincremental: unknown subcommand %q", cmd)
	}
}
