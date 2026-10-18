package news

import (
	"reflect"
	"testing"
)

func testUniverse() *Universe {
	return NewUniverse(UniverseConfig{
		Names: []TickerName{
			{Ticker: "SBER", ShortName: "Сбербанк", SecName: "Сбербанк России ПАО ао", LatName: "Sberbank"},
			{Ticker: "SBERP", ShortName: "Сбербанк-п", SecName: "Сбербанк России ПАО ап", LatName: "Sberbank (pref)"},
			{Ticker: "GMKN", ShortName: "ГМКНорНик", SecName: "ГМК \"Нор.Никель\" ПАО ао", LatName: "NorNickel GMK"},
			{Ticker: "MGNT", ShortName: "Магнит ао", SecName: "\"Магнит\" ПАО ао", LatName: "Magnit"},
			{Ticker: "MTSS", ShortName: "МТС-ао", SecName: "Мобильные ТелеСистемы ПАО ао", LatName: "MTS"},
			{Ticker: "AFKS", ShortName: "Система ао", SecName: "АФК \"Система\" ПАО ао", LatName: "AFK Sistema"},
			{Ticker: "ROSN", ShortName: "Роснефть", SecName: "Роснефть ПАО ао", LatName: "Rosneft"},
			{Ticker: "DATA", ShortName: "Аренадата", SecName: "Аренадата ПАО", LatName: "Arenadata"},
		},
		Extra: map[string][]string{"GMKN": {"норильский никель", "норникель"}},
	})
}

func tickersOf(hits []Hit) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h.Ticker)
	}
	return out
}

func TestUniverseMatchesInflectionsAndPairs(t *testing.T) {
	u := testUniverse()
	cases := []struct {
		text string
		want []string
	}{
		{"Прибыль Сбербанка выросла на 12%", []string{"SBER", "SBERP"}},
		{"Роснефть и Норильский никель обсудили поставки", []string{"GMKN", "ROSN"}},
		{"Акции Норникеля упали", []string{"GMKN"}},
		{"Магнита ждёт рост", []string{"MGNT"}},
		{"МТС запускает сеть", []string{"MTSS"}},
		{"Курс Sberbank на бирже", []string{"SBER", "SBERP"}},
		{"Покупаем #ROSN и $SBER", []string{"ROSN", "SBER"}},
	}
	for _, tc := range cases {
		if got := tickersOf(u.Match(tc.text)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestUniverseAvoidsCommonFalsePositives(t *testing.T) {
	u := testUniverse()
	for _, text := range []string{
		"Магнитное поле Земли ослабло",
		"Выбор системы налогообложения для малого бизнеса",
		"Новая система оплаты проезда",
		"Мобильные приложения и операторы связи",
		"Data science в банках",
		"Индекс MOEX вырос",
	} {
		if got := tickersOf(u.Match(text)); len(got) != 0 {
			t.Errorf("%q: unexpected tickers %v", text, got)
		}
	}
}

func TestUniverseDropsAmbiguousAliasAcrossCompanies(t *testing.T) {
	u := NewUniverse(UniverseConfig{Names: []TickerName{
		{Ticker: "MSNG", ShortName: "Мосэнерго", SecName: "Мосэнерго ПАО ао"},
		{Ticker: "MSRS", ShortName: "Мосэнерго", SecName: "Другое ПАО ао"},
	}})
	if got := tickersOf(u.Match("Мосэнерго повысило тариф")); len(got) != 0 {
		t.Fatalf("ambiguous alias must not match, got %v", got)
	}
}

func TestUniverseBlockedAliasAndCounts(t *testing.T) {
	u := NewUniverse(UniverseConfig{
		Names:   []TickerName{{Ticker: "ROSN", ShortName: "Роснефть", SecName: "Роснефть ПАО ао"}},
		Blocked: map[string]bool{"роснефть": true},
	})
	if len(u.Match("Роснефть отчиталась")) != 0 {
		t.Fatal("blocked alias must not match")
	}
	if len(testUniverse().Tickers()) != 8 || testUniverse().AliasCount() == 0 {
		t.Fatal("unexpected ticker or alias count")
	}
}
