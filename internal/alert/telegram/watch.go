package telegram

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	defaultProbabilityWindow = 20
	defaultCollapseFraction  = 0.9
	probabilityBandLo        = 0.45
	probabilityBandHi        = 0.55
	defaultStaleCandleAge    = 24 * time.Hour
	defaultCooldown          = time.Hour
)

type WatchConfig struct {
	ProbabilityWindow int
	CollapseFraction  float64
	StaleCandleAge    time.Duration
	Cooldown          time.Duration
	Now               func() time.Time
}

func DefaultWatchConfig() WatchConfig {
	return WatchConfig{
		ProbabilityWindow: defaultProbabilityWindow,
		CollapseFraction:  defaultCollapseFraction,
		StaleCandleAge:    defaultStaleCandleAge,
		Cooldown:          defaultCooldown,
	}
}

type Watch struct {
	client *Client
	cfg    WatchConfig

	mu           sync.Mutex
	probs        []float64
	lastCollapse time.Time
	lastStale    map[string]time.Time
	lastPSI      map[string]time.Time
}

func NewWatch(client *Client, cfg WatchConfig) *Watch {
	if cfg.ProbabilityWindow <= 0 {
		cfg.ProbabilityWindow = defaultProbabilityWindow
	}
	if cfg.CollapseFraction <= 0 || cfg.CollapseFraction > 1 {
		cfg.CollapseFraction = defaultCollapseFraction
	}
	if cfg.StaleCandleAge <= 0 {
		cfg.StaleCandleAge = defaultStaleCandleAge
	}
	if cfg.Cooldown < 0 {
		cfg.Cooldown = defaultCooldown
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Watch{
		client:    client,
		cfg:       cfg,
		lastStale: make(map[string]time.Time),
		lastPSI:   make(map[string]time.Time),
	}
}

func (w *Watch) ObserveProbability(ctx context.Context, probability float64) (bool, error) {
	if w == nil || w.client == nil || !w.client.Enabled() {
		return false, nil
	}

	w.mu.Lock()
	w.probs = append(w.probs, probability)
	if len(w.probs) > w.cfg.ProbabilityWindow {
		w.probs = w.probs[len(w.probs)-w.cfg.ProbabilityWindow:]
	}
	collapsed := w.collapsedLocked()
	now := w.cfg.Now()
	alert := collapsed && w.cooldownElapsed(now, w.lastCollapse)
	if alert {
		w.lastCollapse = now
	}
	w.mu.Unlock()

	if !alert {
		return false, nil
	}
	return true, w.client.Send(ctx, fmt.Sprintf(
		"MOEX trader: model probability collapse: more than %.0f%% of the last %d probabilities are inside [%.2f, %.2f]",
		w.cfg.CollapseFraction*100, w.cfg.ProbabilityWindow, probabilityBandLo, probabilityBandHi,
	))
}

func (w *Watch) ObserveCandleAge(ctx context.Context, ticker string, age time.Duration, inSession bool) (bool, error) {
	if w == nil || w.client == nil || !w.client.Enabled() {
		return false, nil
	}
	if !inSession || age <= w.cfg.StaleCandleAge {
		return false, nil
	}

	now := w.cfg.Now()
	w.mu.Lock()
	alert := w.cooldownElapsed(now, w.lastStale[ticker])
	if alert {
		w.lastStale[ticker] = now
	}
	w.mu.Unlock()

	if !alert {
		return false, nil
	}
	return true, w.client.Send(ctx, fmt.Sprintf(
		"MOEX trader: stale candle data for %s: newest candle is %s old",
		ticker, age.Round(time.Second),
	))
}

func (w *Watch) ObservePSI(ctx context.Context, feature string, psi float64) (bool, error) {
	if w == nil || w.client == nil || !w.client.Enabled() {
		return false, nil
	}

	now := w.cfg.Now()
	w.mu.Lock()
	alert := w.cooldownElapsed(now, w.lastPSI[feature])
	if alert {
		w.lastPSI[feature] = now
	}
	w.mu.Unlock()

	if !alert {
		return false, nil
	}
	return true, w.client.Send(ctx, fmt.Sprintf("MOEX trader: feature drift breach: %s PSI %.3f", feature, psi))
}

func (w *Watch) collapsedLocked() bool {
	if len(w.probs) < w.cfg.ProbabilityWindow {
		return false
	}
	inside := 0
	for _, probability := range w.probs {
		if probability >= probabilityBandLo && probability <= probabilityBandHi {
			inside++
		}
	}
	return float64(inside)/float64(len(w.probs)) > w.cfg.CollapseFraction
}

func (w *Watch) cooldownElapsed(now, last time.Time) bool {
	if w.cfg.Cooldown == 0 {
		return true
	}
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= w.cfg.Cooldown
}

func TradingSessionActive(now time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = mskLocation()
	}
	local := now.In(loc)
	weekday := local.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	minutes := local.Hour()*60 + local.Minute()
	return minutes >= 10*60 && minutes < 18*60+45
}

func mskLocation() *time.Location {
	if loc, err := time.LoadLocation("Europe/Moscow"); err == nil {
		return loc
	}
	return time.FixedZone("MSK", 3*3600)
}
