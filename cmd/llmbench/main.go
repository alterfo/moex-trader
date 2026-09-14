package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/llm"
	"github.com/olegsidorkin/moex-trader/internal/llmbench"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		host    = flag.String("host", "192.168.88.193:11434", "Ollama host")
		model   = flag.String("model", "qwen3.8", "Ollama model")
		timeout = flag.Duration("timeout", 10*time.Second, "per-request timeout")
		count   = flag.Int("n", 10, "number of sample requests to run")
	)
	flag.Parse()

	if *host == "" {
		return fmt.Errorf("host must not be empty")
	}
	if *model == "" {
		return fmt.Errorf("model must not be empty")
	}
	if *count <= 0 {
		return fmt.Errorf("n must be positive")
	}

	templates := llmbench.DefaultSamples()
	samples := make([]llmbench.Sample, 0, *count)
	for i := 0; i < *count; i++ {
		template := templates[i%len(templates)]
		samples = append(samples, llmbench.Sample{
			Name:    fmt.Sprintf("%s-%d", template.Name, i+1),
			Feature: template.Feature,
		})
	}

	client := llm.New(*host, *model, *timeout)
	runner := llmbench.NewRunner(client, llm.NewPromptBuilder())
	results := runner.Run(context.Background(), samples)
	stats := llmbench.Summarize(results)

	fmt.Printf("samples:  %d\n", stats.Total)
	fmt.Printf("success:  %d (%.1f%%)\n", stats.Success, stats.SuccessRate*100)
	fmt.Printf("failed:   %d\n", stats.Failed)
	fmt.Printf("latency:  avg=%s p50=%s p90=%s p95=%s p99=%s\n",
		stats.AvgLatency, stats.P50, stats.P90, stats.P95, stats.P99)

	for _, result := range results {
		if result.Err != nil {
			fmt.Printf("  %s: error: %v\n", result.Name, result.Err)
			continue
		}
		signal := result.Signal
		fmt.Printf("  %s: %s lots=%d confidence=%s latency=%s\n",
			result.Name, signal.Action, signal.TargetLots, signal.Confidence, result.Latency)
	}
	return nil
}
