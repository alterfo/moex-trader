# Dividend run-up: point-in-time announcement dates from the news history (2026-10-26)

Status: DONE 2026-10-26 (English only; scored once, DA2 passes: the lead survives on events with a public announcement), registered before the run. Follows `20261025-algopack-dividend-runup.md` (a lead whose result is an upper bound because the calendar file carries no announcement date). The owner asked to look for historical data for the items waiting on forward evaluation; this is the one item whose missing input exists in history: `data/news_history.jsonl` holds 155,601 ticker-tagged headlines from 2022-01, including board dividend recommendations.

## Question

For how many of the 115 events was the dividend known to the public by the entry close (11 trading days before the last-buy day), and does the economic read of DV1 survive when restricted to those events?

## Announcement date (fixed ex ante)

For an event of ticker T with last-buy day L, the announcement is the earliest headline tagged T, published after the previous calendar last-buy day of T (or 150 calendar days before L if there is none or it is earlier) and before L, that satisfies all of:

1. the title contains `дивиденд`;
2. the title contains a board word (`совет директоров`, `наблюдательный совет`, `набсовет`, or `СД` as a word) followed within 60 characters by `рекоменд`;
3. the title contains the first five letters (lower case) of a company alias of T from `data/ticker_aliases.json`, which removes headlines that carry another company's text under T's tag.

A headline belongs to the first trading date whose 18:30 Moscow time is not earlier than its timestamp (the repository convention). An event is **verified** if the announcement date is at or before the entry date (trading index of L minus 11). An event with no matching headline is **unverified** and is excluded from the verified set, not assumed known. Rumour headlines ("expect", "may pay") do not match rule 2.

## Rows

| Id | Row |
|---|---|
| DA1 | counts: events with a match, verified, and the distribution of the lead (trading days from the announcement to the entry) |
| DA2 | the DE economics (yield thresholds 0, 3%, 6%, same costs) on the verified events only, with the bootstrap interval of the mean and the month-cluster interval at 3% |
| DA3 | the same economics on the unverified events (descriptive, to see whether the lead came from events whose timing could not be checked) |

**Pass** for DA2: at the 3% threshold at least 30 verified events and the 95% bootstrap interval of the net mean above zero. Below 30 verified events the row is "not testable". Fewer than half of the events verified is reported as a data limit, not as evidence against the lead.

## Honest expectations

Headline coverage is incomplete (about 25 thousand headlines a year, mostly large names), so many events will be unverified; the verified subset will lean to large, liquid payers, where the run-up may be smaller. A pass would raise the lead from an upper bound to a measurement on events whose timing was public; a fail would remove the lead. Nothing enters the live path.

## Run order

1. `go run ./cmd/divrunup -news data/news_history.jsonl` on the existing matrix.
2. Numbers into `docs/metrics.md` 8.37 and `docs/metrics/s8b-portfolio.md`.

## Result (scored once, 2026-10-26)

Numbers in `docs/metrics.md` 8.37, output `artifacts/algopack-dividend/divrunup-announce.txt`. 69 of 115 events matched, all verified (lead 17 to 92 trading days, median 36). **DA2 passes:** yield at least 3%, 48 verified events, net +2.20% [+1.07%, +3.31%], month-cluster [+1.05%, +3.26%]; positive in every year, weakest in 2025-2026 (+1.15%, +1.02%). DA3: the 46 unverified events give +1.27% at yield 0%, the same size as the verified set. Registered reading: the foreknowledge caveat of the 8.33 lead is removed for the events whose announcement can be found; the rule remains a lead until the forward read. Caveat: the earliest-match rule can attach a headline about another payout when the previous payout is not in the calendar.
