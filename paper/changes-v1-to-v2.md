# Changes from Zenodo v1 to v2

**v1 (published):** 29 October 2026, `alpha_on_paper_preprint-1.pdf`, 41 pages,
md5 `163220507bc2c8db85972ebd7427c229`, version DOI 10.5281/zenodo.23752213.
**v2 (this one):** 10 November 2026, `main.pdf`, 44 pages, titled *Predictability Without Monetization:
A Point-in-Time Audit of Order-Flow Signals on the Moscow Exchange*.

Both versions share one concept DOI. v1 stays available and unchanged, because both referee reports
cite it.

## 1. Title and thesis

The title changed: the ironic "When Alpha Survives Only on Paper" became "Predictability Without
Monetization". The reason is substantive — the thesis moved after a new test, and the title now
states what the text proves.

| | v1 | v2 |
|---|---|---|
| Thesis | "predictability exists, monetization does not" | "the composite is not worth more than a free 12-month momentum rank; neither book survives costs and the measured carry" |
| Borrow rate | assumed 17% | measured: 20.0–125.0%, median 25.0%, quartiles 22.0/36.9, mean 32.1% |
| Headline economics | 0.0751%/day at 17% | 0.0383%/day at 25% (t 0.89, interval includes zero) |
| Information beyond momentum | "not established", test not run | E1 run 29.10.2026: contrast +0.0194%/day (t 0.44), null |
| Power of the confirmation rule | not reported | 14.8%, MDE 0.0787; 561/1 056/1 317 dates to 80% |
| Ledger directions | 46 | 42 |
| Length | 41 pages | 44 pages |

## 2. Numbers that changed

| Claim | v1 | v2 |
|---|---|---|
| Main book net | 0.075%/day | 0.0383 at the 25% median; 0.0679 for the frozen book at 17% |
| Break-even borrow | ≈37% (from the 0.075 book) | 35.3% (frozen book), 36.8% (published, r=0.5) |
| Policy-band threshold | not computed | borrow ≤ 20.3% against a market minimum of 20.0% |
| Short-leg dividends | omitted | bound 0.96·Δy/252: 0.0126%/day worst case, 0.0038% per 1 pp gap |
| Sharpe | 1.56 | 1.56 and 1.14–1.27 autocorrelation-adjusted |
| Positive quarters | 7 of 7 (p 0.008) | 7 of 8 (p 0.035); partial 2026Q4, 16 dates, IC −0.266 |
| Monthly sign tests | p 0.002 | 0.0022 raw, 0.0037 adjusted; 38 of 45 blocks, p < 10⁻⁴ |
| "2 300 days" of equivalence | no formula | T = ((z₀.₉₇₅+z₀.₈₀)σ_LR/δ)², σ_LR = 0.941%; 1 226 and 2 215 days |
| MDE of the increment | 0.07 | 0.063 |

## 3. What was added

- **§6.1 "Risk profile"** — maximum drawdown −18.2% (16 Jul → 4 Aug 2026), 8 negative months out of
  22, historical 99% loss −1.74%, CVaR −3.63%, Cornish–Fisher −4.29%; at 25% carry the drawdown is
  −18.5%.
- **§6.1 "Capacity"** — tens of millions of roubles gross; 7.5–15 thousand roubles expected per day
  against ~90 thousand rouble daily SD at that size.
- **§6.2 "Latency"** — the file for date d lands in the pipeline at 00:20 on d+1; same-evening
  execution is declared untested rather than assumed.
- **§6.1 "The cost model is a decision variable"** — the momentum contrast is +0.0194% under quoted
  spreads against −0.0102% under the flat tariff.
- **§7.6 "A second real-time window"** — dates from 19.10.2026, one reading at 561 dates
  (about November 2028), 60 dates for falsification only.
- **§4.6 "One standard or two?"** — an explicit statement that the composite's gate is an interval,
  while |t| ≥ 3.0 applies to single-feature families.
- **§4.3 "Why the average and not the better component"** — ridge 0.0970 against the blend's 0.0957.
- **Appendix F** — a "Power" line in the reproducibility checklist, a "Registration" line, and the
  reconciliation of 163 709 / 159 708 / 159 726 rows.

## 4. What was withdrawn or corrected

- The "3–6% a year" range in §5.1 is withdrawn: 12.3% without controls, 5.8% with them; ledger row
  14 was brought into line.
- "the regenerated history inflated every quantity" became "is associated with" — the calendar-time
  confound is real.
- Quarters: "all seven" became seven of eight, with the partial quarter named.
- A clarification that the 0.0753 → 0.0679 step is the adjustment speed, not a dividend adjustment.
- Three corrupted sentences repaired, and the break-even anchors synchronised.

## 5. Bibliography

Both printed reference lists were compared entry by entry.

- **One entry added:** `Bank of Russia. Key rate decisions, 2025–2026` — v1 mentioned the 14–21%
  policy rate in the text without citing it.
- **Nothing removed, no entry edited.** The other 67 records match; the differences an automated
  comparison reported were ligatures (ﬁ/ﬂ) and line breaks only.
- Total 67 → 68.

## 6. What did not change

Data, universe, verdict window (3 January 2025 – 16 October 2026, 457 dates), frozen models
(SHA-256 `793bf125…`), hyper-parameters (`89f77b6d…`), matrix (163 709 rows, 1 644 dates), the 75
features, and the signal and economics criteria — except the power rule, which was tightened.

## 7. Title block

Affiliation and email restored as in v1: *Oleg Sidorkin, Independent researcher,
osidorkin85@gmail.com*, with a footnote on the public repository, the Apache 2.0 licence and the
disclaimer. The title block takes one more line, which accounts for 44 pages instead of 43.

## 8. Draft description for the Zenodo v2 record

> **Version 2 — revision after two independent reviews.** The headline result has changed: the
> order-flow composite is not worth more than a free 12-month momentum rank, and the short-borrow
> fee, now measured at the broker (median 25.0% a year across 72 names, range 20.0–125.0%), leaves
> the book below the policy rate at every realistic rate. Additions: the registered momentum test E1
> (contrast +0.019% per day, interval containing zero), a power ladder for the confirmation rule
> (14.8% power at 60 dates; 561 dates for 80%), a risk profile (maximum drawdown −18.2%, 99%
> Cornish–Fisher loss −4.3%), a capacity bound, a measurement of pipeline latency, and a registered
> protocol for a second real-time window from 19 October 2026. The bibliography gains one reference.
> The full change list is in `changes-v1-to-v2.md`; the v1 file remains available for the two reviews
> that cite it.
