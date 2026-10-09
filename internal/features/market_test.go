package features

import (
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/dividends"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/macro"
)

func TestTrailingYieldUsesOnlyKnownDividendsInsideTheYear(t *testing.T) {
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	records := []dividends.Record{
		{Ticker: "SBER", DeclaredDate: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), LastBuyDate: time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC), DividendNet: "10"},
		{Ticker: "SBER", DeclaredDate: time.Date(2025, 5, 10, 0, 0, 0, 0, time.UTC), LastBuyDate: time.Date(2025, 5, 9, 0, 0, 0, 0, time.UTC), DividendNet: "99"},
		{Ticker: "SBER", DeclaredDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), LastBuyDate: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), DividendNet: "500"},
		{Ticker: "GAZP", DeclaredDate: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), LastBuyDate: time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC), DividendNet: "77"},
	}
	enrich := NewMarketEnricher(nil, records)
	feature := domain.FeatureContext{Ticker: "sber", LastPrice: decimal.NewFromInt(200)}
	if err := enrich("sber", day, &feature); err != nil {
		t.Fatal(err)
	}
	if got := feature.Extra[DivYieldFeature]; math.Abs(got-5) > 1e-9 {
		t.Fatalf("div yield = %v, want 5 (only the 10 RUB dividend known before the day and inside 12 months)", got)
	}
}

func TestBuilderFailsLoudlyWhenEnricherErrors(t *testing.T) {
	builder := NewBuilder(func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) })
	provider := macro.NewProvider(nil, nil, nil, nil)
	builder.SetEnricher(NewMarketEnricher(provider, nil))

	_, err := builder.Build(Input{
		Ticker: "SBER",
		Price:  PriceSnapshot{LastPrice: decimal.NewFromInt(100), PrevClose: decimal.NewFromInt(99)},
	})
	if err == nil {
		t.Fatal("empty macro history must fail the build instead of producing zero features")
	}
}
