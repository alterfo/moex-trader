package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/dividends"
	"github.com/olegsidorkin/moex-trader/internal/features"
	"github.com/olegsidorkin/moex-trader/internal/macro"
)

const macroRefreshInterval = time.Hour

func loadMarketEnricher(ctx context.Context, cfg *config.Config, logger *log.Logger) (features.Enricher, error) {
	if cfg.Model.MacroDir == "" {
		return nil, nil
	}
	provider, err := macro.LoadProvider(cfg.Model.MacroDir)
	if err != nil {
		return nil, fmt.Errorf("load macro series from %s (run cmd/macrofetch): %w", cfg.Model.MacroDir, err)
	}
	var records []dividends.Record
	if cfg.Model.DividendsPath != "" {
		records, err = dividends.Load(cfg.Model.DividendsPath)
		if err != nil {
			return nil, fmt.Errorf("load dividends %s: %w", cfg.Model.DividendsPath, err)
		}
	}
	fetcher := macro.NewFetcher()
	if err := provider.Refresh(ctx, fetcher, time.Now().UTC()); err != nil {
		logger.Printf("trader: macro refresh at start failed, using stored series: %v", err)
	}
	go func() {
		ticker := time.NewTicker(macroRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := provider.Refresh(ctx, fetcher, time.Now().UTC()); err != nil {
					logger.Printf("trader: macro refresh failed: %v", err)
				}
			}
		}
	}()
	logger.Printf("trader: market features enabled (macro_dir=%s, %d dividend records)", cfg.Model.MacroDir, len(records))
	return features.NewMarketEnricher(provider, records), nil
}
