# Announcement post — version 2

## Long version (Telegram, LinkedIn, Habr)

**We measured the one input that decided the strategy. It killed the strategy.**

Version 2 of *Predictability Without Monetization* is out. Two reviews — a mathematical audit
and a trader's due diligence — converged on the same question from opposite sides: what does the
short-borrow fee actually cost on the Moscow Exchange? The first version assumed 17% a year and
said so. We went and measured it at the broker: 72 of 99 admitted names resolve, the annual rate
runs from 20.0% to 125.0%, the median is 25.0%. **Not one name is available at 17%.**

At the median the book earns 0.038% a day with an interval that contains zero. At the upper
quartile it is negative. Break-even sits at 35.3%; clearing the lower edge of the policy band
needs a rate at or below 20.3%, and the cheapest rate in the market is 20.0%. The survivable range
is narrower than one percentage point and empty.

Three other things changed in the same direction:

- **A free 12-month momentum rank is worth as much.** The registered test puts the composite
  +0.019% a day ahead of a momentum book, with an interval containing zero. At the level of
  ranking the composite is not a momentum proxy — 91% of its IC is orthogonal to it — but that
  residual buys nothing.
- **The confirmation rule had 14.8% power** against the effect it was designed to confirm. It now
  needs 561 dates for 80% power, and the 60-date reading is labelled a falsification test.
- **The missing risk numbers.** Maximum drawdown −18.2%, 99% Cornish–Fisher daily loss −4.3%,
  against −1.9% from a Gaussian rule on the same moments. A capacity ceiling in the tens of
  millions of roubles. And the pipeline writes the day's file at 00:20 the next morning, so the
  same-evening execution the protocol assumes is untested, not demonstrated.

What the paper claims now is narrower and harder to attack: a point-in-time audit of a commercial
order-flow dataset, a documented provenance hazard (the exchange regenerated its 2020–2024
history; effects on it run about twice their real-time size), and a clean null on monetization.

The previous version stays available — both reviews cite it — and the full change list is in the
repository. Code, plans, run logs and model hashes are public.

Zenodo: https://zenodo.org/records/23752213 · Repository: https://github.com/alterfo/moex-trader

## Short version (three lines, for a feed or a reply)

Version 2 is out. We measured the short-borrow fee instead of assuming it: median 25.0% a year,
nothing at the assumed 17%. At that rate the book earns 0.038% a day with an interval containing
zero, and a free 12-month momentum rank is worth as much. The claim is now an audit, a provenance
hazard and a null on monetization — not alpha.

## One-liner (for a Zenodo comment or a changelog)

v2 replaces an assumed borrow rate with a measured one (median 25.0%, 20–125%), adds the
registered momentum test (null), a power ladder for the confirmation rule and the missing risk
profile; the book no longer clears the policy rate at any rate the market offers.
