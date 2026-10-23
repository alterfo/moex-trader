# Soft models, structural stability and parametric optimization (Arnold)


Read before tuning thresholds, sizing, horizons or any other parameter to a backtest optimum, before trusting a conclusion drawn from a single hard model, or before adding another layer to the decision path.

Source: V. I. Arnold, "Hard and soft mathematical models" — a report given in 1997 at the seminar of the Presidential Council of the Russian Academy of Sciences, 2nd ed., MTSNMO, 2008, 32 pp. (`https://digitalphysics.ru/pdf/Arnold_V_I--Jestkie_i_myagkie_matematicheskie_modeli.pdf`). The notes below are the parts that carry over to this repo; numbers about this repo live only in `docs/metrics.md`.

## The two kinds of model

- **Hard model** — an exact system with an explicit solution (the multiplication table is the author's first example). Precise and quantitative, but its conclusions can be *qualitatively* wrong as soon as the real system differs from the model.
- **Soft model** — the exact form of the coefficients and functions is unknown, only their qualitative shape is fixed; the conclusion is only qualitative, but it survives perturbation of the model class.
- Acceptance criterion for any hard-model conclusion: **structural stability (roughness)** — the conclusion must survive a small change of the model's parameters and of the functions that describe it. If it does not, the hard model cannot be trusted, and its quantitative optimum is the least trustworthy part of it.

## The war model: what structural stability looks like

- Lanchester: `ẋ = -b·y`, `ẏ = -a·x`, invariant `a·x² - b·y² = const`. The neutral line `√a·x = √b·y` separates "x wins" from "y wins". Replacing the constants `a`, `b` by arbitrary positive functions `a(x,y)`, `b(x,y)` distorts the trajectories on the `(x,y)` plane but preserves this separation: the topological type does not change.
- Consequence: "twice the troops need four times the firepower" survives the softening; the square root in the neutral line is a quantitative prediction of the hard version only.
- The same section gives the counterexample: an equally plausible-looking phase portrait (the win regions interleaved, no separating curve) is *not realizable* for sufficiently tame `a,b` — in particular, not for positive definite polynomials. Structural stability rules it out; no amount of data fitting would have produced it.

## Optimization as a route to catastrophe

- Malthus `ẋ = k·x` (exponential) has to soften into `ẋ = k(x)·x` once the population is large; with `k(x) = a - b·x` this is the logistic model `ẋ = a·x - b·x²`, scaled to `ẋ = x - x²`.
- Harvesting with a hard quota `ε`: `ẋ = x - x² - ε`. Critical value `ε = 1/4`, the maximum of `x - x²`. At `ε = 1/4` the unstable and the stable equilibria merge: the stationary state exists but is unstable, and any small random decrease destroys the population in finite time, however large it was. **Optimizing the quota to the maximum sustainable yield puts the system exactly on that knife edge.**
- Harvesting with feedback (a *differential quota*) `ε = k·x`: `ẋ = x - x² - k·x`. The same maximum average yield `ε = 1/4` is reached at `k = 1/2`, but the state is stable: a deviation of `k` from `1/2` costs a little yield instead of destroying the system.
- Carry-over: a parameter tuned to the historical maximum sits on the boundary of the stability region, so the *shape* of the rule matters more than the value. Prefer a rule that reads the current state (equity, drawdown, drift, realized volatility) over a constant tuned on history.

## Hard conclusions that die under a small perturbation

- Lotka-Volterra, `ẋ = a·x - x·y`, `ẏ = -b·y + d·x·y`, has a structurally *unstable* conclusion: every non-stationary trajectory is periodic. Adding `ε·f(x,y)`, `ε·g(x,y)` with `ε << 1` destroys it and yields one of several structurally stable scenarios (convergence to a stable equilibrium; an "outburst" ending in extinction; a limit cycle of an amplitude independent of the initial perturbation). Which one depends on the form of the small terms — information the hard model does not contain.
- Rule: with a hard model, always test the conclusions for structural stability; when the test yields a *set* of scenarios, extra precision in the hard model does not substitute for the missing information about the small terms.
- Carry-over: "signal X oscillates / mean-reverts" shown on one window is this section's periodic-trajectory claim. A claim that changes scenario under a perturbation of the feature set, the costs or the window is not a finding.

## Multi-stage control is unstable from n >= 3

- Model: stage 1 sets the rate (`ẋ = y`), stage 2 the rate of the rate (`ẏ = z`), up to rank `n`; only rank `n` closes the loop on the goal, `x⁽ⁿ⁾ = -k·(x - X)`, `k > 0`. The characteristic equation is `λⁿ = -k`, so the roots are the vertices of a regular `n`-gon in the complex plane.
- `n = 1`: stable. `n = 2`: the roots sit on the stability boundary (oscillation, no catastrophic growth; whether it is stable depends on the details of the organisation that the simplest model ignores). `n >= 3`: some vertices necessarily have `Re λ > 0` — unstable, with oscillations growing without bound.
- The author's remark on the same model: long-lived multi-stage administrative systems survived only because the intermediate ranks were genuinely interested in the outcome ("shadow alignment of interests"), not because directives were executed.
- Carry-over: every extra layer between the decision and the action (an intermediate model, a second opinion, an approval step) adds a pole pair. This is an argument for keeping the signal chain short (signal source -> risk -> executor) and for treating an added layer as a *stability* change rather than a parameter change.

## Six laws of "perestroika" (the author's theory of catastrophes)

About a nonlinear system sitting in a poor but stable state while a better stable state is visible. Recorded here as design heuristics for regime changes in a live strategy, not as measured laws:

1. Moving toward the better state immediately makes things worse; at a steady rate the deterioration accelerates.
2. The closer to the better state, the greater the system's resistance to further change.
3. The maximum resistance is passed *before* the worst state is reached; past that maximum the state keeps worsening.
4. Past the worst state the resistance not only disappears but turns into attraction toward the better state.
5. The deterioration needed to enter the better state is comparable to the final improvement and grows as the system becomes more developed: a weakly developed system can pass almost without loss, a developed one cannot improve continuously.
6. A jump — a discontinuous, not gradual transition — to a point sufficiently close to the good state lets the system evolve there by itself.

## Benford's law and the ergodic principle

- First digits of `2ⁿ` (`1, 2, 4, 8, 1, 3, 6, 1, 2, 5, 1, ...`): `1` about 30 %, `9` about 5 %. The fraction with first digit `i` is `lg(i+1) - lg i`, the length of the segment from `lg i` to `lg(i+1)` of the log axis, and it follows from Weyl's theorem on the uniform distribution of `{n·x}` for irrational `x`, applied to `x = lg 2`. The same holds for any geometric progression whose ratio is not `10^(p/q)`.
- Why cross-sectional data can stand in for a time series: the ergodic principle — a time average may be replaced by an ensemble average. Populations of countries in one year, areas of countries, river lengths, mountain heights and page counts of books all showed the same first-digit distribution. The author uses the principle in the reverse direction here: a per-country statistic is computed from the evolution of one country.
- The author's model for the areas (a theorem for that hard model, an open problem in general): at each step a country either splits into two equal parts or merges with another of equal area, 50/50. Computer experiments showed the result is insensitive to the split ratio, the merge probabilities, the geography and the initial configuration — i.e. the law is a property of the *soft* model, while a rigorous proof is "a hard and far from solved problem".
- Carry-over: a cheap data-quality diagnostic for a batch of magnitudes (turnover per instrument, trade sizes, capital figures) — a gross mismatch hints at fabricated, filtered or wrongly parsed data. Caveats: MOEX prices and volumes are neither geometric progressions nor unit-free, and tick and lot sizes quantise them, so a Benford test on raw price levels is meaningless. The test belongs on cross-sectional magnitudes, and it is a diagnostic, not a gate.

## What does not carry over

- The closing section (mathematics and mathematics education) is a polemic: political insertions, and the claim that the divide between "mathematicians-philosophers" and "mathematicians-computers" has a physiological origin in hemispheric dominance. That neurophysiology is outdated and the policy conclusions are the author's opinion. What survives is Pasteur's line ("there are no applied sciences, only applications of science") and the thesis that what pays off is not the application of ready models but the mathematical approach to a phenomenon.
- The book contains no evidence about this repo, and its data (populations, wages, funding) are from 1997-2008.

## Where this touches the repo

- `AGENTS.md` validation invariants (purged walk-forward, PBO/DSR, the preflight gate, champion/challenger as evidence only, the manual model swap) are the operational form of "do not trust a conclusion that does not survive perturbation".
- `docs/knowledge/model-quality-history.md` (attempts and their outcomes), `docs/knowledge/validation-and-ops.md` (tooling), `docs/metrics.md` (the only place with project numbers).
