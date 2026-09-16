package features

import (
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

func PolarityScore(text string) decimal.Decimal {
	low := strings.ToLower(text)
	pos := countKeywords(low, positiveKeywords)
	neg := countKeywords(low, negativeKeywords)
	total := pos + neg
	if total == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(pos - neg)).Div(decimal.NewFromInt(int64(total)))
}

var (
	negationRe = regexp.MustCompile(`(?:не|без|отсутств|прекрат|отмен|сократ|сниз|потер|ущерб|убыт|штраф|санкц|падени|обвал|авари|кризис|дефолт)`)
	stRe       = regexp.MustCompile(`(?:значительн|серьезн|масштабн|рекордн|кардинальн|резк|стремительн|взрывн|колоссальн)`)
	strongPos  = regexp.MustCompile(`(?:дивиденд|рекорд|прибыл|рост\s+\d|\+\d|повыш|превыш|прогноз|улучш|одобр|сделк|партнер|контракт|заказ|капитал)`)
	strongNeg  = regexp.MustCompile(`(?:убыт|штраф|санкц|дефолт|обвал|авари|кризис|расслед|иск|судеб|банкрот|ликвид|отток|сниж|сократ|проблем)`)
	veryNeg    = regexp.MustCompile(`(?:обвал|катастроф|банкрот|дефолт|конфиск|замороз|блоки|отобр)`)
	veryPos    = regexp.MustCompile(`(?:рекорд|триумф|взрывн|колоссальн|масштабн)`)
)

func RegexScore(text string) decimal.Decimal {
	low := strings.ToLower(text)
	negated := negationRe.MatchString(low)
	strong := stRe.MatchString(low)
	vn := veryNeg.MatchString(low)
	vp := veryPos.MatchString(low)
	hasStrongPos := strongPos.MatchString(low)
	hasStrongNeg := strongNeg.MatchString(low)
	switch {
	case vn:
		return decimal.NewFromFloat(-0.9)
	case vp:
		return decimal.NewFromFloat(0.9)
	case negated && hasStrongPos:
		return decimal.NewFromFloat(-0.6)
	case negated && hasStrongNeg:
		return decimal.NewFromFloat(0.5)
	case hasStrongNeg && strong:
		return decimal.NewFromFloat(-0.8)
	case hasStrongPos && strong:
		return decimal.NewFromFloat(0.8)
	case hasStrongNeg:
		return decimal.NewFromFloat(-0.5)
	case hasStrongPos:
		return decimal.NewFromFloat(0.5)
	default:
		return decimal.Zero
	}
}

func NoneScore(_ string) decimal.Decimal { return decimal.Zero }
