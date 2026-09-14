package domain

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type Action string

const (
	ActionBuy  Action = "BUY"
	ActionSell Action = "SELL"
	ActionHold Action = "HOLD"
)

func (a Action) IsValid() bool {
	switch a {
	case ActionBuy, ActionSell, ActionHold:
		return true
	default:
		return false
	}
}

type FeatureContext struct {
	Ticker             string          `json:"ticker"`
	GeneratedAt        time.Time       `json:"generated_at"`
	LastPrice          decimal.Decimal `json:"last_price"`
	PrevClose          decimal.Decimal `json:"prev_close"`
	Bid                decimal.Decimal `json:"bid"`
	Ask                decimal.Decimal `json:"ask"`
	ReturnPct          decimal.Decimal `json:"return_pct"`
	RealizedVolatility decimal.Decimal `json:"realized_volatility"`
	NewsSentiment      decimal.Decimal `json:"news_sentiment"`
	NewsCount          int             `json:"news_count"`
	OrderBookImbalance decimal.Decimal `json:"order_book_imbalance"`
}

type TradeSignal struct {
	Ticker      string          `json:"ticker"`
	Action      Action          `json:"action"`
	Confidence  decimal.Decimal `json:"confidence"`
	TargetLots  int             `json:"target_lots"`
	Reasoning   string          `json:"reasoning"`
	GeneratedAt time.Time       `json:"generated_at"`
}

func (s TradeSignal) Validate() error {
	if strings.TrimSpace(s.Ticker) == "" {
		return NewValidationError("ticker must not be empty")
	}
	if !s.Action.IsValid() {
		return NewValidationError("action must be one of BUY, SELL, HOLD")
	}
	if s.Confidence.LessThan(decimal.Zero) {
		return NewValidationError("confidence must be in [0,1]")
	}
	if s.Confidence.GreaterThan(decimal.NewFromInt(1)) {
		return NewValidationError("confidence must be in [0,1]")
	}
	return nil
}

type AuditEvent struct {
	ID        string    `json:"id"`
	Ticker    string    `json:"ticker"`
	Stage     string    `json:"stage"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}
