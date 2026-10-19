import glob
import json
import os
import sys
import numpy as np
import pandas as pd
import statsmodels.api as sm

SPLIT = pd.Timestamp("2018-12-31")
COST = 0.05
PREF = {"SBERP": "SBER", "TATNP": "TATN", "SNGSP": "SNGS"}


def hac_mean(x, lags=5):
    x = pd.Series(x).dropna()
    if len(x) < 30:
        return dict(mean=np.nan, t=np.nan, n=len(x))
    m = sm.OLS(x.to_numpy(float), np.ones(len(x))).fit(cov_type="HAC", cov_kwds={"maxlags": lags})
    return dict(mean=float(m.params[0]), t=float(m.tvalues[0]), n=int(len(x)))


def halves(s):
    s = s.dropna()
    return dict(all=hac_mean(s), h1=hac_mean(s[s.index <= SPLIT]), h2=hac_mean(s[s.index > SPLIT]))


def load_bars(d):
    frames = []
    for f in sorted(glob.glob(os.path.join(d, "*.csv"))):
        t = os.path.basename(f)[:-4]
        df = pd.read_csv(f)
        df["ticker"] = "YDEX" if t == "YNDX" else t
        frames.append(df)
    b = pd.concat(frames, ignore_index=True)
    b["begin"] = pd.to_datetime(b["begin"])
    b["date"] = b["begin"].dt.normalize()
    b["hm"] = b["begin"].dt.hour * 60 + b["begin"].dt.minute
    b = b[(b["hm"] >= 600) & (b["hm"] <= 1110)].copy()
    b = b[(b["open"] > 0) & (b["close"] > 0)]
    cnt = b.groupby(["ticker", "date"]).size().rename("n").reset_index()
    cnt["year"] = cnt["date"].dt.year
    med = cnt.groupby(["ticker", "year"])["n"].transform("median")
    ok = cnt[cnt["n"] >= 0.8 * med][["ticker", "date"]]
    b = b.merge(ok, on=["ticker", "date"])
    b = b[~((b["date"] >= "2022-02-25") & (b["date"] <= "2022-03-28"))]
    return b.sort_values(["ticker", "begin"]).reset_index(drop=True)


def ex_dates(path):
    out = {}
    for line in open(path):
        d = json.loads(line)
        ds = [pd.Timestamp(x["lastBuyDate"][:10]) for x in (d.get("dividends") or {}).get("dividends", []) if x.get("lastBuyDate")]
        out["YDEX" if d["ticker"] == "YNDX" else d["ticker"]] = ds
    return out


def daily_table(b, divs):
    g = b.groupby(["ticker", "date"])
    first = b[b["hm"] == 600].set_index(["ticker", "date"])
    rows = pd.DataFrame(index=g.size().index)
    rows["open"] = first["open"].reindex(rows.index)
    rows["close"] = g["close"].last()
    rows["c1020"] = b[b["hm"] == 620].set_index(["ticker", "date"])["close"].reindex(rows.index)
    rows["o1800"] = b[b["hm"] == 1080].set_index(["ticker", "date"])["open"].reindex(rows.index)
    rows["c1820"] = b[b["hm"] == 1100].set_index(["ticker", "date"])["close"].reindex(rows.index)
    rows["r_first"] = np.log(rows["c1020"] / rows["open"]) * 100
    rows["r_last"] = np.log(rows["c1820"] / rows["o1800"]) * 100
    rows["intra"] = np.log(rows["close"] / rows["open"]) * 100
    rows = rows.reset_index().sort_values(["ticker", "date"])
    rows["prev_close"] = rows.groupby("ticker")["close"].shift(1)
    rows["prev_date"] = rows.groupby("ticker")["date"].shift(1)
    rows["over"] = np.log(rows["open"] / rows["prev_close"]) * 100
    rows.loc[(rows["date"] - rows["prev_date"]).dt.days > 7, "over"] = np.nan
    rows.loc[rows["over"].abs() > 25, "over"] = np.nan
    rows["idx"] = rows.groupby("ticker").cumcount()
    bad = set()
    for t, g2 in rows.groupby("ticker"):
        key = PREF.get(t, t)
        for d in divs.get(key, []):
            pos = g2["date"].searchsorted(d, side="right")
            for k in (pos, pos + 1):
                if k < len(g2):
                    bad.add((t, g2["date"].iloc[k]))
    mask = [(t, d) in bad for t, d in zip(rows["ticker"], rows["date"])]
    rows.loc[mask, "over"] = np.nan
    rows["nxt_close_ret"] = np.log(rows.groupby("ticker")["close"].shift(-1) / rows["close"]) * 100
    nd = rows.groupby("ticker")["date"].shift(-1)
    rows.loc[(nd - rows["date"]).dt.days > 7, "nxt_close_ret"] = np.nan
    return rows


def fm(df, ycol, xcol, minn=10):
    out = {}
    for d, g in df.groupby("date"):
        g = g.dropna(subset=[ycol, xcol])
        if len(g) < minn or g[xcol].std() == 0:
            continue
        x = g[xcol].to_numpy()
        x = (x - x.mean()) / x.std()
        y = g[ycol].to_numpy()
        out[d] = float(np.polyfit(x, y, 1)[0])
    if not out:
        nan = dict(mean=np.nan, t=np.nan, n=0)
        return dict(all=nan, h1=nan, h2=nan)
    return halves(pd.Series(out).sort_index())


def main(intraday_dir, reports, out):
    b = load_bars(intraday_dir)
    divs = ex_dates(reports)
    res = dict(bars=int(len(b)), tickers=int(b["ticker"].nunique()), span=[str(b["date"].min().date()), str(b["date"].max().date())])
    t = daily_table(b, divs)
    res["ticker_days"] = int(len(t))

    p = t.dropna(subset=["r_first", "r_last"])
    cl = sm.OLS(p["r_last"].to_numpy(float), sm.add_constant(p["r_first"].to_numpy(float))).fit(cov_type="cluster", cov_kwds={"groups": pd.factorize(p["date"])[0]})
    i1 = dict(coef=float(cl.params[1]), t=float(cl.tvalues[1]), n=int(len(p)))
    for nm, sub in (("h1", p[p["date"] <= SPLIT]), ("h2", p[p["date"] > SPLIT])):
        m = sm.OLS(sub["r_last"].to_numpy(float), sm.add_constant(sub["r_first"].to_numpy(float))).fit(cov_type="cluster", cov_kwds={"groups": pd.factorize(sub["date"])[0]})
        i1[nm] = dict(coef=float(m.params[1]), t=float(m.tvalues[1]), n=int(len(sub)))
    gross = (np.sign(p["r_first"]) * p["r_last"]).groupby(p["date"]).mean()
    i1["strategy_gross"] = halves(gross)
    i1["strategy_net"] = halves(gross - 2 * COST)
    i1["found"] = bool(i1["coef"] > 0 and i1["t"] >= 2.6 and i1["h1"]["coef"] > 0 and i1["h2"]["coef"] > 0)
    i1["tradable"] = bool(i1["found"] and i1["strategy_net"]["all"]["mean"] > 0)
    res["I1"] = i1

    ov = t.dropna(subset=["over", "intra"])
    ds = ov.groupby("date").agg(over=("over", "mean"), intra=("intra", "mean"))
    diff = ds["over"] - ds["intra"]
    i2 = dict(over=halves(ds["over"]), intra=halves(ds["intra"]), diff=halves(diff), over_net=halves(ds["over"] - 2 * COST))
    i2["found"] = bool(i2["diff"]["all"]["t"] >= 2.6 and i2["diff"]["h1"]["mean"] > 0 and i2["diff"]["h2"]["mean"] > 0)
    i2["tradable"] = bool(i2["found"] and i2["over_net"]["all"]["mean"] > 0)
    res["I2"] = i2

    b["ret"] = np.log(b["close"] / b["open"]) * 100
    slots = {}
    for hm, g in b.groupby("hm"):
        s = g.groupby("date")["ret"].mean()
        h = halves(s)
        slot = dict(h)
        slot["found"] = bool(abs(h["all"]["t"]) >= 3.5 and np.sign(h["h1"]["mean"]) == np.sign(h["h2"]["mean"]) == np.sign(h["all"]["mean"]))
        slot["abs_mean"] = float(g["ret"].abs().mean())
        slot["value_share"] = float(g["value"].sum())
        slots[f"{hm // 60:02d}:{hm % 60:02d}"] = slot
    tot = sum(v["value_share"] for v in slots.values())
    for v in slots.values():
        v["value_share"] /= tot
    res["I3"] = slots

    b["sv"] = np.sign(b["close"] - b["open"]) * b["value"]
    full = b.groupby(["ticker", "date"]).agg(sv=("sv", "sum"), v=("value", "sum")).reset_index()
    full["F"] = full["sv"] / full["v"].where(full["v"] > 0)
    am = b[b["hm"] <= 710].groupby(["ticker", "date"]).agg(sv=("sv", "sum"), v=("value", "sum")).reset_index()
    am["F1"] = am["sv"] / am["v"].where(am["v"] > 0)
    t = t.merge(full[["ticker", "date", "F"]], on=["ticker", "date"], how="left").merge(am[["ticker", "date", "F1"]], on=["ticker", "date"], how="left")
    t["ar_next"] = t["nxt_close_ret"] - t.groupby("date")["nxt_close_ret"].transform("mean")
    o720 = b[b["hm"] == 720].set_index(["ticker", "date"])["open"]
    t["o1200"] = o720.reindex(pd.MultiIndex.from_frame(t[["ticker", "date"]])).to_numpy()
    t["rest"] = np.log(t["close"] / t["o1200"]) * 100
    t["ar_rest"] = t["rest"] - t.groupby("date")["rest"].transform("mean")
    i4a = fm(t, "ar_next", "F")
    i4a["found"] = bool(abs(i4a["all"]["t"]) >= 2.6 and np.sign(i4a["h1"]["mean"]) == np.sign(i4a["h2"]["mean"]) == np.sign(i4a["all"]["mean"]))
    i4b = fm(t, "ar_rest", "F1")
    i4b["found"] = bool(abs(i4b["all"]["t"]) >= 2.6 and np.sign(i4b["h1"]["mean"]) == np.sign(i4b["h2"]["mean"]) == np.sign(i4b["all"]["mean"]))
    res["I4a"], res["I4b"] = i4a, i4b
    json.dump(res, open(out, "w"), indent=1, default=float)
    print("bars", res["bars"], "tickers", res["tickers"], "ticker-days", res["ticker_days"], res["span"])
    print("I1", json.dumps({k: v for k, v in i1.items() if k not in ("strategy_gross",)}, default=float)[:900])
    print("I2", json.dumps({k: v["all"] if isinstance(v, dict) and "all" in v else v for k, v in i2.items()}, default=float)[:900])
    print("I3 found slots", [k for k, v in slots.items() if v["found"]])
    print("I4a", json.dumps(i4a, default=float)); print("I4b", json.dumps(i4b, default=float))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3])
