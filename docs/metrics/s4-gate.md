# Metrics: deployment gate and experiments (section 4)

Part of [metrics.md](../metrics.md) (the index, conventions and ledger live there). Numbers here are the single source of truth for this topic.

## 4. Deployment gate and single-change experiments

### 4.1 The gate

Fixed on 2026-10-04 before any experiment numbers, against the purged control (`purged_walkforward_control`, section 3.5) on the same six windows with embargo 10, 1M deposit, costs 0.0005 x 3, kill switch on. A variant is deployable only if all hold: (1) total realized P&L exceeds the control; (2) realized P&L is at least the control in at least 4 of 6 windows; (3) max drawdown is at most 3% in every window and the kill switch never trips; (4) for label or training changes (A, B, C) mean validation AUC is not below the control by more than 0.005; (5) PBO over the plan's attempts is below 0.5; (6) the 90-day preflight passes.

PBO over the eight persisted variants (control, A, B, C-lgb, C-xgb, C-logistic, D, E; a 510-period x 8-candidate matrix, s = 16) is 0.1529 (1968 of 12870 combinations overfit), so criterion 5 passes for the set. Registry-level deflated Sharpe (47 attempts, 6 observations) is 0.6260 (p 0.3740). Per-variant DSR on the six windows (not a criterion): control 0.1290, A 0.0641, B 0.2306, C-lgb 0.0347, C-xgb 0.0942, C-logistic 0.1683, D 0.1323, E 0.0899.

| variant | total realized | trades | windows >= control | worst max DD | kill-tripped windows | mean val AUC | failed criteria |
|---|---:|---:|---:|---:|---:|---:|---|
| control | +59829.47 | 1085 | - | 3.57% | 1 | 0.5370 | (baseline) |
| A dividend-adjusted labels | +30596.25 | 1068 | 2/6 | 3.89% | 1 | 0.5382 | 1, 2, 3 |
| B wide training universe | +86817.47 | 767 | 4/6 | 4.38% | 1 | 0.5185 | 3, 4 |
| C lgb only | +19166.99 | 1578 | 1/6 | 3.26% | 0 | 0.5245 | 1, 2, 3, 4 |
| C xgb only | +41881.38 | 1175 | 2/6 | 3.79% | 1 | 0.5037 | 1, 2, 3, 4 |
| C logistic only | +81382.67 | 904 | 4/6 | 3.69% | 1 | 0.5653 | 3 |
| D volatility-scaled size | +59809.47 | 1070 | 3/6 | 3.60% | 1 | - | 1, 2, 3 |
| E sector cap | +34856.60 | 430 | 2/6 | 2.76% | 0 | - | 1, 2 |

**Verdict: nothing passes.** The closest is C-logistic (passes 1, 2, 4; fails 3 on a 3.69% drawdown with a q1 kill trip); B leads on realized P&L but fails drawdown and AUC; E is the only variant satisfying criterion 3 but fails 1 and 2. The live recipe is untouched.

### 4.2 Experiments A-E (2026-10-04)

- **A, dividend-adjusted (total-return) labels.** `LabelModeAbsoluteTR` adds `dividend_net` to the exit price when an ex-date (first session after `last_buy_date`) falls inside (entry, exit]. Label balance moves from 2855 pos / 3509 neg to 2934 / 3347. Realized -29233.22 (-48.9%) against the control, positive in 3 of 6 windows (control 5 of 6); mean val AUC unchanged (0.5382 vs 0.5370). Rejected.
- **B, wide training universe, unchanged trading universe.** Training on the 262-share TQBR list (`data/train_tickers.json`) with the 10 million RUB trailing-turnover filter adds 27557 labelled rows over 140 names (label 0 = 15965, label 1 = 11592); the 17-ticker validation rows stay byte-identical to the control. Realized +86817.47 (+45.1%) but mean val AUC 0.5185 (-0.0185, outside the band) and q1 max DD 4.38% with the kill switch tripping. Rejected.
- **C, calibration and single members.** `internal/model/reliability.go` provides Brier score, 10-bin ECE and reliability tables (`cmd/strategyvalidation -reliability`). Replaying the control artifacts one member at a time:

| member | samples | Brier | ECE (10 bins) | total realized | trades | worst max DD | kill-tripped | mean val AUC |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| ensemble | 7046 | 0.302700 | 0.209763 | +59829.47 | 1085 | 3.57% | 1 | 0.5370 |
| lgb | 7825 | 0.330966 | 0.255687 | +19166.99 | 1578 | 3.26% | 0 | 0.5245 |
| xgb | 7030 | 0.311151 | 0.222077 | +41881.38 | 1175 | 3.79% | 1 | 0.5037 |
| logistic | 7046 | 0.290461 | 0.199672 | +81382.67 | 904 | 3.69% | 1 | 0.5653 |

The logistic member is the best calibrated and earns the most, but every model is materially miscalibrated in the tails (positive rate near 0.42-0.55 in both extreme bins despite predicted probabilities near 0.07 and 0.93). The ensemble rerun is bit-identical to the control. Rejected on the gate.
- **D, volatility-scaled position size.** `risk.vol_scale` multiplies the target notional by `clamp(sigma_median / sigma_ticker, 0.5, 1.5)` (sizing only, no retraining). Realized +59809.47 against +59829.47 (-20.00 RUB, -0.03%): P&L-neutral. Rejected.
- **E, sector exposure cap.** `risk.sector_caps` at 45000 RUB per sector (banks SBER/VTBR/T; oil and gas LKOH/ROSN/TATN/NVTK/GAZP; metals GMKN/PLZL/CHMF/RUAL), per-ticker projection so risk-reducing orders always pass. Realized +34856.60 (-24972.87, -41.7%), trades 1085 -> 430; every window stays under 3% drawdown and the kill switch never trips, but only 4 of 6 windows are positive (q1 flips from +4301.27 to -5697.73). The cap truncates the concentrated legs that carried the best windows, i.e. cuts market beta. Rejected.

### 4.3 Experiment F: excess-to-IMOEX labels (2026-10-18)

Protocol `docs/plans/completed/20261018-excess-label-walkforward.md`, committed before the run: the section 3.5 recipe with `-label-mode excess` (F) against a fresh same-run `-label-mode absolute` control, both exported, retrained and replayed at HEAD on the ai-box. The fresh absolute export reproduces the earlier dataset byte-for-byte except one extra tail row per ticker. Library versions had drifted since 2026-10-04 (xgb 3.4.1, lgb 4.7.0, sklearn 1.9.1, pandas 3.0.5, numpy 2.5.3); the archived `ens-emb10-q1.json` replays at HEAD to +4002.28 / 74 trades against the recorded +4301.27 / 66 (about 93%), so every gate comparison here is against the fresh control.

| window | control realized | F realized | control trades | F trades | control val AUC | F val AUC |
|---|---:|---:|---:|---:|---:|---:|
| q1 | +858.10 | +13303.17 | 243 | 194 | 0.5673 | 0.4559 |
| q2 | +355.57 | -10261.75 | 205 | 174 | 0.5550 | 0.4858 |
| q3 | +32191.60 | -9240.48 | 158 | 100 | 0.5178 | 0.4767 |
| q4 | -10183.21 | -1543.91 | 183 | 33 | 0.4999 | 0.4915 |
| w2 | +16747.59 | -6836.71 | 115 | 40 | 0.4836 | 0.4892 |
| w1 | +27846.13 | +24248.90 | 252 | 184 | 0.4785 | 0.4989 |
| total / mean | +67815.78 | +9669.22 | 1156 | 725 | 0.5170 | 0.4830 |

Gate: total realized FAIL (+9669.22 vs +67815.78); windows >= control 1/6 FAIL; drawdown PASS (F max 2.35%, no kill; the control itself prints 3.07-3.68% day-close DD without kill trips); mean val AUC 0.4830 vs 0.5170 (-0.034) FAIL; PBO {control, F} = 0.1242 (degenerate at two candidates); preflight not run. **Rejected.** The 2026-10-03 single-window AUC advantage (+0.040, section 5.5) does not transfer to the purged walk-forward on the current stack; the excess book has the expected lower-beta profile (max DD at most 2.35%) because the label removes most of the beta income by design.

### 4.4 Rerun of the gate on refreshed data (2026-10-20)

Core rerun study R2 (`docs/research/20261019-core-rerun.md`, plan `docs/plans/20261019-core-rerun.md`): `scripts/rerun-r2.sh` on the ai-box, data frozen at D = 2026-10-16, the six windows of section 4.1 plus a new window x (train to 2026-09-16, test 2026-09-17..2026-10-16), fresh same-run control, C-logistic (logistic member of the control models) and F (excess labels). Costs, embargo 10 and sizing as in section 4.1.

| window | control realized | C-logistic realized | F realized | control trades | C-logistic trades | F trades |
|---|---:|---:|---:|---:|---:|---:|
| q1 | +2607.26 | +1797.63 | +13050.18 | 237 | 193 | 203 |
| q2 | -650.70 | -1132.66 | -6211.63 | 212 | 212 | 182 |
| q3 | +36768.40 | +44663.60 | -11808.39 | 157 | 117 | 94 |
| q4 | -12043.88 | -6176.50 | -3403.71 | 182 | 187 | 38 |
| w2 | +17530.66 | +15832.40 | -3604.79 | 113 | 56 | 33 |
| w1 | +24237.51 | +11510.17 | +13888.98 | 226 | 48 | 178 |
| x (new) | -2475.87 | -676.86 | -4347.77 | 55 | 41 | 19 |
| total, six original windows | +68449.25 | +66494.64 | +1910.64 | 1127 | 813 | 728 |
| total, seven windows | +65973.38 | +65817.78 | -2437.13 | 1182 | 854 | 747 |

Worst max drawdown over seven windows: control 3.52%, C-logistic 4.35% (kill switch trips in w1), F 2.24%; mean validation AUC over seven windows: control 0.5105, C-logistic member 0.5269, F 0.4859. Windows at or above control: C-logistic 3 of 7, F 2 of 7.

Gate (criteria 1-4 of section 4.1; 5 and 6 not run): C-logistic fails 1 (-155.60), 2 and 3; F fails 1, 2 and 4 (-0.0246). **Nothing passes**, as in the original run. Reproduction against section 4.3: the control's six-window total is +68449.25 against +67815.78 (within 1%), per-window values differ by up to 6000 RUB (q1 +2607.26 against +858.10, w1 +24237.51 against +27846.13) because the news archive grew and libraries drifted; F's six-window total is +1910.64 against +9669.22, same sign pattern (F loses in q3, q4, w2 in both). The C-logistic lead of section 4.1 (+81382.67 against +59829.47) does not reproduce: on the same fresh control it is level (-0.2% over seven windows). The new window x is a small out-of-sample extension, below the noise of single-window comparisons.

