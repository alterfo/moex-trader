package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

const (
	defaultConfigPath  = "config.yaml"
	defaultHorizonsStr = "1,3,5"
	defaultStride      = 3
)

type options struct {
	configPath  string
	tickersFlag string
	fromStr     string
	tillStr     string
	horizonsStr string
	stride      int
	outPath     string
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

	from, till, err := resolveWindow(opts, time.Now())
	if err != nil {
		return err
	}

	tickers := splitComma(opts.tickersFlag)
	if len(tickers) == 0 {
		tickers = cfg.Tickers
	}
	if len(tickers) == 0 {
		return fmt.Errorf("tickers must not be empty")
	}

	horizons, err := parseHorizons(opts.horizonsStr)
	if err != nil {
		return err
	}

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("calibrate: window=%s..%s tickers=%d horizons=%v stride=%d",
		from.Format("2006-01-02"), till.Format("2006-01-02"), len(tickers), horizons, opts.stride)

	samples, err := model.BuildCalibrationSamples(ctx, source, tickers, from, till, horizons, opts.stride)
	if err != nil {
		return fmt.Errorf("build calibration samples: %w", err)
	}
	if len(samples) == 0 {
		return fmt.Errorf("no calibration samples in the selected window")
	}

	report := model.BuildCalibrationReport(samples, tickers, horizons)
	if opts.outPath != "" {
		if err := os.WriteFile(opts.outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", opts.outPath, err)
		}
		log.Printf("calibrate: report written to %s", opts.outPath)
		return nil
	}
	fmt.Fprint(stdout, report)
	return nil
}

func parseOptions(args []string) (options, error) {
	opts := options{
		configPath:  defaultConfigPath,
		horizonsStr: defaultHorizonsStr,
		stride:      defaultStride,
	}
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to config YAML")
	fs.StringVar(&opts.tickersFlag, "tickers", "", "comma-separated tickers (default: config tickers)")
	fs.StringVar(&opts.fromStr, "from", "", "calibration window start date YYYY-MM-DD (default: two years ago)")
	fs.StringVar(&opts.tillStr, "till", "", "calibration window end date YYYY-MM-DD (default: today)")
	fs.StringVar(&opts.horizonsStr, "horizons", opts.horizonsStr, "comma-separated forward-return horizons in trading days")
	fs.IntVar(&opts.stride, "stride", opts.stride, "sample every N trading days (reduces overlap between observations)")
	fs.StringVar(&opts.outPath, "out", "", "path to write markdown report (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

func resolveWindow(opts options, now time.Time) (from, till time.Time, err error) {
	if opts.fromStr != "" {
		from, err = time.Parse("2006-01-02", opts.fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse -from: %w", err)
		}
	} else {
		from = now.AddDate(-2, 0, 0)
	}
	if opts.tillStr != "" {
		till, err = time.Parse("2006-01-02", opts.tillStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse -till: %w", err)
		}
	} else {
		till = now
	}
	if !from.Before(till) {
		return time.Time{}, time.Time{}, fmt.Errorf("from must be before till")
	}
	return from, till, nil
}

func parseHorizons(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	horizons := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		h, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("parse -horizons %q: %w", s, err)
		}
		horizons = append(horizons, h)
	}
	if len(horizons) == 0 {
		return nil, fmt.Errorf("horizons must not be empty")
	}
	return horizons, nil
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
