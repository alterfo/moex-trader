package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/executor"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/metrics"
	"github.com/olegsidorkin/moex-trader/internal/risk"
)

const (
	StageIngest   = "ingest"
	StageSignal   = "llm"
	StageRisk     = "risk_gate"
	StageExecutor = "executor"
	StageSkip     = "executor_skip"
)

type Ingestor interface {
	Ingest(ctx context.Context, ticker string) (features.Input, error)
}

type CycleResetter interface {
	ResetCycle()
}

type CyclePreparer interface {
	PrepareCycle(ctx context.Context, tickers []string)
}

type SignalSource interface {
	Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error)
}

type AuditWriter interface {
	InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error
}

type KillSwitchState interface {
	IsKillSwitchActive(ctx context.Context) (bool, error)
}

type AccountSource interface {
	Snapshot(ctx context.Context) (risk.Account, error)
}

type Decision struct {
	Ticker   string
	Signal   domain.TradeSignal
	Price    decimal.Decimal
	Approved bool
	Fill     executor.Fill
	Err      error
}

type DecisionObserver interface {
	Observe(ctx context.Context, decision Decision)
}

type CandleObserver interface {
	ObserveCandle(ctx context.Context, ticker string, age time.Duration)
}

type Options struct {
	Tickers          []string
	Ingestor         Ingestor
	Builder          *features.Builder
	Source           SignalSource
	Gate             risk.Gate
	Executor         executor.Executor
	Positions        risk.PositionReader
	Audit            AuditWriter
	PollInterval     time.Duration
	Logger           *log.Logger
	Now              func() time.Time
	Metrics          *metrics.Metrics
	Account          risk.Account
	AccountSource    AccountSource
	Observer         DecisionObserver
	CandleObserver   CandleObserver
	KillSwitch       KillSwitchState
	Shadow           *ShadowReconciler
	ShadowDigestPath string
	VolScaleEnabled  bool
}

type Orchestrator struct {
	tickers          []string
	ingestor         Ingestor
	builder          *features.Builder
	source           SignalSource
	gate             risk.Gate
	exec             executor.Executor
	positions        risk.PositionReader
	audit            AuditWriter
	pollInterval     time.Duration
	logger           *log.Logger
	now              func() time.Time
	metrics          *metrics.Metrics
	account          risk.Account
	accountSource    AccountSource
	observer         DecisionObserver
	candleObserver   CandleObserver
	killSwitch       KillSwitchState
	shadow           *ShadowReconciler
	shadowDigestPath string
	shadowDigest     *ShadowDigest
	volScaleEnabled  bool
	currentMedian    decimal.Decimal
}

func New(opts Options) (*Orchestrator, error) {
	if len(opts.Tickers) == 0 {
		return nil, fmt.Errorf("orchestrator: tickers must not be empty")
	}
	if opts.Ingestor == nil {
		return nil, fmt.Errorf("orchestrator: ingestor is required")
	}
	if opts.Source == nil {
		return nil, fmt.Errorf("orchestrator: signal source is required")
	}
	if opts.Gate == nil {
		return nil, fmt.Errorf("orchestrator: risk gate is required")
	}
	if opts.Executor == nil {
		return nil, fmt.Errorf("orchestrator: executor is required")
	}
	if opts.Audit == nil {
		return nil, fmt.Errorf("orchestrator: audit writer is required")
	}
	if opts.PollInterval < 0 {
		return nil, fmt.Errorf("orchestrator: poll interval must not be negative")
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}
	builder := opts.Builder
	if builder == nil {
		builder = features.NewBuilder(now)
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	pollInterval := opts.PollInterval
	if pollInterval == 0 {
		pollInterval = time.Second
	}

	return &Orchestrator{
		tickers:          append([]string(nil), opts.Tickers...),
		ingestor:         opts.Ingestor,
		builder:          builder,
		source:           opts.Source,
		gate:             opts.Gate,
		exec:             opts.Executor,
		positions:        opts.Positions,
		audit:            opts.Audit,
		pollInterval:     pollInterval,
		logger:           logger,
		now:              now,
		metrics:          opts.Metrics,
		account:          opts.Account,
		accountSource:    opts.AccountSource,
		observer:         opts.Observer,
		candleObserver:   opts.CandleObserver,
		killSwitch:       opts.KillSwitch,
		shadow:           opts.Shadow,
		shadowDigestPath: opts.ShadowDigestPath,
		shadowDigest:     NewShadowDigest(),
		volScaleEnabled:  opts.VolScaleEnabled,
		currentMedian:    decimal.Zero,
	}, nil
}

func (o *Orchestrator) Run(ctx context.Context) {
	o.RunOnce(ctx)

	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.RunOnce(ctx)
		}
	}
}

func (o *Orchestrator) RunOnce(ctx context.Context) {
	if resetter, ok := o.ingestor.(CycleResetter); ok {
		resetter.ResetCycle()
	}
	if preparer, ok := o.ingestor.(CyclePreparer); ok {
		preparer.PrepareCycle(ctx, o.tickers)
	}
	account, err := o.cycleAccount(ctx)
	if err != nil {
		o.logger.Printf("orchestrator: account snapshot: %v", err)
		return
	}
	if o.volScaleEnabled {
		o.currentMedian = o.crossSectionalMedian(ctx)
	}
	for _, ticker := range o.tickers {
		if ctx.Err() != nil {
			return
		}
		blocked, err := o.killSwitchBlocked(ctx)
		if err != nil {
			o.logger.Printf("orchestrator: check kill switch: %v", err)
			return
		}
		if blocked {
			o.logger.Printf("orchestrator: kill switch active, blocking new signals")
			return
		}
		if err := o.processTicker(ctx, ticker, account); err != nil {
			o.logger.Printf("orchestrator: ticker %s: %v", ticker, err)
		}
	}
	o.writeShadowDigest()
}

func (o *Orchestrator) cycleAccount(ctx context.Context) (risk.Account, error) {
	if o.accountSource == nil {
		return o.account, nil
	}
	return o.accountSource.Snapshot(ctx)
}

func (o *Orchestrator) killSwitchBlocked(ctx context.Context) (bool, error) {
	if o.killSwitch == nil {
		return false, nil
	}
	return o.killSwitch.IsKillSwitchActive(ctx)
}

func (o *Orchestrator) processTicker(ctx context.Context, ticker string, account risk.Account) error {
	input, err := o.ingestor.Ingest(ctx, ticker)
	if err != nil {
		if auditErr := o.record(ctx, ticker, StageIngest, auditError("ingest", err)); auditErr != nil {
			return errors.Join(fmt.Errorf("ingest %s: %w", ticker, err), auditErr)
		}
		return fmt.Errorf("ingest %s: %w", ticker, err)
	}

	feature, err := o.builder.Build(input)
	if err != nil {
		if auditErr := o.record(ctx, ticker, StageIngest, auditError("build features", err)); auditErr != nil {
			return errors.Join(fmt.Errorf("build features for %s: %w", ticker, err), auditErr)
		}
		return fmt.Errorf("build features for %s: %w", ticker, err)
	}
	feature.CrossSectionalVolatility = o.currentMedian
	if err := o.record(ctx, ticker, StageIngest, auditJSON(feature)); err != nil {
		return err
	}

	if o.metrics != nil || o.candleObserver != nil {
		age := candleAge(input.Candles, o.now())
		if o.metrics != nil {
			o.metrics.SetCandleAge(ticker, age.Seconds())
		}
		if o.candleObserver != nil {
			o.candleObserver.ObserveCandle(ctx, ticker, age)
		}
	}

	started := time.Now()
	signal, err := o.source.Generate(ctx, feature)
	if o.metrics != nil {
		o.metrics.ObserveInference(time.Since(started))
	}
	if err != nil {
		if auditErr := o.record(ctx, ticker, StageSignal, auditError("generate signal", err)); auditErr != nil {
			return errors.Join(fmt.Errorf("generate signal for %s: %w", ticker, err), auditErr)
		}
		return fmt.Errorf("generate signal for %s: %w", ticker, err)
	}
	if err := o.record(ctx, ticker, StageSignal, auditJSON(signal)); err != nil {
		return err
	}
	if o.metrics != nil {
		o.metrics.IncSignalsGenerated()
		o.metrics.ObserveSignalProbability(signal.Probability)
	}
	if strings.TrimSpace(signal.Ticker) != "" && !strings.EqualFold(signal.Ticker, ticker) {
		mismatchErr := fmt.Errorf("signal ticker %q does not match requested ticker %q", signal.Ticker, ticker)
		if auditErr := o.record(ctx, ticker, StageSignal, auditError("signal ticker mismatch", mismatchErr)); auditErr != nil {
			return errors.Join(mismatchErr, auditErr)
		}
		return mismatchErr
	}

	if o.shadow != nil {
		comparison := o.shadow.Reconcile(ctx, input, feature, signal)
		if o.shadowDigest != nil {
			o.shadowDigest.Add(comparison)
		}
		if !comparison.FeatureMatch || !comparison.SignalMatch {
			o.logger.Printf("orchestrator: shadow reconcile %s %s: feature_match=%v max_delta=%.9f signal_match=%v live=%s replay=%s err=%q",
				comparison.Ticker, comparison.Day, comparison.FeatureMatch, comparison.MaxFeatureDelta,
				comparison.SignalMatch, comparison.LiveAction, comparison.ReplayAction, comparison.Err)
		}
	}

	exposureDelta, err := o.exposureDeltaLots(ctx, signal)
	if err != nil {
		if auditErr := o.record(ctx, ticker, StageRisk, auditError("read current position", err)); auditErr != nil {
			return errors.Join(fmt.Errorf("read current position for %s: %w", ticker, err), auditErr)
		}
		return fmt.Errorf("read current position for %s: %w", ticker, err)
	}

	decision, err := o.gate.ApproveReason(ctx, risk.Request{
		Signal:            signal,
		ExposureDeltaLots: exposureDelta,
		Market: risk.Market{
			OrderPrice: feature.LastPrice,
			Bid:        feature.Bid,
			Ask:        feature.Ask,
			PrevClose:  feature.PrevClose,
			LotSize:    feature.LotSize,
		},
		Account: account,
	})
	if err != nil {
		if auditErr := o.record(ctx, ticker, StageRisk, auditError("risk gate", err)); auditErr != nil {
			return errors.Join(fmt.Errorf("risk gate for %s: %w", ticker, err), auditErr)
		}
		return fmt.Errorf("risk gate for %s: %w", ticker, err)
	}
	if err := o.record(ctx, ticker, StageRisk, auditJSON(struct {
		Approved bool   `json:"approved"`
		Reason   string `json:"reason,omitempty"`
	}{Approved: decision.Approved, Reason: decision.Reason})); err != nil {
		return err
	}
	if !decision.Approved {
		if o.metrics != nil {
			o.metrics.IncRiskRejections(decision.Reason)
		}
		o.observe(ctx, Decision{Ticker: ticker, Signal: signal, Price: feature.LastPrice, Approved: false})
		return nil
	}

	fill, err := o.exec.Execute(ctx, signal, feature.LastPrice)
	if err != nil {
		o.observe(ctx, Decision{Ticker: ticker, Signal: signal, Price: feature.LastPrice, Approved: true, Err: err})
		if auditErr := o.record(ctx, ticker, StageExecutor, auditError("execute", err)); auditErr != nil {
			return errors.Join(fmt.Errorf("execute %s: %w", ticker, err), auditErr)
		}
		return fmt.Errorf("execute %s: %w", ticker, err)
	}
	if fill.Lots == 0 {
		if o.metrics != nil {
			o.metrics.IncExecutorSkips(executorSkipReason(signal))
		}
		if auditErr := o.record(ctx, ticker, StageSkip, auditJSON(struct {
			Status     string `json:"status"`
			Action     string `json:"action"`
			TargetLots int    `json:"target_lots"`
		}{
			Status:     "skipped",
			Action:     string(signal.Action),
			TargetLots: signal.TargetLots,
		})); auditErr != nil {
			return auditErr
		}
	}
	o.observe(ctx, Decision{Ticker: ticker, Signal: signal, Price: feature.LastPrice, Approved: true, Fill: fill})
	return nil
}

func (o *Orchestrator) exposureDeltaLots(ctx context.Context, signal domain.TradeSignal) (*int, error) {
	if o.positions == nil || (signal.Action != domain.ActionBuy && signal.Action != domain.ActionSell) {
		return nil, nil
	}
	current, err := o.positions.CurrentLots(ctx, signal.Ticker)
	if err != nil {
		return nil, err
	}
	desired := signal.TargetLots
	if signal.Action == domain.ActionSell {
		desired = -desired
	}
	delta := desired - current
	return &delta, nil
}

func (o *Orchestrator) crossSectionalMedian(ctx context.Context) decimal.Decimal {
	vols := make([]decimal.Decimal, 0, len(o.tickers))
	for _, ticker := range o.tickers {
		if ctx.Err() != nil {
			return domain.MedianDecimal(vols)
		}
		input, err := o.ingestor.Ingest(ctx, ticker)
		if err != nil {
			continue
		}
		feature, err := o.builder.Build(input)
		if err != nil || !feature.RealizedVolatility.IsPositive() {
			continue
		}
		vols = append(vols, feature.RealizedVolatility)
	}
	return domain.MedianDecimal(vols)
}

func (o *Orchestrator) observe(ctx context.Context, decision Decision) {
	if o.observer == nil {
		return
	}
	o.observer.Observe(ctx, decision)
}

func (o *Orchestrator) writeShadowDigest() {
	if o.shadowDigestPath == "" || o.shadowDigest == nil {
		return
	}
	if err := UpdateShadowDigestFile(o.shadowDigestPath, o.shadowDigest.Markdown()); err != nil {
		o.logger.Printf("orchestrator: write shadow digest: %v", err)
	}
}

func (o *Orchestrator) record(ctx context.Context, ticker, stage, payload string) error {
	event := domain.AuditEvent{
		ID:        uuid.NewString(),
		Ticker:    ticker,
		Stage:     stage,
		Payload:   payload,
		CreatedAt: o.now(),
	}
	if err := o.audit.InsertAuditEvent(ctx, event); err != nil {
		o.logger.Printf("orchestrator: record %s event for %s: %v", stage, ticker, err)
		return fmt.Errorf("orchestrator: persist %s audit event for %s: %w", stage, ticker, err)
	}
	return nil
}

func auditJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func auditError(operation string, err error) string {
	return auditJSON(map[string]string{"operation": operation, "error": err.Error()})
}

func executorSkipReason(signal domain.TradeSignal) string {
	if signal.HoldReason != "" {
		return signal.HoldReason
	}
	if signal.Action == domain.ActionHold {
		return domain.HoldReasonModel
	}
	return "skip"
}
