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

// TargetPositionSource optionally extends SignalSource with absolute target
// position semantics. The BUY/SELL/HOLD vocabulary cannot express "close to
// flat" (HOLD means "keep the current position"), so a portfolio rebalancing
// source (for example the momentum benchmark) implements this interface and
// returns the desired signed lots: positive = long, negative = short, zero =
// flat. When implemented, the engine drives fills from TargetPosition instead
// of the action returned by Generate, while Generate is still called for
// decision logging and caching.
type TargetPositionSource interface {
	SignalSource
	TargetPosition(feature domain.FeatureContext) (signedLots int, ok bool)
}

type EquityPoint struct {
	Date   time.Time
	Equity decimal.Decimal
}

// OpenPosition is a position still held at the end of a backtest run, marked
// to the final close of each ticker. UnrealizedPnl is already multiplied by
// lots and uses the same long/short sign convention as closeTrade.
type OpenPosition struct {
	Ticker        string
	Side          domain.Action
	Lots          int
	EntryPrice    decimal.Decimal
	MarkPrice     decimal.Decimal
	UnrealizedPnl decimal.Decimal
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
	Tickers              []string
	Start                time.Time
	End                  time.Time
	Deposit              decimal.Decimal
	FinalEquity          decimal.Decimal
	NetPnl               decimal.Decimal
	RealizedPnl          decimal.Decimal
	RealizedPnlNetBorrow decimal.Decimal
	UnrealizedPnl        decimal.Decimal
	GrossPnl             decimal.Decimal
	TotalCommission      decimal.Decimal
	TotalBorrow          decimal.Decimal
	ClosedTrades         int
	WinningTrades        int
	HitRate              float64
	Sharpe               float64
	MaxDrawdownPct       float64
	MaxDrawdownRub       decimal.Decimal
	KillSwitchTripped    bool
	KillSwitchFrozenDays int
	DailyLossBlockedDays int
	Decisions            int
	HoldReasons          map[string]int
	Trades               []Trade
	OpenPositions        []OpenPosition
	Attribution          Attribution
	EquityCurve          []EquityPoint
}

type Config struct {
	Tickers               []string
	From                  time.Time
	Till                  time.Time
	Deposit               decimal.Decimal
	MaxLots               int
	CommissionRate        decimal.Decimal
	SpreadPct             decimal.Decimal
	SpreadPcts            map[string]decimal.Decimal
	SlippagePct           decimal.Decimal
	BorrowPctPerDay       decimal.Decimal
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
	TopicSignalOverrides  map[string]map[string]features.TopicSignalAggregate
}

type NewsAggregate struct {
	Sentiment float64
	Count     int
}

type topicAccum struct {
	negotiationsWeighted float64
	negotiationsWeight   float64
	sanctionsWeighted    float64
	sanctionsWeight      float64
}

type NewsOverrides struct {
	News   map[string]map[string]NewsAggregate
	Events map[string]map[string]features.EventFlags
	Topics map[string]map[string]features.TopicSignalAggregate
}

func LoadNewsOverrides(path string) (NewsOverrides, error) {
	type record struct {
		Ticker      string  `json:"ticker"`
		Sentiment   float64 `json:"sentiment"`
		TrustWeight float64 `json:"trust_weight"`
		PubTS       int64   `json:"published_ts"`
		Title       string  `json:"title"`
	}
	f, err := os.Open(path)
	if err != nil {
		return NewsOverrides{}, fmt.Errorf("backtest: open news history: %w", err)
	}
	defer f.Close()
	type accum struct {
		wSum float64
		w    float64
		n    int
	}
	acc := make(map[string]map[string]*accum)
	evAcc := make(map[string]map[string]*features.EventFlags)
	topicAcc := make(map[string]map[string]*topicAccum)
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
		trustWeight := r.TrustWeight
		if trustWeight <= 0 {
			trustWeight = 1
		}
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
		a.wSum += r.Sentiment * trustWeight
		a.w += trustWeight
		a.n++
		if r.Title != "" {
			flags := features.DetectEvents(r.Title)
			if flags.Negotiations != 0 || flags.Sanctions != 0 {
				topicByDate, ok := topicAcc[ticker]
				if !ok {
					topicByDate = make(map[string]*topicAccum)
					topicAcc[ticker] = topicByDate
				}
				ta, ok := topicByDate[date]
				if !ok {
					ta = &topicAccum{}
					topicByDate[date] = ta
				}
				if flags.Negotiations != 0 {
					ta.negotiationsWeighted += r.Sentiment * trustWeight
					ta.negotiationsWeight += trustWeight
				}
				if flags.Sanctions != 0 {
					ta.sanctionsWeighted += r.Sentiment * trustWeight
					ta.sanctionsWeight += trustWeight
				}
			}
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
		return NewsOverrides{}, fmt.Errorf("backtest: read news: %w", err)
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
	topicOut := make(map[string]map[string]features.TopicSignalAggregate, len(topicAcc))
	for ticker, byDate := range topicAcc {
		topicOut[ticker] = make(map[string]features.TopicSignalAggregate, len(byDate))
		for d, a := range byDate {
			var agg features.TopicSignalAggregate
			if a.negotiationsWeight > 0 {
				agg.Negotiations = a.negotiationsWeighted / a.negotiationsWeight
			}
			if a.sanctionsWeight > 0 {
				agg.Sanctions = a.sanctionsWeighted / a.sanctionsWeight
			}
			topicOut[ticker][d] = agg
		}
	}
	return NewsOverrides{News: out, Events: evOut, Topics: topicOut}, nil
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
	if c.BorrowPctPerDay.IsNegative() {
		c.BorrowPctPerDay = decimal.Zero
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
	borrow    decimal.Decimal
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
	runs := make([]*tickerBacktest, 0, len(e.cfg.Tickers))
	for _, ticker := range tickersNormalized(e.cfg.Tickers) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		run, err := e.prepareTicker(ctx, ticker)
		if err != nil {
			e.cfg.Logger.Printf("backtest: %s: skipping (history failed): %v", ticker, err)
			continue
		}
		if run == nil {
			continue
		}
		runs = append(runs, run)
	}

	days := sortedBacktestDays(runs)
	curve := make(map[time.Time]decimal.Decimal)
	prevEquity := e.cfg.Deposit
	dailyLossLimit := e.cfg.Deposit.Mul(risk.DefaultConfig().MaxDailyLossPct).Div(decimal.NewFromInt(100))
	dailyLossBlockedDays := 0
	var firstKillDay time.Time
	var prevAccrualDay time.Time
	for _, day := range days {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		marks := marksForDay(runs, day)
		dayStartEquity := prevEquity
		active, _ := e.kill.IsKillSwitchActive(ctx)
		if !active {
			for _, run := range runs {
				idx, ok := run.tradeableDays[day]
				if !ok {
					continue
				}
				if err := e.processTickerDay(ctx, run, idx, dayStartEquity, marks); err != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					e.cfg.Logger.Printf("backtest: %s on %s: %v", run.ticker, day.Format("2006-01-02"), err)
				}
			}
		} else if firstKillDay.IsZero() {
			firstKillDay = day
		}
		e.accrueBorrow(borrowDaysFor(prevAccrualDay, day))
		prevAccrualDay = day
		equity := e.portfolioEquity(marks).Sub(e.borrow)
		if !active {
			if loss := dayStartEquity.Sub(equity); loss.Sign() > 0 && loss.GreaterThan(dailyLossLimit) {
				dailyLossBlockedDays++
			}
		}
		curve[day] = equity
		prevEquity = equity
		if !active {
			activeAfter, _ := e.kill.IsKillSwitchActive(ctx)
			if activeAfter {
				firstKillDay = day
			}
		}
	}
	var finalMarks map[string]decimal.Decimal
	if len(days) > 0 {
		finalMarks = marksForDay(runs, days[len(days)-1])
	}
	result := e.buildResult(curve, finalMarks)
	result.DailyLossBlockedDays = dailyLossBlockedDays
	if !firstKillDay.IsZero() {
		for _, day := range days {
			if !day.Before(firstKillDay) {
				result.KillSwitchFrozenDays++
			}
		}
	}
	return result, nil
}

type tickerBacktest struct {
	ticker        string
	candles       []moex.Candle
	builder       *features.Builder
	tradeableDays map[time.Time]int
}

func (e *Engine) prepareTicker(ctx context.Context, ticker string) (*tickerBacktest, error) {
	from := e.cfg.From
	if from.IsZero() {
		from = time.Now().AddDate(0, 0, -365)
	}
	till := e.cfg.Till
	if till.IsZero() {
		till = time.Now()
	}
	fetchFrom := from.AddDate(0, 0, -e.cfg.WarmupDays)
	if extra := e.cfg.FeatureConfig.FetchCalendarDays(); extra > e.cfg.WarmupDays {
		fetchFrom = from.AddDate(0, 0, -extra)
	}

	candles, err := e.cfg.Source.History(ctx, ticker, fetchFrom, till)
	if err != nil {
		return nil, fmt.Errorf("backtest: history %s: %w", ticker, err)
	}
	warmup := e.cfg.FeatureConfig.WarmupCandles()
	if len(candles) < warmup+1 {
		e.cfg.Logger.Printf("backtest: %s: only %d candles in history, skipping", ticker, len(candles))
		return nil, nil
	}

	builder := features.NewBuilderWithConfig(time.Now, e.cfg.FeatureConfig)

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

	tradeableDays := make(map[time.Time]int, len(tradeable))
	for _, d := range tradeable {
		tradeableDays[candles[d].Begin] = d
	}
	return &tickerBacktest{
		ticker:        ticker,
		candles:       candles,
		builder:       builder,
		tradeableDays: tradeableDays,
	}, nil
}

func sortedBacktestDays(runs []*tickerBacktest) []time.Time {
	days := make(map[time.Time]struct{})
	for _, run := range runs {
		for day := range run.tradeableDays {
			days[day] = struct{}{}
		}
	}
	out := make([]time.Time, 0, len(days))
	for day := range days {
		out = append(out, day)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func marksForDay(runs []*tickerBacktest, day time.Time) map[string]decimal.Decimal {
	marks := make(map[string]decimal.Decimal, len(runs))
	for _, run := range runs {
		marks[run.ticker] = candleCloseAtOrBefore(run.candles, day)
	}
	return marks
}

func candleCloseAtOrBefore(candles []moex.Candle, day time.Time) decimal.Decimal {
	idx := sort.Search(len(candles), func(i int) bool { return candles[i].Begin.After(day) })
	if idx == 0 {
		return decimal.Zero
	}
	return candles[idx-1].Close
}

func (e *Engine) portfolioEquity(marks map[string]decimal.Decimal) decimal.Decimal {
	e.mu.Lock()
	defer e.mu.Unlock()

	equity := e.cfg.Deposit
	for ticker, mark := range marks {
		if mark.Sign() <= 0 {
			continue
		}
		equity = equity.Add(e.realized[ticker])
		if pos := e.positions[ticker]; pos != nil {
			unrealized := mark.Sub(pos.avg)
			if pos.action == domain.ActionSell {
				unrealized = pos.avg.Sub(mark)
			}
			equity = equity.Add(unrealized.Mul(decimal.NewFromInt(int64(pos.lots))))
		}
	}
	return equity
}

// accrueBorrow charges the configured per-calendar-day borrow cost against
// every open short position held at the end of the current bar. The cost is
// short-leg notional (entry average price times lots) multiplied by the borrow
// fraction and the number of calendar days elapsed since the previous trading
// bar, and accumulates in e.borrow, which is then subtracted from the
// aggregate equity curve. Long positions are not charged.
func (e *Engine) accrueBorrow(days int) {
	if e.cfg.BorrowPctPerDay.Sign() <= 0 || days <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, pos := range e.positions {
		if pos.action != domain.ActionSell || pos.lots <= 0 {
			continue
		}
		notional := pos.avg.Mul(decimal.NewFromInt(int64(pos.lots)))
		e.borrow = e.borrow.Add(notional.Mul(e.cfg.BorrowPctPerDay).Mul(decimal.NewFromInt(int64(days))))
	}
}

func borrowDaysFor(prev, day time.Time) int {
	if prev.IsZero() {
		return 1
	}
	days := int(math.Round(day.Sub(prev).Hours() / 24))
	if days < 1 {
		return 1
	}
	return days
}

func (e *Engine) processTickerDay(ctx context.Context, run *tickerBacktest, d int, dayStartEquity decimal.Decimal, marks map[string]decimal.Decimal) error {
	ticker := run.ticker
	candles := run.candles
	decisionDay := candles[d].Begin
	execPrice := decisionPrice(candles, d)
	if execPrice.Sign() <= 0 {
		return nil
	}

	if e.cfg.MaxHoldBars > 0 && e.expirePosition(ticker, execPrice, decisionDay) {
		return nil
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

	feature, err := run.builder.Build(input)
	if err != nil {
		e.cfg.Logger.Printf("backtest: %s on %s: build features: %v", ticker, decisionDay.Format("2006-01-02"), err)
		return nil
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
	if e.cfg.TopicSignalOverrides != nil {
		dateKey := decisionDay.Format("2006-01-02")
		if byDate, ok := e.cfg.TopicSignalOverrides[ticker]; ok {
			if topic, ok := byDate[dateKey]; ok {
				feature.NegotiationsSignal = decimal.NewFromFloat(topic.Negotiations)
				feature.SanctionsSignal = decimal.NewFromFloat(topic.Sanctions)
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
		return nil
	}

	currentEquity := e.portfolioEquity(marks)
	account := risk.Account{
		Deposit:        e.cfg.Deposit,
		DayStartEquity: dayStartEquity,
		CurrentEquity:  currentEquity,
	}
	if targetSource, isTarget := e.cfg.SignalSource.(TargetPositionSource); isTarget {
		if signedLots, useTarget := targetSource.TargetPosition(feature); useTarget {
			return e.applyTargetPositionWithAccount(ctx, ticker, signedLots, execPrice, decisionDay, account)
		}
	}

	approved, err := e.gate.Approve(ctx, risk.Request{
		Signal: signal,
		Market: risk.Market{
			OrderPrice: execPrice,
			PrevClose:  candles[d].Close,
		},
		Account: account,
	})
	if err != nil {
		e.cfg.Logger.Printf("backtest: %s on %s: risk gate: %v", ticker, decisionDay.Format("2006-01-02"), err)
		return nil
	}

	if signal.Action != domain.ActionHold && approved {
		fillPrice := e.fillPrice(ticker, execPrice, signal.Action)
		if err := e.recordFill(ticker, signal, fillPrice, decisionDay); err != nil {
			e.cfg.Logger.Printf("backtest: %s on %s: record fill: %v", ticker, decisionDay.Format("2006-01-02"), err)
		}
	}
	return nil
}

// applyTargetPosition reconciles the current position to an absolute signed
// target lot count. Closing to flat bypasses the risk gate because it only
// reduces exposure; opening or reversing goes through the same gate as every
// other fill so the kill-switch, drawdown, fat-finger and max-lots checks
// remain identical for all signal sources.
func (e *Engine) applyTargetPosition(ctx context.Context, ticker string, signedLots int, execPrice decimal.Decimal, day time.Time) error {
	return e.applyTargetPositionWithAccount(ctx, ticker, signedLots, execPrice, day, risk.Account{
		Deposit:       e.cfg.Deposit,
		CurrentEquity: e.cfg.Deposit.Add(e.tickerPnL(ticker, execPrice)),
	})
}

func (e *Engine) applyTargetPositionWithAccount(ctx context.Context, ticker string, signedLots int, execPrice decimal.Decimal, day time.Time, account risk.Account) error {
	e.mu.Lock()
	current := 0
	if pos := e.positions[ticker]; pos != nil {
		current = pos.lots
		if pos.action == domain.ActionSell {
			current = -pos.lots
		}
	}
	e.mu.Unlock()

	if signedLots == current {
		return nil
	}

	if signedLots == 0 {
		e.mu.Lock()
		pos := e.positions[ticker]
		if pos == nil {
			e.mu.Unlock()
			return nil
		}
		closeAction := domain.ActionSell
		if pos.action == domain.ActionSell {
			closeAction = domain.ActionBuy
		}
		e.closePositionLocked(ticker, pos, e.fillPrice(ticker, execPrice, closeAction), day, "target-flat")
		e.mu.Unlock()
		return nil
	}

	action := domain.ActionBuy
	if signedLots < 0 {
		action = domain.ActionSell
	}
	targetLots := signedLots
	if targetLots < 0 {
		targetLots = -targetLots
	}
	signal := domain.TradeSignal{
		Ticker:      ticker,
		Action:      action,
		Confidence:  decimal.NewFromInt(1),
		TargetLots:  targetLots,
		Reasoning:   "target-position",
		GeneratedAt: day,
	}
	approved, err := e.gate.Approve(ctx, risk.Request{
		Signal: signal,
		Market: risk.Market{
			OrderPrice: execPrice,
			PrevClose:  execPrice,
		},
		Account: account,
	})
	if err != nil {
		return err
	}
	if !approved {
		return nil
	}
	return e.recordFill(ticker, signal, e.fillPrice(ticker, execPrice, action), day)
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
func (e *Engine) fillPrice(ticker string, price decimal.Decimal, action domain.Action) decimal.Decimal {
	cost := e.spreadPct(ticker).Add(e.cfg.SlippagePct)
	if cost.Sign() <= 0 {
		return price
	}
	if action == domain.ActionSell {
		return price.Mul(decimal.NewFromInt(1).Sub(cost))
	}
	return price.Mul(decimal.NewFromInt(1).Add(cost))
}

func (e *Engine) spreadPct(ticker string) decimal.Decimal {
	key := strings.ToUpper(strings.TrimSpace(ticker))
	if value, ok := e.cfg.SpreadPcts[key]; ok {
		return value
	}
	return e.cfg.SpreadPct
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
	e.closePositionLocked(ticker, pos, e.fillPrice(ticker, price, closeAction), day, "time-exit")
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

// openPositionsLocked snapshots positions still held at the end of a run,
// marked against the provided final ticker marks. Callers must hold e.mu.
func openPositionsLocked(positions map[string]*position, finalMarks map[string]decimal.Decimal) []OpenPosition {
	out := make([]OpenPosition, 0, len(positions))
	for ticker, pos := range positions {
		mark := finalMarks[ticker]
		side := domain.ActionBuy
		if pos.action == domain.ActionSell {
			side = domain.ActionSell
		}
		unrealized := mark.Sub(pos.avg)
		if pos.action == domain.ActionSell {
			unrealized = pos.avg.Sub(mark)
		}
		out = append(out, OpenPosition{
			Ticker:        ticker,
			Side:          side,
			Lots:          pos.lots,
			EntryPrice:    pos.avg,
			MarkPrice:     mark,
			UnrealizedPnl: unrealized.Mul(decimal.NewFromInt(int64(pos.lots))),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
	return out
}

func (e *Engine) buildResult(curve map[time.Time]decimal.Decimal, finalMarks map[string]decimal.Decimal) *Result {
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
	res.TotalBorrow = e.borrow
	res.RealizedPnlNetBorrow = res.RealizedPnl.Sub(res.TotalBorrow)
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
	res.OpenPositions = openPositionsLocked(e.positions, finalMarks)
	res.Attribution = computeAttribution(trades, res.OpenPositions, DefaultAttributionTopN)
	res.EquityCurve = sortedCurve(curve)
	res.FinalEquity = lastEquity(curve)
	res.NetPnl = res.FinalEquity.Sub(res.Deposit)
	res.UnrealizedPnl = res.NetPnl.Sub(res.RealizedPnlNetBorrow)
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
