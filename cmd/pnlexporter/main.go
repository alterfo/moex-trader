package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shopspring/decimal"
	_ "modernc.org/sqlite"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/dailysummary"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/pnl"
)

type collectors struct {
	equity           prometheus.Gauge
	deposit          prometheus.Gauge
	realized         prometheus.Gauge
	unrealized       prometheus.Gauge
	commissions      prometheus.Gauge
	fills            prometheus.Gauge
	closedTrades     prometheus.Gauge
	winningTrades    prometheus.Gauge
	openPositions    prometheus.Gauge
	grossExposure    prometheus.Gauge
	netExposure      prometheus.Gauge
	maxDrawdown      prometheus.Gauge
	dayRealized      prometheus.Gauge
	dayPnL           prometheus.Gauge
	dayTrades        prometheus.Gauge
	lastFill         prometheus.Gauge
	positionLots     *prometheus.GaugeVec
	positionRealized *prometheus.GaugeVec
	positionUnreal   *prometheus.GaugeVec
	positionExposure *prometheus.GaugeVec
}

func newCollectors(reg prometheus.Registerer) *collectors {
	c := &collectors{
		equity:           prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_equity_rub", Help: "Account equity in RUB: deposit + realized + unrealized P&L."}),
		deposit:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_deposit_rub", Help: "Configured account deposit in RUB."}),
		realized:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_realized_pnl_rub", Help: "Cumulative realized P&L net of commission in RUB."}),
		unrealized:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_unrealized_pnl_rub", Help: "Open-position P&L marked to the latest close, in RUB."}),
		commissions:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_commissions_rub", Help: "Cumulative commission paid in RUB."}),
		fills:            prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_fills", Help: "Number of executed fills replayed from the audit log."}),
		closedTrades:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_closed_trades", Help: "Number of fills that realized P&L."}),
		winningTrades:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_winning_trades", Help: "Number of realizing fills with positive P&L."}),
		openPositions:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_open_positions", Help: "Number of tickers with a non-zero open position."}),
		grossExposure:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_gross_exposure_rub", Help: "Sum of absolute open-position notional in RUB."}),
		netExposure:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_net_exposure_rub", Help: "Signed open-position notional in RUB."}),
		maxDrawdown:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_max_drawdown_rub", Help: "Max peak-to-trough drawdown of the realized P&L curve in RUB."}),
		dayRealized:      prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_day_realized_pnl_rub", Help: "Realized P&L of the current MSK trading day in RUB."}),
		dayPnL:           prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_day_pnl_rub", Help: "Current day P&L: realized plus open-position mark-to-market change, in RUB."}),
		dayTrades:        prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_day_trades", Help: "Fills executed during the current MSK trading day."}),
		lastFill:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "moex_bot_last_fill_timestamp_seconds", Help: "Unix timestamp of the most recent fill."}),
		positionLots:     prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "moex_bot_position_lots", Help: "Signed open lots per ticker."}, []string{"ticker"}),
		positionRealized: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "moex_bot_position_realized_pnl_rub", Help: "Realized P&L per ticker in RUB."}, []string{"ticker"}),
		positionUnreal:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "moex_bot_position_unrealized_pnl_rub", Help: "Unrealized P&L per ticker in RUB."}, []string{"ticker"}),
		positionExposure: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "moex_bot_position_exposure_rub", Help: "Absolute open-position notional per ticker in RUB."}, []string{"ticker"}),
	}
	reg.MustRegister(
		c.equity, c.deposit, c.realized, c.unrealized, c.commissions, c.fills,
		c.closedTrades, c.winningTrades, c.openPositions, c.grossExposure, c.netExposure,
		c.maxDrawdown, c.dayRealized, c.dayPnL, c.dayTrades, c.lastFill,
		c.positionLots, c.positionRealized, c.positionUnreal, c.positionExposure,
	)
	return c
}

type exporter struct {
	dbPath  string
	issBase string
	mtm     bool
	deposit decimal.Decimal
	logger  *log.Logger
	cols    *collectors
}

func (e *exporter) refresh(ctx context.Context) {
	fills, err := loadFills(e.dbPath)
	if err != nil {
		e.logger.Printf("pnlexporter: %v", err)
		return
	}
	snap := pnl.Replay(fills)

	var closes map[string]dailysummary.Close
	if e.mtm {
		tickers := snap.OpenTickers()
		if len(tickers) > 0 {
			src := backtest.NewISSSource(e.issBase, moex.NewClient(e.issBase, nil))
			got, closeErr := dailysummary.DayCloses(ctx, src, tickers, time.Now())
			if closeErr != nil {
				e.logger.Printf("pnlexporter: closes: %v", closeErr)
			} else {
				closes = got
			}
		}
	}
	closeMap := make(map[string]decimal.Decimal, len(closes))
	for ticker, c := range closes {
		if c.Close.IsPositive() {
			closeMap[ticker] = c.Close
		}
	}

	unrealized := snap.Unrealized(closeMap)
	day := dailysummary.Account(fills, time.Now(), dailysummary.MSKLocation(), closes)

	c := e.cols
	c.equity.Set(e.deposit.Add(snap.RealizedTotal).Add(unrealized).InexactFloat64())
	c.deposit.Set(e.deposit.InexactFloat64())
	c.realized.Set(snap.RealizedTotal.InexactFloat64())
	c.unrealized.Set(unrealized.InexactFloat64())
	c.commissions.Set(snap.CommissionsTotal.InexactFloat64())
	c.fills.Set(float64(snap.Fills))
	c.closedTrades.Set(float64(snap.ClosedTrades))
	c.winningTrades.Set(float64(snap.WinningTrades))
	c.openPositions.Set(float64(snap.OpenPositions()))
	c.grossExposure.Set(snap.GrossExposure(closeMap).InexactFloat64())
	c.netExposure.Set(snap.NetExposure(closeMap).InexactFloat64())
	c.maxDrawdown.Set(snap.MaxDrawdown.InexactFloat64())
	c.dayRealized.Set(day.Realized.InexactFloat64())
	c.dayPnL.Set(day.PnL().InexactFloat64())
	c.dayTrades.Set(float64(day.Trades))
	if !snap.LastFillAt.IsZero() {
		c.lastFill.Set(float64(snap.LastFillAt.Unix()))
	}

	c.positionLots.Reset()
	c.positionRealized.Reset()
	c.positionUnreal.Reset()
	c.positionExposure.Reset()
	unrealByTicker := snap.UnrealizedByTicker(closeMap)
	exposureByTicker := snap.ExposureByTicker(closeMap)
	for ticker, pos := range snap.Positions {
		c.positionLots.WithLabelValues(ticker).Set(float64(pos.Lots))
		c.positionRealized.WithLabelValues(ticker).Set(pos.Realized.InexactFloat64())
		c.positionUnreal.WithLabelValues(ticker).Set(unrealByTicker[ticker].InexactFloat64())
		c.positionExposure.WithLabelValues(ticker).Set(exposureByTicker[ticker].InexactFloat64())
	}
}

func loadFills(path string) ([]pnl.Fill, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT ticker, stage, payload, created_at FROM audit_events WHERE stage = 'executor'")
	if err != nil {
		return nil, fmt.Errorf("query audit_events: %w", err)
	}
	defer rows.Close()

	var events []domain.AuditEvent
	for rows.Next() {
		var event domain.AuditEvent
		var created int64
		if err := rows.Scan(&event.Ticker, &event.Stage, &event.Payload, &created); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		event.CreatedAt = time.Unix(0, created)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dailysummary.FillsFromAuditEvents(events), nil
}

func main() {
	dbPath := flag.String("db", "trader-sandbox.db", "path to the trader SQLite database")
	listen := flag.String("listen", ":9101", "address for the Prometheus /metrics endpoint")
	issBase := flag.String("iss-base", "https://iss.moex.com/iss", "MOEX ISS base URL for mark-to-market closes")
	depositStr := flag.String("deposit", "1000000", "account deposit in RUB")
	poll := flag.Duration("poll", 60*time.Second, "refresh interval")
	mtm := flag.Bool("mtm", true, "fetch daily closes from MOEX ISS to mark open positions to market")
	flag.Parse()

	logger := log.New(os.Stderr, "", log.LstdFlags)
	deposit, err := decimal.NewFromString(*depositStr)
	if err != nil {
		logger.Fatalf("pnlexporter: invalid deposit %q: %v", *depositStr, err)
	}

	registry := prometheus.NewRegistry()
	exp := &exporter{
		dbPath:  *dbPath,
		issBase: *issBase,
		mtm:     *mtm,
		deposit: deposit,
		logger:  logger,
		cols:    newCollectors(registry),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exp.refresh(ctx)
	go func() {
		ticker := time.NewTicker(*poll)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				exp.refresh(ctx)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})

	logger.Printf("pnlexporter: serving %s (db=%s, deposit=%s, mtm=%v, poll=%s)", *listen, *dbPath, deposit, *mtm, *poll)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		logger.Fatalf("pnlexporter: %v", err)
	}
}
