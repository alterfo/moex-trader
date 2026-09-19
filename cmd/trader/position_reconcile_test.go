package main

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"
)

type fakePortfolioLots struct {
	lots  map[string]int
	err   error
	calls int
}

func (f *fakePortfolioLots) PositionLots(context.Context, []string) (map[string]int, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.lots, nil
}

type fakeRiskPositionReader struct {
	lots  map[string]int
	err   error
	calls int
}

func (f *fakeRiskPositionReader) CurrentLots(_ context.Context, ticker string) (int, error) {
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	return f.lots[ticker], nil
}

func TestReconcilingPositionReaderUsesBrokerLots(t *testing.T) {
	source := &fakePortfolioLots{lots: map[string]int{"SBER": -4, "GAZP": 13}}
	fallback := &fakeRiskPositionReader{lots: map[string]int{"SBER": 99}}
	reader := newReconcilingPositionReader(source, fallback, []string{"SBER", "GAZP"}, time.Minute, time.Now, log.New(io.Discard, "", 0))

	for ticker, want := range map[string]int{"SBER": -4, "GAZP": 13, "LKOH": 0} {
		got, err := reader.CurrentLots(context.Background(), ticker)
		if err != nil {
			t.Fatalf("CurrentLots(%s) error = %v", ticker, err)
		}
		if got != want {
			t.Fatalf("CurrentLots(%s) = %d, want %d", ticker, got, want)
		}
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}
}

func TestReconcilingPositionReaderCachesWithinTTL(t *testing.T) {
	now := time.Unix(0, 0)
	source := &fakePortfolioLots{lots: map[string]int{"SBER": 3}}
	reader := newReconcilingPositionReader(source, &fakeRiskPositionReader{}, []string{"SBER"}, time.Minute, func() time.Time { return now }, log.New(io.Discard, "", 0))

	for i := 0; i < 5; i++ {
		if _, err := reader.CurrentLots(context.Background(), "SBER"); err != nil {
			t.Fatalf("CurrentLots() error = %v", err)
		}
	}
	if source.calls != 1 {
		t.Fatalf("source calls = %d, want 1 within TTL", source.calls)
	}

	now = now.Add(time.Minute + time.Second)
	if _, err := reader.CurrentLots(context.Background(), "SBER"); err != nil {
		t.Fatalf("CurrentLots() after TTL error = %v", err)
	}
	if source.calls != 2 {
		t.Fatalf("source calls after TTL = %d, want 2", source.calls)
	}
}

func TestReconcilingPositionReaderFallsBackOnError(t *testing.T) {
	now := time.Unix(0, 0)
	source := &fakePortfolioLots{err: errors.New("portfolio unavailable")}
	fallback := &fakeRiskPositionReader{lots: map[string]int{"SBER": 9}}
	reader := newReconcilingPositionReader(source, fallback, []string{"SBER"}, time.Minute, func() time.Time { return now }, log.New(io.Discard, "", 0))

	for i := 0; i < 3; i++ {
		got, err := reader.CurrentLots(context.Background(), "SBER")
		if err != nil {
			t.Fatalf("CurrentLots() error = %v", err)
		}
		if got != 9 {
			t.Fatalf("CurrentLots() = %d, want fallback 9", got)
		}
	}
	if source.calls != 1 {
		t.Fatalf("source calls = %d, want 1 (retry bounded by TTL)", source.calls)
	}
	if fallback.calls != 3 {
		t.Fatalf("fallback calls = %d, want 3", fallback.calls)
	}
}

func TestReconcilingPositionReaderPropagatesFallbackError(t *testing.T) {
	fallbackErr := errors.New("fallback read failed")
	source := &fakePortfolioLots{err: errors.New("portfolio unavailable")}
	fallback := &fakeRiskPositionReader{err: fallbackErr}
	reader := newReconcilingPositionReader(source, fallback, []string{"SBER"}, time.Minute, time.Now, log.New(io.Discard, "", 0))

	if _, err := reader.CurrentLots(context.Background(), "SBER"); !errors.Is(err, fallbackErr) {
		t.Fatalf("CurrentLots() error = %v, want %v", err, fallbackErr)
	}
}
