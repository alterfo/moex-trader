package llm

func TradeSignalSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticker": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"action": map[string]any{
				"type": "string",
				"enum": []string{"BUY", "SELL", "HOLD"},
			},
			"confidence": map[string]any{
				"type":    "number",
				"minimum": 0,
				"maximum": 1,
			},
			"target_lots": map[string]any{
				"type":    "integer",
				"minimum": 0,
			},
			"reasoning": map[string]any{
				"type": "string",
			},
			"generated_at": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
		},
		"required":             []string{"ticker", "action", "confidence", "target_lots", "reasoning"},
		"additionalProperties": false,
	}
}
