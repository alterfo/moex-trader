package filltracking

import (
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const (
	StageExecutor  = "executor"
	StatusFilled   = "filled"
	StatusRejected = "rejected"
)

// Options configures the digest derived from executor audit events.
type Options struct {
	// MaxSlippagePct is the live marketable-limit cap (risk.max_slippage_pct).
	// It is carried through for cross-reference reporting only; it does not
	// affect the computed statistics.
	MaxSlippagePct decimal.Decimal
}

// Observation is one realized fill with its expected (bounded limit) price.
type Observation struct {
	Ticker        string
	Action        domain.Action
	ExpectedPrice decimal.Decimal
	ActualPrice   decimal.Decimal
	SlippageBps   decimal.Decimal
	Lots          int
}

// Digest summarizes fills and order statuses from executor audit events.
type Digest struct {
	Observations   []Observation
	StatusCounts   map[string]int
	Rejected       int
	Submitted      int
	MaxSlippagePct decimal.Decimal
}

// SlippageBps returns the signed realized-fill slippage in basis points of
// the expected bounded price. A positive value means the fill was better than
// expected (price improvement); a negative value means it was worse. A zero
// expected or actual price yields zero (no price bound to measure against).
func SlippageBps(expected, actual decimal.Decimal, action domain.Action) decimal.Decimal {
	if expected.Sign() <= 0 || actual.Sign() <= 0 {
		return decimal.Zero
	}
	var signed decimal.Decimal
	switch action {
	case domain.ActionBuy:
		signed = expected.Sub(actual)
	case domain.ActionSell:
		signed = actual.Sub(expected)
	default:
		return decimal.Zero
	}
	return signed.Div(expected).Mul(decimal.NewFromInt(10000))
}

type rawEvent struct {
	Status        string          `json:"status"`
	Ticker        string          `json:"ticker"`
	Action        domain.Action   `json:"action"`
	Lots          int             `json:"lots"`
	Price         decimal.Decimal `json:"price"`
	ExpectedPrice decimal.Decimal `json:"expected_price"`
}

// FromAuditEvents derives fill quality from persisted executor events. Each
// order's final event (fill, reject, cancel, new, partial, or unspecified) is
// counted once; the pre-submission intent is overwritten by the final event
// in the live executor, so it never appears twice.
func FromAuditEvents(events []domain.AuditEvent, opts Options) Digest {
	digest := Digest{
		StatusCounts:   make(map[string]int),
		MaxSlippagePct: opts.MaxSlippagePct,
	}
	for _, event := range events {
		if event.Stage != StageExecutor {
			continue
		}
		var raw rawEvent
		if err := json.Unmarshal([]byte(event.Payload), &raw); err != nil {
			continue
		}
		if raw.Action != domain.ActionBuy && raw.Action != domain.ActionSell {
			continue
		}
		ticker := strings.ToUpper(strings.TrimSpace(event.Ticker))
		if ticker == "" {
			ticker = strings.ToUpper(strings.TrimSpace(raw.Ticker))
		}
		if ticker == "" {
			continue
		}
		if raw.Status != "" {
			digest.Submitted++
			digest.StatusCounts[raw.Status]++
			if raw.Status == StatusRejected {
				digest.Rejected++
			}
			continue
		}
		if raw.Lots <= 0 || raw.Price.Sign() <= 0 {
			continue
		}
		digest.Submitted++
		digest.StatusCounts[StatusFilled]++
		digest.Observations = append(digest.Observations, Observation{
			Ticker:        ticker,
			Action:        raw.Action,
			ExpectedPrice: raw.ExpectedPrice,
			ActualPrice:   raw.Price,
			SlippageBps:   SlippageBps(raw.ExpectedPrice, raw.Price, raw.Action),
			Lots:          raw.Lots,
		})
	}
	return digest
}

func (d Digest) FillCount() int {
	return len(d.Observations)
}

// RejectionRate is the fraction of submitted orders rejected by the broker.
// It is zero when nothing has been submitted yet.
func (d Digest) RejectionRate() decimal.Decimal {
	if d.Submitted == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(d.Rejected)).Div(decimal.NewFromInt(int64(d.Submitted)))
}

func (d Digest) Count(status string) int {
	return d.StatusCounts[status]
}

func (d Digest) MeanSlippageBps() decimal.Decimal {
	if len(d.Observations) == 0 {
		return decimal.Zero
	}
	total := decimal.Zero
	for _, obs := range d.Observations {
		total = total.Add(obs.SlippageBps)
	}
	return total.Div(decimal.NewFromInt(int64(len(d.Observations))))
}

// MaxAdverseSlippageBps is the most negative observed slippage (worst fill).
// It is zero when no fill with a price bound has been observed.
func (d Digest) MaxAdverseSlippageBps() decimal.Decimal {
	worst := decimal.Zero
	for _, obs := range d.Observations {
		if obs.SlippageBps.LessThan(worst) {
			worst = obs.SlippageBps
		}
	}
	return worst
}

// MaxFavorableSlippageBps is the most positive observed slippage (best fill).
func (d Digest) MaxFavorableSlippageBps() decimal.Decimal {
	best := decimal.Zero
	for _, obs := range d.Observations {
		if obs.SlippageBps.GreaterThan(best) {
			best = obs.SlippageBps
		}
	}
	return best
}

// MeanSlippageBpsByTicker returns the average signed slippage per ticker.
func (d Digest) MeanSlippageBpsByTicker() map[string]decimal.Decimal {
	sums := make(map[string]decimal.Decimal)
	counts := make(map[string]int)
	for _, obs := range d.Observations {
		sums[obs.Ticker] = sums[obs.Ticker].Add(obs.SlippageBps)
		counts[obs.Ticker]++
	}
	out := make(map[string]decimal.Decimal, len(sums))
	for ticker, sum := range sums {
		out[ticker] = sum.Div(decimal.NewFromInt(int64(counts[ticker])))
	}
	return out
}
