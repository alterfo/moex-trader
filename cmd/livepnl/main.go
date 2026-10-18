package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	_ "modernc.org/sqlite"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/dailysummary"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/pnl"
)

func main() {
	dbPath := flag.String("db", "trader-sandbox.db", "sandbox audit database")
	from := flag.String("from", "2026-09-17", "first day of the window (MSK)")
	cut := flag.String("cut", "2026-10-10", "first day with the feature mask in the binary")
	to := flag.String("to", "2026-10-19", "exclusive end of the window")
	iss := flag.String("iss-base", "https://iss.moex.com/iss", "MOEX ISS base URL")
	flag.Parse()

	fills, err := load(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	loc := dailysummary.MSKLocation()
	start := mustDay(*from, loc)
	cutoff := mustDay(*cut, loc)
	end := mustDay(*to, loc)

	sort.SliceStable(fills, func(i, j int) bool { return fills[i].ExecutedAt.Before(fills[j].ExecutedAt) })
	upTo := func(t time.Time) []pnl.Fill {
		var out []pnl.Fill
		for _, f := range fills {
			if f.ExecutedAt.Before(t) {
				out = append(out, f)
			}
		}
		return out
	}
	between := func(a, b time.Time) (buys, sells int) {
		for _, f := range fills {
			if !f.ExecutedAt.Before(a) && f.ExecutedAt.Before(b) {
				if f.Action == domain.ActionBuy {
					buys++
				} else if f.Action == domain.ActionSell {
					sells++
				}
			}
		}
		return
	}

	base := pnl.Replay(upTo(start))
	fmt.Printf("fills total %d, first %s, last %s\n", len(fills), fills[0].ExecutedAt.In(loc).Format("2006-01-02"), fills[len(fills)-1].ExecutedAt.In(loc).Format("2006-01-02"))
	fmt.Printf("realized before %s: %s\n\n", *from, base.RealizedTotal.StringFixed(0))
	fmt.Println("day         buys sells  realized_day  realized_cum  closed  open")
	prev := base.RealizedTotal
	prevClosed := base.ClosedTrades
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		next := d.AddDate(0, 0, 1)
		snap := pnl.Replay(upTo(next))
		b, s := between(d, next)
		if b+s == 0 && snap.RealizedTotal.Equal(prev) {
			continue
		}
		fmt.Printf("%s  %4d %5d  %12s  %12s  %6d  %4d\n", d.Format("2006-01-02"), b, s,
			snap.RealizedTotal.Sub(prev).StringFixed(0), snap.RealizedTotal.StringFixed(0), snap.ClosedTrades-prevClosed, snap.OpenPositions())
		prev = snap.RealizedTotal
		prevClosed = snap.ClosedTrades
	}

	a := pnl.Replay(upTo(cutoff))
	z := pnl.Replay(upTo(end))
	bb, bs := between(start, cutoff)
	mb, ms := between(cutoff, end)
	fmt.Println()
	report := func(name string, realized decimal.Decimal, closed, buys, sells int) {
		fmt.Printf("%-28s realized %10s  closed trades %3d  buys %4d  sells %4d\n", name, realized.StringFixed(0), closed, buys, sells)
	}
	report("unmasked "+*from+".."+*cut, a.RealizedTotal.Sub(base.RealizedTotal), a.ClosedTrades-base.ClosedTrades, bb, bs)
	report("masked "+*cut+".."+*to, z.RealizedTotal.Sub(a.RealizedTotal), z.ClosedTrades-a.ClosedTrades, mb, ms)
	fmt.Printf("open positions at %s: %d, at %s: %d\n", *cut, a.OpenPositions(), *to, z.OpenPositions())

	src := backtest.NewISSSource(*iss, moex.NewClient(*iss, nil))
	equity := func(t time.Time) (decimal.Decimal, decimal.Decimal, decimal.Decimal, decimal.Decimal, decimal.Decimal) {
		snap := pnl.Replay(upTo(t))
		closes, err := dailysummary.DayCloses(context.Background(), src, snap.OpenTickers(), t.AddDate(0, 0, -1))
		if err != nil {
			log.Fatal(err)
		}
		m := make(map[string]decimal.Decimal, len(closes))
		for k, c := range closes {
			if c.Close.IsPositive() {
				m[k] = c.Close
			}
		}
		return snap.RealizedTotal, snap.Unrealized(m), closes["IMOEX"].Close, snap.NetExposure(m), snap.GrossExposure(m)
	}
	fmt.Println()
	fmt.Println("mark-to-market (realized + unrealized, RUB)")
	var prevEq, prevIdx decimal.Decimal
	for i, t := range []time.Time{start, cutoff, end} {
		r, u, idx, net, gross := equity(t)
		fmt.Printf("  net exposure %s gross %s\n", net.StringFixed(0), gross.StringFixed(0))
		eq := r.Add(u)
		if i == 0 {
			fmt.Printf("at %s: realized %s unrealized %s equity %s IMOEX %s\n", t.Format("2006-01-02"), r.StringFixed(0), u.StringFixed(0), eq.StringFixed(0), idx.StringFixed(0))
		} else {
			fmt.Printf("at %s: realized %s unrealized %s equity %s (period %s) IMOEX %s (%s%%)\n", t.Format("2006-01-02"), r.StringFixed(0), u.StringFixed(0), eq.StringFixed(0), eq.Sub(prevEq).StringFixed(0), idx.StringFixed(0), idx.Sub(prevIdx).Div(prevIdx).Mul(decimal.NewFromInt(100)).StringFixed(2))
		}
		prevEq, prevIdx = eq, idx
	}
}

func mustDay(s string, loc *time.Location) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		log.Fatal(err)
	}
	return t
}

func load(path string) ([]pnl.Fill, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT ticker, stage, payload, created_at FROM audit_events WHERE stage = 'executor'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.AuditEvent
	for rows.Next() {
		var e domain.AuditEvent
		var created int64
		if err := rows.Scan(&e.Ticker, &e.Stage, &e.Payload, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(0, created)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dailysummary.FillsFromAuditEvents(events), nil
}
