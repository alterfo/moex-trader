# Pre-registered protocol: event-driven dividend-drift study (2)

Status: PROTOCOL ONLY. No numbers exist yet for this study. Acceptance bar is
fixed below BEFORE the first run; this document is the read-before-numbers
gate for both sessions (peer review required before results are reported).

## 1. Context and non-goals (2026-09-25 business split)

Path (1) (managed-beta leverage) is validated for 2-3x only on the
beta-NEUTRAL book; live can't hold that neutrality (futures unavailable,
SHARE-only broker layer), so (1)-over-current-book caps at ~2.5-3%/yr and the
leverage claim is deferred until a net-exposure cap exists. Path (2) is the
income bet to mid-2027: premium events with %-scale payoffs, NOT 1bp/day.
This study tests ONLY the ex-dividend drift premium on the 18 canonical
names. Deliberately EXCLUDED (demoted 2026-09-25 consensus): index-rebalance
flows (all 18 already index members; weight changes small and pre-announced)
and CBR/OPEC macro rate events (regime-adjacent; no ingestion infra). If the
drift is rejected, (2) stays research-only and the business case rests on the
(1)-cap path alone.

## 2. Reproducible inputs (fixed)

- Dividends: T-Invest `InstrumentsService.GetDividends(figi, from, to)` per
  canonical figi (18: YDEX..POSI). Available fields: DeclaredDate,
  LastBuyDate, PaymentDate, DividendNet, Regularity, DividendType.
  Probe 2026-09-25 (sandbox endpoint): SBER -> 4 events 2023..2026, LKOH ->
  7 events, GAZP -> empty (correct: no dividends since 2022). History of
  ALREADY-PAID events is returned — this is the data-gate evidence.
- Pull window for the dataset: 2021-01-01..2027-12-31, persisted once to
  `data/dividends.jsonl` by `cmd/dividendscalendar` (net-new cmd). Event must
  have `LastBuyDate >= 2024-01-01` AND `LastBuyDate <= 2026-12-19` to be a
  studied event (leaves a usable buffer; spring wave Feb-May 2027 lands in
  the 12-month horizon).
- Prices: MOEX ISS daily candles, same pipeline as `cmd/alphahedge`
  (ISSSource + memory + `model.ToVector` feature builder, warmup standard).
- Exclusions: `DividendType == "Cancelled"`. Events for tickers with
  `apiTradeAvailableFlag=false` at pull time (e.g. data quality guard).
- Position per event: target notional 30000 RUB (2x ladder — matches (1)
  axis; long-only, cash-flow funded, no borrow), lot-quantized via the real
  lot size (`ResolveLotSize` path used in live), `max_lots` ceiling.
- Costs per fill (both legs): commission 0.0005, spread 0.0005, slippage
  0.0005; dividend Tax 13% withheld at receipt.
- Tax basis assumption: `DividendNet` treated as GROSS amount; verify against
  actual receipts for 1-2 historical events BEFORE computing numbers
  (sub-gate; if net-of-tax, retax assumption collapses and amounts reduce).

## 3. Event definition and trade timing (fixed)

- Event = dividend payment with a valid LastBuyDate in the study window.
- ex-date = first trading day strictly AFTER LastBuyDate (MOEX T+1
  settlement as of 2025+). Record-date semantics verified against 1-2
  historical LastBuy/record pairs before running (sub-gate).
- Entry (primary): close of LastBuyDate - 3 trading days.
- Exit (primary): close of ex-date (LastBuyDate + 1 trading day).
- Held-to-+5TD exit for the same events is SENSITIVITY ONLY (never a
  separate acceptance claim).
- Overlap rule: if two events of the SAME ticker overlap a position (odd,
  e.g. interim + final within days), the later entry is sized after the
  earlier exits; never two overlapping capture positions in one ticker.

## 4. Gate statistics (pre-registered)

- Per-event net return r_e = (exitClose + (1-0.13)*div*divLots) / (entryClose
  * divLots) - 1 minus both-leg costs (comm+spread+slip x order).
- Primary (A): bootstrap CI of the mean net return over events, with
  per-ticker clustering stationary block bootstrap (geometric blocks,
  mean block = 3 events, 2000 repl, seed 42); continuous exclusion of 0 over
  the event-count grid spanning at least the central 60% of events.
- Primary (B): leave-one-ticker-out — mean net return stays positive after
  dropping any of the 18 tickers (catches a single-name story masquerading
  as a premium).
- Mass bar: >= 30 studied events total (else "not enough events" verdict, not
  accept/reject).
- Secondary (descriptive): gap ratio = (ex-date open - prev close)/divnet
  (a <1 ratio is the classic drift seed); reported but never acceptance.

## 5. Go/no-go consequence

- ACCEPT -> dividend capture becomes a pre-registered overlay on the same 18
  names, sized within the 3% DD cap (max a third of the book in dividend
  positions), routed through the existing risk gates (TradingStatus,
  marketHours, no_trade_after_open, kill-switch); paper-validated first in
  the live loop mirror (replay), then live in the sandbox for its own
  2-3-month proving window into the spring 2027 wave.
- REJECT or "not enough events" -> (2) closes as research-only; business case
  = (1)-cap 2x path alone; dividend capture not re-opened without new data.