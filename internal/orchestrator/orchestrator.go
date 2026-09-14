package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

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

type SignalSource interface {
	Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error)
}

type AuditWriter interface {
	InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error
}

type Options struct {
	Tickers      []string
	Ingestor     Ingestor
	Builder      *features.Builder
	Source       SignalSource
	Gate         risk.Gate
	Executor     executor.Executor
	Audit        AuditWriter
	PollInterval time.Duration
	Logger       *log.Logger
	Now          func() time.Time
	Metrics      *metrics.Metrics
	Account      risk.Account
}

type Orchestrator struct {
	tickers      []string
	ingestor     Ingestor
	builder      *features.Builder
	source       SignalSource
	gate         risk.Gate
	exec         executor.Executor
	audit        AuditWriter
	pollInterval time.Duration
	logger       *log.Logger
	now          func() time.Time
	metrics      *metrics.Metrics
	account      risk.Account
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
		tickers:      append([]string(nil), opts.Tickers...),
		ingestor:     opts.Ingestor,
		builder:      builder,
		source:       opts.Source,
		gate:         opts.Gate,
		exec:         opts.Executor,
		audit:        opts.Audit,
		pollInterval: pollInterval,
		logger:       logger,
		now:          now,
		metrics:      opts.Metrics,
		account:      opts.Account,
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
	for _, ticker := range o.tickers {
		if ctx.Err() != nil {
			return
		}
		if err := o.processTicker(ctx, ticker); err != nil {
			o.logger.Printf("orchestrator: ticker %s: %v", ticker, err)
		}
	}
}

func (o *Orchestrator) processTicker(ctx context.Context, ticker string) error {
	input, err := o.ingestor.Ingest(ctx, ticker)
	if err != nil {
		o.record(ctx, ticker, StageIngest, auditError("ingest", err))
		return fmt.Errorf("ingest %s: %w", ticker, err)
	}

	feature, err := o.builder.Build(input)
	if err != nil {
		o.record(ctx, ticker, StageIngest, auditError("build features", err))
		return fmt.Errorf("build features for %s: %w", ticker, err)
	}
	o.record(ctx, ticker, StageIngest, auditJSON(feature))

	started := time.Now()
	signal, err := o.source.Generate(ctx, feature)
	if o.metrics != nil {
		o.metrics.ObserveLLMInference(time.Since(started))
	}
	if err != nil {
		o.record(ctx, ticker, StageSignal, auditError("generate signal", err))
		return fmt.Errorf("generate signal for %s: %w", ticker, err)
	}
	o.record(ctx, ticker, StageSignal, auditJSON(signal))
	if o.metrics != nil {
		o.metrics.IncSignalsGenerated()
	}

	approved, err := o.gate.Approve(ctx, risk.Request{
		Signal: signal,
		Market: risk.Market{
			OrderPrice: feature.LastPrice,
			Bid:        feature.Bid,
			Ask:        feature.Ask,
		},
		Account: o.account,
	})
	if err != nil {
		o.record(ctx, ticker, StageRisk, auditError("risk gate", err))
		return fmt.Errorf("risk gate for %s: %w", ticker, err)
	}
	o.record(ctx, ticker, StageRisk, auditJSON(struct {
		Approved bool `json:"approved"`
	}{Approved: approved}))
	if !approved {
		if o.metrics != nil {
			o.metrics.IncRiskRejections()
		}
		return nil
	}

	if _, err := o.exec.Execute(ctx, signal, feature.LastPrice); err != nil {
		o.record(ctx, ticker, StageExecutor, auditError("execute", err))
		return fmt.Errorf("execute %s: %w", ticker, err)
	}
	return nil
}

func (o *Orchestrator) record(ctx context.Context, ticker, stage, payload string) {
	event := domain.AuditEvent{
		ID:        uuid.NewString(),
		Ticker:    ticker,
		Stage:     stage,
		Payload:   payload,
		CreatedAt: o.now(),
	}
	if err := o.audit.InsertAuditEvent(ctx, event); err != nil {
		o.logger.Printf("orchestrator: record %s event for %s: %v", stage, ticker, err)
	}
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
