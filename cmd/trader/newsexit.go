package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/risk"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

const (
	newsTradeStateEventID = "news-trade-state"
	newsTradeStateStage   = "news_trade_state"
)

type newsStateStore interface {
	GetAuditEvent(ctx context.Context, id string) (domain.AuditEvent, error)
	UpsertAuditEvent(ctx context.Context, event domain.AuditEvent) error
}

const newsTradePendingGrace = 15 * time.Minute

type newsExitConfig struct {
	TakeProfit decimal.Decimal
	Trail      decimal.Decimal
	StopLoss   decimal.Decimal
	MaxHold    time.Duration
	Cooldown   time.Duration
	StatePath  string
}

func newsExitConfigFrom(news config.News) newsExitConfig {
	return newsExitConfig{
		TakeProfit: news.ExitTakeProfitPct,
		Trail:      news.ExitTrailPct,
		StopLoss:   news.ExitStopLossPct,
		MaxHold:    news.ExitMaxHold.Std(),
		Cooldown:   news.ExitCooldown.Std(),
		StatePath:  news.ExitStatePath,
	}
}

type newsTrade struct {
	Ticker     string          `json:"ticker"`
	Short      bool            `json:"short"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	EntryAt    time.Time       `json:"entry_at"`
	PeakGain   decimal.Decimal `json:"peak_gain"`
	ExitReason string          `json:"exit_reason,omitempty"`
}

type newsTradeState struct {
	Trades    map[string]*newsTrade `json:"trades"`
	Cooldowns map[string]time.Time  `json:"cooldowns"`
	Incidents map[string]time.Time  `json:"incidents,omitempty"`
}

type newsExitManager struct {
	cfg       newsExitConfig
	positions risk.PositionReader
	now       func() time.Time
	store     newsStateStore

	mu    sync.Mutex
	state newsTradeState
}

func newNewsExitManager(cfg newsExitConfig, positions risk.PositionReader, now func() time.Time, store newsStateStore) (*newsExitManager, error) {
	if now == nil {
		now = time.Now
	}
	m := &newsExitManager{
		cfg:       cfg,
		positions: positions,
		now:       now,
		store:     store,
		state:     newsTradeState{Trades: map[string]*newsTrade{}, Cooldowns: map[string]time.Time{}, Incidents: map[string]time.Time{}},
	}
	var data []byte
	switch {
	case store != nil:
		event, err := store.GetAuditEvent(context.Background(), newsTradeStateEventID)
		if errors.Is(err, storage.ErrAuditEventNotFound) {
			return m, nil
		}
		if err != nil {
			return nil, fmt.Errorf("news exit: read state: %w", err)
		}
		data = []byte(event.Payload)
	case cfg.StatePath != "":
		var err error
		data, err = os.ReadFile(cfg.StatePath)
		if os.IsNotExist(err) {
			return m, nil
		}
		if err != nil {
			return nil, fmt.Errorf("news exit: read state %s: %w", cfg.StatePath, err)
		}
	default:
		return m, nil
	}
	var loaded newsTradeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, fmt.Errorf("news exit: parse state %s: %w", cfg.StatePath, err)
	}
	if loaded.Trades != nil {
		m.state.Trades = loaded.Trades
	}
	if loaded.Cooldowns != nil {
		m.state.Cooldowns = loaded.Cooldowns
	}
	if loaded.Incidents != nil {
		m.state.Incidents = loaded.Incidents
	}
	return m, nil
}

func (m *newsExitManager) saveLocked() error {
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	if m.store != nil {
		return m.store.UpsertAuditEvent(context.Background(), domain.AuditEvent{
			ID:        newsTradeStateEventID,
			Stage:     newsTradeStateStage,
			Payload:   string(data),
			CreatedAt: m.now(),
		})
	}
	if m.cfg.StatePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.cfg.StatePath), 0o755); err != nil {
		return err
	}
	tmp := m.cfg.StatePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.cfg.StatePath)
}

func (m *newsExitManager) shortSuppressed(ticker string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, open := m.state.Trades[ticker]; open {
		return true
	}
	until, ok := m.state.Cooldowns[ticker]
	return ok && m.now().Before(until)
}

func (m *newsExitManager) tracked(ticker string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.state.Trades[ticker]
	return ok
}

func (m *newsExitManager) noteShortEntry(ctx context.Context, ticker string, price decimal.Decimal) error {
	if m.positions == nil || price.Sign() <= 0 {
		return nil
	}
	lots, err := m.positions.CurrentLots(ctx, ticker)
	if err != nil {
		return err
	}
	if lots < 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.state.Trades[ticker]; ok {
		return nil
	}
	m.state.Trades[ticker] = &newsTrade{
		Ticker:     ticker,
		Short:      true,
		EntryPrice: price,
		EntryAt:    m.now(),
		PeakGain:   decimal.Zero,
	}
	return m.saveLocked()
}

type newsExitDecision struct {
	Signal domain.TradeSignal
	Reason string
	Gain   decimal.Decimal
	First  bool
}

func (m *newsExitManager) evaluate(ctx context.Context, ticker string, last decimal.Decimal) (*newsExitDecision, error) {
	m.mu.Lock()
	trade, ok := m.state.Trades[ticker]
	m.mu.Unlock()
	if !ok || m.positions == nil {
		return nil, nil
	}
	lots, err := m.positions.CurrentLots(ctx, ticker)
	if err != nil {
		return nil, err
	}
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	trade, ok = m.state.Trades[ticker]
	if !ok {
		return nil, nil
	}
	sameSide := (trade.Short && lots < 0) || (!trade.Short && lots > 0)
	if lots == 0 {
		if trade.ExitReason != "" || now.Sub(trade.EntryAt) > newsTradePendingGrace {
			delete(m.state.Trades, ticker)
			return nil, m.saveLocked()
		}
		return nil, nil
	}
	if !sameSide {
		delete(m.state.Trades, ticker)
		return nil, m.saveLocked()
	}
	if last.Sign() <= 0 || trade.EntryPrice.Sign() <= 0 {
		return nil, nil
	}

	gain := trade.EntryPrice.Sub(last).Div(trade.EntryPrice)
	if !trade.Short {
		gain = gain.Neg()
	}
	if gain.GreaterThan(trade.PeakGain) {
		trade.PeakGain = gain
	}

	reason := trade.ExitReason
	if reason == "" {
		switch {
		case gain.Neg().GreaterThanOrEqual(m.cfg.StopLoss):
			reason = "stop_loss"
		case trade.PeakGain.GreaterThanOrEqual(m.cfg.TakeProfit) && trade.PeakGain.Sub(gain).GreaterThanOrEqual(m.cfg.Trail):
			reason = "take_profit"
		case m.cfg.MaxHold > 0 && now.Sub(trade.EntryAt) >= m.cfg.MaxHold:
			reason = "time_exit"
		}
	}
	if reason == "" {
		return nil, m.saveLocked()
	}
	first := trade.ExitReason == ""
	if first {
		trade.ExitReason = reason
		if m.cfg.Cooldown > 0 {
			m.state.Cooldowns[ticker] = now.Add(m.cfg.Cooldown)
		}
		if err := m.saveLocked(); err != nil {
			return nil, err
		}
		m.recordExitLocked(trade, gain, now)
	}

	action := domain.ActionBuy
	if !trade.Short {
		action = domain.ActionSell
	}
	return &newsExitDecision{
		Reason: reason,
		Gain:   gain,
		First:  first,
		Signal: domain.TradeSignal{
			Ticker:      ticker,
			Action:      action,
			Confidence:  decimal.NewFromInt(1),
			Flat:        true,
			Reasoning:   fmt.Sprintf("выход из новостной позиции: %s, результат %s%%", reason, gain.Mul(decimal.NewFromInt(100)).StringFixed(2)),
			GeneratedAt: now,
		},
	}, nil
}

func (m *newsExitManager) recordExitLocked(trade *newsTrade, gain decimal.Decimal, now time.Time) {
	if m.store == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"ticker":      trade.Ticker,
		"short":       trade.Short,
		"reason":      trade.ExitReason,
		"entry_price": trade.EntryPrice.String(),
		"entry_at":    trade.EntryAt,
		"peak_gain":   trade.PeakGain.String(),
		"gain":        gain.String(),
		"held_sec":    int64(now.Sub(trade.EntryAt).Seconds()),
	})
	if err != nil {
		return
	}
	_ = m.store.UpsertAuditEvent(context.Background(), domain.AuditEvent{
		ID:        fmt.Sprintf("news-exit-%s-%d", trade.Ticker, trade.EntryAt.UnixNano()),
		Ticker:    trade.Ticker,
		Stage:     "news_exit",
		Payload:   string(payload),
		CreatedAt: now,
	})
}

func (m *newsExitManager) claimIncident(ticker string, first, latest time.Time, maxAge time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := m.state.Incidents[ticker]
	fresh := first.IsZero() || m.now().Sub(first) <= maxAge
	isNew := seen.IsZero() || first.IsZero() || first.After(seen)
	if latest.After(seen) {
		m.state.Incidents[ticker] = latest
		_ = m.saveLocked()
	}
	return isNew && fresh
}

func (m *newsExitManager) forceExit(ticker, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	trade, ok := m.state.Trades[ticker]
	if !ok || trade.ExitReason != "" {
		return
	}
	trade.ExitReason = reason
	if m.cfg.Cooldown > 0 {
		m.state.Cooldowns[ticker] = m.now().Add(m.cfg.Cooldown)
	}
	_ = m.saveLocked()
}
