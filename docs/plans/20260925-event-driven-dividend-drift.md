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
- Entry (grid, mirrors the (0) block-length robustness rule): close of
  LastBuyDate - k trading days for k in {-1, -2, -3, -5, -10}. PRIMARY
  offset k=-3 (no literature canon behind it — classic capture buys close
  T-1 and sells on/after ex-date — hence a grid, never a single point).
  Acceptance (section 4) must hold on a CONTIGUOUS subset of offsets
  CONTAINING {-2, -3, -5}: a single lucky offset decides nothing.
- Exit (primary): close of ex-date (LastBuyDate + 1 trading day).
- Held-to-+5TD exit for the same events is SENSITIVITY ONLY (never a
  separate acceptance claim).
- Overlap rule: if two events of the SAME ticker overlap a position (odd,
  e.g. interim + final within days), the later entry is sized after the
  earlier exits; never two overlapping capture positions in one ticker.

## 4. Gate statistics (pre-registered)

- Per-event net return r_e(k) = (exitClose(k) + (1-0.13)*div*divLots) /
  (entryClose(k) * divLots) - 1 minus both-leg costs (comm+spread+slip),
  computed for each entry offset k in {-1,-2,-3,-5,-10}.
- MARKET ADJUSTMENT (calendar-clustering guard): every r_e is measured as
  EXCESS over IMOEX close-close total return over the SAME [entry(k), exit]
  window: r_e^ex = r_e - r_imoex(entry->exit). Russian dividends cluster in a
  spring wave (2022+ regime): 60-70% of events can share one 4-6-week market
  regime, correlated across different tickers. The raw mean drift is
  inflated by that common regime; the excess measure removes it to first
  order (the same trick as (0)'s IMOEX regression). All statistics below run
  on r_e^ex.
- Correlation structure: events are NEVER treated as independent when they
  share a regime. Two clusterings:
  * WAVE = SPRING (LastBuyDate in Feb-May), AUTUMN (Aug-Nov), OTHER per
    calendar year. Within-wave residuals are correlated; resampling respects
    it.
  * Season = 6-week window of the year (9 buckets: Feb-1, Mar-2, ...). A
    "one good March" premium (lives in one 6-week season) must fail (B).
- Primary (A): bootstrap CI of the mean excess drift, resampling WHOLE WAVES
  (geometric blocks over the wave sequence, 2000 repl, seed 42); CI excludes
  0 on a CONTIGUOUS subset of offsets CONTAINING {-2,-3,-5} (grid
  robustness, mirroring (0)'s block-length rule).
- Primary (B): stability under drops on BOTH axes:
  * leave-one-ticker-out (any of the 18 tickers dropped; mean stays >0);
  * leave-one-season-out (any 6-week season dropped; mean stays >0) AND
    leave-one-calendar-year-out (any single dividend year dropped) — the
    direct analogues of (0)'s leave-one-quarter-out that killed the
    2026-Q2-driven t-stat;
  * leave-one-WAVE-out (drop the entire spring wave once).
- Mass and coverage bar: >= 30 studied events AND >= 2 distinct waves AND
  >= 20% of events outside the largest wave; else verdict
  "season-concentrated data, insufficient" (NOT accept/reject).
- Secondary (descriptive only): gap ratio = (ex-date open - prev close) /
  divnet (a <1 ratio is the classic drift seed) — reported, never acceptance.

## 5. Out-of-sample framing (explicit, must not be over-read)

No proper historical OOS holdout exists here: too few events for an
independently-trained split. Per our own (0) precedent, a WALK-FORWARD or
held-out split is NOT possible at ~30-60 event scale. The historical ACCEPT
is therefore **hypothesis confirmation on the training data, not proof on
new data**. The true out-of-sample test is the spring 2027 wave traded live
in the sandbox — the historical number is a necessary but never sufficient
gate. This asymmetry is load-bearing: an historical ACCEPT must not trim a
single day of the live proving window, and the go-live checklist keeps its
own 2-3 months of consecutive positive sandbox realized P&L regardless of
this study's outcome.

## 6. Go/no-go consequence

- ACCEPT -> dividend capture becomes a pre-registered overlay on the same 18
  names, sized within the 3% DD cap (max a third of the book in dividend
  positions), routed through the existing risk gates (TradingStatus,
  marketHours, no_trade_after_open, kill-switch); paper-validated first in
  the live loop mirror (replay), then live in the sandbox for its own
  2-3-month proving window into the spring 2027 wave. The historical accept
  IS the paper validation; the live wave decides.
- REJECT or "not enough events" -> (2) closes as research-only; business case
  = (1)-cap 2x path alone; dividend capture not re-opened without new data.