package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

type fixedShadowSource struct {
	action domain.Action
}

func (f *fixedShadowSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	return domain.TradeSignal{Ticker: feature.Ticker, Action: f.action, Confidence: decimal.NewFromFloat(0.8)}, nil
}

type newsAwareShadowSource struct{}

func (n *newsAwareShadowSource) Generate(_ context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	action := domain.ActionHold
	if feature.NewsCount > 0 {
		action = domain.ActionBuy
	}
	return domain.TradeSignal{Ticker: feature.Ticker, Action: action, Confidence: decimal.NewFromFloat(0.8)}, nil
}

func closedBarFixture() []moex.Candle {
	return []moex.Candle{
		{Begin: time.Date(2024, 1, 9, 10, 0, 0, 0, time.UTC), Close: decimal.NewFromFloat(100)},
		{Begin: time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC), Close: decimal.NewFromFloat(110)},
		{Begin: time.Date(2024, 1, 11, 10, 0, 0, 0, time.UTC), Close: decimal.NewFromFloat(105)},
	}
}

func TestShadowReconcilerMatchesClosedBarReplay(t *testing.T) {
	builder := features.NewBuilder(nil)
	source := &fixedShadowSource{action: domain.ActionBuy}
	reconciler := NewShadowReconciler(builder, source)
	candles := closedBarFixture()

	liveInput := features.Input{
		Ticker: "SBER",
		Price: features.PriceSnapshot{
			LastPrice: decimal.NewFromFloat(112),
			PrevClose: decimal.NewFromFloat(105),
			Bid:       decimal.NewFromFloat(111.9),
			Ask:       decimal.NewFromFloat(112.1),
		},
		Candles: candles,
	}
	liveFeature, err := builder.Build(liveInput)
	if err != nil {
		t.Fatalf("Build(live) error = %v", err)
	}
	liveSignal, err := source.Generate(context.Background(), liveFeature)
	if err != nil {
		t.Fatalf("Generate(live) error = %v", err)
	}

	cmp := reconciler.Reconcile(context.Background(), liveInput, liveFeature, liveSignal)
	if cmp.Err != "" {
		t.Fatalf("Reconcile error = %q", cmp.Err)
	}
	if !cmp.FeatureMatch {
		t.Fatalf("FeatureMatch = false, want true; max delta %f", cmp.MaxFeatureDelta)
	}
	if !cmp.SignalMatch {
		t.Fatalf("SignalMatch = false, want true; live=%s replay=%s", cmp.LiveAction, cmp.ReplayAction)
	}
	if cmp.Day != "2024-01-11" {
		t.Fatalf("Day = %q, want 2024-01-11", cmp.Day)
	}
}

func TestShadowReconcilerDetectsLiveOnlySignalDivergence(t *testing.T) {
	builder := features.NewBuilder(nil)
	source := &newsAwareShadowSource{}
	reconciler := NewShadowReconciler(builder, source)
	candles := closedBarFixture()

	liveInput := features.Input{
		Ticker: "SBER",
		Price: features.PriceSnapshot{
			LastPrice: decimal.NewFromFloat(112),
			PrevClose: decimal.NewFromFloat(105),
		},
		Candles: candles,
		News: []news.MatchedArticle{{
			Ticker:      "SBER",
			Title:       "Прибыль выросла",
			TrustWeight: decimal.NewFromFloat(1),
		}},
	}
	liveFeature, err := builder.Build(liveInput)
	if err != nil {
		t.Fatalf("Build(live) error = %v", err)
	}
	liveSignal, err := source.Generate(context.Background(), liveFeature)
	if err != nil {
		t.Fatalf("Generate(live) error = %v", err)
	}

	cmp := reconciler.Reconcile(context.Background(), liveInput, liveFeature, liveSignal)
	if cmp.Err != "" {
		t.Fatalf("Reconcile error = %q", cmp.Err)
	}
	if !cmp.FeatureMatch {
		t.Fatalf("FeatureMatch = false, want true; max delta %f", cmp.MaxFeatureDelta)
	}
	if cmp.SignalMatch {
		t.Fatalf("SignalMatch = true, want false for live-only news divergence")
	}
	if cmp.LiveAction != domain.ActionBuy || cmp.ReplayAction != domain.ActionHold {
		t.Fatalf("unexpected actions: live=%s replay=%s", cmp.LiveAction, cmp.ReplayAction)
	}
}

func TestShadowReconcilerDetectsFeatureDivergence(t *testing.T) {
	builder := features.NewBuilder(nil)
	source := &fixedShadowSource{action: domain.ActionBuy}
	reconciler := NewShadowReconciler(builder, source)
	candles := closedBarFixture()

	liveInput := features.Input{
		Ticker: "SBER",
		Price: features.PriceSnapshot{
			LastPrice: decimal.NewFromFloat(112),
			PrevClose: decimal.NewFromFloat(105),
		},
		Candles: candles,
	}
	liveFeature, err := builder.Build(liveInput)
	if err != nil {
		t.Fatalf("Build(live) error = %v", err)
	}
	liveFeature.ReturnPct = liveFeature.ReturnPct.Add(decimal.NewFromFloat(0.5))
	liveSignal, _ := source.Generate(context.Background(), liveFeature)

	cmp := reconciler.Reconcile(context.Background(), liveInput, liveFeature, liveSignal)
	if cmp.FeatureMatch {
		t.Fatalf("FeatureMatch = true, want false after tampering with return_pct")
	}
}

func TestShadowReconcilerRequiresEnoughCandles(t *testing.T) {
	builder := features.NewBuilder(nil)
	source := &fixedShadowSource{action: domain.ActionHold}
	reconciler := NewShadowReconciler(builder, source)
	liveInput := features.Input{
		Ticker: "SBER",
		Price: features.PriceSnapshot{
			LastPrice: decimal.NewFromFloat(112),
			PrevClose: decimal.NewFromFloat(105),
		},
		Candles: []moex.Candle{{Close: decimal.NewFromFloat(105)}},
	}
	liveFeature, _ := builder.Build(liveInput)
	liveSignal, _ := source.Generate(context.Background(), liveFeature)

	cmp := reconciler.Reconcile(context.Background(), liveInput, liveFeature, liveSignal)
	if !strings.Contains(cmp.Err, "fewer than two closed candles") {
		t.Fatalf("Err = %q, want fewer-than-two-candles error", cmp.Err)
	}
}

func TestShadowDigestMarkdownAndFileUpdate(t *testing.T) {
	digest := NewShadowDigest()
	digest.Add(ShadowComparison{
		Ticker: "SBER", Day: "2024-01-11",
		LiveAction: domain.ActionBuy, ReplayAction: domain.ActionHold,
		FeatureMatch: true, SignalMatch: false,
	})
	md := digest.Markdown()
	if !strings.Contains(md, "| SBER | 2024-01-11 | BUY | HOLD | true | 0.000000000 | false |") {
		t.Fatalf("digest markdown missing expected row:\n%s", md)
	}

	path := filepath.Join(t.TempDir(), "metrics.md")
	if err := os.WriteFile(path, []byte("# Metrics\n\nexisting content\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := UpdateShadowDigestFile(path, md); err != nil {
		t.Fatalf("UpdateShadowDigestFile: %v", err)
	}

	second := NewShadowDigest()
	second.Add(ShadowComparison{Ticker: "GAZP", Day: "2024-01-11", LiveAction: domain.ActionSell, ReplayAction: domain.ActionSell, FeatureMatch: true, SignalMatch: true})
	if err := UpdateShadowDigestFile(path, second.Markdown()); err != nil {
		t.Fatalf("UpdateShadowDigestFile (second): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "| GAZP |") {
		t.Fatalf("updated digest missing GAZP row:\n%s", content)
	}
	if strings.Contains(content, "| SBER |") {
		t.Fatalf("updated digest still contains replaced SBER row:\n%s", content)
	}
	if !strings.Contains(content, "# Metrics") {
		t.Fatalf("updated digest dropped pre-existing content:\n%s", content)
	}
}
