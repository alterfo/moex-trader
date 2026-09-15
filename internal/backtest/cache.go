package backtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type CachedSignalSource struct {
	inner SignalSource
	path  string
	mu    sync.Mutex
	cache map[string]domain.TradeSignal
}

func NewCachedSignalSource(inner SignalSource, path string) (*CachedSignalSource, error) {
	s := &CachedSignalSource{inner: inner, path: path, cache: make(map[string]domain.TradeSignal)}
	if path == "" {
		return s, nil
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *CachedSignalSource) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("backtest: read decision cache %q: %w", s.path, err)
	}
	if err := json.Unmarshal(data, &s.cache); err != nil {
		return fmt.Errorf("backtest: parse decision cache %q: %w", s.path, err)
	}
	return nil
}

func (s *CachedSignalSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	key := decisionKey(feature)
	s.mu.Lock()
	if cached, ok := s.cache[key]; ok {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	signal, err := s.inner.Generate(ctx, feature)
	if err != nil {
		return domain.TradeSignal{}, err
	}

	s.mu.Lock()
	s.cache[key] = signal
	s.mu.Unlock()
	return signal, nil
}

func (s *CachedSignalSource) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" || len(s.cache) == 0 {
		return nil
	}
	data, err := json.Marshal(s.cache)
	if err != nil {
		return fmt.Errorf("backtest: marshal decision cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("backtest: mkdir %q: %w", filepath.Dir(s.path), err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("backtest: write decision cache: %w", err)
	}
	return os.Rename(tmp, s.path)
}

func decisionKey(feature domain.FeatureContext) string {
	payload, err := json.Marshal(feature)
	if err != nil {
		return "fallback"
	}
	sum := sha256.Sum256(payload)
	date := feature.GeneratedAt.Format("2006-01-02")
	return feature.Ticker + "|" + date + "|" + hex.EncodeToString(sum[:4])
}
