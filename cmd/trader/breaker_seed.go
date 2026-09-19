package main

import (
	"log"
	"strings"

	"github.com/olegsidorkin/moex-trader/internal/dailysummary"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/risk"
)

func seedTickerBreaker(breaker *risk.TickerBreaker, events []domain.AuditEvent, logger *log.Logger) {
	if breaker == nil {
		return
	}
	if logger == nil {
		logger = log.Default()
	}
	fills := dailysummary.FillsFromAuditEvents(events)
	seen := make(map[string]struct{})
	for _, fill := range fills {
		breaker.RecordFill(risk.Fill{
			Ticker:     fill.Ticker,
			Action:     fill.Action,
			Lots:       fill.Lots,
			Price:      fill.Price,
			Commission: fill.Commission,
		})
		seen[strings.ToUpper(strings.TrimSpace(fill.Ticker))] = struct{}{}
	}
	if len(fills) == 0 {
		logger.Printf("trader: circuit breaker seeded from 0 historical fills")
		return
	}
	open := 0
	for ticker := range seen {
		lots, _, _, _, _ := breaker.State(ticker)
		if lots != 0 {
			open++
		}
	}
	logger.Printf("trader: circuit breaker seeded from %d historical fills across %d tickers, open positions: %d",
		len(fills), len(seen), open)
}
