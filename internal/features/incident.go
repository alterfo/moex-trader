package features

import (
	"regexp"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

const IncidentWindow = 48 * time.Hour

const incidentStart = `(?:^|[^а-яёa-z])`

var (
	incStrike   = regexp.MustCompile(incidentStart + `(?:атак|удар|налёт|налет|обстрел)`)
	incDrone    = regexp.MustCompile(incidentStart + `(?:бпла|беспилотн|дрон|ракет)`)
	incFire     = regexp.MustCompile(incidentStart + `(?:пожар|взрыв|возгоран)`)
	incStop     = regexp.MustCompile(incidentStart + `(?:останови|приостанови|остановк)`)
	incWork     = regexp.MustCompile(incidentStart + `работ`)
	incFacility = regexp.MustCompile(incidentStart + `(?:завод|дата-?центр|цод|нпз|терминал|склад|хаб|предприят|подстанц|электростанц|станци|месторожд|порт[а-яё]*(?:$|[^а-яё])|комбинат|фабрик|логистическ)`)
	incImpact   = regexp.MustCompile(incidentStart + `(?:акци|упал|останови|приостанови|перебо|сбо[йяе]|повреж|пожар|недоступн)`)
	incExclude  = regexp.MustCompile(incidentStart + `(?:страхов|полис|застрахов)`)
)

func DetectIncident(text string) bool {
	low := strings.ToLower(text)
	if incExclude.MatchString(low) {
		return false
	}
	facility := incFacility.MatchString(low)
	switch {
	case incFire.MatchString(low) && facility:
		return true
	case incStrike.MatchString(low) && incDrone.MatchString(low) && (facility || incImpact.MatchString(low)):
		return true
	case incStop.MatchString(low) && incWork.MatchString(low) && facility:
		return true
	}
	return false
}

func CountIncidentSources(articles []news.MatchedArticle, asOf time.Time, window time.Duration) int {
	sources := make(map[string]struct{})
	for _, a := range articles {
		if a.TrustWeight.Sign() <= 0 {
			continue
		}
		if !asOf.IsZero() && !a.PublishedAt.IsZero() {
			age := asOf.Sub(a.PublishedAt)
			if age > window || age < -time.Hour {
				continue
			}
		}
		if !DetectIncident(a.Title) {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(a.SourceName))
		if strings.HasPrefix(name, "google news") {
			name = "google news"
		}
		sources[name] = struct{}{}
	}
	return len(sources)
}
