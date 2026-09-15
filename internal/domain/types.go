package domain

import (
	"fmt"
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

// HoldReason distinguishes why a HOLD was emitted. In backtest analytics a
// HOLD produced by the model must not be conflated with a HOLD produced by an
// LLM timeout or an unparseable response, otherwise the metrics lie.
const (
	HoldReasonModel      = "model"      // the model itself chose HOLD
	HoldReasonTimeout    = "timeout"    // the LLM call timed out (host slow/unreachable)
	HoldReasonInvalid    = "invalid"    // LLM answered but output failed validation
	HoldReasonError      = "error"      // signal source errored for another reason
	HoldReasonConfidence = "confidence" // signal filtered out below min-confidence gate
)

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
	Mom5d              decimal.Decimal `json:"mom_5d"`
	Mom21d             decimal.Decimal `json:"mom_21d"`
	Mom63d             decimal.Decimal `json:"mom_63d"`
	Reversal1d         decimal.Decimal `json:"reversal_1d"`
	RSI14              decimal.Decimal `json:"rsi_14"`
	DistMA20Pct        decimal.Decimal `json:"dist_ma20_pct"`
	DistMA50Pct        decimal.Decimal `json:"dist_ma50_pct"`
	RealizedVol21d     decimal.Decimal `json:"realized_vol_21d_annualized_pct"`
	VolumeZScore20d    decimal.Decimal `json:"volume_zscore_20d"`
}

type TradeSignal struct {
	Ticker      string          `json:"ticker"`
	Action      Action          `json:"action"`
	Confidence  decimal.Decimal `json:"confidence"`
	TargetLots  int             `json:"target_lots"`
	Reasoning   string          `json:"reasoning"`
	GeneratedAt time.Time       `json:"generated_at"`
	HoldReason  string          `json:"hold_reason,omitempty"`
}

func (s TradeSignal) Validate() error {
	if strings.TrimSpace(s.Ticker) == "" {
		return fmt.Errorf("ticker must not be empty")
	}
	if !s.Action.IsValid() {
		return fmt.Errorf("action must be one of BUY, SELL, HOLD")
	}
	if s.Confidence.LessThan(decimal.Zero) {
		return fmt.Errorf("confidence must be in [0,1]")
	}
	if s.Confidence.GreaterThan(decimal.NewFromInt(1)) {
		return fmt.Errorf("confidence must be in [0,1]")
	}
	if (s.Action == ActionBuy || s.Action == ActionSell) && s.TargetLots <= 0 {
		return fmt.Errorf("target lots must be positive for BUY/SELL")
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
