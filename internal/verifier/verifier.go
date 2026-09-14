package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const (
	stageSignal   = "llm"
	stageExecutor = "executor"
)

type EventReader interface {
	ListAuditEvents(ctx context.Context, since time.Time) ([]domain.AuditEvent, error)
}

type Verifier struct {
	events EventReader
	now    func() time.Time
}

func New(events EventReader, now func() time.Time) *Verifier {
	if now == nil {
		now = time.Now
	}
	return &Verifier{events: events, now: now}
}

type LosingTrade struct {
	Ticker      string
	Direction   domain.Action
	Lots        int
	EntryPrice  decimal.Decimal
	ExitPrice   decimal.Decimal
	OpenedAt    time.Time
	ClosedAt    time.Time
	RealizedPnL decimal.Decimal
	Signal      *domain.TradeSignal
	Adequacy    string
}

type Report struct {
	GeneratedAt      time.Time
	Since            time.Time
	Until            time.Time
	ClosedTrades     int
	LosingTrades     int
	TotalRealizedPnL decimal.Decimal
	Trades           []LosingTrade
}

type fillRecord struct {
	ID         string          `json:"id"`
	Ticker     string          `json:"ticker"`
	Action     domain.Action   `json:"action"`
	Lots       int             `json:"lots"`
	Price      decimal.Decimal `json:"price"`
	ExecutedAt time.Time       `json:"executed_at"`
}

type tradeResult struct {
	ticker     string
	direction  domain.Action
	lots       int
	entryPrice decimal.Decimal
	exitPrice  decimal.Decimal
	openedAt   time.Time
	closedAt   time.Time
	pnl        decimal.Decimal
}

type position struct {
	direction domain.Action
	lots      int
	avgPrice  decimal.Decimal
	openedAt  time.Time
}

func (v *Verifier) Run(ctx context.Context, since time.Time) (*Report, error) {
	if v.events == nil {
		return nil, fmt.Errorf("verifier: event reader is nil")
	}

	now := v.now()
	events, err := v.events.ListAuditEvents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("verifier: list audit events: %w", err)
	}

	signals, fills := parseEvents(events)
	sortFills(fills)
	sortSignals(signals)

	positions := make(map[string]*position)
	results := make([]tradeResult, 0)
	for _, fill := range fills {
		if fill.Action != domain.ActionBuy && fill.Action != domain.ActionSell {
			continue
		}
		if fill.Lots <= 0 {
			continue
		}
		results = applyFill(positions, results, fill)
	}

	report := &Report{
		GeneratedAt: now,
		Since:       since,
		Until:       now,
	}
	for _, result := range results {
		if result.closedAt.Before(since) || result.closedAt.After(now) {
			continue
		}
		report.ClosedTrades++
		report.TotalRealizedPnL = report.TotalRealizedPnL.Add(result.pnl)
		if result.pnl.Sign() < 0 {
			report.Trades = append(report.Trades, buildLosingTrade(result, findSignal(signals[result.ticker], result.openedAt)))
		}
	}
	report.LosingTrades = len(report.Trades)

	return report, nil
}

func parseEvents(events []domain.AuditEvent) (map[string][]domain.TradeSignal, []fillRecord) {
	signals := make(map[string][]domain.TradeSignal)
	fills := make([]fillRecord, 0, len(events)/2)

	for _, event := range events {
		switch event.Stage {
		case stageSignal:
			var signal domain.TradeSignal
			if err := json.Unmarshal([]byte(event.Payload), &signal); err != nil {
				continue
			}
			if strings.TrimSpace(signal.Ticker) == "" {
				signal.Ticker = event.Ticker
			}
			if err := signal.Validate(); err != nil {
				continue
			}
			signals[signal.Ticker] = append(signals[signal.Ticker], signal)
		case stageExecutor:
			var fill fillRecord
			if err := json.Unmarshal([]byte(event.Payload), &fill); err != nil {
				continue
			}
			if strings.TrimSpace(fill.Ticker) == "" {
				fill.Ticker = event.Ticker
			}
			if fill.ExecutedAt.IsZero() {
				fill.ExecutedAt = event.CreatedAt
			}
			fills = append(fills, fill)
		}
	}

	return signals, fills
}

func sortFills(fills []fillRecord) {
	sort.SliceStable(fills, func(i, j int) bool {
		return fills[i].ExecutedAt.Before(fills[j].ExecutedAt)
	})
}

func sortSignals(signals map[string][]domain.TradeSignal) {
	for ticker := range signals {
		sort.SliceStable(signals[ticker], func(i, j int) bool {
			return signals[ticker][i].GeneratedAt.Before(signals[ticker][j].GeneratedAt)
		})
	}
}

func applyFill(positions map[string]*position, results []tradeResult, fill fillRecord) []tradeResult {
	pos := positions[fill.Ticker]
	if pos == nil {
		positions[fill.Ticker] = &position{
			direction: fill.Action,
			lots:      fill.Lots,
			avgPrice:  fill.Price,
			openedAt:  fill.ExecutedAt,
		}
		return results
	}

	if pos.direction == fill.Action {
		oldLots := decimal.NewFromInt(int64(pos.lots))
		newLots := decimal.NewFromInt(int64(pos.lots + fill.Lots))
		pos.avgPrice = pos.avgPrice.Mul(oldLots).Add(fill.Price.Mul(decimal.NewFromInt(int64(fill.Lots)))).Div(newLots)
		pos.lots += fill.Lots
		return results
	}

	closed := minInt(pos.lots, fill.Lots)
	var pnl decimal.Decimal
	if pos.direction == domain.ActionBuy {
		pnl = fill.Price.Sub(pos.avgPrice).Mul(decimal.NewFromInt(int64(closed)))
	} else {
		pnl = pos.avgPrice.Sub(fill.Price).Mul(decimal.NewFromInt(int64(closed)))
	}

	results = append(results, tradeResult{
		ticker:     fill.Ticker,
		direction:  pos.direction,
		lots:       closed,
		entryPrice: pos.avgPrice,
		exitPrice:  fill.Price,
		openedAt:   pos.openedAt,
		closedAt:   fill.ExecutedAt,
		pnl:        pnl,
	})

	pos.lots -= closed
	if pos.lots == 0 {
		delete(positions, fill.Ticker)
	}

	remainder := fill.Lots - closed
	if remainder > 0 {
		positions[fill.Ticker] = &position{
			direction: fill.Action,
			lots:      remainder,
			avgPrice:  fill.Price,
			openedAt:  fill.ExecutedAt,
		}
	}

	return results
}

func findSignal(signals []domain.TradeSignal, openedAt time.Time) *domain.TradeSignal {
	var best *domain.TradeSignal
	for i := range signals {
		candidate := &signals[i]
		if candidate.GeneratedAt.After(openedAt) {
			continue
		}
		if best == nil || candidate.GeneratedAt.After(best.GeneratedAt) {
			best = candidate
		}
	}
	return best
}

func buildLosingTrade(result tradeResult, signal *domain.TradeSignal) LosingTrade {
	trade := LosingTrade{
		Ticker:      result.ticker,
		Direction:   result.direction,
		Lots:        result.lots,
		EntryPrice:  result.entryPrice,
		ExitPrice:   result.exitPrice,
		OpenedAt:    result.openedAt,
		ClosedAt:    result.closedAt,
		RealizedPnL: result.pnl,
		Signal:      signal,
	}
	trade.Adequacy = adequacy(signal, result.direction)
	return trade
}

func adequacy(signal *domain.TradeSignal, direction domain.Action) string {
	if signal == nil {
		return fmt.Sprintf("Unknown — no LLM signal was recorded before this %s trade", directionWord(direction))
	}
	if signal.Action == domain.ActionHold {
		return fmt.Sprintf("Not applicable — the model returned HOLD and did not authorize this %s trade", directionWord(direction))
	}
	if signal.Action != direction {
		return fmt.Sprintf("No — the last signal was %s, but the losing trade was %s", signal.Action, directionWord(direction))
	}
	return fmt.Sprintf("No — the %s signal was followed by a losing %s trade", signal.Action, directionWord(direction))
}

func directionWord(direction domain.Action) string {
	if direction == domain.ActionBuy {
		return "long"
	}
	return "short"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
