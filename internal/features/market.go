package features

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/dividends"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/macro"
)

const (
	DivYieldFeature = "div_yield_ttm_pct"
	macroMaxStale   = 7 * 24 * time.Hour
)

func ExtraFeatureNames() []string {
	return append([]string{DivYieldFeature}, macro.FeatureNames...)
}

type divEvent struct {
	known time.Time
	net   decimal.Decimal
}

func NewMarketEnricher(provider *macro.Provider, records []dividends.Record) Enricher {
	byTicker := make(map[string][]divEvent)
	for _, r := range records {
		net, err := r.Net()
		if err != nil || net.Sign() <= 0 {
			continue
		}
		known := r.DeclaredDate
		if r.LastBuyDate.After(known) {
			known = r.LastBuyDate
		}
		ticker := strings.ToUpper(strings.TrimSpace(r.Ticker))
		byTicker[ticker] = append(byTicker[ticker], divEvent{known: macro.Day(known), net: net})
	}

	return func(ticker string, decisionDay time.Time, feature *domain.FeatureContext) error {
		extra := make(map[string]float64, 1+len(macro.FeatureNames))
		extra[DivYieldFeature] = trailingYieldPct(byTicker[strings.ToUpper(strings.TrimSpace(ticker))], macro.Day(decisionDay), feature.LastPrice)

		if provider != nil {
			values, err := provider.At(decisionDay, macroMaxStale)
			if err != nil {
				return err
			}
			for i, name := range macro.FeatureNames {
				extra[name] = values.Values()[i]
			}
		}
		feature.Extra = extra
		return nil
	}
}

func trailingYieldPct(events []divEvent, day time.Time, price decimal.Decimal) float64 {
	if price.Sign() <= 0 {
		return 0
	}
	from := day.AddDate(-1, 0, 0)
	sum := decimal.Zero
	for _, e := range events {
		if e.known.Before(day) && !e.known.Before(from) {
			sum = sum.Add(e.net)
		}
	}
	return sum.Div(price).Mul(decimal.NewFromInt(100)).InexactFloat64()
}

func LoadMarketEnricher(macroDir, dividendsPath string) (Enricher, error) {
	if macroDir == "" {
		return nil, nil
	}
	provider, err := macro.LoadProvider(macroDir)
	if err != nil {
		return nil, err
	}
	var records []dividends.Record
	if dividendsPath != "" {
		records, err = dividends.Load(dividendsPath)
		if err != nil {
			return nil, err
		}
	}
	return NewMarketEnricher(provider, records), nil
}
