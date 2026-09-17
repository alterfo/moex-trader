# Short-borrow measurement and stress (Task 11)

- Sandbox account: 39f02dbf-0979-4e28-b005-d3f151749f40
- Operations lookback: 180 days
- Margin-fee operations observed: 0 (0 RUB)

## Per-ticker short terms

| ticker | short enabled | Kshort | Dshort | DshortMin |
|---|---|---|---|---|
| CHMF | true | 0 | 0.223 | 0.1428 |
| DATA | false | 0 | 0 | 0 |
| GAZP | true | 0 | 0.2767 | 0.1818 |
| GMKN | true | 0 | 0.2 | 0.1428 |
| LKOH | true | 0 | 0.22 | 0.1666 |
| MGNT | true | 0 | 0.2 | 0.1428 |
| MTSS | true | 0 | 0.25 | 0.1666 |
| NVTK | true | 0 | 0.2144 | 0.1886 |
| OZON | true | 0 | 0.28 | 0.26 |
| PLZL | true | 0 | 0.2501 | 0.1818 |
| POSI | true | 0 | 0.27 | 0.25 |
| ROSN | true | 0 | 0.2299 | 0.1666 |
| RUAL | true | 0 | 0.2589 | 0.1428 |
| SBER | true | 0 | 0.2 | 0.1428 |
| T | true | 0 | 0.2 | 0.2 |
| TATN | true | 0 | 0.231 | 0.1428 |
| VTBR | true | 0 | 0.3333 | 0.2222 |
| YDEX | true | 0 | 0.3689 | 0.2 |

## Borrow-rate resolution

- Measurable: false
- Using stress fallback: true
- Rate per day: 0.00005 (0.0050%)
- The broker API does not expose a borrow fee rate, and no margin-fee operations were observed in the account history, so the pre-registered 0.005%/day stress is applied.

- Go/no-go rule: use only the stress scenario unless a real measured rate exists.
