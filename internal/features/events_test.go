package features

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

func TestDetectEventsNegotiations(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "negotiation progress",
			text: "США и Россия начали переговоры о прекращении конфликта",
			want: true,
		},
		{
			name: "witkoff cyrillic",
			text: "Уиткофф заявил о прогрессе в переговорах",
			want: true,
		},
		{
			name: "witkoff latin",
			text: "Witkoff to visit Moscow for peace talks",
			want: true,
		},
		{
			name: "peace plan",
			text: "Стороны одобрили мирный план урегулирования",
			want: true,
		},
		{
			name: "peace agreement",
			text: "Подписано мирное соглашение",
			want: true,
		},
		{
			name: "ceasefire",
			text: "Объявлено прекращение огня на фронте",
			want: true,
		},
		{
			name: "settlement",
			text: "Урегулирование конфликта вышло на новый этап",
			want: true,
		},
		{
			name: "unrelated oil",
			text: "Цена нефти выросла на три процента",
			want: false,
		},
		{
			name: "peace treaty without plan or agreement",
			text: "Стороны подписали мирный договор",
			want: false,
		},
		{
			name: "trading halt is not ceasefire",
			text: "Мосбиржа объявила о прекращении торгов",
			want: false,
		},
		{
			name: "debt settlement is not negotiation",
			text: "Компания планирует урегулировать задолженность",
			want: false,
		},
		{
			name: "rate decision unrelated",
			text: "ЦБ повысил ключевую ставку",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectEvents(tc.text).Negotiations == 1
			if got != tc.want {
				t.Fatalf("DetectEvents(%q).Negotiations = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestAggregateEventsSumsNegotiations(t *testing.T) {
	articles := []news.MatchedArticle{
		{Title: "Уиткофф сообщил о прогрессе", TrustWeight: decimal.NewFromInt(1)},
		{Title: "Цена нефти выросла", TrustWeight: decimal.NewFromInt(1)},
		{Title: "Стороны обсудили прекращение огня", TrustWeight: decimal.NewFromInt(1)},
	}
	got := AggregateEvents(articles)
	if got.Negotiations != 2 {
		t.Fatalf("AggregateEvents negotiations = %d, want 2", got.Negotiations)
	}
}

func TestDetectEventsSanctionsTariffEnforcement(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "sanctions",
			text: "США ввели новые санкции против банков",
			want: true,
		},
		{
			name: "tariff against oil buyers",
			text: "Трамп ввел 100% пошлины на покупателей российской нефти",
			want: true,
		},
		{
			name: "oil price move is not tariff enforcement",
			text: "Цена нефти выросла на три процента",
			want: false,
		},
		{
			name: "utility tariff is not sanctions signal",
			text: "Тарифы на электроэнергию выросли",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectEvents(tc.text).Sanctions == 1
			if got != tc.want {
				t.Fatalf("DetectEvents(%q).Sanctions = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestEventFlagsVectorIncludesNegotiations(t *testing.T) {
	got := EventFlags{
		Dividend: 1, Buyback: 1, Sanctions: 1, IPO: 1,
		Report: 1, Delisting: 1, MNA: 1, Default: 1,
		Negotiations: 1,
	}.Vector()
	if len(got) != 9 {
		t.Fatalf("Vector() length = %d, want 9", len(got))
	}
	if got[len(got)-1] != 1 {
		t.Fatalf("Vector() negotiations = %v, want 1", got[len(got)-1])
	}
}
