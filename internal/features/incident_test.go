package features

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

func TestDetectIncident(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"Дата-центр «Яндекса» был атакован БПЛА", true},
		{"«Яндекс» сообщил об остановке работы дата-центра в Рязанской области", true},
		{"Часть сервисов \"Яндекса\" работает с перебоями из-за пожара в ЦОДе под Рязанью", true},
		{"Склад Ozon в Саратове подвергся атаке БПЛА, начался пожар", true},
		{"Цена акций Ozon на Мосбирже упала на 5% после атак БПЛА", true},
		{"Украинские БПЛА атаковали компрессорную станцию «Краснодарская»", true},
		{"Россияне чаще страхуют жильё от последствий атак беспилотников", false},
		{"Украина атаковала автобус с пассажирами в Запорожской области", false},
		{"МТС Банк разработал подход для борьбы с кибератаками", false},
		{"Яндекс объявил о запуске нового дата-центра", false},
		{"Российский экспорт нефти вырос", false},
	}
	for _, c := range cases {
		if got := DetectIncident(c.title); got != c.want {
			t.Errorf("DetectIncident(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestCountIncidentSourcesDistinctAndWindow(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	mk := func(title, source string, age time.Duration, weight int64) news.MatchedArticle {
		return news.MatchedArticle{Title: title, SourceName: source, PublishedAt: now.Add(-age), TrustWeight: decimal.NewFromInt(weight)}
	}
	articles := []news.MatchedArticle{
		mk("Дата-центр «Яндекса» атакован БПЛА", "Finam", time.Hour, 1),
		mk("Дата-центр «Яндекса» атакован БПЛА", "Finam", 2*time.Hour, 1),
		mk("Пожар в ЦОДе «Яндекса»", "Google News: YDEX", 3*time.Hour, 1),
		mk("Пожар в ЦОДе «Яндекса»", "Google News: YDEX 2", 3*time.Hour, 1),
		mk("Пожар в ЦОДе «Яндекса»", "Старая лента", 72*time.Hour, 1),
		mk("Пожар в ЦОДе «Яндекса»", "Нулевое доверие", time.Hour, 0),
		mk("Яндекс повысил цены", "Ведомости", time.Hour, 1),
	}
	if got := CountIncidentSources(articles, now, IncidentWindow); got != 2 {
		t.Fatalf("CountIncidentSources = %d, want 2 (Finam + Google News)", got)
	}
}

func TestCountIncidentSourcesCollapsesPublisherFamilies(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	mk := func(source string) news.MatchedArticle {
		return news.MatchedArticle{Title: "Пожар на НПЗ", SourceName: source, PublishedAt: now.Add(-time.Hour), TrustWeight: decimal.NewFromInt(1)}
	}
	same := []news.MatchedArticle{mk("Ведомости"), mk("Ведомости.Финансы"), mk("Ведомости.Бизнес"), mk("ЦБ РФ: Новости"), mk("ЦБ РФ: Пресс-релизы")}
	if got := CountIncidentSources(same, now, IncidentWindow); got != 2 {
		t.Fatalf("one publisher family must count once: got %d, want 2 (Ведомости + ЦБ РФ)", got)
	}
	mixed := append(same, mk("MarketTwits (TG)"), mk("Банкста (TG)"))
	if got := CountIncidentSources(mixed, now, IncidentWindow); got != 4 {
		t.Fatalf("independent channels must stay distinct: got %d, want 4", got)
	}
}
