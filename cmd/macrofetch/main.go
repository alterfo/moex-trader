package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/macro"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("macrofetch", flag.ContinueOnError)
	out := fs.String("out", "data/macro", "directory for imoex.csv, cnyrub.csv, brent.csv, keyrate.csv")
	fromFlag := fs.String("from", "2023-01-01", "history start YYYY-MM-DD")
	update := fs.Bool("update", false, "extend the existing series in -out instead of refetching everything")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fetcher := macro.NewFetcher()
	now := time.Now().UTC()
	var provider *macro.Provider
	if *update {
		existing, err := macro.LoadProvider(*out)
		if err != nil {
			return fmt.Errorf("load existing series: %w", err)
		}
		if err := existing.Refresh(ctx, fetcher, now); err != nil {
			return err
		}
		provider = existing
	} else {
		from, err := time.Parse("2006-01-02", *fromFlag)
		if err != nil {
			return fmt.Errorf("parse -from: %w", err)
		}
		fetched, err := fetcher.All(ctx, from, now)
		if err != nil {
			return err
		}
		provider = fetched
	}
	if err := provider.Save(*out); err != nil {
		return err
	}
	imoex, cnyrub, brent, keyRate := provider.Snapshot()
	for name, s := range map[string]macro.Series{"imoex": imoex, "cnyrub": cnyrub, "brent": brent, "keyrate": keyRate} {
		last, _ := s.Last()
		fmt.Printf("%-8s %5d points  %s .. %s\n", name, len(s), s[0].Date.Format("2006-01-02"), last.Date.Format("2006-01-02"))
	}
	return nil
}
