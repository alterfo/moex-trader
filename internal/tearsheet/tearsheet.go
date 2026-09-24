package tearsheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

type metricCell struct {
	Value string
	Class string
}

type attributionRow struct {
	Ticker     string
	Trades     int
	Wins       int
	Losses     int
	Gross      string
	Commission string
	Realized   metricCell
	Unrealized metricCell
	Total      metricCell
}

type tradeRow struct {
	Ticker     string
	Side       string
	Lots       int
	Entry      string
	Exit       string
	Gross      string
	Commission string
	Net        metricCell
	From       string
	To         string
}

type viewModel struct {
	GeneratedAt string
	Tickers     string
	Start       string
	End         string
	Deposit     string
	FinalEquity string

	NetPnl          metricCell
	RealizedPnl     metricCell
	UnrealizedPnl   metricCell
	GrossPnl        string
	TotalCommission string
	TotalBorrow     string
	HasBorrow       bool

	ClosedTrades  int
	WinningTrades int
	HitRatePct    string

	CAGR    metricCell
	Sharpe  metricCell
	Sortino metricCell
	Calmar  metricCell

	MaxDrawdownPct string
	MaxDrawdownRub string

	Significant bool
	MinTrades   int

	EquitySVG   template.HTML
	DrawdownSVG template.HTML

	AttributionRows []attributionRow
	TradeRows       []tradeRow
}

type MetricsExport struct {
	Start                    string   `json:"start"`
	End                      string   `json:"end"`
	Tickers                  []string `json:"tickers"`
	Deposit                  string   `json:"deposit"`
	FinalEquity              string   `json:"final_equity"`
	NetPnl                   string   `json:"net_pnl"`
	RealizedPnl              string   `json:"realized_pnl"`
	UnrealizedPnl            string   `json:"unrealized_pnl"`
	GrossPnl                 string   `json:"gross_pnl"`
	TotalCommission          string   `json:"total_commission"`
	TotalBorrow              string   `json:"total_borrow"`
	ClosedTrades             int      `json:"closed_trades"`
	WinningTrades            int      `json:"winning_trades"`
	HitRate                  float64  `json:"hit_rate"`
	CAGRPct                  float64  `json:"cagr_pct"`
	Sharpe                   float64  `json:"sharpe"`
	Sortino                  float64  `json:"sortino"`
	Calmar                   float64  `json:"calmar"`
	MaxDrawdownPct           float64  `json:"max_drawdown_pct"`
	MaxDrawdownRub           string   `json:"max_drawdown_rub"`
	StatisticallySignificant bool     `json:"statistically_significant"`
	MinTradesForSignificance int      `json:"min_trades_for_significance"`
	GeneratedAt              string   `json:"generated_at"`
}

func Render(r backtest.Result) (html []byte, metricsJSON []byte, err error) {
	generatedAt := time.Now().UTC()

	vm := viewModel{
		GeneratedAt:     generatedAt.Format("2006-01-02 15:04:05 UTC"),
		Tickers:         strings.Join(r.Tickers, ", "),
		Start:           formatDay(r.Start),
		End:             formatDay(r.End),
		Deposit:         r.Deposit.StringFixed(2),
		FinalEquity:     r.FinalEquity.StringFixed(2),
		NetPnl:          money(r.NetPnl),
		RealizedPnl:     money(r.RealizedPnl),
		UnrealizedPnl:   money(r.UnrealizedPnl),
		GrossPnl:        r.GrossPnl.StringFixed(2),
		TotalCommission: r.TotalCommission.StringFixed(2),
		TotalBorrow:     r.TotalBorrow.StringFixed(2),
		HasBorrow:       !r.TotalBorrow.IsZero(),
		ClosedTrades:    r.ClosedTrades,
		WinningTrades:   r.WinningTrades,
		HitRatePct:      fmt.Sprintf("%.1f", r.HitRate*100),
		CAGR:            floatCell(r.CAGR, "%.2f%%"),
		Sharpe:          floatCell(r.Sharpe, "%.2f"),
		Sortino:         floatCell(r.Sortino, "%.2f"),
		Calmar:          floatCell(r.Calmar, "%.2f"),
		MaxDrawdownPct:  fmt.Sprintf("%.2f", r.MaxDrawdownPct),
		MaxDrawdownRub:  r.MaxDrawdownRub.StringFixed(2),
		Significant:     r.StatisticallySignificant(),
		MinTrades:       backtest.MinTradesForSignificance,
	}

	equity, drawdown := equityAndDrawdownSeries(r.EquityCurve)
	vm.EquitySVG = lineChartSVG(equity, "--ts-equity", "--ts-equity", false)
	vm.DrawdownSVG = lineChartSVG(drawdown, "--ts-critical", "--ts-critical", true)

	for _, t := range r.Attribution.Tickers {
		total := t.RealizedPnl.Add(t.UnrealizedPnl)
		vm.AttributionRows = append(vm.AttributionRows, attributionRow{
			Ticker:     t.Ticker,
			Trades:     t.Trades,
			Wins:       t.Wins,
			Losses:     t.Losses,
			Gross:      t.GrossPnl.StringFixed(2),
			Commission: t.Commission.StringFixed(2),
			Realized:   money(t.RealizedPnl),
			Unrealized: money(t.UnrealizedPnl),
			Total:      money(total),
		})
	}

	for _, t := range r.Trades {
		vm.TradeRows = append(vm.TradeRows, tradeRow{
			Ticker:     t.Ticker,
			Side:       string(t.Action),
			Lots:       t.Lots,
			Entry:      t.EntryPrice.StringFixed(2),
			Exit:       t.ExitPrice.StringFixed(2),
			Gross:      t.GrossPnl.StringFixed(2),
			Commission: t.Commission.StringFixed(2),
			Net:        money(t.NetPnl),
			From:       formatDay(t.OpenedAt),
			To:         formatDay(t.ClosedAt),
		})
	}

	tmpl, err := template.New("tearsheet").Parse(pageTemplate)
	if err != nil {
		return nil, nil, fmt.Errorf("tearsheet: parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vm); err != nil {
		return nil, nil, fmt.Errorf("tearsheet: render template: %w", err)
	}

	export := MetricsExport{
		Start:                    formatDay(r.Start),
		End:                      formatDay(r.End),
		Tickers:                  r.Tickers,
		Deposit:                  r.Deposit.StringFixed(2),
		FinalEquity:              r.FinalEquity.StringFixed(2),
		NetPnl:                   r.NetPnl.StringFixed(2),
		RealizedPnl:              r.RealizedPnl.StringFixed(2),
		UnrealizedPnl:            r.UnrealizedPnl.StringFixed(2),
		GrossPnl:                 r.GrossPnl.StringFixed(2),
		TotalCommission:          r.TotalCommission.StringFixed(2),
		TotalBorrow:              r.TotalBorrow.StringFixed(2),
		ClosedTrades:             r.ClosedTrades,
		WinningTrades:            r.WinningTrades,
		HitRate:                  r.HitRate,
		CAGRPct:                  r.CAGR,
		Sharpe:                   r.Sharpe,
		Sortino:                  r.Sortino,
		Calmar:                   r.Calmar,
		MaxDrawdownPct:           r.MaxDrawdownPct,
		MaxDrawdownRub:           r.MaxDrawdownRub.StringFixed(2),
		StatisticallySignificant: r.StatisticallySignificant(),
		MinTradesForSignificance: backtest.MinTradesForSignificance,
		GeneratedAt:              generatedAt.Format(time.RFC3339),
	}
	metricsJSON, err = json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("tearsheet: marshal metrics: %w", err)
	}

	return buf.Bytes(), metricsJSON, nil
}

func money(d decimal.Decimal) metricCell {
	return metricCell{Value: d.StringFixed(2), Class: signClass(d.Sign())}
}

func floatCell(v float64, format string) metricCell {
	class := "ts-flat"
	switch {
	case v > 0:
		class = "ts-pos"
	case v < 0:
		class = "ts-neg"
	}
	return metricCell{Value: fmt.Sprintf(format, v), Class: class}
}

func signClass(sign int) string {
	switch {
	case sign > 0:
		return "ts-pos"
	case sign < 0:
		return "ts-neg"
	default:
		return "ts-flat"
	}
}

func formatDay(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func equityAndDrawdownSeries(points []backtest.EquityPoint) (equity []float64, drawdownPct []float64) {
	equity = make([]float64, len(points))
	drawdownPct = make([]float64, len(points))
	peak := 0.0
	for i, p := range points {
		v, _ := p.Equity.Float64()
		equity[i] = v
		if i == 0 || v > peak {
			peak = v
		}
		if peak > 0 {
			drawdownPct[i] = (v - peak) / peak * 100
		}
	}
	return equity, drawdownPct
}
