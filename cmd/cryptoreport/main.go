package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type cellJSON struct {
	Signal   string  `json:"signal"`
	Lookback int     `json:"lookback"`
	Rate     float64 `json:"rate"`
	Days     int     `json:"days"`
	IC       float64 `json:"ic"`
	Gross    float64 `json:"gross"`
	Cost     float64 `json:"cost"`
	Fund     float64 `json:"fund"`
	Net      float64 `json:"net"`
	T        float64 `json:"t"`
	Sharpe   float64 `json:"sharpe"`
	DSR      float64 `json:"dsr"`
	H1       float64 `json:"h1"`
	H2       float64 `json:"h2"`
	Turn     float64 `json:"turn"`
	AvgCost  float64 `json:"avg_cost_per_side"`
}

type tierJSON struct {
	Name      string  `json:"name"`
	Fee       float64 `json:"fee"`
	Cross     float64 `json:"cross"`
	AllIn     float64 `json:"all_in"`
	RoundTrip float64 `json:"round_trip"`
}

type fundingJSON struct {
	APR   float64 `json:"apr"`
	Day1  float64 `json:"day1"`
	Day5  float64 `json:"day5"`
	Day10 float64 `json:"day10"`
	Day40 float64 `json:"day40"`
}

type coverageJSON struct {
	Month   string `json:"month"`
	Symbols int    `json:"symbols"`
}

type variantJSON struct {
	Label  string  `json:"label"`
	Net    float64 `json:"net"`
	T      float64 `json:"t"`
	Sharpe float64 `json:"sharpe"`
}

type leadJSON struct {
	Signal    string        `json:"signal"`
	Lookback  int           `json:"lookback"`
	Rate      float64       `json:"rate"`
	Base      cellJSON      `json:"base"`
	Variants  []variantJSON `json:"variants"`
	Placebo   []float64     `json:"placebo"`
	BreakEven float64       `json:"break_even_per_side"`
	Charged   float64       `json:"charged_per_side"`
}

type reportJSON struct {
	Generated string `json:"generated"`
	Window    string `json:"window"`
	Top       int    `json:"top"`
	Venue     string `json:"venue"`
	Exec      string `json:"exec"`
	Panel     struct {
		Symbols int    `json:"symbols"`
		Dates   int    `json:"dates"`
		First   string `json:"first"`
		Last    string `json:"last"`
	} `json:"panel"`
	Coverage []coverageJSON `json:"coverage"`
	Costs    []tierJSON     `json:"costs"`
	Funding  []fundingJSON  `json:"funding"`
	Close    []cellJSON     `json:"close_entry"`
	Open     []cellJSON     `json:"open_entry"`
	Lead     leadJSON       `json:"lead"`
	Bars     struct {
		Bonferroni10 float64 `json:"bonferroni_10"`
		Gate         float64 `json:"gate_t"`
	} `json:"bars"`
}

type options struct {
	data string
	out  string
}

func main() {
	var o options
	flag.StringVar(&o.data, "data", filepath.Join("artifacts", "crypto", "w1.json"), "report data produced by cryptofit -mode export")
	flag.StringVar(&o.out, "out", filepath.Join("docs", "reports", "crypto-w1.html"), "output HTML path")
	flag.Parse()
	if err := run(o); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(o options) error {
	raw, err := os.ReadFile(o.data)
	if err != nil {
		return err
	}
	var rep reportJSON
	if err := json.Unmarshal(raw, &rep); err != nil {
		return err
	}
	html := render(rep)
	if err := os.MkdirAll(filepath.Dir(o.out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(o.out, []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", o.out, len(html))
	return nil
}

const styleSheet = `
:root{
  --paper:#f7f4ee; --surface:#efeae1; --surface-2:#e6dfd3;
  --ink:#1c1a17; --muted:#6d655c; --rule:#d9d2c7;
  --accent:#9c5b1e; --accent-2:#3f5f66; --neg:#8f3a32; --pos:#3f6b4a;
  --serif:"Charter","Iowan Old Style","Palatino Linotype",Georgia,serif;
  --sans:ui-sans-serif,-apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
  --mono:"SF Mono",ui-monospace,Menlo,Consolas,monospace;
  --s1:4px; --s2:8px; --s3:12px; --s4:16px; --s5:24px; --s6:32px; --s7:48px; --s8:72px;
  --measure:74ch;
}
*{box-sizing:border-box}
html{background:var(--paper)}
body{margin:0;color:var(--ink);font-family:var(--sans);font-size:16.5px;line-height:1.62;-webkit-font-smoothing:antialiased}
.page{max-width:960px;margin:0 auto;padding:var(--s7) var(--s5) var(--s8)}
.masthead{border-bottom:2px solid var(--ink);padding-bottom:var(--s5);margin-bottom:var(--s6)}
.kicker{font-family:var(--mono);font-size:11px;letter-spacing:.14em;text-transform:uppercase;color:var(--muted);margin:0 0 var(--s3)}
h1{font-family:var(--serif);font-size:clamp(30px,4.2vw,42px);line-height:1.06;letter-spacing:-.01em;margin:0 0 var(--s4);font-weight:600;max-width:26ch}
.lede{font-size:19px;line-height:1.55;max-width:var(--measure);color:#2a2621;margin:0 0 var(--s5)}
h2{font-family:var(--serif);font-size:25px;line-height:1.2;margin:var(--s7) 0 var(--s3);font-weight:600;letter-spacing:-.005em}
h2 .num{font-family:var(--mono);font-size:12px;color:var(--accent);letter-spacing:.1em;margin-right:var(--s3);vertical-align:.35em}
p{max-width:var(--measure);margin:0 0 var(--s4)}
ul,ol{max-width:var(--measure);margin:0 0 var(--s4);padding-left:var(--s5)}
li{margin-bottom:var(--s2)}
a{color:var(--accent);text-decoration-thickness:1px;text-underline-offset:2px}
code,.mono{font-family:var(--mono);font-size:.88em}
.facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:var(--s3) var(--s5);margin:var(--s5) 0 0;padding:0}
.facts div{border-top:1px solid var(--rule);padding-top:var(--s2)}
.facts dt{font-family:var(--mono);font-size:10px;letter-spacing:.12em;text-transform:uppercase;color:var(--muted);margin:0 0 var(--s1)}
.facts dd{margin:0;font-size:14.5px}
.facts dd.mono{font-size:13px;white-space:nowrap;overflow:visible}
figure{margin:var(--s5) 0 var(--s6)}
figure svg{width:100%;height:auto;display:block}
figcaption{font-size:13px;color:var(--muted);margin-top:var(--s2);max-width:var(--measure)}
table{width:100%;border-collapse:collapse;margin:var(--s4) 0 var(--s6);font-size:14px}
caption{caption-side:top;text-align:left;font-size:13px;color:var(--muted);padding-bottom:var(--s2)}
th{text-align:left;font-weight:600;font-size:11.5px;letter-spacing:.07em;text-transform:uppercase;color:var(--muted);border-bottom:1.5px solid var(--ink);padding:var(--s2) var(--s2) var(--s2) 0}
td{padding:6px var(--s2) 6px 0;border-bottom:1px solid var(--rule);vertical-align:baseline}
td.num,th.num{text-align:right;font-family:var(--mono);font-size:13px;font-variant-numeric:tabular-nums}
tr:last-child td{border-bottom:1.5px solid var(--ink)}
.tag{font-family:var(--mono);font-size:11px;letter-spacing:.06em;text-transform:uppercase;padding:1px 6px;border:1px solid currentColor;border-radius:2px;white-space:nowrap}
.tag.lead{color:var(--accent)}
.tag.null{color:var(--muted)}
.tag.neg{color:var(--neg)}
.tag.cand{color:var(--pos)}
.callout{border-left:3px solid var(--accent);background:var(--surface);padding:var(--s4) var(--s5);margin:var(--s5) 0 var(--s6);max-width:var(--measure)}
.callout p:last-child{margin-bottom:0}
.callout .big{font-family:var(--mono);font-size:15px}
footer{border-top:2px solid var(--ink);margin-top:var(--s8);padding-top:var(--s4);font-size:13px;color:var(--muted);max-width:var(--measure)}
footer code{color:var(--ink)}
`

type doc struct{ b strings.Builder }

func (d *doc) w(s string)                { d.b.WriteString(s) }
func (d *doc) f(format string, a ...any) { fmt.Fprintf(&d.b, format, a...) }

func render(rep reportJSON) string {
	d := &doc{}
	d.w("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	d.w("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	d.w("<title>Crypto transfer of the 46 registered lines — research note</title>\n")
	d.w("<style>" + styleSheet + "</style>\n</head>\n<body>\n<main class=\"page\">\n")
	masthead(d, rep)
	method(d)
	data(d, rep)
	costs(d, rep)
	cells(d, rep)
	leadSection(d, rep)
	verdict(d, rep)
	next(d)
	footer(d, rep)
	d.w("</main>\n</body>\n</html>\n")
	return d.b.String()
}

func masthead(d *doc, rep reportJSON) {
	lead := rep.Lead.Base
	fast := findCell(rep, "momfast", 0.10)
	d.w(`<header class="masthead">
<p class="kicker">Research note · moex-trader · metrics section 10 · `)
	d.w(rep.Generated[:10])
	d.w(`</p>
<h1>Crypto transfer of the 46 registered lines</h1>
<p class="lede">The MOEX programme holds 46 registered research lines. Each was mapped to a tradable crypto analogue and scored on USDⓈ-M perpetuals with that market's own costs: exchange fees, spread charged by liquidity tier, and funding charged over the holding period. <strong>Result of the first stage: no candidate, two leads.</strong> The order-flow book earns `)
	d.f("%.1f", lead.Net)
	d.w(` bps a day net with t `)
	d.f("%.2f", lead.T)
	d.w(`, and the one-month momentum book `)
	d.f("%.1f", fast.Net)
	d.w(` bps a day with t `)
	d.f("%.2f", fast.T)
	d.w(` — both below the registered gate of absolute t `)
	d.f("%.1f", rep.Bars.Gate)
	d.w(` gate, and two fifths of the flow book's net is funding carry rather than signal.</p>
<dl class="facts">
<div><dt>window</dt><dd class="mono">`)
	d.w(rep.Window)
	d.w(`</dd></div>
<div><dt>universe</dt><dd>top `)
	d.f("%d", rep.Top)
	d.w(` by trailing liquidity</dd></div>
<div><dt>contracts in panel</dt><dd class="mono">`)
	d.f("%d (%s → %s)", rep.Panel.Symbols, rep.Panel.First, rep.Panel.Last)
	d.w(`</dd></div>
<div><dt>costs</dt><dd>`)
	d.w(rep.Venue)
	d.w(`, retail tier, funding included</dd></div>
<div><dt>cells</dt><dd class="mono">10 registered, run once</dd></div>
</dl>
</header>
`)
}

func findCell(rep reportJSON, signal string, rate float64) cellJSON {
	for _, c := range rep.Close {
		if c.Signal == signal && c.Rate == rate {
			return c
		}
	}
	return cellJSON{}
}

func method(d *doc) {
	d.w(`<section id="method">
<h2><span class="num">01</span>What was asked, and the order of operations</h2>
<p>The request was to check all 46 strategies on crypto markets, with the costs of those markets accounted for. "All 46" is read as the 46 plan files the programme registers, each mapped to a tradable analogue or marked not transferable with the reason. The mapping lives in the plan; this note reports what ran.</p>
<p>The order matters. Each line is priced first, not backtested: a book that turns over a fraction <span class="mono">t</span> of notional a side per day must earn <span class="mono">2·t·c</span> before it earns anything, where <span class="mono">c</span> is the all-in cost per side. A row whose gross edge sits below that line is closed without a backtest, because a backtest would only decorate the arithmetic. Ten cells had a plausible cost path; they were run once, on a frozen panel.</p>
<p>Three things decide a crypto verdict before any signal does: the fee tier, the spread by liquidity tier, and funding — which is not a fee but a payment on the held position, and at these horizons it is larger than the entire fee budget.</p>
</section>
`)
}

func data(d *doc, rep reportJSON) {
	d.w(`<section id="data">
<h2><span class="num">02</span>The panel, and what it cannot show</h2>
<p>Prices come from the venue's own monthly bulk dumps: daily klines and the funding-rate history of every USDⓈ-M perpetual that ever traded, delisted contracts included, so the cross-section is not a survivor panel. Tokenised equities, commodities and index contracts are excluded — 239 of 942 live contracts are not crypto, and a tokenised NVDA would smuggle an equity factor into a crypto cross-section.</p>
<figure>`)
	d.w(coverageChart(rep.Coverage, rep.Window))
	d.w(`<figcaption>Contracts with a bar at each month end. The panel starts sparse — 137 contracts by October 2021, 233 by mid-2023, 428 by early 2025 — which is why the evaluation window starts in 2024: a top-100 book needs a cross-section comfortably wider than 100 names.</figcaption></figure>
<p>What the data cannot show, and this bounds everything below: there are no order books. The flow signal here is the kline field <span class="mono">taker_buy_quote_volume / quote_volume</span> — an aggressive-flow share over 20 days — not the book-imbalance signal the MOEX A2 test used. The spread and impact in the cost model are liquidity-tier assumptions, not measurements. Both gaps are closable from the same public source and are registered as the next pull.</p>
<table><caption>Sources actually used in this stage</caption>
<thead><tr><th>source</th><th>content</th><th>role here</th></tr></thead>
<tbody>
<tr><td class="mono">futures/um/monthly/klines 1d</td><td>OHLC, volume, quote volume, trade count, taker-buy volumes</td><td>prices, signals, cost tiers</td></tr>
<tr><td class="mono">futures/um/monthly/fundingRate</td><td>settlement time, interval hours, rate</td><td>carry charged to the held side</td></tr>
<tr><td class="mono">fapi/v1/exchangeInfo</td><td>contract classification, quote asset, status</td><td>universe filter</td></tr>
<tr><td class="mono">fapi/v1/ticker/24hr</td><td>24-hour quote volume</td><td>download order</td></tr>
</tbody></table>
</section>
`)
}

func costs(d *doc, rep reportJSON) {
	var long40 float64
	for _, f := range rep.Funding {
		if f.APR > 0.10 && f.APR < 0.12 {
			long40 = f.Day40
		}
	}
	d.w(`<section id="costs">
<h2><span class="num">03</span>What crypto actually costs</h2>
<p>Perpetual futures are cheap to trade and expensive to hold. The taker fee at the retail tier is 0.05 per cent a side, half the MOEX model's 0.05 commission plus 0.05 spread plus 0.05 slippage; the spread widens with illiquidity, and funding accrues every hour or every eight hours for as long as the position stays on.</p>
<figure>`)
	d.w(costChart(rep.Costs))
	d.w(`<figcaption>All-in cost per side by trailing-liquidity tier, retail tier, 2026-11-02. The dashed line is what the MOEX programme charges per side. For the most liquid contracts crypto is about twice as cheap; below 10M USD a day it is 1.7 times dearer, and that is before funding.</figcaption></figure>
<figure>`)
	d.w(fundingChart(rep.Funding))
	d.w(`<figcaption>Funding paid by a long against the holding period. At the median perpetual rate of about 11 per cent a year, ten days of holding cost 30 bps of notional — three times the round-trip fee on a major. A dollar-neutral book nets this to roughly zero; a directional book pays it. This is the single most important line on the page.</figcaption></figure>
<div class="callout"><p class="big">A 40-day position at the median rate pays about `)
	d.f("%.0f", long40)
	d.w(` bps in funding; the same position pays about 13 bps in fees.</p><p>That is why the programme's 10-40 day horizons cannot be transferred as directional books, and why every cell below is dollar-neutral by construction.</p></div>
</section>
`)
}

func cells(d *doc, rep reportJSON) {
	d.w(`<section id="cells">
<h2><span class="num">04</span>The ten registered cells, run once</h2>
<p>Five signals at two partial-adjustment rates: 10 and 33 per cent of notional turned over a side per day. Dollar-neutral top and bottom quintile, costs charged per name by its own liquidity tier, funding from the venue's settlement history. Window 2024-01-01 to 2026-10-31, 1033 days, everything net.</p>
<figure>`)
	d.w(cellsBarChart(rep.Close, "bps a day, net of costs and funding"))
	d.w(`<figcaption>Net of costs and funding, bps of book notional a day. Two books carry the programme — order flow and one-month momentum. One is significantly negative: the daily cross-section trends at one day, so there is no reversal to trade.</figcaption></figure>
<figure>`)
	d.w(tChart(rep.Close, rep.Bars))
	d.w(`<figcaption>The same cells as Newey-West t of the daily net. The dotted line is the Bonferroni bar over ten cells (`)
	d.f("%.2f", rep.Bars.Bonferroni10)
	d.w(`); the solid line is the gate registered before the run (`)
	d.f("%.1f", rep.Bars.Gate)
	d.w(`). One cell clears the first and none reaches the second.</figcaption></figure>
<table><caption>Full read, close entry; the next-open form moves no cell by more than a tenth of a bp a day</caption>
<thead><tr><th>signal</th><th class="num">lookback</th><th class="num">rate</th><th class="num">IC</th><th class="num">gross</th><th class="num">cost</th><th class="num">funding</th><th class="num">net</th><th class="num">t</th><th class="num">DSR</th><th class="num">h1</th><th class="num">h2</th></tr></thead>
<tbody>`)
	for _, c := range rep.Close {
		d.f(`<tr><td>%s</td><td class="num">%d</td><td class="num">%.2f</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td></tr>`,
			signalName(c.Signal), c.Lookback, c.Rate, fmt3(c.IC), fmt1(c.Gross), fmt2(c.Cost), fmt2(c.Fund), fmt1(c.Net), fmt2(c.T), fmt3(c.DSR), fmt1(c.H1), fmt1(c.H2))
	}
	d.w(`</tbody></table>
</section>
`)
}

func signalName(s string) string {
	switch s {
	case "mom":
		return "momentum 12m"
	case "momfast":
		return "momentum 1m"
	case "rev":
		return "reversal 1d"
	case "taker":
		return "order flow"
	case "funding":
		return "funding carry"
	}
	return s
}

func leadSection(d *doc, rep reportJSON) {
	lead := rep.Lead
	base := variantJSON{Label: "base (registered cell)", Net: lead.Base.Net, T: lead.Base.T, Sharpe: lead.Base.Sharpe}
	var noFunding float64
	for _, v := range lead.Variants {
		if strings.HasPrefix(v.Label, "no funding") {
			noFunding = v.T
		}
	}
	d.w(`<section id="lead">
<h2><span class="num">05</span>The one thing above the bar, and why it is still only a lead</h2>
<p>Order flow — the share of aggressive buying in a contract's turnover, averaged over 20 days — is the only cell that clears the Bonferroni bar for ten trials. It is also the one that most obviously tempts a false conclusion, so it was put through ten single-change variants, a placebo, and a carry decomposition.</p>
<figure>`)
	d.w(variantsChart(lead.Variants, base))
	d.w(`<figcaption>Each variant changes exactly one thing. The sign never flips and the magnitude stays between 8 and 18 bps a day, which is what a real effect looks like. The first variant is the informative one: switching funding off takes t from `)
	d.f("%.2f", lead.Base.T)
	d.w(` to `)
	d.f("%.2f", noFunding)
	d.w(`. Part of what the signal "predicts" is that crowded longs pay shorts.</figcaption></figure>
<figure>`)
	d.w(placeboChart(lead.Placebo, lead.Base.Net))
	d.w(`<figcaption>Five within-date permutations of the same signal through the same book, costs and turnover. None reproduces the real net; the average permutation loses money, which is what a cost-bearing shuffle should do.</figcaption></figure>
<div class="callout"><p class="big">Break-even: `)
	d.f("%.1f", lead.BreakEven)
	d.w(` bps a side. Charged on average: `)
	d.f("%.1f", lead.Charged)
	d.w(` bps a side.</p><p>The economics are not the obstacle for once: a book with this edge per unit of turnover can pay crypto costs several times over. The obstacles are statistical and structural — t `)
	d.f("%.2f", lead.Base.T)
	d.w(` against a `)
	d.f("%.1f", rep.Bars.Gate)
	d.w(` gate, a single venue, one window, and a net that is two fifths carry. By the programme's own convention that is a lead, not a candidate, and it stays a lead until a forward record and a measured-spread re-read exist.</p></div>
</section>
`)
}

func verdict(d *doc, rep reportJSON) {
	flow := rep.Lead.Base
	fast := findCell(rep, "momfast", 0.10)
	slow := findCell(rep, "mom", 0.10)
	rev := findCell(rep, "rev", 0.10)
	carry := findCell(rep, "funding", 0.10)
	d.w(`<section id="verdict">
<h2><span class="num">06</span>Verdict</h2>
<p>Zero candidates, two leads, three families closed with a number instead of an opinion.</p>
<table><caption>What the first crypto stage settles</caption>
<thead><tr><th>line</th><th>family</th><th>measured</th><th>verdict</th></tr></thead>
<tbody>
<tr><td class="mono">C11</td><td>order flow</td><td>`)
	d.f("%.1f bps a day, t %.2f, DSR %.3f, placebo negative, sign stable across ten variants", flow.Net, flow.T, flow.DSR)
	d.w(`</td><td><span class="tag lead">lead</span></td></tr>
<tr><td class="mono">C37</td><td>fast momentum</td><td>`)
	d.f("%.1f bps a day at 10 per cent turnover, t %.2f; the 12-month form earns %.1f with t %.2f", fast.Net, fast.T, slow.Net, slow.T)
	d.w(`</td><td><span class="tag lead">lead, fast form only</span></td></tr>
<tr><td class="mono">C35</td><td>12-month momentum</td><td>`)
	d.f("IC %s and a positive book, but t %.2f and a first half of %.1f bps", fmt3(slow.IC), slow.T, slow.H1)
	d.w(`</td><td><span class="tag null">null</span></td></tr>
<tr><td class="mono">C41</td><td>reversal / gap</td><td>`)
	d.f("one-day reversal is significantly negative, t %.2f: the cross-section trends at one day", rev.T)
	d.w(`</td><td><span class="tag neg">closed</span></td></tr>
<tr><td class="mono">C15</td><td>funding carry</td><td>`)
	d.f("%.1f bps a day: the price move against the short leg costs more than the carry earns", carry.Net)
	d.w(`</td><td><span class="tag neg">closed</span></td></tr>
<tr><td class="mono">41 rows</td><td>the rest</td><td>priced at stage S1, or waiting on data the panel does not hold</td><td><span class="tag null">untouched</span></td></tr>
</tbody></table>
<p>No row is called confirmed. No cell reached the gate, no cell has an out-of-sample forward record, and no cell has a completed reviewer pass.</p>
</section>
`)
}

func next(d *doc) {
	d.w(`<section id="next">
<h2><span class="num">07</span>What would change the verdict</h2>
<ol>
<li><strong>Measure the spread instead of assuming it.</strong> Best-bid and offer history exists per contract per day; sampling it by pair and hour replaces the tier assumption in every cost line, and doubles as the execution-timing measurement the programme wants anyway.</li>
<li><strong>Rebuild the flow signal from the book.</strong> One-minute depth snapshots give an imbalance series closer to the MOEX A2 test than the aggressive-flow proxy used here, and settle whether the lead is flow or carry.</li>
<li><strong>Split the carry out.</strong> A funding-neutral construction, or one restricted to contracts with near-zero funding, separates the two halves of the net without touching the signal.</li>
<li><strong>Walk it forward.</strong> Eight weeks of paper records on one venue at frozen parameters, which is the only path the programme accepts from lead to candidate.</li>
</ol>
<p>Until those four exist, the honest summary of transferring 46 MOEX research lines to crypto is this page: four of them have a measurable answer, one of those answers is that the market trends and does not reverse, two are leads worth following, and the other 41 were never about a signal that costs could not eat.</p>
</section>
`)
}

func footer(d *doc, rep reportJSON) {
	d.w(`<footer>
<p>Generated `)
	d.w(rep.Generated)
	d.w(` by <code>cmd/cryptoreport</code> from <code>artifacts/crypto/w1.json</code>, written by <code>cmd/cryptofit -mode export</code> from the frozen panel. Numbers match <code>docs/metrics/s10-crypto.md</code> 10.1-10.7; the plan is <code>docs/plans/20261102-crypto-transfer-46.md</code>.</p>
<p>Panel: `)
	d.f("%d", rep.Panel.Symbols)
	d.w(` contracts, `)
	d.w(rep.Panel.First)
	d.w(` to `)
	d.w(rep.Panel.Last)
	d.w(`, one venue (`)
	d.w(rep.Venue)
	d.w(`), perpetuals only, tokenised TradFi excluded. Chart colours mirror the CSS tokens; charts are inline SVG and the page makes no external requests.</p>
</footer>
`)
}
