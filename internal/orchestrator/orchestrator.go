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

type Options struct {
	Tickers       []string
	Ingestor      Ingestor
	Builder       *features.Builder
	Source        SignalSource
	Gate          risk.Gate
	Executor      executor.Executor
	Audit         AuditWriter
	PollInterval  time.Duration
	Logger        *log.Logger
	Now           func() time.Time
	Metrics       *metrics.Metrics
	Account       risk.Account
	AccountSource AccountSource
	Observer      DecisionObserver
	KillSwitch    KillSwitchState
}

type Orchestrator struct {
	tickers       []string
	ingestor      Ingestor
	builder       *features.Builder
	source        SignalSource
	gate          risk.Gate
	exec          executor.Executor
	audit         AuditWriter
	pollInterval  time.Duration
	logger        *log.Logger
	now           func() time.Time
	metrics       *metrics.Metrics
	account       risk.Account
	accountSource AccountSource
	observer      DecisionObserver
	killSwitch    KillSwitchState
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
		tickers:       append([]string(nil), opts.Tickers...),
		ingestor:      opts.Ingestor,
		builder:       builder,
		source:        opts.Source,
		gate:          opts.Gate,
		exec:          opts.Executor,
		audit:         opts.Audit,
		pollInterval:  pollInterval,
		logger:        logger,
		now:           now,
		metrics:       opts.Metrics,
		account:       opts.Account,
		accountSource: opts.AccountSource,
		observer:      opts.Observer,
		killSwitch:    opts.KillSwitch,
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
	if err := o.record(ctx, ticker, StageIngest, auditJSON(feature)); err != nil {
		return err
	}

	started := time.Now()
	signal, err := o.source.Generate(ctx, feature)
	if o.metrics != nil {
		o.metrics.ObserveLLMInference(time.Since(started))
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
	}
	if strings.TrimSpace(signal.Ticker) != "" && !strings.EqualFold(signal.Ticker, ticker) {
		mismatchErr := fmt.Errorf("signal ticker %q does not match requested ticker %q", signal.Ticker, ticker)
		if auditErr := o.record(ctx, ticker, StageSignal, auditError("signal ticker mismatch", mismatchErr)); auditErr != nil {
			return errors.Join(mismatchErr, auditErr)
		}
		return mismatchErr
	}

	approved, err := o.gate.Approve(ctx, risk.Request{
		Signal: signal,
		Market: risk.Market{
			OrderPrice: feature.LastPrice,
			Bid:        feature.Bid,
			Ask:        feature.Ask,
			PrevClose:  feature.PrevClose,
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
		Approved bool `json:"approved"`
	}{Approved: approved})); err != nil {
		return err
	}
	if !approved {
		if o.metrics != nil {
			o.metrics.IncRiskRejections()
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
	o.observe(ctx, Decision{Ticker: ticker, Signal: signal, Price: feature.LastPrice, Approved: true, Fill: fill})
	return nil
}

func (o *Orchestrator) observe(ctx context.Context, decision Decision) {
	if o.observer == nil {
		return
	}
	o.observer.Observe(ctx, decision)
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
