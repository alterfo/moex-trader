package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newAlertFixture(t *testing.T, cfg WatchConfig) (*Watch, *[]string, *time.Time) {
	t.Helper()

	var messages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
			return
		}
		messages = append(messages, r.FormValue("text"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	client := newClient(server.URL, "token-123", "chat-456", server.Client())
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return now }
	}
	return NewWatch(client, cfg), &messages, &now
}

func TestProbabilityCollapseAlertsAndCooldown(t *testing.T) {
	ctx := context.Background()
	watch, messages, now := newAlertFixture(t, WatchConfig{
		ProbabilityWindow: 4,
		CollapseFraction:  0.9,
		Cooldown:          time.Hour,
	})

	for i := 0; i < 4; i++ {
		alerted, err := watch.ObserveProbability(ctx, 0.5)
		if err != nil {
			t.Fatalf("ObserveProbability() error = %v", err)
		}
		if i < 3 && alerted {
			t.Fatalf("ObserveProbability() alerted before window filled")
		}
		if i == 3 && !alerted {
			t.Fatalf("ObserveProbability() did not alert on collapse")
		}
	}
	if len(*messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(*messages))
	}

	for i := 0; i < 4; i++ {
		alerted, err := watch.ObserveProbability(ctx, 0.5)
		if err != nil {
			t.Fatalf("ObserveProbability() error = %v", err)
		}
		if alerted {
			t.Fatalf("ObserveProbability() alerted inside cooldown")
		}
	}
	if len(*messages) != 1 {
		t.Fatalf("messages = %d, want 1 after cooldown suppression", len(*messages))
	}

	*now = now.Add(time.Hour)
	alerted, err := watch.ObserveProbability(ctx, 0.5)
	if err != nil {
		t.Fatalf("ObserveProbability() error = %v", err)
	}
	if !alerted {
		t.Fatal("ObserveProbability() did not alert after cooldown elapsed")
	}
	if len(*messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(*messages))
	}
}

func TestProbabilityCollapseNoAlertWhenMixed(t *testing.T) {
	ctx := context.Background()
	watch, messages, _ := newAlertFixture(t, WatchConfig{
		ProbabilityWindow: 4,
		CollapseFraction:  0.9,
		Cooldown:          time.Hour,
	})

	for _, probability := range []float64{0.5, 0.8, 0.5, 0.9} {
		alerted, err := watch.ObserveProbability(ctx, probability)
		if err != nil {
			t.Fatalf("ObserveProbability() error = %v", err)
		}
		if alerted {
			t.Fatalf("ObserveProbability() alerted for mixed probabilities")
		}
	}
	if len(*messages) != 0 {
		t.Fatalf("messages = %d, want 0", len(*messages))
	}
}

func TestStaleCandleAlertsInSessionAndCooldown(t *testing.T) {
	ctx := context.Background()
	watch, messages, now := newAlertFixture(t, WatchConfig{
		StaleCandleAge: time.Hour,
		Cooldown:       time.Hour,
	})

	alerted, err := watch.ObserveCandleAge(ctx, "SBER", 2*time.Hour, true)
	if err != nil {
		t.Fatalf("ObserveCandleAge() error = %v", err)
	}
	if !alerted {
		t.Fatal("ObserveCandleAge() did not alert for a stale candle")
	}
	if len(*messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(*messages))
	}

	alerted, err = watch.ObserveCandleAge(ctx, "SBER", 2*time.Hour, true)
	if err != nil {
		t.Fatalf("ObserveCandleAge() error = %v", err)
	}
	if alerted {
		t.Fatal("ObserveCandleAge() alerted inside cooldown")
	}

	*now = now.Add(time.Hour)
	alerted, err = watch.ObserveCandleAge(ctx, "SBER", 2*time.Hour, true)
	if err != nil {
		t.Fatalf("ObserveCandleAge() error = %v", err)
	}
	if !alerted {
		t.Fatal("ObserveCandleAge() did not alert after cooldown elapsed")
	}
	if len(*messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(*messages))
	}
}

func TestStaleCandleSilentOutsideSession(t *testing.T) {
	ctx := context.Background()
	watch, messages, now := newAlertFixture(t, WatchConfig{
		StaleCandleAge: time.Hour,
		Cooldown:       time.Hour,
	})

	alerted, err := watch.ObserveCandleAge(ctx, "SBER", 2*time.Hour, false)
	if err != nil {
		t.Fatalf("ObserveCandleAge() error = %v", err)
	}
	if alerted {
		t.Fatal("ObserveCandleAge() alerted outside the trading session")
	}
	_ = now
	if len(*messages) != 0 {
		t.Fatalf("messages = %d, want 0", len(*messages))
	}
}

func TestPSIBreachAlertsAndCooldown(t *testing.T) {
	ctx := context.Background()
	watch, messages, now := newAlertFixture(t, WatchConfig{
		Cooldown: time.Hour,
	})

	alerted, err := watch.ObservePSI(ctx, "mom_5d", 0.31)
	if err != nil {
		t.Fatalf("ObservePSI() error = %v", err)
	}
	if !alerted {
		t.Fatal("ObservePSI() did not alert on breach")
	}

	alerted, err = watch.ObservePSI(ctx, "mom_5d", 0.32)
	if err != nil {
		t.Fatalf("ObservePSI() error = %v", err)
	}
	if alerted {
		t.Fatal("ObservePSI() alerted inside cooldown")
	}

	*now = now.Add(time.Hour)
	alerted, err = watch.ObservePSI(ctx, "mom_5d", 0.33)
	if err != nil {
		t.Fatalf("ObservePSI() error = %v", err)
	}
	if !alerted {
		t.Fatal("ObservePSI() did not alert after cooldown elapsed")
	}
	if len(*messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(*messages))
	}
}

func TestWatchDisabledClientIsNoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request made for disabled client")
	}))
	defer server.Close()

	client := newClient(server.URL, "", "", server.Client())
	watch := NewWatch(client, WatchConfig{
		ProbabilityWindow: 4,
		Cooldown:          time.Hour,
	})
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		alerted, err := watch.ObserveProbability(ctx, 0.5)
		if err != nil {
			t.Fatalf("ObserveProbability() error = %v", err)
		}
		if alerted {
			t.Fatal("ObserveProbability() alerted for disabled client")
		}
	}
	if alerted, err := watch.ObserveCandleAge(ctx, "SBER", 2*time.Hour, true); err != nil || alerted {
		t.Fatalf("ObserveCandleAge() = (%v, %v), want (false, nil)", alerted, err)
	}
	if alerted, err := watch.ObservePSI(ctx, "mom_5d", 0.3); err != nil || alerted {
		t.Fatalf("ObservePSI() = (%v, %v), want (false, nil)", alerted, err)
	}
}

func TestTradingSessionActive(t *testing.T) {
	loc := time.FixedZone("MSK", 3*3600)
	tests := []struct {
		name string
		time time.Time
		want bool
	}{
		{name: "monday before open", time: time.Date(2026, 10, 5, 9, 59, 0, 0, loc), want: false},
		{name: "monday at open", time: time.Date(2026, 10, 5, 10, 0, 0, 0, loc), want: true},
		{name: "monday before close", time: time.Date(2026, 10, 5, 18, 44, 0, 0, loc), want: true},
		{name: "monday at close", time: time.Date(2026, 10, 5, 18, 45, 0, 0, loc), want: false},
		{name: "saturday midday", time: time.Date(2026, 10, 3, 12, 0, 0, 0, loc), want: false},
		{name: "sunday midday", time: time.Date(2026, 10, 4, 12, 0, 0, 0, loc), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := TradingSessionActive(test.time, loc); got != test.want {
				t.Fatalf("TradingSessionActive(%v) = %v, want %v", test.time, got, test.want)
			}
		})
	}
}
