# Registration: second real-time window for the order-flow composite

Prepared 10 November 2026 for deposit with a public timestamping service (OSF Registries or
equivalent) before the first reading. Text English only; numbers frozen as of this date.

## 1. Question

Does a regularized composite of 75 five-minute order-flow statistics rank the 10-day cross-section
of the 100 most liquid Moscow Exchange stocks better than (a) a single-feature baseline and (b) a
12-month momentum rank, and does either book earn a net return after quoted and flat costs at the
measured short-borrow rate?

The first window answered: (a) yes by 0.037 of daily rank IC (t 2.51), (b) no — the composite and
the momentum book are indistinguishable at +0.019% per day with an interval containing zero. This
registration repeats the measurement once, on dates no one in the programme has seen, because the
first window is spent.

## 2. Window

Feature dates from **19 October 2026** — the first trading date after 16 October 2026, the last date
of the spent window — to the reading date. No date in this window has been used for a verdict, a
plan or an exploratory screen.

## 3. Frozen objects

| Object | Identifier | SHA-256 |
|---|---|---|
| Frozen models (ridge, elastic net, A2 baseline), 78 inputs | `artifacts/algopack-alphafit/alphafit-models-20261023.json` | `793bf125b38b2022dae53021f5185032bb464c506ce2f507d8fb39fb4a2f8892` |
| Frozen hyper-parameters (ridge 0.003, elastic net 0.001, alpha 0.5, trade rate 0.33) | `artifacts/algopack-alphafit/alphafit-hyper-20261023.json` | `89f77b6dfb18e1d63fc34f1b2ef3443665d168dc4e54a277a957e19e471d0d9e` |
| Matrix builder | `cmd/flowscan -mode matrix`, `-lag 30`, `-lag-from 2025-01-01` | rebuilt per reading; the builder is frozen with the repository commit |
| Evaluation tool | `cmd/alphafit -mode forward -from 2026-10-19` | same |

Portfolio rule: dollar-neutral top-minus-bottom quintile, inverse-volatility legs, Garleanu-Pedersen
partial adjustment at rate 0.33 under flat costs and 0.50 under quoted-spread costs, horizon 10
days, entry at the close of the signal day. Cost models as in Section 4 of the paper: flat 0.15% per
side; quoted 0.05% plus half the stock-day BBO spread; RUB 1 mn stress. Carry at 25.0% (measured
median) and 17.0% (for comparability with the first window).

## 4. Criteria

Two co-primary signals — the composite and the 256-day momentum sleeve — through identical
machinery.

1. **Signal.** Mean daily out-of-fold IC of the composite minus the baseline is positive with a 95%
   date-block-bootstrap interval above zero, positive in both halves, and the composite beats its
   within-date permutation placebo in at least four of five seeds. The same row is reported for the
   momentum sleeve.
2. **Contrast.** Composite minus momentum net return at 25% carry: reported with its interval. The
   claim "the composite is worth more than momentum" requires that interval above zero.
3. **Economics.** Net mean at 25% carry with a 95% interval above zero in both halves, placebo books
   non-positive.
4. **Ranking content.** IC of the composite residual after its within-date projection on the
   momentum rank; the claim "the composite contains information orthogonal to momentum" requires
   the residual IC interval above zero **and** criterion 2 to hold.
5. **Power.** All of the above are to be read at the horizon at which 80% power is reached against
   the effect of the first window: 561 dates at the interval rule, 1 056 at |t| >= 3.0, 1 317 at
   |t| >= 3.45. Reported power is computed from the second window's own standard error.

## 5. Reading dates

- **Falsification check, 60 dates (about February 2027):** permitted, and only as falsification.
  The lead is retired if any of the following holds at 60 dates: (a) the mean of the composite-minus-
  baseline IC increment is zero or negative; (b) its 95% block-bootstrap interval lies entirely below
  +0.037, the first-window estimate; (c) the increment is negative in both halves; (d) the composite
  book nets less than the momentum book at the 25% carry. Falsification is reported as such and is
  never read as confirmation. No other interim statistic is computed.
- **Confirmation, 561 dates (about November 2028 at the current accrual rate):** one reading, once.

## 6. What will be reported

Every row above, pass or fail, with its interval, both cost models and both carry scenarios, plus
the count of dates actually available and any date on which the matrix produced no rows. A failure
is final for these definitions; changing them requires a new registration.

## 7. Deviations policy

Amendments are recorded with their date, their reason and the fact that no outcome had been read at
the time. An amendment made after a reading is labelled as such and does not replace the registered
analysis. The deposited text is the version of record for this window.

## 8. Known holes

- Short-borrow availability by date is not measurable through the broker's API and remains an
  assumption; the fee is recorded daily from 29 October 2026.
- Fills are modelled, not executed: quoted spreads and a flat tariff, no market impact beyond half
  the touch.
- One market, one author, and an AI reviewer. The second window does not fix the replication
  problem, which no additional dates on one market can fix.
