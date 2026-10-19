import sys
import numpy as np
import pandas as pd
import intraday_study as st


def build(intraday_dir, reports):
    b = st.load_bars(intraday_dir)
    t = st.daily_table(b, st.ex_dates(reports))
    b["sv"] = np.sign(b["close"] - b["open"]) * b["value"]
    f = b.groupby(["ticker", "date"]).agg(sv=("sv", "sum"), v=("value", "sum")).reset_index()
    f["F"] = f["sv"] / f["v"].where(f["v"] > 0)
    t = t.merge(f[["ticker", "date", "F"]], on=["ticker", "date"], how="left")
    t["ret_day"] = np.log(t["close"] / t["prev_close"]) * 100
    t.loc[(t["date"] - t["prev_date"]).dt.days > 7, "ret_day"] = np.nan
    t["ar_next"] = t["nxt_close_ret"] - t.groupby("date")["nxt_close_ret"].transform("mean")
    return t


def zs(g, c):
    return (g[c] - g[c].mean()) / g[c].std()


def fm_ctrl(t):
    rows = []
    for d, g in t.groupby("date"):
        g = g.dropna(subset=["ar_next", "F", "ret_day"])
        if len(g) < 10:
            continue
        X = np.column_stack([np.ones(len(g)), zs(g, "F"), zs(g, "ret_day")])
        beta = np.linalg.lstsq(X, g["ar_next"].to_numpy(), rcond=None)[0]
        rows.append((d, beta[1], beta[2]))
    d = pd.DataFrame(rows, columns=["date", "bF", "bR"]).set_index("date")
    return st.halves(d["bF"]), st.halves(d["bR"])


def long_short(t, k=3):
    out = {}
    for d, g in t.groupby("date"):
        g = g.dropna(subset=["F", "nxt_close_ret"])
        if len(g) < 10:
            continue
        g = g.sort_values("F")
        out[d] = (g["nxt_close_ret"].iloc[-k:].mean() - g["nxt_close_ret"].iloc[:k].mean(), g["nxt_close_ret"].iloc[-k:].mean() - g["nxt_close_ret"].mean())
    s = pd.DataFrame(out, index=["ls", "long_vs_ew"]).T.sort_index()
    return s


def main(intraday_dir, reports):
    t = build(intraday_dir, reports)
    bF, bR = fm_ctrl(t)
    print("FM slope F with same-day return control:", bF)
    print("FM slope same-day return:", bR["all"])
    s = long_short(t)
    for name, cost in (("0.05%/side", 0.05), ("0.15%/side", 0.15)):
        net = s["ls"] - 4 * cost
        print("LS top3-bottom3 gross", st.halves(s["ls"])["all"], "| net", name, "(4 sides)", st.halves(net)["all"])
    lo = s["long_vs_ew"]
    print("long top3 minus EW gross", st.halves(lo)["all"], "net 0.05%", st.halves(lo - 2 * 0.05)["all"])
    print("halves LS gross", st.halves(s["ls"]))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
