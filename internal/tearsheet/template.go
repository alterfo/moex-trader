package tearsheet

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Backtest Tearsheet · {{.Tickers}}</title>
<style>
  :root {
    --ts-surface:   #fcfcfb;
    --ts-page:      #f9f9f7;
    --ts-ink:       #0b0b0b;
    --ts-ink-2:     #52514e;
    --ts-muted:     #898781;
    --ts-grid:      #e1e0d9;
    --ts-axis:      #c3c2b7;
    --ts-border:    rgba(11,11,11,0.10);
    --ts-equity:    #2a78d6;
    --ts-critical:  #d03b3b;
    --ts-good:      #0ca30c;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --ts-surface:  #1a1a19;
      --ts-page:     #0d0d0d;
      --ts-ink:      #ffffff;
      --ts-ink-2:    #c3c2b7;
      --ts-muted:    #898781;
      --ts-grid:     #2c2c2a;
      --ts-axis:     #383835;
      --ts-border:   rgba(255,255,255,0.10);
      --ts-equity:   #3987e5;
      --ts-critical: #e66767;
      --ts-good:     #0ca30c;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    background: var(--ts-page);
    color: var(--ts-ink);
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    font-size: 14px;
    line-height: 1.45;
  }
  .ts-wrap { max-width: 1040px; margin: 0 auto; padding: 24px 20px 60px; }
  .ts-header { margin-bottom: 20px; }
  .ts-header h1 { font-size: 20px; margin: 0 0 6px; }
  .ts-header .ts-sub { color: var(--ts-ink-2); font-size: 13px; }
  .ts-card {
    background: var(--ts-surface);
    border: 1px solid var(--ts-border);
    border-radius: 10px;
    padding: 18px 20px;
    margin-bottom: 18px;
  }
  .ts-card h2 { font-size: 15px; margin: 0 0 14px; color: var(--ts-ink); }
  .ts-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 14px;
  }
  .ts-metric .ts-label {
    color: var(--ts-muted);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .ts-metric .ts-value {
    font-size: 18px;
    font-variant-numeric: tabular-nums;
    margin-top: 2px;
  }
  .ts-pos { color: var(--ts-good); }
  .ts-neg { color: var(--ts-critical); }
  .ts-flat { color: var(--ts-ink); }
  .ts-warn {
    color: var(--ts-critical);
    font-size: 13px;
    margin-top: 4px;
  }
  line.ts-svg-grid { stroke: var(--ts-grid); stroke-width: 1; }
  line.ts-svg-axis { stroke: var(--ts-axis); stroke-width: 1; }
  table { width: 100%; border-collapse: collapse; font-variant-numeric: tabular-nums; }
  th, td { text-align: right; padding: 6px 10px; border-bottom: 1px solid var(--ts-border); white-space: nowrap; }
  th:first-child, td:first-child, th:nth-child(2), td:nth-child(2) { text-align: left; }
  th {
    position: sticky;
    top: 0;
    background: var(--ts-surface);
    color: var(--ts-muted);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    font-weight: 600;
  }
  .ts-table-scroll { max-height: 480px; overflow: auto; }
  .ts-footer { color: var(--ts-muted); font-size: 12px; margin-top: 24px; }
</style>
</head>
<body>
<div class="ts-wrap">

  <div class="ts-header">
    <h1>Backtest Tearsheet</h1>
    <div class="ts-sub">{{.Tickers}} &nbsp;·&nbsp; {{.Start}} &rarr; {{.End}} &nbsp;·&nbsp; deposit {{.Deposit}} RUB &nbsp;·&nbsp; final equity {{.FinalEquity}} RUB</div>
  </div>

  <div class="ts-card">
    <h2>Key metrics</h2>
    <div class="ts-grid">
      <div class="ts-metric"><div class="ts-label">Net P&amp;L</div><div class="ts-value {{.NetPnl.Class}}">{{.NetPnl.Value}}</div></div>
      <div class="ts-metric"><div class="ts-label">Realized P&amp;L</div><div class="ts-value {{.RealizedPnl.Class}}">{{.RealizedPnl.Value}}</div></div>
      <div class="ts-metric"><div class="ts-label">Unrealized P&amp;L</div><div class="ts-value {{.UnrealizedPnl.Class}}">{{.UnrealizedPnl.Value}}</div></div>
      <div class="ts-metric"><div class="ts-label">Gross P&amp;L</div><div class="ts-value">{{.GrossPnl}}</div></div>
      <div class="ts-metric"><div class="ts-label">Commission</div><div class="ts-value">{{.TotalCommission}}</div></div>
      {{if .HasBorrow}}<div class="ts-metric"><div class="ts-label">Borrow cost</div><div class="ts-value">{{.TotalBorrow}}</div></div>{{end}}
      <div class="ts-metric"><div class="ts-label">CAGR</div><div class="ts-value {{.CAGR.Class}}">{{.CAGR.Value}}</div></div>
      <div class="ts-metric"><div class="ts-label">Sharpe</div><div class="ts-value {{.Sharpe.Class}}">{{.Sharpe.Value}}</div></div>
      <div class="ts-metric"><div class="ts-label">Sortino</div><div class="ts-value {{.Sortino.Class}}">{{.Sortino.Value}}</div></div>
      <div class="ts-metric"><div class="ts-label">Calmar</div><div class="ts-value {{.Calmar.Class}}">{{.Calmar.Value}}</div></div>
      <div class="ts-metric"><div class="ts-label">Max drawdown</div><div class="ts-value ts-neg">{{.MaxDrawdownPct}}% ({{.MaxDrawdownRub}})</div></div>
      <div class="ts-metric"><div class="ts-label">Hit rate</div><div class="ts-value">{{.HitRatePct}}% ({{.WinningTrades}}/{{.ClosedTrades}})</div></div>
    </div>
    {{if not .Significant}}<div class="ts-warn">&#9888; {{.ClosedTrades}} closed trades &lt; {{.MinTrades}} &mdash; metrics are not statistically significant.</div>{{end}}
  </div>

  <div class="ts-card">
    <h2>Equity curve</h2>
    {{.EquitySVG}}
  </div>

  <div class="ts-card">
    <h2>Drawdown (underwater)</h2>
    {{.DrawdownSVG}}
  </div>

  {{if .AttributionRows}}
  <div class="ts-card">
    <h2>Attribution by ticker</h2>
    <table>
      <thead><tr><th>Ticker</th><th>Trades</th><th>Wins</th><th>Losses</th><th>Gross</th><th>Commission</th><th>Realized</th><th>Unrealized</th><th>Total</th></tr></thead>
      <tbody>
        {{range .AttributionRows}}
        <tr>
          <td>{{.Ticker}}</td>
          <td>{{.Trades}}</td>
          <td>{{.Wins}}</td>
          <td>{{.Losses}}</td>
          <td>{{.Gross}}</td>
          <td>{{.Commission}}</td>
          <td class="{{.Realized.Class}}">{{.Realized.Value}}</td>
          <td class="{{.Unrealized.Class}}">{{.Unrealized.Value}}</td>
          <td class="{{.Total.Class}}">{{.Total.Value}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </div>
  {{end}}

  {{if .TradeRows}}
  <div class="ts-card">
    <h2>Closed trades</h2>
    <div class="ts-table-scroll">
    <table>
      <thead><tr><th>Ticker</th><th>Side</th><th>Lots</th><th>Entry</th><th>Exit</th><th>Gross</th><th>Commission</th><th>Net</th><th>From</th><th>To</th></tr></thead>
      <tbody>
        {{range .TradeRows}}
        <tr>
          <td>{{.Ticker}}</td>
          <td>{{.Side}}</td>
          <td>{{.Lots}}</td>
          <td>{{.Entry}}</td>
          <td>{{.Exit}}</td>
          <td>{{.Gross}}</td>
          <td>{{.Commission}}</td>
          <td class="{{.Net.Class}}">{{.Net.Value}}</td>
          <td>{{.From}}</td>
          <td>{{.To}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    </div>
  </div>
  {{end}}

  <div class="ts-footer">Generated {{.GeneratedAt}} by moex-trader internal/tearsheet.</div>

</div>
</body>
</html>
`
