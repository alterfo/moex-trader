# Pre-start gate failure: what is established and the acceptance protocol (2026-10-18)


Status: the acceptance protocol and the corrections to three documents were accepted by the owner on 2026-10-18 ("yes, all three"); nothing was run on the live path, and `config.sandbox.yaml`, `ensemble_model.json` and `./trader` were not changed. All numbers were obtained offline through `cmd/backtest` in a scratch directory and are reproduced by the command below.

## Established facts

1. **The gate refuses stably.** From 2026-10-18 03:44 to 19:43 (MSK) all 30 watchdog starts ended in a refusal: realized net of borrow about −4 690 against a threshold of +1, 99 trades, hit rate 42.4%. The config hash `5396378c` and `ensemble_model.json` (2026-09-18) did not change.
2. **The `out_of_range` guard fix (`ae6ffdd`) is irrelevant to the gate.** The gate-form replay (command below) was built in two variants from one HEAD source: as is and with the `ae6ffdd` patch. The results matched to the last digit on both windows (till 10-18: net of borrow −2467.25, 117 trades; till 10-17: +1209.23, 118 trades). The fix explains none of the live gate figures (−4859.13 before and after the binary replacement at 03:45:42).
3. **The conclusion in `docs/plans/completed/20261018-sell-bias-diagnostic.md` ("−4859 with the fix against +602 for the veto on the same window") compares different windows:** +602.32 is the gate of 2026-10-17 15:09 (window start 07-19), −4859.13 is the window starting 07-20. The claim "a random veto works as a profitable regime filter" is not supported by this data; the veto does not take part in the replay. The rollback `dcb601d` rests on this comparison.
4. **The gate result is determined by the window start date, not by new bars.** The window end does not matter (till 10-15, 10-16, 10-17, 10-18: −2630, −2636, −2630, −2467, because there are no bars after 10-16). Shifting the start at till 10-18:

| window start | realized net of borrow, RUB | trades | hit |
|---|---:|---:|---:|
| 07-13 | +20 380 | 146 | 57.5% |
| 07-14 | +14 584 | 144 | 59.0% |
| 07-15 | +11 455 | 143 | 55.9% |
| 07-16 | +9 382 | 134 | 56.0% |
| 07-17 | +9 100 | 129 | 56.6% |
| 07-18 | +3 435 | 126 | 56.3% |
| 07-19 | +1 372 | 119 | 52.9% |
| 07-20 (gate) | −2 467 | 117 | 52.1% |
| 07-21 | −5 085 | 112 | 51.8% |
| 07-22 | −5 959 | 107 | 46.7% |
| 07-23 | −2 375 | 104 | 49.0% |
| 07-24 | −3 034 | 102 | 49.0% |
| 07-25 | −3 514 | 98 | 51.0% |
| 07-26 | −5 536 | 94 | 50.0% |
| 07-27 | −784 | 97 | 48.5% |

   The level differs from the live gate by a constant shift (the CLI has no lot quantisation): 07-19 gives +1372 in the CLI and +602 in the gate.
5. **Consequence.** The gate honestly reports that over the last 90 days the configuration does not earn, but the sign of the result rests on a few large trades near the left boundary (a step of +3.8 thousand when moving 07-19 → 07-20, +19 thousand between 07-13 and 07-19). A "more robust" gate (the median over starts) is also not passed today: of 15 starts 7 are positive (07-13..07-19) and 8 are negative.

## What this means for the decision

- Do not loosen the threshold and do not bypass the gate: the data show not a gate defect but a weak configuration on the current window.
- The cause of the refusal **must not** be tied to the fix `ae6ffdd`. The decision about it (whether to veto `event_dividend` in the dividend season on the live path, where the veto removes ~59% of decisions) is separate and independent of the gate. The rollback is based on a wrong comparison; reconsidering the decision belongs to the owner, the document corrections are made by the active writer (AGENTS.md rule 7).
- Earlier documents that quote "−4859 against +602 on the same window" or "a random veto as a regime filter" must be corrected: `20261018-sell-bias-diagnostic.md`, `docs/metrics.md` ("Sell-bias diagnostic"), `docs/knowledge/model-quality-history.md`.

## Candidate acceptance protocol (proposed to be fixed before any experiments)

A candidate model (or rule) is accepted for replacement only if all of the following hold:

1. It passes the Task 13 criteria unchanged (purged walk-forward, 6 windows, criteria 1-6 of `docs/metrics.md`).
2. The replay gate form (command below) at `till = today` is positive for at least 12 of 15 window starts `today − 90 − k`, `k = 0..14`, and the median over starts is ≥ `preflight.min_net_pnl`. This is stricter than the present gate and removes the dependence on a single date.
3. The share of the three largest trades in the sum of the window's positive trades is ≤ 50% (a report for the gate window and for the two previous non-overlapping 90-day windows).
4. For each of the three non-overlapping 90-day windows (till = today, today−90, today−180) the maximum drawdown is ≤ 3% and the kill switch does not fire.

For the current `ensemble_model.json` criterion 2 is not met (7 of 15), so replacement is not required until a better candidate appears, and the current model remains forbidden to start.

Which hypotheses to test is determined by the existing plan (`20261018-seven-hypotheses.md`, steps 6 and 7, the IMOEX regime filter and cross-sectional ranking) and by what has already been checked (breaker in the replay: did not help; windows of 10/30/90 days: did not help). This document introduces no new hypotheses on the window that produced the failure: selecting by the September tickers (PLZL, CHMF, POSI, NVTK) without pre-registration and independent windows would be fitting to the same window.

## Reproduction

```
git archive HEAD | tar -x -C <veto>; cp -R <veto>/. <fix>/
git show ae6ffdd -- internal/model/ensemble.go | (cd <fix> && git apply)
(cd <veto> && go build -o bt-veto ./cmd/backtest); (cd <fix> && go build -o bt-fix ./cmd/backtest)
./bt-<variant> -config config.sandbox.yaml -tickers YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI -from <S> -till <T> -deposit 1000000 -max-lots 1000 -commission-rate 0.0005 -spread-pct 0.0005 -spread-db trader-sandbox.db -slippage-pct 0.0005 -borrow-pct-day 0.00005 -target-notional 15000 -max-net-short 90000 -max-net-long 90000 -lookback-days 0 -signal-source ensemble -ensemble-path ensemble_model.json -wf-dir <scratch>
```

The metric source is the "Realized P&L net of borrow" line in the output. The raw outputs are in the session scratch directory (`gv/out-*`, `gv/sweep/out-*`) and were not added to the repository.

## Not verified

- The level difference from the live gate (≈ +770 for start 07-19) is explained by lot quantisation according to the documentation, but not verified at the trade level.
- The ai-box status (the `:9091` endpoint answers 404 on `/` and `/status`).

## T-Bank sandbox positions (checked through the API, evening of 2026-10-18, read-only)

Requests `SandboxService/GetSandboxAccounts`, `GetSandboxPositions`, `GetSandboxPortfolio`, `GetSandboxOrders` with the token from `.env`; no orders or records were created. The only account is `39f02dbf-0979-4e28-b005-d3f151749f40` (opened 2026-09-16, `ACCOUNT_STATUS_OPEN`).

- 17 open positions, all short (one for each of the 17 config tickers), no active orders, no futures.
- Cash 1 247 822 RUB, value of shares −270 285 RUB, portfolio 977 537 RUB (deposit 1 000 000 RUB), `expectedYield` −2.25%.
- Unrealized result on positions −20 533 RUB: 13 of 17 are losing (PLZL −3 417, OZON −3 266, GAZP −2 835, LKOH −2 537, MGNT −1 955, CHMF −1 822, VTBR −1 607, NVTK −1 550, POSI −1 422, ROSN −1 058, T −827, SBER −545, RUAL −357), profitable are GMKN +1 426, YDEX +1 008, MTSS +144, TATN +88.
- Total exposure of the short positions 270 285 RUB (up to 18 236 RUB per ticker).
- The kill switch in `trader-sandbox.db` did not fire (`active=0`).

The positions have been unattended since 2026-10-18 03:43 (trader stop), when there is no trading; the exchange opens on Monday 2026-10-19. Closing the positions or leaving them as they are is the owner's decision: no closing orders were sent as part of this check.
