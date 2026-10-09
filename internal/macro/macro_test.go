package macro

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func date(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func ramp(start string, n int, first, step float64) Series {
	var out Series
	d := date(start)
	for len(out) < n {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			out = append(out, Point{Date: d, Value: first + step*float64(len(out))})
		}
		d = d.AddDate(0, 0, 1)
	}
	return out
}

func flat(start string, days int, value float64) Series {
	var out Series
	d := date(start)
	for i := 0; i < days; i++ {
		out = append(out, Point{Date: d.AddDate(0, 0, i), Value: value})
	}
	return out
}

func TestProviderUsesOnlyDaysBeforeDecisionDay(t *testing.T) {
	imoex := ramp("2026-01-01", 120, 2000, 1)
	provider := NewProvider(imoex, ramp("2026-01-01", 120, 80, 0.1), ramp("2026-01-01", 120, 90, 0.2), flat("2025-06-01", 400, 16))

	day := imoex[100].Date
	got, err := provider.At(day, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := (imoex[99].Value/imoex[94].Value - 1) * 100
	if math.Abs(got.ImoexRet5d-want) > 1e-9 {
		t.Fatalf("imoex_ret_5d = %v, want %v (must end at the close before the decision day)", got.ImoexRet5d, want)
	}

	future := imoex[:100:100]
	future = append(future, Point{Date: day, Value: 9999})
	leaky := NewProvider(future, ramp("2026-01-01", 120, 80, 0.1), ramp("2026-01-01", 120, 90, 0.2), flat("2025-06-01", 400, 16))
	same, err := leaky.At(day, 0)
	if err != nil {
		t.Fatal(err)
	}
	if same.ImoexRet5d != got.ImoexRet5d {
		t.Fatalf("a point dated on the decision day changed the feature: %v vs %v", same.ImoexRet5d, got.ImoexRet5d)
	}
}

func TestProviderRejectsStaleAndShortHistory(t *testing.T) {
	imoex := ramp("2026-01-01", 120, 2000, 1)
	provider := NewProvider(imoex, ramp("2026-01-01", 120, 80, 0.1), ramp("2026-01-01", 120, 90, 0.2), flat("2025-06-01", 400, 16))

	if _, err := provider.At(imoex[119].Date.AddDate(0, 0, 30), 5*24*time.Hour); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("want stale error, got %v", err)
	}
	if _, err := provider.At(imoex[10].Date, 0); err == nil {
		t.Fatalf("want short history error")
	}
}

func TestKeyRateChange(t *testing.T) {
	var key Series
	for d := date("2026-01-01"); d.Before(date("2026-06-30")); d = d.AddDate(0, 0, 1) {
		rate := 16.0
		if !d.Before(date("2026-05-01")) {
			rate = 14
		}
		key = append(key, Point{Date: d, Value: rate})
	}
	provider := NewProvider(ramp("2026-01-01", 150, 2000, 1), ramp("2026-01-01", 150, 80, 0.1), ramp("2026-01-01", 150, 90, 0.2), key)
	got, err := provider.At(date("2026-06-15"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeyRatePct != 14 || got.KeyRateChg90d != -2 {
		t.Fatalf("key rate = %v chg90d = %v, want 14 and -2", got.KeyRatePct, got.KeyRateChg90d)
	}
}

func TestChainFrontIgnoresRollGap(t *testing.T) {
	near := Series{}
	far := Series{}
	d := date("2026-03-02")
	for i := 0; i < 20; i++ {
		day := d.AddDate(0, 0, i)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		if i < 10 {
			near = append(near, Point{Date: day, Value: 100 + float64(i)})
		}
		far = append(far, Point{Date: day, Value: 130 + float64(i)})
	}
	chained := ChainFront(map[string]Series{"BRH6": near, "BRJ6": far}, 2*24*time.Hour)

	for i := 1; i < len(chained); i++ {
		step := chained[i].Value/chained[i-1].Value - 1
		if math.Abs(step) > 0.05 {
			t.Fatalf("roll leaked a %.1f%% jump on %s", step*100, chained[i].Date.Format("2006-01-02"))
		}
	}
}

func TestCSVRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.csv")
	in := ramp("2026-01-01", 5, 1.5, 0.25)
	if err := SaveCSV(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) || out[4].Value != in[4].Value || !out[4].Date.Equal(in[4].Date) {
		t.Fatalf("round trip mismatch: %v vs %v", out, in)
	}
}

func TestFetcherPagesISSAndParsesKeyRate(t *testing.T) {
	const total = 230
	mux := http.NewServeMux()
	mux.HandleFunc("/iss/history/engines/stock/markets/index/securities/IMOEX.json", func(w http.ResponseWriter, r *http.Request) {
		start := 0
		fmt.Sscanf(r.URL.Query().Get("start"), "%d", &start)
		rows := []string{}
		for i := start; i < start+100 && i < total; i++ {
			day := date("2025-01-01").AddDate(0, 0, i).Format("2006-01-02")
			closeValue := "2000.5"
			if i == 3 {
				closeValue = "null"
			}
			rows = append(rows, fmt.Sprintf(`["%s",%s]`, day, closeValue))
		}
		fmt.Fprintf(w, `{"history":{"columns":["TRADEDATE","CLOSE"],"data":[%s]}}`, strings.Join(rows, ","))
	})
	mux.HandleFunc("/hd_base/KeyRate/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<table class="data"><tr><th>Дата</th><th>Ставка</th></tr>
<tr><td>09.10.2026</td><td>14,00</td></tr>
<tr><td>08.10.2026</td><td>14,50</td></tr></table>`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := &Fetcher{ISSBase: server.URL + "/iss", CBRBase: server.URL, HTTP: server.Client()}
	imoex, err := fetcher.IMOEX(context.Background(), date("2025-01-01"), date("2025-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if len(imoex) != total-1 {
		t.Fatalf("got %d points, want %d (null close skipped, 3 pages)", len(imoex), total-1)
	}
	key, err := fetcher.KeyRate(context.Background(), date("2026-10-01"), date("2026-10-10"))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 2 || key[0].Value != 14.5 || key[1].Value != 14 {
		t.Fatalf("key rate parsed wrong: %v", key)
	}
}
