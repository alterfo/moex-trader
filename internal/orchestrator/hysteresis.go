package orchestrator

import (
	"context"
	"sync"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const DefaultSignalHysteresisPolls = 2

type SignalHysteresisSource struct {
	source   SignalSource
	required int

	mu    sync.Mutex
	state map[string]*signalBandState
}

type signalBandState struct {
	committed     domain.TradeSignal
	candidate     domain.Action
	candidatePoll int
	initialized   bool
}

func NewSignalHysteresisSource(source SignalSource, requiredPolls int) *SignalHysteresisSource {
	if requiredPolls < 1 {
		requiredPolls = 1
	}
	return &SignalHysteresisSource{
		source:   source,
		required: requiredPolls,
		state:    make(map[string]*signalBandState),
	}
}

func (h *SignalHysteresisSource) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	raw, err := h.source.Generate(ctx, feature)
	if err != nil {
		h.forget(feature.Ticker)
		return raw, err
	}
	if h.required <= 1 {
		return raw, nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	st := h.state[feature.Ticker]
	if st == nil {
		st = &signalBandState{}
		h.state[feature.Ticker] = st
	}
	if !st.initialized {
		st.committed = raw
		st.initialized = true
		return raw, nil
	}
	if raw.Action == st.committed.Action {
		st.committed = raw
		st.candidate = ""
		st.candidatePoll = 0
		return raw, nil
	}
	if raw.Action == st.candidate {
		st.candidatePoll++
	} else {
		st.candidate = raw.Action
		st.candidatePoll = 1
	}
	if st.candidatePoll >= h.required {
		st.committed = raw
		st.candidate = ""
		st.candidatePoll = 0
		return raw, nil
	}
	return st.committed, nil
}

func (h *SignalHysteresisSource) forget(ticker string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.state, ticker)
}
