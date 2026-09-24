package walkforward

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

const metricsFileName = "metrics.json"

type Metrics struct {
	WindowID       string    `json:"window_id"`
	From           time.Time `json:"from"`
	Split          time.Time `json:"split"`
	Till           time.Time `json:"till"`
	ClosedTrades   int       `json:"closed_trades"`
	WinningTrades  int       `json:"winning_trades"`
	HitRate        float64   `json:"hit_rate"`
	NetPnl         string    `json:"net_pnl"`
	RealizedPnl    string    `json:"realized_pnl"`
	Sharpe         float64   `json:"sharpe"`
	Sortino        float64   `json:"sortino"`
	Calmar         float64   `json:"calmar"`
	CAGR           float64   `json:"cagr_pct"`
	MaxDrawdownPct float64   `json:"max_drawdown_pct"`
	MaxDrawdownRub string    `json:"max_drawdown_rub"`
	Significant    bool      `json:"statistically_significant"`
}

func MetricsFromResult(windowID string, spec WindowSpec, r backtest.Result) Metrics {
	return Metrics{
		WindowID:       windowID,
		From:           spec.From,
		Split:          spec.Split,
		Till:           spec.Till,
		ClosedTrades:   r.ClosedTrades,
		WinningTrades:  r.WinningTrades,
		HitRate:        r.HitRate,
		NetPnl:         r.NetPnl.StringFixed(2),
		RealizedPnl:    r.RealizedPnl.StringFixed(2),
		Sharpe:         r.Sharpe,
		Sortino:        r.Sortino,
		Calmar:         r.Calmar,
		CAGR:           r.CAGR,
		MaxDrawdownPct: r.MaxDrawdownPct,
		MaxDrawdownRub: r.MaxDrawdownRub.StringFixed(2),
		Significant:    r.StatisticallySignificant(),
	}
}

func SaveMetrics(dir string, m Metrics) error {
	payload, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("walk-forward: marshal metrics: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("walk-forward: create window dir: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, metricsFileName), payload)
}

func LoadMetrics(dir string) (Metrics, error) {
	data, err := os.ReadFile(filepath.Join(dir, metricsFileName))
	if err != nil {
		return Metrics{}, fmt.Errorf("walk-forward: read metrics %q: %w", dir, err)
	}
	var m Metrics
	if err := json.Unmarshal(data, &m); err != nil {
		return Metrics{}, fmt.Errorf("walk-forward: parse metrics %q: %w", dir, err)
	}
	return m, nil
}
