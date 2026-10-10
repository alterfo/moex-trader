# News exit: tuning, replay backtest, buy side

Status: PLANNED (created 2026-10-10). Direction: `docs/VISION.md`. Context: `docs/knowledge/news-and-events.md` ("News-position exit").

## 1. Tune exit parameters (blocked on data)

- Defaults 1.5% take-profit arm / 0.7% trail / 2.5% stop / 24h max hold / 48h cooldown are scaled from incident CAR[0,+1] ~ -2.3% on N=12, not fitted.
- Trigger: >= 30 live incident shorts in `news_trade_state` history and `incident_shorts_total`. Until then do not touch the numbers.
- Protocol (fix before looking): grid take-profit {1,1.5,2}%, trail {0.5,0.7,1}%, max hold {12,24,48}h; metric = mean net-of-commission return per trade with date-bootstrap CI; choose the parameter set by the lower CI bound, not the mean; report the full grid in `docs/metrics.md` in the same commit.
- Add a `news_exit_total{ticker,reason}` counter and an audit event per exit so the data for this step accumulates.

## 2. Replay backtest of incident short + exit

- Preflight replay cannot see news, so the shorts have no backtest. `data/news_history.jsonl` (~10k lines since 2026-04) is enough to replay `CountIncidentSources` per day.
- Build `cmd/incidentreplay`: for each ticker-day with IncidentCount >= 2, enter at next bar open, run the exit rules on daily bars (intraday if ISS 1m candles are available), subtract commission and borrow cost. Same exit logic must be shared with `cmd/trader` (extract the decision function from `newsExitManager.evaluate` into a package) so replay and live cannot diverge.
- Success criterion pre-registered: mean net return per trade > 0 with CI lower bound > -0.5%, N >= 30. Otherwise keep `incident_short_enabled` off.

## 3. Buy side

- No reliable positive-news direction today: the lexicon classifier is dead (val AUC 0.49-0.54) and event topics showed no effect.
- Candidate (from arXiv 2412.19245): score headlines with a pretrained Russian-capable sentiment model without fine-tuning on price labels; aggregate per ticker-day; test next-day abnormal return with the `cmd/eventstudy` machinery (market-adjusted, bootstrap by date).
- Constraints: ToS forbids training on Telegram/Google News data (inference scoring only); one local LLM instance on ai-box and shared GPU, so run offline in batches, never in the live path.
- Pre-register the protocol before the first run (window, metric, success threshold). Only if it passes, emit a BUY from the news gate; the existing exit manager already handles long trades (`Short=false`) but `noteShortEntry` has no long counterpart yet.
- Coordinate with the session that owns the news analyzer before touching `internal/ingestion/news`.
