package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

const (
	defaultConfigPath   = "config.yaml"
	defaultDays         = 750
	defaultMaxLag       = 5
	defaultGrangerLag   = 2
	defaultMinOverlap   = 60
	defaultMinAsymmetry = 0.05
	defaultTop          = 3
	defaultLeadDays     = 1
	defaultWindowDays   = 150
)

type options struct {
	configPath     string
	targetsFlag    string
	candidatesFlag string
	days           int
	maxLag         int
	grangerLag     int
	minOverlap     int
	minAsymmetry   float64
	top            int
	outPath        string
	robustness     bool
	candidateFlag  string
	targetFlag     string
	leadDays       int
	windowDays     int
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if opts.robustness {
		return runRobustness(opts, cfg, stdout)
	}

	targets := splitComma(opts.targetsFlag)
	if len(targets) == 0 {
		targets = cfg.Tickers
	}
	if len(targets) == 0 {
		return fmt.Errorf("targets must not be empty")
	}
	explicitCandidates := splitComma(opts.candidatesFlag)

	universeTickers := unionStrings(targets, explicitCandidates)
	if len(explicitCandidates) == 0 {
		universeTickers = unionStrings(universeTickers, cfg.Tickers)
	}

	till := time.Now()
	from := till.AddDate(0, 0, -opts.days)

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("leadlag: window=%s..%s targets=%d universe=%d max_lag=%d granger_lag=%d",
		from.Format("2006-01-02"), till.Format("2006-01-02"), len(targets), len(universeTickers), opts.maxLag, opts.grangerLag)

	returns, err := model.BuildReturnUniverse(ctx, source, universeTickers, from, till)
	if err != nil {
		return fmt.Errorf("build return universe: %w", err)
	}

	candidates := explicitCandidates
	if len(candidates) == 0 {
		candidates = make([]string, 0, len(returns))
		for ticker := range returns {
			candidates = append(candidates, ticker)
		}
	}

	results, bonferroniAlpha, nTests := model.FindLeaders(returns, targets, candidates, opts.maxLag, opts.grangerLag, opts.minOverlap, opts.minAsymmetry)
	report := model.BuildLeadLagReport(results, bonferroniAlpha, nTests, opts.top)

	if opts.outPath != "" {
		if err := os.WriteFile(opts.outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", opts.outPath, err)
		}
		log.Printf("leadlag: report written to %s", opts.outPath)
		return nil
	}
	fmt.Fprint(stdout, report)
	return nil
}

func parseOptions(args []string) (options, error) {
	opts := options{
		configPath:   defaultConfigPath,
		days:         defaultDays,
		maxLag:       defaultMaxLag,
		grangerLag:   defaultGrangerLag,
		minOverlap:   defaultMinOverlap,
		minAsymmetry: defaultMinAsymmetry,
		top:          defaultTop,
	}
	fs := flag.NewFlagSet("leadlag", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to config YAML")
	fs.StringVar(&opts.targetsFlag, "targets", "", "comma-separated target tickers (default: config tickers)")
	fs.StringVar(&opts.candidatesFlag, "candidates", "", "comma-separated candidate leader tickers (default: config tickers + IMOEX + USDRUB)")
	fs.IntVar(&opts.days, "days", opts.days, "lookback window in calendar days")
	fs.IntVar(&opts.maxLag, "max-lag", opts.maxLag, "max lag (trading days) for cross-correlation")
	fs.IntVar(&opts.grangerLag, "granger-lag", opts.grangerLag, "number of lags in the Granger causality test")
	fs.IntVar(&opts.minOverlap, "min-overlap", opts.minOverlap, "minimum overlapping observations required")
	fs.Float64Var(&opts.minAsymmetry, "min-asymmetry", opts.minAsymmetry, "minimum |lead corr| - |reverse corr| to report a candidate")
	fs.IntVar(&opts.top, "top", opts.top, "how many leaders to show per target")
	fs.StringVar(&opts.outPath, "out", "", "path to write markdown report (default: stdout)")
	fs.BoolVar(&opts.robustness, "robustness", false, "check one candidate->target pair on non-overlapping windows instead of scanning all pairs")
	fs.StringVar(&opts.candidateFlag, "candidate", "", "leader ticker for -robustness")
	fs.StringVar(&opts.targetFlag, "target", "", "target ticker for -robustness")
	fs.IntVar(&opts.leadDays, "lead-days", defaultLeadDays, "lag (trading days) to check for -robustness")
	fs.IntVar(&opts.windowDays, "window-days", defaultWindowDays, "non-overlapping window length (trading days) for -robustness")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.robustness && (strings.TrimSpace(opts.candidateFlag) == "" || strings.TrimSpace(opts.targetFlag) == "") {
		return options{}, fmt.Errorf("-robustness requires -candidate and -target")
	}
	return opts, nil
}

func runRobustness(opts options, cfg *config.Config, stdout io.Writer) error {
	candidate := strings.ToUpper(strings.TrimSpace(opts.candidateFlag))
	target := strings.ToUpper(strings.TrimSpace(opts.targetFlag))

	till := time.Now()
	from := till.AddDate(0, 0, -opts.days)

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("leadlag: robustness %s -> %s, lag=%d window_days=%d", candidate, target, opts.leadDays, opts.windowDays)

	returns, err := model.BuildReturnUniverse(ctx, source, []string{candidate, target}, from, till)
	if err != nil {
		return fmt.Errorf("build return universe: %w", err)
	}
	candidateReturns, ok := returns[candidate]
	if !ok {
		return fmt.Errorf("no history for candidate %s", candidate)
	}
	targetReturns, ok := returns[target]
	if !ok {
		return fmt.Errorf("no history for target %s", target)
	}

	windows := model.WindowedRobustness(candidateReturns, targetReturns, opts.leadDays, opts.windowDays, opts.grangerLag, opts.minOverlap)
	if len(windows) == 0 {
		return fmt.Errorf("not enough overlapping history for a single %d-day window", opts.windowDays)
	}
	report := model.BuildRobustnessReport(candidate, target, opts.leadDays, windows)

	if opts.outPath != "" {
		if err := os.WriteFile(opts.outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", opts.outPath, err)
		}
		log.Printf("leadlag: report written to %s", opts.outPath)
		return nil
	}
	fmt.Fprint(stdout, report)
	return nil
}

func splitComma(s string) []string {
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

func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, s := range list {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
