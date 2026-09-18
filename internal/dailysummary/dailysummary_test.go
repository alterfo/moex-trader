package dailysummary

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

var testLoc = time.FixedZone("MSK", 3*3600)

func dec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	v, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("parse decimal %q: %v", s, err)
	}
	return v
}

func testFill(t *testing.T, ticker string, action domain.Action, lots int, price, commission string, at time.Time) Fill {
	t.Helper()
	return Fill{
		Ticker:     ticker,
		Action:     action,
		Lots:       lots,
		Price:      dec(t, price),
		Commission: dec(t, commission),
		ExecutedAt: at,
	}
}

func closes(m map[string]Close) map[string]Close {
	return m
}

func TestAccountLongPartialClose(t *testing.T) {
	yesterday := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	today := time.Date(2026, 9, 18, 15, 0, 0, 0, time.UTC)
	fills := []Fill{
		testFill(t, "SBER", domain.ActionBuy, 2, "100", "0.5", yesterday),
		testFill(t, "SBER", domain.ActionSell, 1, "110", "0.25", today),
	}
	got := Account(fills, time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC), testLoc, closes(map[string]Close{
		"SBER": {Close: dec(t, "110"), PrevClose: dec(t, "100"), Date: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)},
	}))

	if got.Trades != 1 {
		t.Fatalf("Trades = %d, want 1", got.Trades)
	}
	if want := dec(t, "9.50"); !got.Realized.Equal(want) {
		t.Fatalf("Realized = %s, want %s", got.Realized, want)
	}
	if got.OpenPositions != 1 {
		t.Fatalf("OpenPositions = %d, want 1", got.OpenPositions)
	}
	if want := dec(t, "-0.50"); !got.DayStartMTM.Equal(want) {
		t.Fatalf("DayStartMTM = %s, want %s", got.DayStartMTM, want)
	}
	if want := dec(t, "9.75"); !got.EndMTM.Equal(want) {
		t.Fatalf("EndMTM = %s, want %s", got.EndMTM, want)
	}
	if want := dec(t, "19.75"); !got.PnL().Equal(want) {
		t.Fatalf("PnL = %s, want %s", got.PnL(), want)
	}
}

func TestAccountHeldThroughDay(t *testing.T) {
	fills := []Fill{
		testFill(t, "SBER", domain.ActionBuy, 2, "100", "0.5", time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)),
	}
	got := Account(fills, time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC), testLoc, closes(map[string]Close{
		"SBER": {Close: dec(t, "105"), PrevClose: dec(t, "100"), Date: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)},
	}))

	if got.Trades != 0 || !got.Realized.IsZero() {
		t.Fatalf("expected no trades today, got %d trades realized %s", got.Trades, got.Realized)
	}
	if got.OpenPositions != 1 {
		t.Fatalf("OpenPositions = %d, want 1", got.OpenPositions)
	}
	if want := dec(t, "-0.50"); !got.DayStartMTM.Equal(want) {
		t.Fatalf("DayStartMTM = %s, want %s", got.DayStartMTM, want)
	}
	if want := dec(t, "9.50"); !got.EndMTM.Equal(want) {
		t.Fatalf("EndMTM = %s, want %s", got.EndMTM, want)
	}
	if want := dec(t, "10.00"); !got.PnL().Equal(want) {
		t.Fatalf("PnL = %s, want %s", got.PnL(), want)
	}
}

func TestAccountOpenAndCloseSameDay(t *testing.T) {
	today := time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC)
	fills := []Fill{
		testFill(t, "GAZP", domain.ActionBuy, 1, "100", "0.1", time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)),
		testFill(t, "GAZP", domain.ActionSell, 1, "110", "0.1", time.Date(2026, 9, 18, 15, 0, 0, 0, time.UTC)),
	}
	got := Account(fills, today, testLoc, closes(map[string]Close{
		"GAZP": {Close: dec(t, "110"), PrevClose: dec(t, "99"), Date: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)},
	}))

	if got.Trades != 2 {
		t.Fatalf("Trades = %d, want 2", got.Trades)
	}
	if want := dec(t, "9.80"); !got.Realized.Equal(want) {
		t.Fatalf("Realized = %s, want %s", got.Realized, want)
	}
	if got.OpenPositions != 0 || !got.DayStartMTM.IsZero() || !got.EndMTM.IsZero() {
		t.Fatalf("expected flat day, got OpenPositions=%d DayStartMTM=%s EndMTM=%s", got.OpenPositions, got.DayStartMTM, got.EndMTM)
	}
	if want := dec(t, "9.80"); !got.PnL().Equal(want) {
		t.Fatalf("PnL = %s, want %s", got.PnL(), want)
	}
}

func TestAccountReversalOpensShort(t *testing.T) {
	today := time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC)
	fills := []Fill{
		testFill(t, "LKOH", domain.ActionBuy, 1, "100", "0.1", time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)),
		testFill(t, "LKOH", domain.ActionSell, 2, "110", "0.1", time.Date(2026, 9, 18, 15, 0, 0, 0, time.UTC)),
	}
	got := Account(fills, today, testLoc, closes(map[string]Close{
		"LKOH": {Close: dec(t, "110"), PrevClose: dec(t, "100"), Date: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)},
	}))

	if got.Trades != 2 {
		t.Fatalf("Trades = %d, want 2", got.Trades)
	}
	if want := dec(t, "9.85"); !got.Realized.Equal(want) {
		t.Fatalf("Realized = %s, want %s", got.Realized, want)
	}
	if got.OpenPositions != 1 {
		t.Fatalf("OpenPositions = %d, want 1 (short)", got.OpenPositions)
	}
	if want := dec(t, "-0.05"); !got.EndMTM.Equal(want) {
		t.Fatalf("EndMTM = %s, want %s", got.EndMTM, want)
	}
	if want := dec(t, "9.80"); !got.PnL().Equal(want) {
		t.Fatalf("PnL = %s, want %s", got.PnL(), want)
	}
}

func TestAccountNoFills(t *testing.T) {
	got := Account(nil, time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC), testLoc, nil)
	if got.Trades != 0 || !got.Realized.IsZero() || got.OpenPositions != 0 ||
		!got.DayStartMTM.IsZero() || !got.EndMTM.IsZero() || !got.PnL().IsZero() {
		t.Fatalf("expected empty day, got %+v", got)
	}
}

func TestFillsFromAuditEvents(t *testing.T) {
	executed := time.Date(2026, 9, 18, 14, 35, 0, 0, time.UTC)
	events := []domain.AuditEvent{
		{
			Stage:     "executor",
			Payload:   `{"id":"a","ticker":"SBER","action":"BUY","lots":2,"price":"100.5","expected_price":"100","commission":"0.51","executed_at":"2026-09-18T14:35:00Z"}`,
			CreatedAt: executed,
		},
		{Stage: "ingest", Payload: `{"ticker":"SBER"}`, CreatedAt: executed},
		{Stage: "executor", Payload: `not json`, CreatedAt: executed},
		{Stage: "executor", Payload: `{"id":"b","ticker":"","action":"BUY","lots":0,"price":"10","commission":"0","executed_at":"2026-09-18T14:35:00Z"}`, CreatedAt: executed},
	}
	fills := FillsFromAuditEvents(events)
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want 1", len(fills))
	}
	f := fills[0]
	if f.Ticker != "SBER" || f.Action != domain.ActionBuy || f.Lots != 2 ||
		!f.Price.Equal(decimal.NewFromFloat(100.5)) || !f.Commission.Equal(decimal.NewFromFloat(0.51)) {
		t.Fatalf("unexpected fill: %+v", f)
	}
	if !f.ExecutedAt.Equal(executed) {
		t.Fatalf("ExecutedAt = %v, want %v", f.ExecutedAt, executed)
	}
}

type fakeCandleSource struct {
	byTicker map[string][]moex.Candle
	errs     map[string]error
}

func (f *fakeCandleSource) History(_ context.Context, ticker string, _, _ time.Time) ([]moex.Candle, error) {
	if err := f.errs[ticker]; err != nil {
		return nil, err
	}
	return f.byTicker[ticker], nil
}

func TestDayCloses(t *testing.T) {
	d := func(day int) time.Time {
		return time.Date(2026, 9, day, 10, 0, 0, 0, time.UTC)
	}
	last := func(prevClose, close string) []moex.Candle {
		return []moex.Candle{
			{Begin: d(17), Close: dec(t, prevClose)},
			{Begin: d(18), Close: dec(t, close)},
		}
	}
	src := &fakeCandleSource{
		byTicker: map[string][]moex.Candle{
			"IMOEX": last("120", "122.5"),
			"SBER":  last("300", "309"),
			"VKCO":  []moex.Candle{{Begin: d(16), Close: dec(t, "10")}, {Begin: d(17), Close: dec(t, "10.5")}},
			"BAD":   nil,
		},
		errs: map[string]error{"NOPE": errors.New("no data")},
	}

	got, err := DayCloses(context.Background(), src, []string{"SBER", "VKCO", "NOPE"}, d(18))
	if err != nil {
		t.Fatalf("DayCloses error: %v", err)
	}
	if missing, ok := got["NOPE"]; ok {
		t.Fatalf("failed ticker should be skipped, got %+v", missing)
	}
	imoex, ok := got["IMOEX"]
	if !ok || !imoex.Valid() || !imoex.Close.Equal(dec(t, "122.5")) || !imoex.PrevClose.Equal(dec(t, "120")) {
		t.Fatalf("unexpected IMOEX close: %+v", imoex)
	}
	sber, ok := got["SBER"]
	if !ok || !sber.Valid() || !sber.Close.Equal(dec(t, "309")) || !sber.PrevClose.Equal(dec(t, "300")) {
		t.Fatalf("unexpected SBER close: %+v", sber)
	}
	vkco, ok := got["VKCO"]
	if !ok || vkco.Date.Format("2006-01-02") != "2026-09-17" {
		t.Fatalf("unexpected VKCO close (stale candle should stand): %+v", vkco)
	}
}

func TestDayClosesIMOEXFailureIsFatal(t *testing.T) {
	src := &fakeCandleSource{
		byTicker: map[string][]moex.Candle{"SBER": {{Begin: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC), Close: dec(t, "1")}}},
		errs:     map[string]error{"IMOEX": errors.New("iss down")},
	}
	_, err := DayCloses(context.Background(), src, []string{"SBER"}, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error when IMOEX fetch fails")
	}
}

func TestAnchorDay(t *testing.T) {
	imoexTrading := Close{Date: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)}
	anchor, ok := AnchorDay(imoexTrading, time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC), testLoc)
	if !ok || anchor != "2026-09-18" {
		t.Fatalf("AnchorDay = %q, %v; want 2026-09-18, true", anchor, ok)
	}
	if _, ok := AnchorDay(imoexTrading, time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC), testLoc); ok {
		t.Fatal("AnchorDay should be false on the next calendar day")
	}
	if _, ok := AnchorDay(Close{}, time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC), testLoc); ok {
		t.Fatal("AnchorDay should be false without an IMOEX candle")
	}
}

func TestNextFire(t *testing.T) {
	fire := 19*time.Hour + 5*time.Minute
	cases := []struct {
		now  time.Time
		want time.Time
	}{
		{
			time.Date(2026, 9, 18, 18, 0, 0, 0, testLoc),
			time.Date(2026, 9, 18, 19, 5, 0, 0, testLoc),
		},
		{
			time.Date(2026, 9, 18, 19, 5, 0, 0, testLoc),
			time.Date(2026, 9, 19, 19, 5, 0, 0, testLoc),
		},
		{
			time.Date(2026, 9, 18, 23, 59, 0, 0, testLoc),
			time.Date(2026, 9, 19, 19, 5, 0, 0, testLoc),
		},
	}
	for _, tc := range cases {
		got := NextFire(tc.now, fire, testLoc)
		if !got.Equal(tc.want) {
			t.Fatalf("NextFire(%v) = %v, want %v", tc.now, got, tc.want)
		}
	}
}

func TestMarketDirection(t *testing.T) {
	d := func(day int) time.Time {
		return time.Date(2026, 9, day, 10, 0, 0, 0, time.UTC)
	}
	cs := closes(map[string]Close{
		"IMOEX": {Close: dec(t, "123.03"), PrevClose: dec(t, "122"), Date: d(18)},
		"SBER":  {Close: dec(t, "309"), PrevClose: dec(t, "300"), Date: d(18)},
		"GAZP":  {Close: dec(t, "100"), PrevClose: dec(t, "110"), Date: d(18)},
		"LKOH":  {Close: dec(t, "50"), PrevClose: dec(t, "50"), Date: d(18)},
		"VKCO":  {Close: dec(t, "10"), PrevClose: dec(t, "9"), Date: d(17)}, // stale candle: no data today
	})
	m := MarketDirection(cs["IMOEX"], cs, []string{"SBER", "GAZP", "LKOH", "VKCO"}, "2026-09-18")
	if !m.IndexPresent {
		t.Fatal("IndexPresent = false, want true")
	}
	if m.IndexPct.Round(2).String() != dec(t, "0.84").String() {
		t.Fatalf("IndexPct = %s, want ~0.84", m.IndexPct)
	}
	if m.UpTickers != 1 || m.TotalTickers != 3 {
		t.Fatalf("green = %d/%d, want 1/3", m.UpTickers, m.TotalTickers)
	}
}

func TestMessage(t *testing.T) {
	bot := BotDay{
		Trades:      1,
		Realized:    dec(t, "9.50"),
		DayStartMTM: dec(t, "-0.50"),
		EndMTM:      dec(t, "9.75"),
	}
	market := Market{IndexPresent: true, IndexPct: dec(t, "0.84"), UpTickers: 3, TotalTickers: 4}
	msg := Message(time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC), bot, market, testLoc)

	for _, want := range []string{
		"📊 Дневная сводка · 18.09.2026",
		"Рынок: IMOEX +0.84%",
		"в плюсе 3/4 (75%)",
		"Бот: сделок 1",
		"день +19.75 ₽",
		"реализ. +9.50",
		"MTM +10.25₽",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestMessageNoMarketData(t *testing.T) {
	msg := Message(time.Date(2026, 9, 18, 19, 5, 0, 0, time.UTC), BotDay{}, Market{}, testLoc)
	if !strings.Contains(msg, "Рынок: нет данных") {
		t.Fatalf("expected no-data market line:\n%s", msg)
	}
	if !strings.Contains(msg, "Бот: сделок 0 · день +0.00 ₽") {
		t.Fatalf("expected empty bot line:\n%s", msg)
	}
}

func TestMoneyFormatting(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0", "+0.00"},
		{"1234.5", "+1 234.50"},
		{"-1234.56", "-1 234.56"},
		{"999", "+999.00"},
		{"1234567.89", "+1 234 567.89"},
		{"0.844262", "+0.84"},
	}
	for _, tc := range cases {
		got := money(dec(t, tc.in))
		if got != tc.want {
			t.Fatalf("money(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
