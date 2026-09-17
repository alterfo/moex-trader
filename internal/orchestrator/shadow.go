package orchestrator

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

const (
	shadowDigestStartMarker = "<!-- shadow-reconciliation:start -->"
	shadowDigestEndMarker   = "<!-- shadow-reconciliation:end -->"
	featureDeltaTolerance   = 1e-9
)

type ShadowComparison struct {
	Ticker           string
	Day              string
	LiveAction       domain.Action
	ReplayAction     domain.Action
	LiveConfidence   decimal.Decimal
	ReplayConfidence decimal.Decimal
	FeatureMatch     bool
	MaxFeatureDelta  float64
	SignalMatch      bool
	Err              string
}

type ShadowReconciler struct {
	builder *features.Builder
	source  SignalSource
}

func NewShadowReconciler(builder *features.Builder, source SignalSource) *ShadowReconciler {
	return &ShadowReconciler{builder: builder, source: source}
}

func (s *ShadowReconciler) Reconcile(ctx context.Context, liveInput features.Input, liveFeature domain.FeatureContext, liveSignal domain.TradeSignal) ShadowComparison {
	cmp := ShadowComparison{
		Ticker:         liveFeature.Ticker,
		Day:            shadowDay(liveInput.Candles, liveFeature.GeneratedAt),
		LiveAction:     liveSignal.Action,
		LiveConfidence: liveSignal.Confidence,
	}
	if s == nil || s.builder == nil || s.source == nil {
		cmp.Err = "shadow reconciler: builder and source are required"
		return cmp
	}
	replay, ok := replayInput(liveInput)
	if !ok {
		cmp.Err = "shadow reconciler: fewer than two closed candles"
		return cmp
	}
	replayFeature, err := s.builder.Build(replay)
	if err != nil {
		cmp.Err = fmt.Sprintf("shadow reconciler: build replay feature: %v", err)
		return cmp
	}
	cmp.MaxFeatureDelta = candleFeatureDelta(liveFeature, replayFeature)
	cmp.FeatureMatch = cmp.MaxFeatureDelta <= featureDeltaTolerance
	replaySignal, err := s.source.Generate(ctx, replayFeature)
	if err != nil {
		cmp.Err = fmt.Sprintf("shadow reconciler: replay signal: %v", err)
		return cmp
	}
	cmp.ReplayAction = replaySignal.Action
	cmp.ReplayConfidence = replaySignal.Confidence
	cmp.SignalMatch = liveSignal.Action == replaySignal.Action
	return cmp
}

func replayInput(live features.Input) (features.Input, bool) {
	candles := live.Candles
	if len(candles) < 2 {
		return features.Input{}, false
	}
	last := candles[len(candles)-1]
	prev := candles[len(candles)-2]
	return features.Input{
		Ticker: live.Ticker,
		Price: features.PriceSnapshot{
			LastPrice: last.Close,
			PrevClose: prev.Close,
			AsOf:      last.Begin,
		},
		Candles: candles,
	}, true
}

func shadowDay(candles []moex.Candle, generatedAt time.Time) string {
	if len(candles) > 0 && !candles[len(candles)-1].Begin.IsZero() {
		return candles[len(candles)-1].Begin.Format("2006-01-02")
	}
	if !generatedAt.IsZero() {
		return generatedAt.Format("2006-01-02")
	}
	return ""
}

func candleFeatureDelta(a, b domain.FeatureContext) float64 {
	pairs := [][2]decimal.Decimal{
		{a.ReturnPct, b.ReturnPct},
		{a.RealizedVolatility, b.RealizedVolatility},
		{a.Mom5d, b.Mom5d},
		{a.Mom21d, b.Mom21d},
		{a.Mom63d, b.Mom63d},
		{a.Reversal1d, b.Reversal1d},
		{a.RSI14, b.RSI14},
		{a.DistMA20Pct, b.DistMA20Pct},
		{a.DistMA50Pct, b.DistMA50Pct},
		{a.RealizedVol21d, b.RealizedVol21d},
		{a.VolumeZScore20d, b.VolumeZScore20d},
		{a.MACDHistPct, b.MACDHistPct},
		{a.StochK14, b.StochK14},
		{a.WilliamsR14, b.WilliamsR14},
		{a.AlligatorSpreadPct, b.AlligatorSpreadPct},
	}
	maxDelta := 0.0
	for _, pair := range pairs {
		av, _ := pair[0].Float64()
		bv, _ := pair[1].Float64()
		delta := math.Abs(av - bv)
		if delta > maxDelta {
			maxDelta = delta
		}
	}
	return maxDelta
}

type ShadowDigest struct {
	mu    sync.Mutex
	byKey map[string]ShadowComparison
}

func NewShadowDigest() *ShadowDigest {
	return &ShadowDigest{byKey: make(map[string]ShadowComparison)}
}

func (d *ShadowDigest) Add(cmp ShadowComparison) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := cmp.Ticker + "|" + cmp.Day
	d.byKey[key] = cmp
}

func (d *ShadowDigest) Comparisons() []ShadowComparison {
	d.mu.Lock()
	defer d.mu.Unlock()
	keys := make([]string, 0, len(d.byKey))
	for key := range d.byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ShadowComparison, 0, len(keys))
	for _, key := range keys {
		out = append(out, d.byKey[key])
	}
	return out
}

func (d *ShadowDigest) Markdown() string {
	var b strings.Builder
	b.WriteString(shadowDigestStartMarker)
	b.WriteString("\n\n## Shadow reconciliation (Task 3)\n\n")
	b.WriteString("Live decision vs replayed backtest decision for the same ticker and last ")
	b.WriteString("closed day. `FeatureMatch` compares candle-derived features (return_pct and ")
	b.WriteString("price/volume indicators); news, order-book and event fields are live-only and ")
	b.WriteString("excluded. `SignalMatch` compares the model action.\n\n")
	b.WriteString("| ticker | day | live | replay | feature match | max delta | signal match |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, cmp := range d.Comparisons() {
		featureMatch := "false"
		if cmp.FeatureMatch {
			featureMatch = "true"
		}
		signalMatch := "false"
		if cmp.SignalMatch {
			signalMatch = "true"
		}
		live := cmp.LiveAction
		if live == "" {
			live = "-"
		}
		replay := cmp.ReplayAction
		if replay == "" {
			replay = "-"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %.9f | %s |\n",
			cmp.Ticker, cmp.Day, live, replay, featureMatch, cmp.MaxFeatureDelta, signalMatch))
	}
	b.WriteString("\n")
	b.WriteString(shadowDigestEndMarker)
	b.WriteString("\n")
	return b.String()
}

func UpdateShadowDigestFile(path string, section string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	start := strings.Index(content, shadowDigestStartMarker)
	end := strings.Index(content, shadowDigestEndMarker)
	switch {
	case start >= 0 && end >= 0:
		content = content[:start] + section + content[end+len(shadowDigestEndMarker):]
	case start >= 0:
		content = content[:start] + section
	default:
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += section
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
