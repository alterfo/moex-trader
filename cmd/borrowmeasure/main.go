package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/borrowcost"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/tinkoff"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var configPath, tickersStr, outPath, shortNotionalDaysStr string
	var lookbackDays int

	flag.StringVar(&configPath, "config", "config.sandbox.yaml", "path to config YAML")
	flag.StringVar(&tickersStr, "tickers", "", "comma-separated tickers (overrides config)")
	flag.StringVar(&outPath, "out", "", "path to write markdown report (default: stdout)")
	flag.IntVar(&lookbackDays, "lookback-days", 180, "operations history window in calendar days")
	flag.StringVar(&shortNotionalDaysStr, "short-notional-days", "0", "sum of short-leg notional over holding days in RUB-days, for measuring the observed fee rate")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Tinkoff.Token) == "" {
		return fmt.Errorf("MOEX_TRADER_TINKOFF_TOKEN is not set (env or .env)")
	}
	if !cfg.Tinkoff.Sandbox {
		return fmt.Errorf("tinkoff.sandbox must be true for the borrow measurement")
	}
	tickers := cfg.Tickers
	if tickersStr != "" {
		tickers = splitComma(tickersStr)
	}
	if len(tickers) == 0 {
		return fmt.Errorf("borrowmeasure: no tickers configured")
	}
	shortNotionalDays, err := decimal.NewFromString(shortNotionalDaysStr)
	if err != nil {
		return fmt.Errorf("parse -short-notional-days: %w", err)
	}
	if shortNotionalDays.IsNegative() {
		return fmt.Errorf("-short-notional-days must not be negative")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := tinkoff.New(ctx, tinkoff.Config{
		Endpoint: cfg.Tinkoff.Endpoint,
		Token:    cfg.Tinkoff.Token,
		AppName:  "moex-trader-borrowmeasure",
	})
	if err != nil {
		return fmt.Errorf("dial tinkoff sandbox: %w", err)
	}
	defer client.Close()

	accountID := strings.TrimSpace(cfg.Tinkoff.AccountID)
	if accountID == "" {
		accounts, err := client.SandboxAccounts(ctx)
		if err != nil {
			return fmt.Errorf("sandbox accounts: %w", err)
		}
		for _, account := range accounts {
			if id := strings.TrimSpace(account.GetId()); id != "" {
				accountID = id
				break
			}
		}
	}
	if accountID == "" {
		return fmt.Errorf("no sandbox account available; run cmd/sandboxcheck once to create one")
	}

	terms := make([]borrowcost.ShareTerm, 0, len(tickers))
	for _, ticker := range tickers {
		ticker = strings.ToUpper(strings.TrimSpace(ticker))
		uid, err := client.ResolveInstrumentUID(ctx, ticker)
		if err != nil {
			log.Printf("%-12s resolve failed: %v", ticker, err)
			continue
		}
		share, err := client.ShareBy(ctx, uid)
		if err != nil {
			log.Printf("%-12s share lookup failed: %v", ticker, err)
			continue
		}
		terms = append(terms, borrowcost.ShareTerm{
			Ticker:       ticker,
			ShortEnabled: share.GetShortEnabledFlag(),
			Kshort:       quotationToDecimal(share.GetKshort()),
			Dshort:       quotationToDecimal(share.GetDshort()),
			DshortMin:    quotationToDecimal(share.GetDshortMin()),
		})
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].Ticker < terms[j].Ticker })

	from := time.Now().AddDate(0, 0, -lookbackDays)
	operations, err := client.SandboxOperations(ctx, accountID, from, time.Now())
	if err != nil {
		return fmt.Errorf("sandbox operations: %w", err)
	}
	var marginFees decimal.Decimal
	feeOps := 0
	for _, op := range operations {
		if op == nil || op.GetOperationType() != pb.OperationType_OPERATION_TYPE_MARGIN_FEE {
			continue
		}
		payment, err := tinkoff.MoneyValueToDecimal(op.GetPayment())
		if err != nil {
			log.Printf("margin fee operation %q: parse payment: %v", op.GetId(), err)
			continue
		}
		marginFees = marginFees.Add(payment.Abs())
		feeOps++
	}

	resolution := borrowcost.ResolveDailyRate(marginFees, shortNotionalDays)
	report := buildReport(accountID, terms, feeOps, marginFees, shortNotionalDays, resolution, lookbackDays)
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("write report %q: %w", outPath, err)
		}
		log.Printf("borrowmeasure: report written to %s", outPath)
	} else {
		fmt.Print(report)
	}
	return nil
}

func quotationToDecimal(quotation *pb.Quotation) decimal.Decimal {
	if quotation == nil {
		return decimal.Zero
	}
	whole := decimal.NewFromInt(quotation.GetUnits())
	fraction := decimal.NewFromInt(int64(quotation.GetNano())).Div(decimal.NewFromInt(1000000000))
	return whole.Add(fraction)
}

func buildReport(accountID string, terms []borrowcost.ShareTerm, feeOps int, marginFees, shortNotionalDays decimal.Decimal, resolution borrowcost.Resolution, lookbackDays int) string {
	var b strings.Builder
	b.WriteString("# Short-borrow measurement and stress (Task 11)\n\n")
	fmt.Fprintf(&b, "- Sandbox account: %s\n", accountID)
	fmt.Fprintf(&b, "- Operations lookback: %d days\n", lookbackDays)
	fmt.Fprintf(&b, "- Margin-fee operations observed: %d (%s RUB)\n\n", feeOps, marginFees.Round(2).String())

	b.WriteString("## Per-ticker short terms\n\n")
	b.WriteString("| ticker | short enabled | Kshort | Dshort | DshortMin |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, term := range terms {
		fmt.Fprintf(&b, "| %s | %t | %s | %s | %s |\n",
			term.Ticker, term.ShortEnabled, term.Kshort.String(), term.Dshort.String(), term.DshortMin.String())
	}
	b.WriteString("\n")

	b.WriteString("## Borrow-rate resolution\n\n")
	fmt.Fprintf(&b, "- Measurable: %t\n", resolution.Measurable)
	fmt.Fprintf(&b, "- Using stress fallback: %t\n", resolution.UsingStress)
	fmt.Fprintf(&b, "- Rate per day: %s (%.4f%%)\n", resolution.MeasuredRatePerDay.String(), resolution.MeasuredRatePerDay.Mul(decimal.NewFromInt(100)).InexactFloat64())
	if !shortNotionalDays.IsZero() {
		fmt.Fprintf(&b, "- Short notional-days supplied: %s RUB-days\n", shortNotionalDays.String())
	}
	if resolution.UsingStress {
		b.WriteString("- The broker API does not expose a borrow fee rate, and no margin-fee operations were observed in the account history, so the pre-registered 0.005%/day stress is applied.\n")
	} else {
		b.WriteString("- A measured rate is available from observed margin fees and short exposure; the go/no-go decision may use it instead of the stress rate.\n")
	}
	b.WriteString("\n- Go/no-go rule: use only the stress scenario unless a real measured rate exists.\n")
	return b.String()
}

func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
