package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/olegsidorkin/moex-trader/internal/model"
	"github.com/olegsidorkin/moex-trader/internal/strategyvalidation"
	"github.com/olegsidorkin/moex-trader/internal/walkforward"
)

func main() {
	var matricesPath string
	var returnsDirs string
	var reliabilityDir string
	flag.StringVar(&matricesPath, "matrices", "", "path to a JSON {key: period-return-matrix} archive (e.g. produced by cmd/walkforward -candidates) to merge into the registry")
	flag.StringVar(&returnsDirs, "returns-dirs", "", "comma-separated wf-dirs, each containing per-window period_returns.csv files; joins variants by date and computes PBO")
	flag.StringVar(&reliabilityDir, "reliability", "", "wf-dir whose per-window reliability.csv files are aggregated into Brier/ECE/reliability-table report")
	flag.Parse()

	registry := strategyvalidation.DefaultRegistry()
	if matricesPath != "" {
		if err := loadMatricesInto(&registry, matricesPath); err != nil {
			fmt.Fprintf(os.Stderr, "load -matrices %q: %v\n", matricesPath, err)
			os.Exit(1)
		}
	}
	if returnsDirs != "" {
		matrix, err := loadReturnsDirsMatrix(splitReturnsDirs(returnsDirs))
		if err != nil {
			fmt.Fprintf(os.Stderr, "load -returns-dirs %q: %v\n", returnsDirs, err)
			os.Exit(1)
		}
		if registry.Matrices == nil {
			registry.Matrices = make(map[string][][]float64)
		}
		registry.Matrices["returns_dirs"] = matrix
	}
	if reliabilityDir != "" {
		pairs, err := loadReliabilityPairs(reliabilityDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load -reliability %q: %v\n", reliabilityDir, err)
			os.Exit(1)
		}
		printReliabilityReport(pairs)
		return
	}
	summary := registry.Summary()
	returns := registry.Series["abs10d_quarterly_realized_return"]
	psr := strategyvalidation.ProbabilisticSharpe(returns, 0)
	dsr := strategyvalidation.DeflatedSharpe(returns, registry.AttemptCount(), 0)
	harveyLiuCritical := strategyvalidation.HarveyLiuSharpeThreshold(len(returns))
	conservativeBenchmark := math.Max(dsr.ExpectedMaxSharpe, harveyLiuCritical)
	conservativeDSR := strategyvalidation.ProbabilisticSharpe(returns, conservativeBenchmark)

	fmt.Printf("Attempt registry: %d attempts (%d selected, %d rejected, %d other)\n", summary.Attempts, summary.Selected, summary.Rejected, summary.Other)
	fmt.Println("Sources:")
	sources := make([]string, 0, len(summary.Sources))
	for source := range summary.Sources {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		fmt.Printf("- %s: %d\n", source, summary.Sources[source])
	}

	fmt.Println()
	fmt.Println("Realized series: abs10d_quarterly_realized_return")
	fmt.Printf("- observations: %d\n", len(returns))
	fmt.Printf("- mean: %.6f\n", psr.Mean)
	fmt.Printf("- sample stdev: %.6f\n", psr.StdDev)
	fmt.Printf("- Sharpe: %.6f\n", psr.Sharpe)
	fmt.Printf("- skewness: %.6f\n", psr.Skewness)
	fmt.Printf("- kurtosis (non-excess): %.6f\n", psr.Kurtosis)
	fmt.Printf("- probabilistic Sharpe (0 benchmark): %.6f\n", psr.ProbabilisticSharpe)
	fmt.Printf("- expected max Sharpe (%d trials): %.6f\n", registry.AttemptCount(), dsr.ExpectedMaxSharpe)
	fmt.Printf("- deflated Sharpe: %.6f\n", dsr.DeflatedSharpe)
	fmt.Printf("- deflated Sharpe p-value: %.6f\n", dsr.PValue)
	fmt.Printf("- Harvey-Liu t: %.3f\n", strategyvalidation.HarveyLiuT)
	fmt.Printf("- Harvey-Liu critical Sharpe: %.6f\n", harveyLiuCritical)
	fmt.Printf("- conservative deflated Sharpe (max of expected and Harvey-Liu): %.6f\n", conservativeDSR.ProbabilisticSharpe)

	fmt.Println()
	if !registry.PBOComputable() {
		fmt.Println("PBO: NOT computable from summary-only registry (no per-strategy period-return matrices archived; pass -matrices <path> or -returns-dirs a,b,c)")
	} else {
		keys := make([]string, 0, len(registry.Matrices))
		for key := range registry.Matrices {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			matrix := registry.Matrices[key]
			splits := strategyvalidation.DefaultSplits(len(matrix))
			if splits < 2 {
				fmt.Printf("PBO[%s]: NOT computable (only %d periods archived, need >= 2)\n", key, len(matrix))
				continue
			}
			result, _ := registry.ComputePBO(key, splits)
			fmt.Printf("PBO[%s]: %.4f (s=%d, %d/%d combinations overfit, %d periods x %d candidates)\n", key, result.PBO, splits, result.Overfit, result.Trials, len(matrix), len(matrix[0]))
		}
	}

	fmt.Println()
	fmt.Println("Attempts:")
	for _, attempt := range registry.Attempts {
		fmt.Printf("- %s | %s | %s | %.4f %s | %s | %s\n", attempt.Name, attempt.Source, attempt.Metric, attempt.Value, attempt.Unit, attempt.Status, attempt.Notes)
	}
}

func loadReturnsDirsMatrix(returnsDirs []string) ([][]float64, error) {
	variants := make([][]strategyvalidation.DailyReturn, 0, len(returnsDirs))
	for _, dir := range returnsDirs {
		series, err := loadVariantDailyReturns(dir)
		if err != nil {
			return nil, fmt.Errorf("returns dir %q: %w", dir, err)
		}
		variants = append(variants, series)
	}
	_, matrix, err := strategyvalidation.JoinDailyReturns(variants)
	if err != nil {
		return nil, err
	}
	return matrix, nil
}

func loadReliabilityPairs(dir string) ([]model.ReliabilityPair, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var pairs []model.ReliabilityPair
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rows, err := walkforward.LoadReliability(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			pairs = append(pairs, model.ReliabilityPair{Probability: row.Probability, Label: row.Label})
		}
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("no reliability rows found under %q", dir)
	}
	return pairs, nil
}

func printReliabilityReport(pairs []model.ReliabilityPair) {
	stats, table := model.ComputeReliability(pairs, 10)
	fmt.Printf("Reliability samples: %d\n", stats.Samples)
	fmt.Printf("Brier score: %.6f\n", stats.Brier)
	fmt.Printf("ECE (10 bins): %.6f\n", stats.ECE)
	fmt.Println("Reliability table:")
	fmt.Printf("%-12s %-8s %-14s %-12s %-12s\n", "bin", "count", "mean predicted", "positive rate", "error")
	for _, bin := range table {
		fmt.Printf("[%4.2f,%4.2f] %-8d %-14.4f %-12.4f %-12.4f\n", bin.Start, bin.End, bin.Count, bin.MeanProbability, bin.PositiveRate, bin.CalibrationError)
	}
}

func loadVariantDailyReturns(dir string) ([]strategyvalidation.DailyReturn, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var series []strategyvalidation.DailyReturn
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		windowDir := filepath.Join(dir, entry.Name())
		path := filepath.Join(windowDir, walkforward.PeriodReturnsFileName)
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		returns, err := walkforward.LoadPeriodReturns(windowDir)
		if err != nil {
			return nil, err
		}
		for _, r := range returns {
			value, _ := r.RealizedNet.Float64()
			series = append(series, strategyvalidation.DailyReturn{Date: r.Date, Value: value})
		}
	}
	if len(series) == 0 {
		return nil, fmt.Errorf("no %s files found under %q", walkforward.PeriodReturnsFileName, dir)
	}
	return series, nil
}

func splitReturnsDirs(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func loadMatricesInto(registry *strategyvalidation.Registry, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var archive map[string][][]float64
	if err := json.Unmarshal(data, &archive); err != nil {
		return err
	}
	if len(archive) == 0 {
		return fmt.Errorf("archive is empty")
	}
	if registry.Matrices == nil {
		registry.Matrices = make(map[string][][]float64, len(archive))
	}
	maps.Copy(registry.Matrices, archive)
	return nil
}
