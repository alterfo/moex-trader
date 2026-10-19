import json
import sys
import numpy as np
import pandas as pd
import statsmodels.api as sm
from scipy.stats import norm

SEED = 20261019
rng = np.random.default_rng(SEED)


def load(path):
    df = pd.read_csv(path, parse_dates=["date"])[["date", "close"]].dropna()
    df = df.sort_values("date").drop_duplicates("date").reset_index(drop=True)
    df["r"] = np.log(df["close"]).diff() * 100
    df.loc[df["date"].diff().dt.days > 7, "r"] = np.nan
    return df


def hac(y, x, lags=5):
    d = pd.DataFrame({"y": y, "x": x}).dropna()
    if len(d) < 30 or d["x"].nunique() < 2:
        return dict(diff=np.nan, t=np.nan, n=len(d))
    m = sm.OLS(d["y"].to_numpy(float), sm.add_constant(d["x"].to_numpy(float))).fit(cov_type="HAC", cov_kwds={"maxlags": lags})
    return dict(diff=float(m.params[1]), t=float(m.tvalues[1]), n=int(len(d)))


def month_ends(df):
    d = df.dropna(subset=["close"]).copy()
    d["ym"] = d["date"].dt.to_period("M")
    m = d.groupby("ym").agg(date=("date", "last"), close=("close", "last"))
    m["gap"] = m.index.to_series().diff().apply(lambda x: x.n if pd.notna(x) else 1)
    m["r"] = np.log(m["close"]).diff() * 100
    m.loc[m["gap"] > 1, "r"] = np.nan
    return m


def day_flags(df):
    d = df.copy()
    d["dow"] = d["date"].dt.dayofweek
    ym = d["date"].dt.to_period("M")
    d["first3"] = d.groupby(ym).cumcount() < 3
    d["last1"] = d.groupby(ym).cumcount(ascending=False) == 0
    d["tom"] = d["first3"] | d["last1"]
    nxt = d["date"].shift(-1)
    gap = (nxt - d["date"]).dt.days
    d["pre_hol"] = ((d["dow"] < 4) & (gap > 1) & (gap <= 7)) | ((d["dow"] == 4) & (gap > 3) & (gap <= 10))
    return d


def sample(df, a, b):
    return df[(df["date"] >= a) & (df["date"] <= b)]


def calendar_block(us, ru):
    out = {}
    samples = dict(disc=("1953-01-01", "1989-12-31"), conf=("1990-01-01", "2026-10-14"))
    ud, rd = day_flags(us), day_flags(ru)
    um, rm = month_ends(us), month_ends(ru)
    um["mon"] = um["date"].dt.month
    rm["mon"] = rm["date"].dt.month
    daily_tests = {f"S1_dow{k}": (lambda d, k=k: d["dow"] == k) for k in range(5)}
    daily_tests["S2_tom"] = lambda d: d["tom"]
    daily_tests["S5_prehol"] = lambda d: d["pre_hol"]
    for name, f in daily_tests.items():
        row = {}
        for s, (a, b) in samples.items():
            d = sample(ud, a, b)
            row[s] = hac(d["r"], f(d).astype(float))
        d = sample(rd, "1997-09-22", "2026-10-16")
        row["ru"] = hac(d["r"], f(d).astype(float))
        out[name] = row
    monthly_tests = {"S3_jan": lambda m: (m["mon"] == 1), "S4_halloween": lambda m: m["mon"].isin([11, 12, 1, 2, 3, 4])}
    for name, f in monthly_tests.items():
        row = {}
        for s, (a, b) in samples.items():
            m = um[(um["date"] >= a) & (um["date"] <= b)]
            row[s] = hac(m["r"], f(m).astype(float))
        m = rm[(rm["date"] >= "1997-09-22")]
        row["ru"] = hac(m["r"], f(m).astype(float))
        out[name] = row
    md = um[(um["date"] >= samples["disc"][0]) & (um["date"] <= samples["disc"][1])]
    best = int(md.groupby("mon")["r"].mean().idxmax())
    row = {"best_month": best}
    for s, (a, b) in samples.items():
        m = um[(um["date"] >= a) & (um["date"] <= b)]
        row[s] = hac(m["r"], (m["mon"] == best).astype(float))
    row["ru"] = hac(rm["r"], (rm["mon"] == best).astype(float))
    out["S6_bestmonth"] = row
    desc = {}
    uy = us.copy()
    uy["year"] = uy["date"].dt.year
    ann = uy.groupby("year")["close"].last()
    ar = np.log(ann).diff() * 100
    ar.index = ann.index
    desc["presidential_cycle_mean_by_year_mod4"] = {int(k): float(v) for k, v in ar.groupby(ar.index % 4).mean().items()}
    desc["us_month_means"] = {int(k): float(v) for k, v in um[um["date"] >= "1953-01-01"].groupby("mon")["r"].mean().items()}
    desc["ru_month_means"] = {int(k): float(v) for k, v in rm.groupby("mon")["r"].mean().items()}
    out["descriptive"] = desc
    return out


def found(row, cost=0.06):
    d, c, r = row["disc"], row["conf"], row["ru"]
    ok = abs(d["t"]) >= 3.0 and np.sign(c["diff"]) == np.sign(d["diff"]) and abs(c["t"]) >= 2.0 and np.sign(r["diff"]) == np.sign(d["diff"]) and abs(r["t"]) >= 2.0
    trad = ok and abs(c["diff"]) > cost and abs(r["diff"]) > cost
    return bool(ok), bool(trad)


def periodogram_y1(df, name):
    d = df.dropna(subset=["r"]).copy()
    sd = d["r"].rolling(63).std().shift(1)
    z = (d["r"] / sd).dropna().to_numpy()
    n = len(z)
    z = z - z.mean()

    def per(x):
        f = np.abs(np.fft.rfft(x)) ** 2 / len(x)
        k = np.arange(len(f))
        period = np.where(k > 0, len(x) / np.maximum(k, 1), np.inf)
        mask = (k > 0) & (period >= 2) & (period <= 4000)
        return f[mask], period[mask]

    I, P = per(z)
    g = I.max() / I.sum()
    gs = np.empty(2000)
    for i in range(2000):
        a, _ = per(rng.permutation(z))
        gs[i] = a.max() / a.sum()
    p = float((np.sum(gs >= g) + 1) / 2001)
    best = float(P[I.argmax()])
    top = [dict(period=float(P[j]), power=float(I[j])) for j in np.argsort(I)[::-1][:5]]
    halves = []
    for h in (z[: n // 2], z[n // 2:]):
        t = np.arange(len(h))
        c, s = np.cos(2 * np.pi * t / best), np.sin(2 * np.pi * t / best)
        obs = (h @ c) ** 2 / len(h) + (h @ s) ** 2 / len(h)
        nul = np.empty(2000)
        for i in range(2000):
            q = rng.permutation(h)
            nul[i] = (q @ c) ** 2 / len(h) + (q @ s) ** 2 / len(h)
        halves.append(dict(power=float(obs), p95=float(np.percentile(nul, 95)), ok=bool(obs > np.percentile(nul, 95))))
    ok = bool(p < 0.005 and all(h["ok"] for h in halves))
    return dict(series=name, n=n, g=float(g), p=p, best_period=best, top5=top, halves=halves, found=ok)


def oos_cycle(m, period, min_obs):
    y = m["r"].dropna().to_numpy()
    s = np.arange(len(y))
    e_m, e_b, fadj = [], [], []
    for t in range(min_obs, len(y)):
        X = np.column_stack([np.ones(t), np.sin(2 * np.pi * s[:t] / period), np.cos(2 * np.pi * s[:t] / period)])
        beta = np.linalg.lstsq(X, y[:t], rcond=None)[0]
        xt = np.array([1, np.sin(2 * np.pi * t / period), np.cos(2 * np.pi * t / period)])
        ym, yb = xt @ beta, y[:t].mean()
        e_m.append(y[t] - ym)
        e_b.append(y[t] - yb)
        fadj.append((y[t] - yb) ** 2 - (y[t] - ym) ** 2 + (yb - ym) ** 2)
    e_m, e_b, fadj = np.array(e_m), np.array(e_b), np.array(fadj)
    r2 = 1 - (e_m ** 2).sum() / (e_b ** 2).sum()
    t = fadj.mean() / (fadj.std(ddof=1) / np.sqrt(len(fadj)))
    return dict(period=period, oos_r2=float(r2), cw_t=float(t), p=float(1 - norm.cdf(t)), n=int(len(fadj)))


def vr_test(r, q):
    r = r[~np.isnan(r)]
    n = len(r)
    mu = r.mean()
    d = r - mu
    var1 = (d ** 2).sum() / (n - 1)
    cs = np.cumsum(np.concatenate([[0], r]))
    sq = cs[q:] - cs[:-q]
    m = q * (n - q + 1) * (1 - q / n)
    varq = ((sq - q * mu) ** 2).sum() / m
    vr = varq / var1
    sumsq = (d ** 2).sum()
    theta = 0.0
    for j in range(1, q):
        dj = n * (d[j:] ** 2 * d[:-j] ** 2).sum() / sumsq ** 2
        theta += (2 * (q - j) / q) ** 2 * dj
    z = np.sqrt(n) * (vr - 1) / np.sqrt(theta)
    return float(vr), float(z)


def tsmom(m, a, b, cost=0.03):
    d = m.copy()
    d["mom"] = np.log(d["close"]).diff(12)
    d["sig"] = np.where(d["mom"] > 0, 1.0, 0.0)
    d["nxt"] = d["r"].shift(-1)
    d = d[(d["date"] >= a) & (d["date"] <= b)].dropna(subset=["nxt", "mom"])
    stat = hac(d["nxt"], d["sig"])
    pos = d["sig"].to_numpy()
    turn = np.abs(np.diff(np.concatenate([[pos[0]], pos])))
    sr = pos * d["nxt"].to_numpy() - turn * cost
    br = d["nxt"].to_numpy()

    def cagr(x):
        return float(np.exp(x.sum() / 100 * 12 / len(x)) - 1) * 100

    def dd(x):
        eq = np.exp(np.cumsum(x / 100))
        return float((eq / np.maximum.accumulate(eq) - 1).min() * 100)

    def sharpe(x):
        return float(x.mean() / x.std(ddof=1) * np.sqrt(12))

    res = dict(stat=stat, cagr_s=cagr(sr), cagr_b=cagr(br), dd_s=dd(sr), dd_b=dd(br), sharpe_s=sharpe(sr), sharpe_b=sharpe(br), invested=float(pos.mean()))
    res["tradable"] = bool(res["cagr_s"] >= res["cagr_b"])
    return res


def main(us_path, ru_path, out):
    us, ru = load(us_path), load(ru_path)
    res = dict(us_rows=len(us), ru_rows=len(ru), us_span=[str(us["date"].min().date()), str(us["date"].max().date())], ru_span=[str(ru["date"].min().date()), str(ru["date"].max().date())])
    cal = calendar_block(us, ru)
    res["calendar"] = cal
    res["calendar_verdict"] = {k: dict(zip(("found", "tradable"), found(v))) for k, v in cal.items() if k.startswith("S")}
    res["Y1"] = [periodogram_y1(us, "GSPC"), periodogram_y1(ru, "IMOEX")]
    um, rm = month_ends(us), month_ends(ru)
    y2 = []
    for nm, m, mo in (("GSPC", um, 360), ("IMOEX", rm, 120)):
        for P in (40, 48, 108):
            r = oos_cycle(m, P, mo)
            r["series"] = nm
            r["found"] = bool(r["oos_r2"] > 0 and r["p"] < 0.0083)
            y2.append(r)
    res["Y2"] = y2
    qs = [2, 5, 10, 21, 63, 252]
    subs = [("1928-01-01", "1952-12-31"), ("1953-01-01", "1977-12-31"), ("1978-01-01", "2001-12-31"), ("2002-01-01", "2026-10-14")]
    y3 = {}
    for q in qs:
        us_sub = [vr_test(sample(us, a, b)["r"].to_numpy(), q) for a, b in subs]
        ru_full = vr_test(ru["r"].to_numpy(), q)
        zs = [z for _, z in us_sub]
        pos = sum(1 for z in zs if z >= 3.0)
        neg = sum(1 for z in zs if z <= -3.0)
        sign = 1 if pos >= 3 else (-1 if neg >= 3 else 0)
        y3[q] = dict(us=[dict(vr=v, z=z) for v, z in us_sub], ru=dict(vr=ru_full[0], z=ru_full[1]), found=bool(sign != 0 and np.sign(ru_full[1]) == sign and abs(ru_full[1]) >= 2.0))
    res["Y3"] = y3
    res["Y4"] = dict(
        us_disc=tsmom(um, "1928-12-31", "1989-12-31"),
        us_conf=tsmom(um, "1990-01-01", "2026-09-30"),
        ru=tsmom(rm, "1998-10-31", "2026-09-30"),
    )
    y4 = res["Y4"]
    y4["found"] = bool(y4["us_disc"]["stat"]["t"] >= 2.6 and y4["us_conf"]["stat"]["t"] >= 2.0 and y4["ru"]["stat"]["t"] >= 2.0)
    y4["tradable"] = bool(y4["found"] and y4["us_conf"]["tradable"] and y4["ru"]["tradable"])
    y5 = {}
    for nm, df in (("GSPC", us), ("IMOEX", ru)):
        a = df["r"].abs().dropna().to_numpy()
        x = a - a.mean()
        ac = np.correlate(x, x, "full")[len(x) - 1:] / (x @ x)
        y5[nm] = dict(acf_abs={lag: float(ac[lag]) for lag in (1, 5, 21, 63, 250)})
        I = np.abs(np.fft.rfft(x)) ** 2 / len(x)
        P = len(x) / np.maximum(np.arange(len(I)), 1)
        mask = (P >= 5) & (P <= 4000)
        idx = np.argsort(np.where(mask, I, 0))[::-1][:5]
        y5[nm]["abs_top5_periods"] = [float(P[j]) for j in idx]
    res["Y5"] = y5
    json.dump(res, open(out, "w"), indent=1, default=float)
    print(json.dumps(res["calendar_verdict"], indent=1))
    for r in res["Y1"]:
        print("Y1", r["series"], "g", round(r["g"], 5), "p", r["p"], "best", round(r["best_period"], 1), "found", r["found"])
    for r in res["Y2"]:
        print("Y2", r["series"], r["period"], "R2", round(r["oos_r2"], 5), "p", round(r["p"], 4), r["found"])
    for q, v in res["Y3"].items():
        print("Y3 q", q, [round(x["z"], 2) for x in v["us"]], "ru", round(v["ru"]["z"], 2), v["found"])
    print("Y4", json.dumps(res["Y4"], default=float))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3])
