package main

import (
	"fmt"

	"github.com/olegsidorkin/moex-trader/internal/cryptocost"
)

type registryRow struct {
	id       string
	name     string
	wave     string
	tier     string
	turnover float64
	note     string
}

var registry = []registryRow{
	{"C01", "moex-pulse realtime serving", "Drop", "", 0, "infrastructure, not a strategy"},
	{"C02", "news exit tuning and buy side", "W3", "large", 0.30, "exit rules for shock shorts"},
	{"C03", "LLM-review hypotheses H1-H22", "W3", "mid", 0.50, "batch of calendar and regime rows"},
	{"C04", "data depth and open problems", "W3", "large", 0.10, "depth, calibration, explainability"},
	{"C05", "gate-failure acceptance protocol", "Drop", "", 0, "process, adopted as the gates"},
	{"C06", "seven hypotheses steps 1-21", "W3", "mid", 0.30, "regime mask and ranking"},
	{"C07", "core rerun on refreshed data", "Drop", "", 0, "freeze discipline, already reused"},
	{"C08", "placebo check of the ridge table", "W2", "large", 0.10, "permutation machinery, applied to every row"},
	{"C09", "research-to-live gap H-R2..R9", "W1", "major", 0.20, "quoted vs executed cost, latency, fill rate"},
	{"C10", "structural stability candidates", "W3", "large", 0.20, "state sizing, ablation, first digit"},
	{"C11", "order flow A2", "W1", "mid", 1.00, "book imbalance and trade flow"},
	{"C12", "100 flow hypotheses", "W3", "mid", 1.00, "crypto columns only"},
	{"C13", "composite alpha and cost-aware execution", "W1", "large", 0.33, "partial-adjustment book"},
	{"C14", "forecast improvement tests", "W3", "large", 0.33, "increment tests, no book"},
	{"C15", "perp-spot basis and funding curve", "W2", "major", 0.30, "carry family"},
	{"C16", "market component and targets", "W2", "major", 0.20, "BTC as the market factor"},
	{"C17", "economic gate on the liquid universe", "W1", "large", 0.33, "gate, not a signal"},
	{"C18", "flow to downstream outcome", "W3", "mid", 1.00, "thin names"},
	{"C19", "transforms of the vital few", "W3", "mid", 0.50, "generated library"},
	{"C20", "meta variables wave 1", "W3", "large", 0.10, "exogenous state"},
	{"C21", "meta variables wave 2", "W3", "large", 0.10, "vital few"},
	{"C22", "costs, horizons, next-open entry", "W1", "large", 0.33, "cost model sweep"},
	{"C23", "five model ideas", "W3", "large", 0.33, "model class"},
	{"C24", "deflated Sharpe and neutrality", "W1", "major", 0.20, "multiplicity and BTC neutrality"},
	{"C25", "announced flows (unlocks, vesting)", "W2", "mid", 0.20, "event calendar"},
	{"C26", "execution timing", "W1", "major", 0.20, "cheapest hour, spread by hour"},
	{"C27", "token moderators", "W3", "mid", 0.30, "age, float, concentration"},
	{"C28", "attention shock reversal", "W2", "mid", 1.00, "news and social shocks"},
	{"C29", "pairs and basis arbitrage", "W2", "major", 0.50, "BTC/ETH, spot/perp, staked/underlying"},
	{"C30", "thin universe ranks 21-100", "W2", "mid", 0.33, "stronger edge, wider spread"},
	{"C31", "foundation model covariance", "W3", "large", 0.33, "placebo controlled"},
	{"C32", "foundation model zero shot", "W3", "large", 0.33, "feature form"},
	{"C33", "world data: on-chain and reserves", "W2", "large", 0.20, "hash rate, flows, stablecoins"},
	{"C34", "event calendars point in time", "W2", "mid", 0.20, "unlocks, mainnet, delistings"},
	{"C35", "12-month momentum", "W1", "large", 0.10, "baseline factor"},
	{"C36", "LLM sentiment feature", "W3", "mid", 0.50, "cross-sectional feature"},
	{"C37", "low-turnover momentum books", "W1", "large", 0.05, "the structure costs reward"},
	{"C38", "calm predicts direction", "W3", "mid", 0.20, "quiet state"},
	{"C39", "audit gaps", "W1", "large", 0.10, "noise, stale, total loss"},
	{"C40", "fund playbook strategies", "W3", "mid", 0.50, "descriptions re-expressed"},
	{"C41", "liquidation cascade reversal", "W2", "mid", 1.00, "no daily gap in 24/7 markets"},
	{"C42", "momentum inside low-beta names", "W2", "large", 0.10, "low-BTC-beta tokens"},
	{"C43", "dependence splits", "W3", "mid", 0.30, "splits of the crypto results"},
	{"C44", "triage of the owner hundred", "Drop", "", 0, "triage only, no data work"},
	{"C45", "signal speed by era", "W2", "large", 0.10, "descriptive, by cycle"},
	{"C46", "value beyond 12-month momentum", "W1", "large", 0.10, "E1 control, mandatory here"},
}

func tierByLabel(label string) tier {
	switch label {
	case "major":
		return tierFor(2e9)
	case "large":
		return tierFor(2e8)
	case "mid":
		return tierFor(2e7)
	default:
		return tierFor(1e6)
	}
}

func printS1() {
	fmt.Println("S1 break-even table: required gross edge per day for the registered turnover assumption")
	fmt.Println("row  wave  tier   turn/side   cost/side  round-trip  gross/side  gross/day   row")
	for _, r := range registry {
		if r.turnover == 0 {
			fmt.Printf("%-4s %-5s %-6s %10s %10s %11s %11s %10s   %s\n", r.id, r.wave, "-", "-", "-", "-", "-", "-", r.note)
			continue
		}
		t := tierByLabel(r.tier)
		c := costConfig(cryptocost.Binance(), cryptocost.Taker, t.minQuoteVol)
		cost := c.AllInBpsPerSide()
		perSide := cost
		perDay := 2 * r.turnover * cost
		fmt.Printf("%-4s %-5s %-6s %10.2f %10.2f %11.2f %11.2f %10.2f   %s\n",
			r.id, r.wave, r.tier, r.turnover, perSide, c.RoundTripBps(), perSide, perDay, r.name)
	}
	fmt.Println()
	fmt.Println("reading: a row whose measured gross per day is below gross/day is closed at S1 without a backtest.")
	fmt.Println("turnover/side is the pre-registered daily fraction of notional replaced on one side;")
	fmt.Println("gross/day = 2 x turnover x cost/side, the edge the book must earn before funding.")
}
