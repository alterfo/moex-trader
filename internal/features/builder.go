package features

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

type PriceSnapshot struct {
	LastPrice decimal.Decimal
	PrevClose decimal.Decimal
	Bid       decimal.Decimal
	Ask       decimal.Decimal
	AsOf      time.Time
}

type Input struct {
	Ticker  string
	Price   PriceSnapshot
	Candles []moex.Candle
	News    []news.MatchedArticle
}

type Builder struct {
	now func() time.Time
}

func NewBuilder(now func() time.Time) *Builder {
	if now == nil {
		now = time.Now
	}
	return &Builder{now: now}
}

func (b *Builder) Build(input Input) (domain.FeatureContext, error) {
	var empty domain.FeatureContext

	ticker := strings.TrimSpace(input.Ticker)
	if ticker == "" {
		return empty, fmt.Errorf("ticker must not be empty")
	}
	if input.Price.LastPrice.Sign() <= 0 {
		return empty, fmt.Errorf("last price must be positive")
	}
	if input.Price.PrevClose.Sign() <= 0 {
		return empty, fmt.Errorf("prev close must be positive")
	}

	newsSentiment, newsCount := aggregateNewsSentiment(input.News)

	generatedAt := b.now()
	if !input.Price.AsOf.IsZero() {
		generatedAt = input.Price.AsOf
	}

	return domain.FeatureContext{
		Ticker:             ticker,
		GeneratedAt:        generatedAt,
		LastPrice:          input.Price.LastPrice,
		PrevClose:          input.Price.PrevClose,
		Bid:                input.Price.Bid,
		Ask:                input.Price.Ask,
		ReturnPct:          percentChange(input.Price.LastPrice, input.Price.PrevClose),
		RealizedVolatility: realizedVolatility(input.Candles),
		NewsSentiment:      newsSentiment,
		NewsCount:          newsCount,
		OrderBookImbalance: decimal.Zero,
	}, nil
}

func percentChange(current, previous decimal.Decimal) decimal.Decimal {
	return current.Sub(previous).Div(previous).Mul(decimal.NewFromInt(100))
}

func realizedVolatility(candles []moex.Candle) decimal.Decimal {
	returns := make([]decimal.Decimal, 0, len(candles))
	for i := 1; i < len(candles); i++ {
		previous := candles[i-1].Close
		current := candles[i].Close
		if previous.Sign() <= 0 || current.Sign() <= 0 {
			continue
		}
		returns = append(returns, percentChange(current, previous))
	}
	if len(returns) < 2 {
		return decimal.Zero
	}

	var sum decimal.Decimal
	for _, value := range returns {
		sum = sum.Add(value)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(returns))))

	var squaredDeviation decimal.Decimal
	for _, value := range returns {
		deviation := value.Sub(mean)
		squaredDeviation = squaredDeviation.Add(deviation.Mul(deviation))
	}
	variance := squaredDeviation.Div(decimal.NewFromInt(int64(len(returns) - 1)))

	value, _ := variance.Float64()
	if value <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromFloat(math.Sqrt(value))
}

func aggregateNewsSentiment(articles []news.MatchedArticle) (decimal.Decimal, int) {
	if len(articles) == 0 {
		return decimal.Zero, 0
	}

	var weightedScore decimal.Decimal
	var totalWeight decimal.Decimal
	for _, article := range articles {
		weight := article.TrustWeight
		if weight.Sign() <= 0 {
			continue
		}
		weightedScore = weightedScore.Add(articlePolarity(article).Mul(weight))
		totalWeight = totalWeight.Add(weight)
	}
	if totalWeight.Sign() <= 0 {
		return decimal.Zero, len(articles)
	}
	return weightedScore.Div(totalWeight), len(articles)
}

func articlePolarity(article news.MatchedArticle) decimal.Decimal {
	text := strings.ToLower(article.Title + " " + article.Description)
	positive := countKeywords(text, positiveKeywords)
	negative := countKeywords(text, negativeKeywords)
	total := positive + negative
	if total == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(positive - negative)).Div(decimal.NewFromInt(int64(total)))
}

func countKeywords(text string, keywords []string) int {
	count := 0
	for _, keyword := range keywords {
		count += strings.Count(text, keyword)
	}
	return count
}

var positiveKeywords = []string{
	"рост", "вырос", "растет", "растёт", "повыш", "увелич", "прибыль",
	"рекорд", "позитив", "дивиденд", "покупа", "спрос", "улучш",
	"одобр", "сделка",
}

var negativeKeywords = []string{
	"падение", "падает", "упал", "снижение", "снизил", "убыток", "убыл",
	"риск", "санкц", "штраф", "дефолт", "негатив", "запрет", "распродаж",
	"кризис", "отток", "проблем", "авария", "судеб", "иск",
}
