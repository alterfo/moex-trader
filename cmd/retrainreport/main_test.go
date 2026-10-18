package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/borrowcost"
)

const sample = `# Backtest Report

- Window: 2026-09-17 → 2026-10-17
- Net P&L (MTM): **-13177.3886 RUB** (LOSS)
- Realized P&L (closed trades, gross - commission): **-3281.9537 RUB** (LOSS)
- Closed trades: 45
- Max drawdown: 1.45% (14511.82 RUB)
`

func TestParseMetrics(t *testing.T) {
	m, err := parseMetrics(sample)
	if err != nil {
		t.Fatal(err)
	}
	if m.Trades != 45 || !m.Realized.Equal(decimal.RequireFromString("-3281.9537")) || !m.DDPct.Equal(decimal.RequireFromString("1.45")) {
		t.Fatalf("unexpected %+v", m)
	}
}

func TestEvaluateRejectsWorseCandidate(t *testing.T) {
	d := Metrics{Realized: decimal.NewFromInt(583), Net: decimal.NewFromInt(-12000), Trades: 32, DDPct: decimal.RequireFromString("1.24")}
	c := Metrics{Realized: decimal.NewFromInt(-3282), Net: decimal.NewFromInt(-13000), Trades: 45, DDPct: decimal.RequireFromString("1.45")}
	ok, why := evaluate(map[string]Metrics{"hold30": d, "pre90": d}, map[string]Metrics{"hold30": c, "pre90": c})
	if ok || len(why) == 0 {
		t.Fatalf("expected rejection, got %v %v", ok, why)
	}
}

func TestEvaluateAcceptsBetterCandidate(t *testing.T) {
	d := Metrics{Realized: decimal.NewFromInt(100), Net: decimal.NewFromInt(-500), Trades: 30, DDPct: decimal.RequireFromString("1.0")}
	c := Metrics{Realized: decimal.NewFromInt(900), Net: decimal.NewFromInt(200), Trades: 30, DDPct: decimal.RequireFromString("1.2")}
	ok, why := evaluate(map[string]Metrics{"hold30": d, "pre90": d}, map[string]Metrics{"hold30": c, "pre90": c})
	if !ok {
		t.Fatalf("expected pass, got %v", why)
	}
}

func TestStreak(t *testing.T) {
	days := []Day{{Pass: true}, {Pass: false}, {Pass: true}, {Pass: true}}
	if streak(days) != 2 {
		t.Fatal("streak")
	}
}

func TestParseMetricsPrefersRealizedNetOfBorrow(t *testing.T) {
	md := sample + "- Short-borrow cost (stress): 646.69 RUB\n- Realized P&L net of borrow: **-3928.6437 RUB** (LOSS)\n"
	m, err := parseMetrics(md)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Realized.Equal(decimal.RequireFromString("-3928.6437")) {
		t.Fatalf("realized = %s, want the net-of-borrow figure the preflight gate uses", m.Realized)
	}
}

func TestDailyScriptReplayMatchesPreflightRisk(t *testing.T) {
	b, err := os.ReadFile("../../scripts/daily-retrain-box.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	m := regexp.MustCompile(`BORROW_PCT_DAY="([0-9.]+)"`).FindStringSubmatch(script)
	if m == nil {
		t.Fatal("BORROW_PCT_DAY not set in the daily script")
	}
	if got, want := decimal.RequireFromString(m[1]), borrowcost.StressRatePerDay(); !got.Equal(want) {
		t.Fatalf("daily script borrow rate %s differs from the preflight stress rate %s", got, want)
	}
	for _, flag := range []string{`-max-net-short "$NET_SHORT"`, `-max-net-long "$NET_LONG"`, `-borrow-pct-day "$BORROW_PCT_DAY"`} {
		if !strings.Contains(script, flag) {
			t.Fatalf("daily script replay is missing %s, which the preflight gate applies", flag)
		}
	}
}
