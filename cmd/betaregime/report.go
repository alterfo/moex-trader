package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/betaregime"
)

type reportInput struct {
	from            time.Time
	till            time.Time
	tickers         []string
	ensemblePath    string
	deposit         decimal.Decimal
	commission      decimal.Decimal
	spread          decimal.Decimal
	slippage        decimal.Decimal
	targetNotional  decimal.Decimal
	k               int
	rebalanceEvery  int
	flatBand        float64
	ensemble        *backtest.Result
	equalWeight     *backtest.Result
	momentum        []namedResult
	decomposition   betaregime.Decomposition
	samples         int
	tradeRegression betaregime.RegressionResult
	dailyRegression betaregime.RegressionResult
	dailySkipped    int
	regimes         []betaregime.WindowRegime
}

func buildReport(in reportInput) string {
	var b strings.Builder
	b.WriteString("# Beta / regime decomposition (Task 9)\n\n")
	fmt.Fprintf(&b, "- Window: %s -> %s\n", in.from.Format("2006-01-02"), in.till.Format("2006-01-02"))
	fmt.Fprintf(&b, "- Tickers: %d (%s)\n", len(in.tickers), strings.Join(in.tickers, ", "))
	fmt.Fprintf(&b, "- Ensemble artifact: %s\n", in.ensemblePath)
	fmt.Fprintf(&b, "- Costs: commission %s, spread %s, slippage %s (fractions of price)\n", in.commission.String(), in.spread.String(), in.slippage.String())
	fmt.Fprintf(&b, "- Deposit: %s RUB, target notional: %s RUB/position, k=%d, momentum rebalance every %d trading days\n", in.deposit.String(), in.targetNotional.String(), in.k, in.rebalanceEvery)
	fmt.Fprintf(&b, "- Regime flat band: |IMOEX quarter return| <= %.0f%% is flat\n\n", in.flatBand*100)

	b.WriteString("## Results (net of costs)\n\n")
	b.WriteString("| strategy | realized P&L | MTM P&L | closed trades | return on trade notional | max DD |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	writeResult := func(name string, r *backtest.Result) {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %.2f%% |\n",
			name, r.RealizedPnl.Round(2).String(), r.NetPnl.Round(2).String(), r.ClosedTrades,
			formatPct(returnOnTradeNotional(r)), r.MaxDrawdownPct)
	}
	writeResult("ensemble (deployed)", in.ensemble)
	writeResult("equal-weight 18-ticker", in.equalWeight)
	for _, bench := range in.momentum {
		writeResult("momentum "+bench.name, bench.result)
	}

	b.WriteString("\n## Realized P&L decomposition net of IMOEX\n\n")
	d := in.decomposition
	fmt.Fprintf(&b, "- Samples: %d closed trades\n", in.samples)
	fmt.Fprintf(&b, "- Total realized P&L: %s RUB\n", d.TotalRealizedPnl.Round(2).String())
	fmt.Fprintf(&b, "- Gross exposure (sum of closed-trade notionals): %s RUB\n", d.GrossExposure.Round(2).String())
	fmt.Fprintf(&b, "- Return on gross exposure: %s\n", formatPct(d.ReturnOnGrossExposure))
	fmt.Fprintf(&b, "- IMOEX (beta) component: %s RUB\n", d.ImoexPnl.Round(2).String())
	fmt.Fprintf(&b, "- Excess (alpha) component: %s RUB\n\n", d.ExcessPnl.Round(2).String())

	b.WriteString("| leg | trades | realized P&L | notional | IMOEX component | excess | return on notional |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	writeLeg := func(name string, leg betaregime.LegStats) {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s | %s |\n",
			name, leg.Trades, leg.RealizedPnl.Round(2).String(), leg.Notional.Round(2).String(),
			leg.ImoexPnl.Round(2).String(), leg.ExcessPnl.Round(2).String(), formatPct(leg.ReturnOnNotional))
	}
	writeLeg("long", d.Long)
	writeLeg("short", d.Short)

	b.WriteString("\n## Regression: P&L ~ alpha + beta1*equal-weight + beta2*momentum\n\n")
	writeRegression := func(title string, res betaregime.RegressionResult, note string) {
		b.WriteString("### " + title + "\n\n")
		if note != "" {
			fmt.Fprintf(&b, "%s\n\n", note)
		}
		fmt.Fprintf(&b, "- N=%d, parameters=%d, clusters=%d, df=%d, R2=%.4f\n", res.Observations, res.Parameters, res.Clusters, res.DegreesFree, res.R2)
		b.WriteString("\n| term | estimate | cluster SE | t-stat | p-value |\n")
		b.WriteString("|---|---|---|---|---|\n")
		names := []string{"alpha", "beta1 (equal-weight)", "beta2 (momentum)"}
		for i := range res.Coef {
			se := "-"
			tStat := "-"
			pVal := "-"
			if i < len(res.StdErr) && res.StdErr[i] > 0 {
				se = fmt.Sprintf("%.6f", res.StdErr[i])
				tStat = fmt.Sprintf("%.3f", res.TStat[i])
				pVal = fmt.Sprintf("%.4f", res.PValue[i])
			}
			fmt.Fprintf(&b, "| %s | %.6f | %s | %s | %s |\n", names[i], res.Coef[i], se, tStat, pVal)
		}
		b.WriteString("\n")
	}
	writeRegression("Per-trade", in.tradeRegression, "Dependent variable is the realized return on entry notional for each closed trade; the independent variables are the equal-weight and momentum long+short benchmark returns over the same holding window. Standard errors are one-way clustered by ticker.")
	writeRegression("Per-day", in.dailyRegression, fmt.Sprintf("Dependent variable is the daily portfolio return; standard errors are one-way clustered by OOS quarter. %d daily observations fell outside the OOS windows or lacked a benchmark return and were dropped.", in.dailySkipped))

	b.WriteString("## OOS quarter regimes (IMOEX)\n\n")
	b.WriteString("| window | IMOEX return | regime |\n")
	b.WriteString("|---|---|---|\n")
	for _, regime := range in.regimes {
		ret := "-"
		label := "no data"
		if regime.HasData {
			ret = fmt.Sprintf("%+.2f%%", regime.ImoexReturn*100)
			label = regime.Regime
		}
		fmt.Fprintf(&b, "| %s -> %s | %s | %s |\n", regime.From.Format("2006-01-02"), regime.Till.Format("2006-01-02"), ret, label)
	}
	return b.String()
}

func returnOnTradeNotional(r *backtest.Result) float64 {
	var notional decimal.Decimal
	for _, tr := range r.Trades {
		notional = notional.Add(tr.EntryPrice.Mul(decimal.NewFromInt(int64(tr.Lots))))
	}
	denom, _ := notional.Float64()
	if denom <= 0 {
		return 0
	}
	pnl, _ := r.RealizedPnl.Float64()
	return pnl / denom
}

func formatPct(v float64) string {
	return fmt.Sprintf("%+.2f%%", v*100)
}
