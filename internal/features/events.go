package features

import (
	"regexp"
	"strings"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

type EventFlags struct {
	Dividend     int
	Buyback      int
	Sanctions    int
	IPO          int
	Report       int
	Delisting    int
	MNA          int
	Default      int
	Negotiations int
}

func (f EventFlags) Vector() []float64 {
	return []float64{
		float64(f.Dividend),
		float64(f.Buyback),
		float64(f.Sanctions),
		float64(f.IPO),
		float64(f.Report),
		float64(f.Delisting),
		float64(f.MNA),
		float64(f.Default),
	}
}

var (
	evDividend     = regexp.MustCompile(`(?i)(?:дивиденд\w*|денежн\s*выплат\w*)`)
	evBuyback      = regexp.MustCompile(`(?i)(?:buyback|выкуп\s*акц\w*|обратн\w*\s*выкуп\w*)`)
	evSanctions    = regexp.MustCompile(`(?i)(?:санкц\w*|ограничен\w*\s*торг|блокиров\w*)`)
	evIPO          = regexp.MustCompile(`(?i)(?:\bipo\b|допэмисс\w*|размещ\w*\s*акц|спо\b|\bspo\b)`)
	evReport       = regexp.MustCompile(`(?i)(?:квартальн\w*\s*(?:результат|отчет)|отчетн\w*|выручк\w*|ebitda|финанс\w*\s*результат|результат\w*\s*за\s*(?:квартал|полугодие|год))`)
	evDelist       = regexp.MustCompile(`(?i)(?:делистинг\w*|исключ\w*\s*из|приостанов\w*\s*(?:торг|обращ)|прекращ\w*\s*(?:торг|обращ))`)
	evMNA          = regexp.MustCompile(`(?i)(?:слиян\w*|поглощ\w*|аквизиц\w*|приобрет\w*\s*(?:акц|дол\w*)|сделк\w*\s*(?:по\s*)?(?:покупк|продаж|слиян)|получ\w*\s*разреш\w*\s*на\s*сделк)`)
	evDefault      = regexp.MustCompile(`(?i)(?:дефолт\w*|банкрот\w*|неисполнен\w*\s*обязательств|просрочк\w*\s*(?:по\s*)?(?:обязательств|долг))`)
	evNegotiations = regexp.MustCompile(`(?i)(?:переговор[а-яё]*|уиткофф[а-яё]*|witkoff\w*|мирн[а-яё]*\s*(?:план|соглашен[а-яё]*)|прекращен[а-яё]*\s*огня|урегулирован[а-яё]*)`)
)

func DetectEvents(text string) EventFlags {
	var f EventFlags
	low := strings.ToLower(text)
	if evDividend.MatchString(low) {
		f.Dividend = 1
	}
	if evBuyback.MatchString(low) {
		f.Buyback = 1
	}
	if evSanctions.MatchString(low) {
		f.Sanctions = 1
	}
	if evIPO.MatchString(low) {
		f.IPO = 1
	}
	if evReport.MatchString(low) {
		f.Report = 1
	}
	if evDelist.MatchString(low) {
		f.Delisting = 1
	}
	if evMNA.MatchString(low) {
		f.MNA = 1
	}
	if evDefault.MatchString(low) {
		f.Default = 1
	}
	if evNegotiations.MatchString(low) {
		f.Negotiations = 1
	}
	return f
}

func AggregateEvents(articles []news.MatchedArticle) EventFlags {
	var out EventFlags
	for _, a := range articles {
		if a.TrustWeight.Sign() <= 0 {
			continue
		}
		flags := DetectEvents(a.Title + " " + a.Description)
		out.Dividend += flags.Dividend
		out.Buyback += flags.Buyback
		out.Sanctions += flags.Sanctions
		out.IPO += flags.IPO
		out.Report += flags.Report
		out.Delisting += flags.Delisting
		out.MNA += flags.MNA
		out.Default += flags.Default
		out.Negotiations += flags.Negotiations
	}
	return out
}
