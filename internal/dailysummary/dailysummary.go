package dailysummary

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

// Fill is one executed bot fill, decoded from the executor audit events. The
// average-cost accounting below is an independent copy of the logic proven in
// internal/risk (tickerState.record): entry basis includes the entry
// commission, realP&L subtracts the exit commission on the closed portion.
type Fill struct {
	Ticker     string
	Action     domain.Action
	Lots       int
	Price      decimal.Decimal
	Commission decimal.Decimal
	ExecutedAt time.Time
}

type parsedFill struct {
	Ticker     string          `json:"ticker"`
	Action     domain.Action   `json:"action"`
	Lots       int             `json:"lots"`
	Price      decimal.Decimal `json:"price"`
	Commission decimal.Decimal `json:"commission"`
	ExecutedAt time.Time       `json:"executed_at"`
}

// FillsFromAuditEvents decodes executor-stage audit events into fills,
// skipping events that do not parse as a real fill (e.g. live order intents
// and status records share the same stage).
func FillsFromAuditEvents(events []domain.AuditEvent) []Fill {
	out := make([]Fill, 0, len(events))
	for _, event := range events {
		if event.Stage != "executor" {
			continue
		}
		var parsed parsedFill
		if err := json.Unmarshal([]byte(event.Payload), &parsed); err != nil {
			continue
		}
		if parsed.Ticker == "" || parsed.Lots <= 0 || parsed.Price.Sign() <= 0 {
			continue
		}
		out = append(out, Fill{
			Ticker:     parsed.Ticker,
			Action:     parsed.Action,
			Lots:       parsed.Lots,
			Price:      parsed.Price,
			Commission: parsed.Commission,
			ExecutedAt: parsed.ExecutedAt,
		})
	}
	return out
}

// Close is the latest settled daily candle for a ticker together with the
// previous one. Date holds the latest candle's begin time in MSK wall-clock
// as reported by ISS (parsed without an offset).
type Close struct {
	Ticker    string
	Date      time.Time
	Close     decimal.Decimal
	PrevClose decimal.Decimal
}

// Valid reports whether both the latest and the previous close are available.
func (c Close) Valid() bool {
	return c.Close.IsPositive() && c.PrevClose.IsPositive() && !c.Date.IsZero()
}

// CandleSource fetches daily candle history. backtest.ISSSource implements it.
type CandleSource interface {
	History(ctx context.Context, ticker string, from, till time.Time) ([]moex.Candle, error)
}

// DayCloses fetches the latest vs previous daily close for every ticker plus
// IMOEX. The window covers the six days before the reference day so the
// previous candle survives market holidays. A failure to fetch a single
// ticker only skips that ticker; a failure to fetch the IMOEX anchor is fatal
// because the trading-day gate depends on it.
func DayCloses(ctx context.Context, src CandleSource, tickers []string, day time.Time) (map[string]Close, error) {
	from := day.AddDate(0, 0, -6)
	till := day.AddDate(0, 0, 1)

	uniq := make([]string, 0, len(tickers)+1)
	seen := make(map[string]bool)
	for _, ticker := range tickers {
		key := normTicker(ticker)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, key)
	}
	if !seen["IMOEX"] {
		uniq = append(uniq, "IMOEX")
	}

	type outcome struct {
		symbol string
		close  Close
		has    bool
		err    error
	}
	ch := make(chan outcome, len(uniq))
	var wg sync.WaitGroup
	for _, symbol := range uniq {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			candles, err := src.History(ctx, symbol, from, till)
			if err != nil {
				ch <- outcome{symbol: symbol, err: err}
				return
			}
			if len(candles) == 0 {
				ch <- outcome{symbol: symbol}
				return
			}
			last := candles[len(candles)-1]
			c := Close{Ticker: symbol, Date: last.Begin, Close: last.Close}
			if len(candles) > 1 {
				c.PrevClose = candles[len(candles)-2].Close
			}
			ch <- outcome{symbol: symbol, close: c, has: true}
		}(symbol)
	}
	wg.Wait()
	close(ch)

	result := make(map[string]Close, len(uniq))
	var anchorErr error
	for o := range ch {
		if !o.has {
			if o.symbol == "IMOEX" && o.err != nil && anchorErr == nil {
				anchorErr = o.err
			}
			continue
		}
		result[normTicker(o.symbol)] = o.close
	}
	if anchorErr != nil {
		return nil, fmt.Errorf("daily before IMOEX close: %w", anchorErr)
	}
	return result, nil
}

// position is the average-cost accounting state for one ticker. It mirrors
// risk.tickerState.record line for line so the summary cannot disagree with
// the breaker.
type position struct {
	openLots int
	avgEntry decimal.Decimal
}

// record applies one fill and returns the realized P&L it produced
// (negative for a loss). Only fills that reduce or close a position realize
// anything.
func (s *position) record(f Fill) decimal.Decimal {
	delta := f.Lots
	if f.Action == domain.ActionSell {
		delta = -delta
	}
	if delta == 0 {
		return decimal.Zero
	}

	if s.openLots == 0 {
		s.openLots = delta
		s.avgEntry = entryBasis(f.Action, f.Price, f.Commission.Div(decimal.NewFromInt(int64(f.Lots))))
		return decimal.Zero
	}

	if (s.openLots > 0) == (delta > 0) {
		oldAbs := absInt(s.openLots)
		addAbs := absInt(delta)
		newAbs := oldAbs + addAbs
		entryCost := entryBasis(f.Action, f.Price, f.Commission.Div(decimal.NewFromInt(int64(addAbs))))
		s.avgEntry = s.avgEntry.Mul(decimal.NewFromInt(int64(oldAbs))).
			Add(entryCost.Mul(decimal.NewFromInt(int64(addAbs)))).
			Div(decimal.NewFromInt(int64(newAbs)))
		s.openLots += delta
		return decimal.Zero
	}

	openAbs := absInt(s.openLots)
	deltaAbs := absInt(delta)
	closeLots := openAbs
	if deltaAbs < openAbs {
		closeLots = deltaAbs
	}

	var pnl decimal.Decimal
	if s.openLots > 0 {
		pnl = f.Price.Sub(s.avgEntry).Mul(decimal.NewFromInt(int64(closeLots)))
	} else {
		pnl = s.avgEntry.Sub(f.Price).Mul(decimal.NewFromInt(int64(closeLots)))
	}
	perLotCommission := f.Commission.Div(decimal.NewFromInt(int64(f.Lots)))
	pnl = pnl.Sub(perLotCommission.Mul(decimal.NewFromInt(int64(closeLots))))

	s.openLots -= closeLots
	if s.openLots == 0 {
		s.avgEntry = decimal.Zero
	}

	remainder := deltaAbs - closeLots
	if remainder > 0 {
		s.openLots = remainder
		if delta < 0 {
			s.openLots = -s.openLots
		}
		s.avgEntry = entryBasis(f.Action, f.Price, perLotCommission)
	}
	return pnl
}

func entryBasis(action domain.Action, price, perLotCommission decimal.Decimal) decimal.Decimal {
	if action == domain.ActionSell {
		return price.Sub(perLotCommission)
	}
	return price.Add(perLotCommission)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// BotDay is the trader's result for one trading day, derived from its own
// fills (it has no visibility into manual trades).
type BotDay struct {
	Trades        int
	Realized      decimal.Decimal // realized from today's fills, net of commission
	DayStartMTM   decimal.Decimal // open positions marked to yesterday's close
	EndMTM        decimal.Decimal // open positions marked to today's close
	OpenPositions int
}

// PnL is the day's net P&L: realized today plus the mark-to-market change of
// open positions between yesterday's and today's close.
func (d BotDay) PnL() decimal.Decimal {
	return d.Realized.Add(d.EndMTM.Sub(d.DayStartMTM))
}

// MTMChange is the day's unrealized swing on open positions.
func (d BotDay) MTMChange() decimal.Decimal {
	return d.EndMTM.Sub(d.DayStartMTM)
}

// Account replays the full fill history and derives the trader's result for
// the MSK trading day containing now. closes keys are normalized tickers;
// positions without a close are excluded from the MTM totals.
func Account(fills []Fill, now time.Time, loc *time.Location, closes map[string]Close) BotDay {
	day := now.In(loc).Format("2006-01-02")

	sorted := append([]Fill(nil), fills...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ExecutedAt.Before(sorted[j].ExecutedAt)
	})

	states := make(map[string]*position)
	var dayStart map[string]*position
	snapshotted := false
	var result BotDay

	for _, f := range sorted {
		isToday := f.ExecutedAt.In(loc).Format("2006-01-02") == day
		if isToday && !snapshotted {
			dayStart = snapshotPositions(states)
			snapshotted = true
		}
		key := normTicker(f.Ticker)
		st := states[key]
		if st == nil {
			st = &position{}
			states[key] = st
		}
		realized := st.record(f)
		if isToday {
			result.Trades++
			result.Realized = result.Realized.Add(realized)
		}
	}
	if !snapshotted {
		dayStart = snapshotPositions(states)
	}

	result.DayStartMTM = markToMarket(dayStart, closes, true)
	result.EndMTM = markToMarket(states, closes, false)
	for _, st := range states {
		if st.openLots != 0 {
			result.OpenPositions++
		}
	}
	return result
}

func snapshotPositions(states map[string]*position) map[string]*position {
	out := make(map[string]*position, len(states))
	for key, st := range states {
		copy := *st
		out[key] = &copy
	}
	return out
}

func markToMarket(states map[string]*position, closes map[string]Close, atPrevClose bool) decimal.Decimal {
	total := decimal.Zero
	for ticker, st := range states {
		if st.openLots == 0 {
			continue
		}
		close, ok := closes[normTicker(ticker)]
		if !ok || !close.Valid() {
			continue
		}
		mark := close.Close
		if atPrevClose {
			mark = close.PrevClose
		}
		side := decimal.NewFromInt(int64(absInt(st.openLots)))
		if st.openLots < 0 {
			side = side.Neg()
		}
		total = total.Add(mark.Sub(st.avgEntry).Mul(side))
	}
	return total
}

// Market is the daily market direction: the last IMOEX move and the share of
// configured tickers that closed green on the anchor trading day.
type Market struct {
	IndexPct     decimal.Decimal
	IndexPresent bool
	UpTickers    int
	TotalTickers int
}

// MarketDirection tallies the moves against closes. anchorDay is the MSK
// trading day (date of the latest IMOEX candle, formatted YYYY-MM-DD) or the
// empty string when unavailable; tickers without a candle on that day are
// treated as no data and excluded from the denominator.
func MarketDirection(imoex Close, closes map[string]Close, tickers []string, anchorDay string) Market {
	var m Market
	if imoex.Valid() {
		m.IndexPresent = true
		m.IndexPct = pctChange(imoex.PrevClose, imoex.Close)
	}
	for _, ticker := range tickers {
		c, ok := closes[normTicker(ticker)]
		if !ok || !c.Valid() {
			continue
		}
		if anchorDay != "" && c.Date.Format("2006-01-02") != anchorDay {
			continue
		}
		m.TotalTickers++
		if c.Close.GreaterThan(c.PrevClose) {
			m.UpTickers++
		}
	}
	return m
}

// AnchorDay formats the IMOEX candle's trading day as YYYY-MM-DD and reports
// whether it is the MSK trading day containing ref. The evening digest is only
// sent when the anchor equals the current day, which is how weekends and
// exchange holidays are skipped.
func AnchorDay(imoex Close, ref time.Time, loc *time.Location) (string, bool) {
	if imoex.Date.IsZero() {
		return "", false
	}
	anchor := imoex.Date.Format("2006-01-02")
	return anchor, anchor == ref.In(loc).Format("2006-01-02")
}

func pctChange(prev, current decimal.Decimal) decimal.Decimal {
	if !prev.IsPositive() {
		return decimal.Zero
	}
	return current.Sub(prev).Div(prev).Mul(decimal.NewFromInt(100))
}

// Message formats the brief evening digest sent to Telegram.
func Message(day time.Time, bot BotDay, market Market, loc *time.Location) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📊 Дневная сводка · %s\n", day.In(loc).Format("02.01.2006"))

	if market.IndexPresent || market.TotalTickers > 0 {
		b.WriteString("Рынок: ")
		if market.IndexPresent {
			fmt.Fprintf(&b, "IMOEX %s", signedPct(market.IndexPct))
		}
		if market.TotalTickers > 0 {
			if market.IndexPresent {
				b.WriteString(" · ")
			}
			pct := int((market.UpTickers*100 + market.TotalTickers/2) / market.TotalTickers)
			fmt.Fprintf(&b, "в плюсе %d/%d (%d%%)", market.UpTickers, market.TotalTickers, pct)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("Рынок: нет данных\n")
	}

	fmt.Fprintf(&b, "Бот: сделок %d · день %s ₽ (реализ. %s · MTM %s₽)",
		bot.Trades, money(bot.PnL()), money(bot.Realized), money(bot.MTMChange()))
	return b.String()
}

func signedPct(v decimal.Decimal) string {
	return money(v) + "%"
}

// money formats a decimal as a signed amount with thousands grouping,
// e.g. +1234.5 -> "+1 234.50".
func money(v decimal.Decimal) string {
	rounded := v.Round(2)
	sign := "+"
	if rounded.IsNegative() {
		sign = "-"
		rounded = rounded.Neg()
	}
	whole := rounded.IntPart()
	frac := strings.TrimPrefix(rounded.Sub(decimal.NewFromInt(whole)).StringFixed(2), "0")
	return sign + groupThousands(whole) + frac
}

func groupThousands(v int64) string {
	digits := strconv.FormatInt(v, 10)
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	first := len(digits) % 3
	if first > 0 {
		b.WriteString(digits[:first])
	}
	for i := first; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// NextFire returns the next occurrence of the daily fire time (offset from
// midnight) in loc strictly after now.
func NextFire(now time.Time, fire time.Duration, loc *time.Location) time.Time {
	local := now.In(loc)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).Add(fire)
	if !candidate.After(now) {
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate
}

// MSKLocation returns Europe/Moscow falling back to a fixed +03:00 zone.
func MSKLocation() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*3600)
	}
	return loc
}

func normTicker(ticker string) string {
	return strings.ToUpper(strings.TrimSpace(ticker))
}
