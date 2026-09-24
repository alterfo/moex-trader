package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"math"
	"os"
	"sort"

	"github.com/olegsidorkin/moex-trader/internal/strategyvalidation"
)

func main() {
	var matricesPath string
	flag.StringVar(&matricesPath, "matrices", "", "path to a JSON {key: period-return-matrix} archive (e.g. produced by cmd/walkforward -candidates) to merge into the registry")
	flag.Parse()

	registry := strategyvalidation.DefaultRegistry()
	if matricesPath != "" {
		if err := loadMatricesInto(&registry, matricesPath); err != nil {
			fmt.Fprintf(os.Stderr, "load -matrices %q: %v\n", matricesPath, err)
			os.Exit(1)
		}
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
		fmt.Println("PBO: NOT computable from summary-only registry (no per-strategy period-return matrices archived; pass -matrices <path> from cmd/walkforward -candidates)")
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
