package backtest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	"github.com/olegsidorkin/moex-trader/internal/risk"
)

const (
	DefaultWarmupDays = 100
	periodsPerYear    = 252.0
)

type SignalSource interface {
	Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error)
}

type EquityPoint struct {
	Date   time.Time
	Equity decimal.Decimal
}

type Trade struct {
	Ticker     string
	Action     domain.Action
	Lots       int
	EntryPrice decimal.Decimal
	ExitPrice  decimal.Decimal
	OpenedAt   time.Time
	ClosedAt   time.Time
	GrossPnl   decimal.Decimal
	Commission decimal.Decimal
	NetPnl     decimal.Decimal
	SignalNote string
}

type Result struct {
	Tickers           []string
	Start             time.Time
	End               time.Time
	Deposit           decimal.Decimal
	FinalEquity       decimal.Decimal
	NetPnl            decimal.Decimal
	RealizedPnl       decimal.Decimal
	UnrealizedPnl     decimal.Decimal
	GrossPnl          decimal.Decimal
	TotalCommission   decimal.Decimal
	ClosedTrades      int
	WinningTrades     int
	HitRate           float64
	Sharpe            float64
	MaxDrawdownPct    float64
	MaxDrawdownRub    decimal.Decimal
	KillSwitchTripped bool
	Decisions         int
	HoldReasons       map[string]int
	Trades            []Trade
	EquityCurve       []EquityPoint
}

type Config struct {
	Tickers               []string
	From                  time.Time
	Till                  time.Time
	Deposit               decimal.Decimal
	MaxLots               int
	CommissionRate        decimal.Decimal
	SpreadPct             decimal.Decimal
	SlippagePct           decimal.Decimal
	WarmupDays            int
	MaxDecisionsPerTicker int
	MaxHoldBars           int
	KillSwitch            bool
	SignalSource          SignalSource
	Source                HistoricalSource
	FeatureConfig         features.PriceFeatureConfig
	Logger                *log.Logger
	NewsOverrides         map[string]map[string]NewsAggregate
	EventOverrides        map[string]map[string]features.EventFlags
}

type NewsAggregate struct {
	Sentiment float64
	Count     int
}

type EventOverrides struct {
	News   map[string]map[string]NewsAggregate
	Events map[string]map[string]features.EventFlags
}

func LoadNewsOverrides(path string) (map[string]map[string]NewsAggregate, map[string]map[string]features.EventFlags, error) {
	type record struct {
		Ticker    string  `json:"ticker"`
		Sentiment float64 `json:"sentiment"`
		PubTS     int64   `json:"published_ts"`
		Title     string  `json:"title"`
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("backtest: open news history: %w", err)
	}
	defer f.Close()
	type accum struct {
		wSum float64
		w    float64
		n    int
	}
	acc := make(map[string]map[string]*accum)
	evAcc := make(map[string]map[string]*features.EventFlags)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		var r record
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		if r.Ticker == "" || r.PubTS <= 0 {
			continue
		}
		ticker := strings.ToUpper(strings.TrimSpace(r.Ticker))
		date := time.Unix(r.PubTS, 0).UTC().Format("2006-01-02")
		byDate, ok := acc[ticker]
		if !ok {
			byDate = make(map[string]*accum)
			acc[ticker] = byDate
		}
		a, ok := byDate[date]
		if !ok {
			a = &accum{}
			byDate[date] = a
		}
		a.wSum += r.Sentiment
		a.w += 1.0
		a.n++
		if r.Title != "" {
			flags := features.DetectEvents(r.Title)
			evByDate, ok := evAcc[ticker]
			if !ok {
				evByDate = make(map[string]*features.EventFlags)
				evAcc[ticker] = evByDate
			}
			ea, ok := evByDate[date]
			if !ok {
				ea = &features.EventFlags{}
				evByDate[date] = ea
			}
			ea.Dividend += flags.Dividend
			ea.Buyback += flags.Buyback
			ea.Sanctions += flags.Sanctions
			ea.IPO += flags.IPO
			ea.Report += flags.Report
			ea.Delisting += flags.Delisting
			ea.MNA += flags.MNA
			ea.Default += flags.Default
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("backtest: read news: %w", err)
	}
	out := make(map[string]map[string]NewsAggregate, len(acc))
	for ticker, byDate := range acc {
		out[ticker] = make(map[string]NewsAggregate, len(byDate))
		for d, a := range byDate {
			sent := 0.0
			if a.w > 0 {
				sent = a.wSum / a.w
			}
			out[ticker][d] = NewsAggregate{Sentiment: sent, Count: a.n}
		}
	}
	evOut := make(map[string]map[string]features.EventFlags, len(evAcc))
	for ticker, byDate := range evAcc {
		evOut[ticker] = make(map[string]features.EventFlags, len(byDate))
		for d, a := range byDate {
			evOut[ticker][d] = *a
		}
	}
	return out, evOut, nil
}

func (c Config) WithDefaults() Config {
	if c.Deposit.Sign() <= 0 {
		c.Deposit = decimal.NewFromInt(100_000)
	}
	if c.MaxLots <= 0 {
		c.MaxLots = 1
	}
	if c.CommissionRate.IsNegative() {
		c.CommissionRate = decimal.Zero
	}
	if c.SpreadPct.IsNegative() {
		c.SpreadPct = decimal.Zero
	}
	if c.SlippagePct.IsNegative() {
		c.SlippagePct = decimal.Zero
	}
	if c.WarmupDays <= 0 {
		c.WarmupDays = DefaultWarmupDays
	}
	return c
}

type Engine struct {
	cfg       Config
	gate      risk.Gate
	kill      *memKillSwitch
	positions map[string]*position
	realized  map[string]decimal.Decimal
	trades    []Trade
	decisions int
	holds     map[string]int

	mu sync.Mutex
}

type memKillSwitch struct {
	mu     sync.Mutex
	active bool
}

func (m *memKillSwitch) IsKillSwitchActive(context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active, nil
}

func (m *memKillSwitch) SetKillSwitchActive(_ context.Context, active bool) error {
	m.mu.Lock()
	m.active = active
	m.mu.Unlock()
	return nil
}

func NewEngine(cfg Config) (*Engine, error) {
	cfg = cfg.WithDefaults()
	if len(cfg.Tickers) == 0 {
		return nil, fmt.Errorf("backtest: tickers must not be empty")
	}
	if cfg.Source == nil {
		return nil, fmt.Errorf("backtest: historical source is required")
	}
	if cfg.SignalSource == nil {
		return nil, fmt.Errorf("backtest: signal source is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	engine := &Engine{
		cfg:       cfg,
		kill:      &memKillSwitch{},
		positions: make(map[string]*position),
		realized:  make(map[string]decimal.Decimal),
		holds:     make(map[string]int),
	}

	riskCfg := risk.DefaultConfig()
	riskCfg.MaxLots = cfg.MaxLots
	riskCfg.Positions = engine
	if cfg.KillSwitch {
		riskCfg.Store = engine.kill
	}
	gate, err := risk.NewHardenedGate(riskCfg)
	if err != nil {
		return nil, fmt.Errorf("backtest: risk gate: %w", err)
	}
	engine.gate = gate
	return engine, nil
}

func (e *Engine) CurrentLots(_ context.Context, ticker string) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	pos := e.positions[ticker]
	if pos == nil {
		return 0, nil
	}
	if pos.action == domain.ActionSell {
		return -pos.lots, nil
	}
	return pos.lots, nil
}

func (e *Engine) Run(ctx context.Context) (*Result, error) {
	var curves []map[time.Time]decimal.Decimal
	for _, ticker := range tickersNormalized(e.cfg.Tickers) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		curve, err := e.runTicker(ctx, ticker)
		if err != nil {
			e.cfg.Logger.Printf("backtest: %s: skipping (history failed): %v", ticker, err)
			continue
		}
		curves = append(curves, curve)
	}
	aggregateCurve := aggregateCurves(curves, e.cfg.Deposit)
	return e.buildResult(aggregateCurve), nil
}

func (e *Engine) runTicker(ctx context.Context, ticker string) (map[time.Time]decimal.Decimal, error) {
	from := e.cfg.From
	if from.IsZero() {
		from = time.Now().AddDate(0, 0, -365)
	}
	till := e.cfg.Till
	if till.IsZero() {
		till = time.Now()
	}
	fetchFrom := from.AddDate(0, 0, -e.cfg.WarmupDays)
	if extra := e.cfg.FeatureConfig.WarmupCalendarDays(); extra > e.cfg.WarmupDays {
		fetchFrom = from.AddDate(0, 0, -extra)
	}

	candles, err := e.cfg.Source.History(ctx, ticker, fetchFrom, till)
	if err != nil {
		return nil, fmt.Errorf("backtest: history %s: %w", ticker, err)
	}
	warmup := e.cfg.FeatureConfig.WarmupCandles()
	if len(candles) < warmup+1 {
		e.cfg.Logger.Printf("backtest: %s: only %d candles in history, skipping", ticker, len(candles))
		return map[time.Time]decimal.Decimal{}, nil
	}

	builder := features.NewBuilderWithConfig(time.Now, e.cfg.FeatureConfig)
	curve := make(map[time.Time]decimal.Decimal)

	tradeable := make([]int, 0, 64)
	for d := warmup; d < len(candles); d++ {
		if candles[d].Begin.Before(from) {
			continue
		}
		tradeable = append(tradeable, d)
	}
	if e.cfg.MaxDecisionsPerTicker > 0 && len(tradeable) > e.cfg.MaxDecisionsPerTicker {
		e.cfg.Logger.Printf("backtest: %s: limiting to last %d of %d tradeable days (lookback-days)", ticker, e.cfg.MaxDecisionsPerTicker, len(tradeable))
		tradeable = tradeable[len(tradeable)-e.cfg.MaxDecisionsPerTicker:]
	}

	for _, d := range tradeable {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		active, _ := e.kill.IsKillSwitchActive(ctx)
		if active {
			e.cfg.Logger.Printf("backtest: kill switch active, skipping %s on %s", ticker, candles[d].Begin.Format("2006-01-02"))
			continue
		}

		decisionDay := candles[d].Begin
		execPrice := decisionPrice(candles, d)
		if execPrice.Sign() <= 0 {
			continue
		}

		if e.cfg.MaxHoldBars > 0 && e.expirePosition(ticker, execPrice, decisionDay) {
			curve[decisionDay] = e.tickerPnL(ticker, candles[d].Close)
			continue
		}

		input := features.Input{
			Ticker: ticker,
			Price: features.PriceSnapshot{
				LastPrice: candles[d-1].Close,
				PrevClose: candles[d-2].Close,
				AsOf:      decisionDay,
			},
			Candles: candles[:d],
		}

		feature, err := builder.Build(input)
		if err != nil {
			e.cfg.Logger.Printf("backtest: %s on %s: build features: %v", ticker, decisionDay.Format("2006-01-02"), err)
			continue
		}

		if e.cfg.NewsOverrides != nil {
			dateKey := decisionDay.Format("2006-01-02")
			if byDate, ok := e.cfg.NewsOverrides[ticker]; ok {
				if agg, ok := byDate[dateKey]; ok {
					feature.NewsSentiment = decimal.NewFromFloat(agg.Sentiment)
					feature.NewsCount = agg.Count
				}
			}
		}
		if e.cfg.EventOverrides != nil {
			dateKey := decisionDay.Format("2006-01-02")
			if byDate, ok := e.cfg.EventOverrides[ticker]; ok {
				if ev, ok := byDate[dateKey]; ok {
					feature.EventDividend = ev.Dividend
					feature.EventBuyback = ev.Buyback
					feature.EventSanctions = ev.Sanctions
					feature.EventIPO = ev.IPO
					feature.EventReport = ev.Report
					feature.EventDelisting = ev.Delisting
					feature.EventMNA = ev.MNA
					feature.EventDefault = ev.Default
				}
			}
		}

		reason := domain.HoldReasonModel
		signal, err := e.cfg.SignalSource.Generate(ctx, feature)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				reason = domain.HoldReasonTimeout
			} else {
				reason = domain.HoldReasonError
			}
			signal = domain.TradeSignal{Action: domain.ActionHold, HoldReason: reason, GeneratedAt: decisionDay}
		} else if signal.Action == domain.ActionHold {
			if signal.HoldReason == "" {
				signal.HoldReason = domain.HoldReasonModel
			}
			reason = signal.HoldReason
		}
		e.recordDecision(signal)

		if err != nil {
			e.cfg.Logger.Printf("backtest: %s on %s: signal: %v", ticker, decisionDay.Format("2006-01-02"), err)
			curve[decisionDay] = e.tickerPnL(ticker, candles[d].Close)
			continue
		}

		approved, err := e.gate.Approve(ctx, risk.Request{
			Signal: signal,
			Market: risk.Market{
				OrderPrice: execPrice,
				PrevClose:  candles[d].Close,
			},
			Account: risk.Account{
				Deposit:       e.cfg.Deposit,
				CurrentEquity: e.cfg.Deposit.Add(e.tickerPnL(ticker, candles[d].Close)),
			},
		})
		if err != nil {
			e.cfg.Logger.Printf("backtest: %s on %s: risk gate: %v", ticker, decisionDay.Format("2006-01-02"), err)
			curve[decisionDay] = e.tickerPnL(ticker, candles[d].Close)
			continue
		}

		if signal.Action != domain.ActionHold && approved {
			fillPrice := e.fillPrice(execPrice, signal.Action)
			if err := e.recordFill(ticker, signal, fillPrice, decisionDay); err != nil {
				e.cfg.Logger.Printf("backtest: %s on %s: record fill: %v", ticker, decisionDay.Format("2006-01-02"), err)
			}
		}
		curve[decisionDay] = e.tickerPnL(ticker, candles[d].Close)
	}
	return curve, nil
}

func decisionPrice(candles []moex.Candle, d int) decimal.Decimal {
	if d+1 < len(candles) {
		return candles[d+1].Open
	}
	return candles[d].Close
}

// fillPrice applies half-spread and slippage against the trader: a BUY fills
// above the quoted price, a SELL fills below it. Both costs are modeled as a
// fraction of price (e.g. 0.0005 = 0.05%) and only affect actual fills, not
// the mark-to-market curve or the risk gate's price check.
func (e *Engine) fillPrice(price decimal.Decimal, action domain.Action) decimal.Decimal {
	cost := e.cfg.SpreadPct.Add(e.cfg.SlippagePct)
	if cost.Sign() <= 0 {
		return price
	}
	if action == domain.ActionSell {
		return price.Mul(decimal.NewFromInt(1).Sub(cost))
	}
	return price.Mul(decimal.NewFromInt(1).Add(cost))
}

func (e *Engine) recordFill(ticker string, signal domain.TradeSignal, price decimal.Decimal, day time.Time) error {
	if signal.Action != domain.ActionBuy && signal.Action != domain.ActionSell {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	pos := e.positions[ticker]
	if signal.TargetLots <= 0 {
		if pos != nil {
			e.closePositionLocked(ticker, pos, price, day, signal.Reasoning)
		}
		return nil
	}
	if pos == nil {
		e.positions[ticker] = newPosition(signal.Action, signal.TargetLots, price, e.cfg.CommissionRate, day)
		return nil
	}

	current := pos.lots
	if pos.action == domain.ActionSell {
		current = -pos.lots
	}
	desired := signal.TargetLots
	if signal.Action == domain.ActionSell {
		desired = -signal.TargetLots
	}
	delta := desired - current
	if delta == 0 {
		return nil
	}

	if pos.action == signal.Action {
		deltaLots := delta
		if deltaLots < 0 {
			deltaLots = -deltaLots
		}
		if absLots(desired) > absLots(current) {
			pos.avg = pos.avg.Mul(decimal.NewFromInt(int64(pos.lots))).
				Add(price.Mul(decimal.NewFromInt(int64(deltaLots)))).
				Div(decimal.NewFromInt(int64(pos.lots + deltaLots)))
			pos.comm = pos.comm.Add(commissionAmount(price, deltaLots, e.cfg.CommissionRate))
			pos.lots += deltaLots
			return nil
		}
		closingLots := deltaLots
		entryComm, exitComm, gross := closeTrade(pos, closingLots, price, e.cfg.CommissionRate)
		net := gross.Sub(entryComm.Add(exitComm))
		e.realized[ticker] = e.realized[ticker].Add(net)
		e.trades = append(e.trades, Trade{
			Ticker:     ticker,
			Action:     pos.action,
			Lots:       closingLots,
			EntryPrice: pos.avg,
			ExitPrice:  price,
			OpenedAt:   pos.openedAt,
			ClosedAt:   day,
			GrossPnl:   gross,
			Commission: entryComm.Add(exitComm),
			NetPnl:     net,
			SignalNote: signal.Reasoning,
		})
		pos.lots -= closingLots
		if pos.lots == 0 {
			delete(e.positions, ticker)
			return nil
		}
		pos.comm = pos.comm.Mul(decimal.NewFromInt(int64(pos.lots))).Div(decimal.NewFromInt(int64(pos.lots + closingLots)))
		return nil
	}

	e.closePositionLocked(ticker, pos, price, day, signal.Reasoning)
	e.positions[ticker] = newPosition(signal.Action, signal.TargetLots, price, e.cfg.CommissionRate, day)
	return nil
}

func (e *Engine) expirePosition(ticker string, price decimal.Decimal, day time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	pos := e.positions[ticker]
	if pos == nil {
		return false
	}
	pos.bars++
	if pos.bars < e.cfg.MaxHoldBars {
		return false
	}
	closeAction := domain.ActionSell
	if pos.action == domain.ActionSell {
		closeAction = domain.ActionBuy
	}
	e.closePositionLocked(ticker, pos, e.fillPrice(price, closeAction), day, "time-exit")
	return true
}

func (e *Engine) closePositionLocked(ticker string, pos *position, price decimal.Decimal, day time.Time, note string) {
	entryComm, exitComm, gross := closeTrade(pos, pos.lots, price, e.cfg.CommissionRate)
	net := gross.Sub(entryComm.Add(exitComm))
	e.realized[ticker] = e.realized[ticker].Add(net)
	e.trades = append(e.trades, Trade{
		Ticker:     ticker,
		Action:     pos.action,
		Lots:       pos.lots,
		EntryPrice: pos.avg,
		ExitPrice:  price,
		OpenedAt:   pos.openedAt,
		ClosedAt:   day,
		GrossPnl:   gross,
		Commission: entryComm.Add(exitComm),
		NetPnl:     net,
		SignalNote: note,
	})
	delete(e.positions, ticker)
}

func absLots(lots int) int {
	if lots < 0 {
		return -lots
	}
	return lots
}

func newPosition(action domain.Action, lots int, price decimal.Decimal, rate decimal.Decimal, openedAt time.Time) *position {
	return &position{
		action:   action,
		lots:     lots,
		avg:      price,
		comm:     commissionAmount(price, lots, rate),
		openedAt: openedAt,
	}
}

func (e *Engine) recordDecision(signal domain.TradeSignal) {
	e.mu.Lock()
	e.decisions++
	if signal.Action == domain.ActionHold {
		if signal.HoldReason == "" {
			signal.HoldReason = domain.HoldReasonModel
		}
		e.holds[signal.HoldReason]++
	}
	e.mu.Unlock()
}

// closeTrade computes entry commission for closing lots, exit commission and
// gross P&L of the closed portion.
func closeTrade(pos *position, closingLots int, price decimal.Decimal, rate decimal.Decimal) (entryComm, exitComm, gross decimal.Decimal) {
	if pos.action == domain.ActionBuy {
		gross = price.Sub(pos.avg).Mul(decimal.NewFromInt(int64(closingLots)))
	} else {
		gross = pos.avg.Sub(price).Mul(decimal.NewFromInt(int64(closingLots)))
	}
	port := decimal.NewFromInt(int64(closingLots)).Div(decimal.NewFromInt(int64(pos.lots)))
	entryComm = pos.comm.Mul(port)
	exitComm = commissionAmount(price, closingLots, rate)
	return entryComm, exitComm, gross
}

func (e *Engine) tickerPnL(ticker string, mark decimal.Decimal) decimal.Decimal {
	e.mu.Lock()
	defer e.mu.Unlock()
	pnl := e.realized[ticker]
	pos := e.positions[ticker]
	if pos != nil {
		unreal := mark.Sub(pos.avg)
		if pos.action == domain.ActionSell {
			unreal = pos.avg.Sub(mark)
		}
		pnl = pnl.Add(unreal.Mul(decimal.NewFromInt(int64(pos.lots))))
	}
	return pnl
}

func (e *Engine) buildResult(curve map[time.Time]decimal.Decimal) *Result {
	e.mu.Lock()
	defer e.mu.Unlock()

	trades := append([]Trade(nil), e.trades...)
	res := &Result{
		Tickers: append([]string(nil), e.cfg.Tickers...),
		Deposit: e.cfg.Deposit,
	}
	for _, t := range trades {
		res.GrossPnl = res.GrossPnl.Add(t.GrossPnl)
		res.TotalCommission = res.TotalCommission.Add(t.Commission)
		res.RealizedPnl = res.RealizedPnl.Add(t.NetPnl)
		if t.NetPnl.Sign() > 0 {
			res.WinningTrades++
		}
	}
	res.ClosedTrades = len(trades)
	if res.ClosedTrades > 0 {
		res.HitRate = float64(res.WinningTrades) / float64(res.ClosedTrades)
	}
	res.Decisions = e.decisions
	res.HoldReasons = make(map[string]int, len(e.holds))
	for reason, count := range e.holds {
		res.HoldReasons[reason] = count
	}
	res.Trades = trades
	res.EquityCurve = sortedCurve(curve)
	res.FinalEquity = lastEquity(curve)
	res.NetPnl = res.FinalEquity.Sub(res.Deposit)
	res.UnrealizedPnl = res.NetPnl.Sub(res.RealizedPnl)
	res.Sharpe, res.MaxDrawdownPct, res.MaxDrawdownRub = curveStats(res.EquityCurve)
	if len(res.EquityCurve) > 0 {
		res.Start = res.EquityCurve[0].Date
		res.End = res.EquityCurve[len(res.EquityCurve)-1].Date
	}
	active, _ := e.kill.IsKillSwitchActive(context.Background())
	res.KillSwitchTripped = active
	return res
}

func aggregateCurves(curves []map[time.Time]decimal.Decimal, deposit decimal.Decimal) map[time.Time]decimal.Decimal {
	days := make(map[time.Time]struct{})
	for _, curve := range curves {
		for day := range curve {
			days[day] = struct{}{}
		}
	}
	out := make(map[time.Time]decimal.Decimal, len(days))
	for day := range days {
		sum := deposit
		for _, curve := range curves {
			sum = sum.Add(curve[day])
		}
		out[day] = sum
	}
	return out
}

func sortedCurve(curve map[time.Time]decimal.Decimal) []EquityPoint {
	days := make([]time.Time, 0, len(curve))
	for day := range curve {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	out := make([]EquityPoint, 0, len(days))
	for _, day := range days {
		out = append(out, EquityPoint{Date: day, Equity: curve[day]})
	}
	return out
}

func lastEquity(curve map[time.Time]decimal.Decimal) decimal.Decimal {
	days := make([]time.Time, 0, len(curve))
	for day := range curve {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	if len(days) == 0 {
		return decimal.Zero
	}
	return curve[days[len(days)-1]]
}

func curveStats(points []EquityPoint) (sharpe float64, maxDDPct float64, maxDDRub decimal.Decimal) {
	if len(points) < 2 {
		return 0, 0, decimal.Zero
	}
	returns := make([]float64, 0, len(points)-1)
	peak := points[0].Equity
	var worstDDPct float64
	var worstDDRub decimal.Decimal
	for i := 1; i < len(points); i++ {
		prev, _ := points[i-1].Equity.Float64()
		cur, _ := points[i].Equity.Float64()
		if prev > 0 {
			returns = append(returns, cur/prev-1)
		}
		if points[i].Equity.GreaterThan(peak) {
			peak = points[i].Equity
		}
		if !peak.IsZero() {
			dd := peak.Sub(points[i].Equity)
			ddPct := dd.Div(peak).Mul(decimal.NewFromInt(100))
			if v, _ := ddPct.Float64(); v > worstDDPct {
				worstDDPct = v
				worstDDRub = dd
			}
		}
	}
	if len(returns) == 0 {
		return 0, worstDDPct, worstDDRub
	}
	mean, std := meanStd(returns)
	if std <= 0 {
		return 0, worstDDPct, worstDDRub
	}
	return mean / std * math.Sqrt(periodsPerYear), worstDDPct, worstDDRub
}

func meanStd(values []float64) (mean, std float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))
	var sq float64
	for _, v := range values {
		d := v - mean
		sq += d * d
	}
	std = math.Sqrt(sq / float64(len(values)))
	return
}

func tickersNormalized(tickers []string) []string {
	out := make([]string, 0, len(tickers))
	for _, t := range tickers {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
