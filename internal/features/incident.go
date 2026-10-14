package features

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

const (
	IncidentWindow     = 48 * time.Hour
	IncidentEpisodeGap = 12 * time.Hour
	IncidentMaxAge     = 6 * time.Hour
)

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

type IncidentSummary struct {
	Sources int
	First   time.Time
	Latest  time.Time
}

func CountIncidentSources(articles []news.MatchedArticle, asOf time.Time, window time.Duration) int {
	return SummarizeIncidents(articles, asOf, window).Sources
}

func SummarizeIncidents(articles []news.MatchedArticle, asOf time.Time, window time.Duration) IncidentSummary {
	var summary IncidentSummary
	var times []time.Time
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
		if a.Indirect {
			continue
		}
		if !DetectIncident(a.Title) {
			continue
		}
		sources[incidentSourceFamily(a.SourceName)] = struct{}{}
		if !a.PublishedAt.IsZero() {
			times = append(times, a.PublishedAt)
		}
	}
	summary.Sources = len(sources)
	if len(times) > 0 {
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		summary.First = times[0]
		for i := 1; i < len(times); i++ {
			if times[i].Sub(times[i-1]) >= IncidentEpisodeGap {
				summary.First = times[i]
			}
		}
		summary.Latest = times[len(times)-1]
	}
	return summary
}

var incidentSourceFamilies = []string{"google news", "ведомости", "цб рф"}

func incidentSourceFamily(sourceName string) string {
	name := strings.ToLower(strings.TrimSpace(sourceName))
	for _, family := range incidentSourceFamilies {
		if strings.HasPrefix(name, family) {
			return family
		}
	}
	return name
}
