package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/olegsidorkin/moex-trader/internal/memberagree"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var inPath, outPath string
	var primarySeed int
	flag.StringVar(&inPath, "in", "", "member CSV file or directory of member CSVs written by cmd/backtest -members-csv")
	flag.StringVar(&outPath, "out", "", "path to write the markdown report (default: stdout)")
	flag.IntVar(&primarySeed, "primary-seed", 42, "seed of the primary arm")
	flag.Parse()

	if strings.TrimSpace(inPath) == "" {
		return fmt.Errorf("-in is required")
	}
	rows, unmatched, err := memberagree.LoadPath(inPath)
	if err != nil {
		return err
	}
	opts := memberagree.DefaultOptions()
	opts.PrimarySeed = primarySeed
	report := memberagree.Analyze(rows, unmatched, opts)
	markdown := report.Markdown()
	if outPath == "" {
		fmt.Print(markdown)
	} else {
		if dir := filepath.Dir(outPath); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}
		}
		if err := os.WriteFile(outPath, []byte(markdown), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", outPath, err)
		}
		fmt.Print(markdown)
	}
	fmt.Printf("\nVERDICT: %s\n", report.Verdict)
	return nil
}
