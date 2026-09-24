# Dividend-capture event-study (2) — REJECTED 2026-09-25

Pre-registration: `docs/plans/20260925-event-driven-dividend-drift.md`.
Tool: `cmd/dividendstudy`; data: `data/dividends.jsonl` (123 events 2021-2027
from T-Invest `GetDividends`, 65 in study window 2024-01-01..2026-12-19).

## Verdict

**Reject.** The pre-registered acceptance gate (A)+(B) fails on the required
contiguous offset subset containing {-2,-3,-5} under BOTH tax conventions.
Dividend capture on these 18 names is not deployable as the (2) income bet.

## Mechanics (why)

- Mean gap ratio **-0.97** (ex-date OPEN is ~one full dividend below LastBuy
  close): the market prices the ex-date drop essentially exhaustively.
- Primary offset k=-3 excess over IMOEX: -0.0063 (tax 0.13) / +0.000063
  (tax 0), CI [-0.022, +0.010] / [-0.0165, +0.0165] — indistinguishable from
  zero; leave-one-out fails on all four axes (ticker/season/year/wave).
- Offsets -5 and -2: CI contains 0; -5 fails year+wave, -2 fails
  season+year+wave (both tax conventions).
- Only offset -1 (buy close T-1, sell ex-date close) has a CI excluding 0:
  excess +0.0006 (tax 0.13) / +0.0071 (tax 0), CI [0.0008,+0.0151] at tax 0.
  It is OUTSIDE the pre-registered acceptance subset, which requires a
  contiguous set containing {-2,-3,-5}; a single lucky offset decides
  nothing (protocol rule).
- Sub-gate (DividendNet gross-vs-net, LastBuy/record T+1 semantics) not fully
  closed; verdict is assumption-robust so it cannot flip the outcome.

## Coverage

65 events, 9 waves (2024/2025/2026 x SPRING/AUTUMN/OTHER), 80% outside the
largest wave — mass bars pass; the rejection is a statistical falsification,
not a data-scarcity artifact.

## Follow-up

No further (2) work without a NEW hypothesis and a fresh pre-registration.
The income lever reverts to (1): 2-3x target-notional with a `max_net_exposure`
net-exposure cap (unhedged DD 3.71%/4.43% currently exceeds the 3% kill-switch
limit at 2x/3x), validated against borrow-stress preflight.