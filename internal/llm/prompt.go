package llm

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const StrictJSONInstruction = "Отвечай ТОЛЬКО валидным JSON, без markdown-оберток и пояснений"

type PromptBuilder struct {
	schemaJSON string
	examples   []fewShotExample
}

type fewShotExample struct {
	Input  map[string]any `json:"input"`
	Output map[string]any `json:"output"`
}

func NewPromptBuilder() *PromptBuilder {
	schemaBytes, err := json.Marshal(TradeSignalSchema())
	if err != nil {
		panic(err)
	}
	return &PromptBuilder{
		schemaJSON: string(schemaBytes),
		examples:   fewShotExamples(),
	}
}

func (b *PromptBuilder) SystemPrompt(ctx domain.FeatureContext) string {
	contextJSON, _ := json.Marshal(featureContextMap(ctx))
	examplesJSON, _ := json.Marshal(b.examples)

	var sb strings.Builder
	sb.WriteString("Ты — агент принятия торговых решений для инструментов MOEX.\n")
	sb.WriteString("На основе признаков инструмента выбери действие BUY, SELL или HOLD и верни ровно один JSON-объект, соответствующий схеме.\n\n")
	sb.WriteString(StrictJSONInstruction)
	sb.WriteString("\n\nПризнаки инструмента:\n")
	sb.WriteString(string(contextJSON))
	sb.WriteString("\n\nJSON-схема ответа:\n")
	sb.WriteString(b.schemaJSON)
	sb.WriteString("\n\nПримеры (вход -> выход):\n")
	sb.WriteString(string(examplesJSON))
	sb.WriteString("\n\nВерни только один JSON-объект для текущего инструмента.")
	return sb.String()
}

func featureContextMap(ctx domain.FeatureContext) map[string]any {
	return map[string]any{
		"ticker":               ctx.Ticker,
		"generated_at":         ctx.GeneratedAt.Format(time.RFC3339),
		"last_price":           json.Number(ctx.LastPrice.String()),
		"prev_close":           json.Number(ctx.PrevClose.String()),
		"bid":                  json.Number(ctx.Bid.String()),
		"ask":                  json.Number(ctx.Ask.String()),
		"return_pct":           json.Number(ctx.ReturnPct.String()),
		"realized_volatility":  json.Number(ctx.RealizedVolatility.String()),
		"news_sentiment":       json.Number(ctx.NewsSentiment.String()),
		"news_count":           ctx.NewsCount,
		"order_book_imbalance": json.Number(ctx.OrderBookImbalance.String()),
	}
}

func tradeSignalMap(signal domain.TradeSignal) map[string]any {
	return map[string]any{
		"ticker":       signal.Ticker,
		"action":       string(signal.Action),
		"confidence":   json.Number(signal.Confidence.String()),
		"target_lots":  signal.TargetLots,
		"reasoning":    signal.Reasoning,
		"generated_at": signal.GeneratedAt.Format(time.RFC3339),
	}
}

func fewShotExamples() []fewShotExample {
	return []fewShotExample{
		{
			Input: featureContextMap(sampleContext("SBER", "310.5", "305.2", "1.74", "0.9", "0.7", 4)),
			Output: tradeSignalMap(domain.TradeSignal{
				Ticker:      "SBER",
				Action:      domain.ActionBuy,
				Confidence:  mustDecimal("0.8"),
				TargetLots:  1,
				Reasoning:   "Позитивная новостная лента и рост цены",
				GeneratedAt: sampleTime(),
			}),
		},
		{
			Input: featureContextMap(sampleContext("GAZP", "128.4", "131.0", "-1.98", "1.4", "-0.6", 5)),
			Output: tradeSignalMap(domain.TradeSignal{
				Ticker:      "GAZP",
				Action:      domain.ActionSell,
				Confidence:  mustDecimal("0.7"),
				TargetLots:  1,
				Reasoning:   "Негативный сентимент и падение ниже предыдущего закрытия",
				GeneratedAt: sampleTime(),
			}),
		},
		{
			Input: featureContextMap(sampleContext("OZON", "4210.0", "4198.0", "0.29", "0.6", "0.0", 0)),
			Output: tradeSignalMap(domain.TradeSignal{
				Ticker:      "OZON",
				Action:      domain.ActionHold,
				Confidence:  mustDecimal("0.6"),
				TargetLots:  0,
				Reasoning:   "Слабый сигнал, новостей нет, цена почти не изменилась",
				GeneratedAt: sampleTime(),
			}),
		},
		{
			Input: featureContextMap(sampleContext("LKOH", "6420.5", "6350.0", "1.11", "0.8", "0.4", 2)),
			Output: tradeSignalMap(domain.TradeSignal{
				Ticker:      "LKOH",
				Action:      domain.ActionBuy,
				Confidence:  mustDecimal("0.65"),
				TargetLots:  1,
				Reasoning:   "Умеренно позитивный сентимент при восходящей динамике",
				GeneratedAt: sampleTime(),
			}),
		},
		{
			Input: featureContextMap(sampleContext("YDEX", "4021.0", "4080.0", "-1.45", "1.1", "-0.3", 1)),
			Output: tradeSignalMap(domain.TradeSignal{
				Ticker:      "YDEX",
				Action:      domain.ActionSell,
				Confidence:  mustDecimal("0.55"),
				TargetLots:  1,
				Reasoning:   "Небольшой негативный перекос при снижении цены",
				GeneratedAt: sampleTime(),
			}),
		},
	}
}

func sampleContext(ticker, lastPrice, prevClose, returnPct, volatility, sentiment string, newsCount int) domain.FeatureContext {
	return domain.FeatureContext{
		Ticker:             ticker,
		GeneratedAt:        sampleTime(),
		LastPrice:          mustDecimal(lastPrice),
		PrevClose:          mustDecimal(prevClose),
		ReturnPct:          mustDecimal(returnPct),
		RealizedVolatility: mustDecimal(volatility),
		NewsSentiment:      mustDecimal(sentiment),
		NewsCount:          newsCount,
	}
}

func sampleTime() time.Time {
	return time.Date(2026, 9, 14, 10, 30, 0, 0, time.UTC)
}

func mustDecimal(value string) decimal.Decimal {
	return decimal.RequireFromString(value)
}
