# Ticker selection check

Seed base: 20261022, K=4, block 91 days. Statistic: sum over evaluated blocks of realized P&L net of borrow, RUB.

| borrow/day | blocks | full | walk-forward | in-sample (leak) | random mean | random sd | q05 | q50 | q95 | p wf | p in-sample | mean Spearman | mean overlap |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 0.00005 | 6 | 1319 | -843 | 9269 | 1300 | 1728 | -1360 | 1231 | 4348 | 0.9104 | 0.0050 | -0.114 | 0.333 |
| 0.00047 | 6 | -3895 | -4008 | 6230 | -2639 | 1876 | -5613 | -2711 | 797 | 0.7711 | 0.0050 | -0.114 | 0.333 |

Verdict:

- dropping losers is indistinguishable from dropping random tickers; the filter is not adopted
