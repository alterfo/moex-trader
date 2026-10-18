package main

import (
	"testing"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

func testSetup() (*news.Universe, map[string]bool) {
	names := []news.TickerName{
		{Ticker: "SBER", ShortName: "Сбербанк", SecName: "Сбербанк России ПАО ао", LatName: "Sberbank"},
		{Ticker: "OZON", ShortName: "Озон", SecName: "Озон Холдингс", LatName: "Ozon"},
	}
	return news.NewUniverse(news.UniverseConfig{Names: names}), map[string]bool{"SBER": true, "OZON": true}
}

func TestDecideRules(t *testing.T) {
	u, known := testSetup()
	cases := []struct {
		name string
		rec  record
		keep bool
		why  string
	}{
		{"feed ticker kept", record{Ticker: "CNYRUB_TOM", Source: "Google News: CNYRUB_TOM", Title: "курс юаня"}, true, "feed_ticker"},
		{"text supports ticker", record{Ticker: "SBER", Source: "MarketTwits (TG)", Title: "Сбербанк повысил прогноз"}, true, "text_match"},
		{"tag backed source kept", record{Ticker: "SBER", Source: "bcs", Title: "Ход торгов. Рынок упал"}, true, "tag_backed_source"},
		{"telegram macro news dropped", record{Ticker: "OZON", Source: "MarketTwits (TG)", Title: "Еврозона PMI = 51.6"}, false, "not_supported_by_text"},
		{"unknown ticker from telegram dropped", record{Ticker: "GLDRUB_TOM", Source: "Банкста (TG)", Title: "золото дорожает"}, false, "ticker_outside_universe"},
		{"other feed for a ticker is not trusted", record{Ticker: "SBER", Source: "Google News: OZON", Title: "погода"}, false, "not_supported_by_text"},
	}
	for _, tc := range cases {
		got := decide(tc.rec, u, known)
		if got.keep != tc.keep || got.reason != tc.why {
			t.Errorf("%s: got %+v, want keep=%v reason=%s", tc.name, got, tc.keep, tc.why)
		}
	}
}

func TestDecideUsesLearnedAliases(t *testing.T) {
	names := []news.TickerName{{Ticker: "UGLD", ShortName: "ЮГК", SecName: "ЮГК ПАО ао"}}
	known := map[string]bool{"UGLD": true}
	plain := news.NewUniverse(news.UniverseConfig{Names: names})
	withLearned := news.NewUniverse(news.UniverseConfig{Names: names, Extra: map[string][]string{"UGLD": {"Южуралзолото"}}})
	rec := record{Ticker: "UGLD", Source: "Интерфакс (TG)", Title: "Южуралзолото увеличила добычу"}
	if got := decide(rec, plain, known); got.keep {
		t.Fatalf("without the learned alias the pair is unsupported: %+v", got)
	}
	if got := decide(rec, withLearned, known); !got.keep || got.reason != "text_match" {
		t.Fatalf("with the learned alias the pair must be kept: %+v", got)
	}
}
