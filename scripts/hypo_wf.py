import sys
import json
import itertools
import numpy as np
import pandas as pd
import lightgbm as lgb
from joblib import Parallel, delayed
from scipy.stats import spearmanr
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import roc_auc_score
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

PRICE = ["return_pct", "realized_volatility", "mom_5d", "mom_21d", "mom_63d", "reversal_1d", "rsi_14", "dist_ma20_pct", "dist_ma50_pct", "realized_vol_21d_annualized_pct", "volume_zscore_20d", "macd_hist_pct", "stoch_k_14", "williams_r_14", "alligator_spread_pct"]
FX = {"GLDRUB_TOM", "SLVRUB_TOM", "CNYRUB_TOM"}
DEADBAND = 0.5
HORIZON = 10
FIRST_TEST = pd.Timestamp("2025-04-01")
TEST_DAYS = 63
TRAIN_START = pd.Timestamp("2024-01-01")


def load(data_path, prices_path):
    df = pd.read_csv(data_path)
    df = df[~df.ticker.isin(FX) & df.label.notna()].copy()
    df["date"] = pd.to_datetime(df["date"])
    df["label_date"] = pd.to_datetime(df["label_date"])
    px = pd.read_csv(prices_path, parse_dates=["date"])
    out = []
    for t, g in px.groupby("ticker"):
        g = g.sort_values("date").reset_index(drop=True)
        g["next_open"] = g["open"].shift(-1)
        g["next_date"] = g["date"].shift(-1)
        ret = g["close"].pct_change()
        g["sigma20"] = ret.shift(1).rolling(20).std()
        for k in (5, 21, 63):
            g[f"pmom{k}"] = (g["close"].shift(1) / g["close"].shift(1 + k) - 1) * 100
        out.append(g.assign(ticker=t))
    px = pd.concat(out)
    ent = px[["ticker", "date", "next_open", "sigma20", "pmom5", "pmom21", "pmom63"]]
    ext = px[["ticker", "date", "close"]].rename(columns={"date": "label_date", "close": "exit_close"})
    df = df.merge(ent, on=["ticker", "date"], how="left").merge(ext, on=["ticker", "label_date"], how="left")
    df["fwd"] = (df["exit_close"] / df["next_open"] - 1) * 100
    idx = px[px.ticker == "IMOEX"].set_index("date").sort_index()
    for k in (5, 21, 63):
        s = (idx["close"].shift(1) / idx["close"].shift(1 + k) - 1) * 100
        df[f"imoex_mom{k}"] = df["date"].map(s)
    df, new = add_rank_features(df)
    df = df[df["fwd"].notna()].copy()
    df["orig"] = (df["fwd"] > 0).astype(int)
    df["keep"] = df["fwd"].abs() >= DEADBAND
    return df.reset_index(drop=True), new


def check_labels(df):
    return float((df["orig"] == df["label"].astype(int)).mean())


def add_rank_features(df):
    out = df.copy()
    new = []
    for f in PRICE:
        name = f"rk_{f}"
        out[name] = out.groupby("date")[f].rank(pct=True)
        new.append(name)
    for k in (5, 21, 63):
        name = f"rel_mom{k}"
        out[name] = out[f"pmom{k}"] - out[f"imoex_mom{k}"]
        new.append(name)
    return out, new


def models(X, y, Xt, seed):
    m1 = lgb.LGBMClassifier(n_estimators=300, learning_rate=0.03, num_leaves=15, colsample_bytree=0.8, subsample=0.8, subsample_freq=1, random_state=seed, verbose=-1, n_jobs=1).fit(X, y)
    m2 = make_pipeline(StandardScaler(), LogisticRegression(C=1.0, max_iter=2000)).fit(X, y)
    return (m1.predict_proba(Xt)[:, 1] + m2.predict_proba(Xt)[:, 1]) / 2


def folds(df, shift):
    start = FIRST_TEST + pd.Timedelta(days=shift)
    end = df.date.max()
    while start < end:
        stop = start + pd.Timedelta(days=TEST_DAYS)
        te = df[(df.date >= start) & (df.date < stop)]
        tr = df[(df.label_date < start) & (df.date >= TRAIN_START)]
        if len(te) > 50 and te["orig"].nunique() == 2 and len(tr) > 500:
            yield tr, te
        start = stop


def oor_share(tr, te, feats):
    mu = tr[feats].mean()
    sd = tr[feats].std().replace(0, 1)
    z = ((te[feats] - mu) / sd).abs()
    return float((z > 12).any(axis=1).mean())


def evaluate(df, cfg, kind, feats, base_feats):
    seed, shift = cfg
    ps, ys, fw, oor_b, oor_v, n = [], [], [], [], [], 0
    sel_meta, sel_base = [], []
    for tr, te in folds(df, shift):
        if kind == "sigma":
            thr = 0.5 * tr["sigma20"] * np.sqrt(HORIZON) * 100
            trs = tr[(tr["fwd"].abs() >= thr) & tr["sigma20"].notna()]
            y = (trs["fwd"] > 0).astype(int).to_numpy()
            p = models(trs[feats].to_numpy(float), y, te[feats].to_numpy(float), seed)
        elif kind == "meta":
            p, m = meta_predict(tr, te, feats, seed)
            ps.append(p)
            ys.append(te["orig"].to_numpy())
            fw.append(te["fwd"].to_numpy())
            sign = np.where(p > 0.5, 1.0, -1.0)
            k = max(1, int(0.3 * len(te)))
            top_b = np.argsort(-np.abs(p - 0.5))[:k]
            top_m = np.argsort(-m)[:k]
            sel_base.append((sign * te["fwd"].to_numpy())[top_b])
            sel_meta.append((sign * te["fwd"].to_numpy())[top_m])
            continue
        else:
            p = models(tr[feats].to_numpy(float), tr["orig"].to_numpy(), te[feats].to_numpy(float), seed)
        ps.append(p)
        ys.append(te["orig"].to_numpy())
        fw.append(te["fwd"].to_numpy())
        oor_b.append(oor_share(tr, te, base_feats))
        oor_v.append(oor_share(tr, te, feats))
        n += len(te)
    P, Y, F = np.concatenate(ps), np.concatenate(ys), np.concatenate(fw)
    res = dict(seed=seed, shift=shift, auc=float(roc_auc_score(Y, P)), rho=float(spearmanr(P, F)[0]), rows=int(len(P)))
    if kind == "meta":
        res["top_base"] = float(np.concatenate(sel_base).mean())
        res["top_meta"] = float(np.concatenate(sel_meta).mean())
    else:
        res["oor_base"] = float(np.mean(oor_b))
        res["oor_var"] = float(np.mean(oor_v))
    return res


def meta_predict(tr, te, feats, seed):
    tr = tr.sort_values("date")
    cut = tr["date"].quantile(0.6)
    gap = pd.Timedelta(days=15)
    a = tr[tr["label_date"] < cut]
    b = tr[tr["date"] >= cut + gap]
    pa = models(a[feats].to_numpy(float), a["orig"].to_numpy(), b[feats].to_numpy(float), seed)
    mf = meta_features(b, pa)
    yb = ((pa > 0.5).astype(int) == b["orig"].to_numpy()).astype(int)
    mm = make_pipeline(StandardScaler(), LogisticRegression(C=1.0, max_iter=2000)).fit(mf, yb)
    p = models(tr[feats].to_numpy(float), tr["orig"].to_numpy(), te[feats].to_numpy(float), seed)
    return p, mm.predict_proba(meta_features(te, p))[:, 1]


def meta_features(d, p):
    return np.column_stack([np.abs(p - 0.5), d["realized_vol_21d_annualized_pct"].to_numpy(float), d["mom_21d"].to_numpy(float), d["imoex_mom5"].to_numpy(float), d["dist_ma50_pct"].to_numpy(float)])


FUTOI_MAP = {"YDEX": "YD", "SBER": "SR", "LKOH": "LK", "GAZP": "GZ", "GMKN": "GK", "ROSN": "RN", "TATN": "TN", "MGNT": "MG", "VTBR": "VB"}
FUTOI_FEATS = ["own_fiz_d5", "own_fiz_lvl", "mx_fiz_d5", "mx_fiz_lvl"]


def futoi_daily(path):
    f = pd.read_csv(path)
    f["date"] = pd.to_datetime(f["date"], errors="coerce", format="%Y-%m-%d")
    f = f[f["date"].notna() & (f["group"] == "FIZ")].copy()
    f["oi"] = f["pos_long"] + f["pos_short"].abs()
    f = f.sort_values(["series", "date"])
    g = f.groupby("series")
    f["fiz_lvl"] = f["pos"] / f["oi"]
    f["fiz_d5"] = (f["pos"] - g["pos"].shift(5)) / f["oi"]
    return f[["series", "date", "fiz_lvl", "fiz_d5"]].dropna()


def add_futoi(df, path, lag_days=0):
    f = futoi_daily(path)
    f["date"] = f["date"] + pd.Timedelta(days=lag_days)
    parts = []
    mx = f[f["series"] == "MX"].sort_values("date").rename(columns={"fiz_lvl": "mx_fiz_lvl", "fiz_d5": "mx_fiz_d5"})[["date", "mx_fiz_lvl", "mx_fiz_d5"]]
    for t, ser in FUTOI_MAP.items():
        d = df[df["ticker"] == t].sort_values("date")
        own = f[f["series"] == ser].sort_values("date").rename(columns={"fiz_lvl": "own_fiz_lvl", "fiz_d5": "own_fiz_d5"})[["date", "own_fiz_lvl", "own_fiz_d5"]]
        d = pd.merge_asof(d, own, on="date", allow_exact_matches=False)
        d = pd.merge_asof(d.sort_values("date"), mx, on="date", allow_exact_matches=False)
        parts.append(d)
    return pd.concat(parts).reset_index(drop=True)


def zero_shot(df, score_col):
    res = []
    for seed, shift in itertools.product([0], [0, 27, 54]):
        rows = pd.concat([te for _, te in folds(df, shift)])
        res.append(dict(shift=shift, auc=float(roc_auc_score(rows["orig"], rows[score_col])), rho=float(spearmanr(rows[score_col], rows["fwd"])[0]), rows=int(len(rows))))
    rows = pd.concat([te for _, te in folds(df, 0)])
    rng = np.random.default_rng(20261019)
    blocks = (rows["date"].rank(method="dense").astype(int) // 10).to_numpy()
    ub = np.unique(blocks)
    idx = {b: np.where(blocks == b)[0] for b in ub}
    aucs = []
    for _ in range(1000):
        pick = rng.choice(ub, size=len(ub), replace=True)
        sel = np.concatenate([idx[b] for b in pick])
        y = rows["orig"].to_numpy()[sel]
        if y.min() == y.max():
            continue
        aucs.append(roc_auc_score(y, rows[score_col].to_numpy()[sel]))
    return res, [float(np.percentile(aucs, 2.5)), float(np.percentile(aucs, 97.5))]


def main():
    mode, data_path, prices_path, out = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
    extra = sys.argv[5] if len(sys.argv) > 5 else None
    full, new = load(data_path, prices_path)
    df = full[full["keep"]].reset_index(drop=True)
    print("rows", len(df), "label agreement", round(check_labels(df), 4), flush=True)
    base_feats = PRICE
    if mode == "rank":
        var_feats = PRICE + new
        kind = "plain"
    elif mode == "sigma":
        var_feats = PRICE
        kind = "sigma"
    elif mode == "meta":
        var_feats = PRICE
        kind = "meta"
    elif mode in ("futoi", "futoi14"):
        df = add_futoi(df, extra, 14 if mode == "futoi14" else 0)
        var_feats = PRICE + FUTOI_FEATS
        kind = "plain"
    elif mode == "tfm":
        t = pd.read_csv(extra, parse_dates=["date"])
        df = df.merge(t, on=["ticker", "date"], how="inner")
        var_feats = PRICE + ["tfm_ret", "tfm_iqr", "tfm_pos"]
        kind = "plain"
        zs, ci = zero_shot(df, "tfm_ret")
        print("zero-shot tfm_ret", zs, "auc95ci(shift0)", [round(x, 4) for x in ci], flush=True)
        json.dump(dict(zero_shot=zs, ci=ci), open(out + ".zs", "w"))
    else:
        raise SystemExit("mode")
    df = df.dropna(subset=list(set(var_feats + base_feats + ["imoex_mom5"]))).reset_index(drop=True)
    print("analysis rows", len(df), "tickers", df["ticker"].nunique(), flush=True)
    cfgs = list(itertools.product(range(5), [0, 27, 54]))
    base = Parallel(n_jobs=6)(delayed(evaluate)(df, c, "plain", base_feats, base_feats) for c in cfgs)
    var = Parallel(n_jobs=6)(delayed(evaluate)(df, c, kind, var_feats, base_feats) for c in cfgs)
    json.dump(dict(base=base, var=var, label_agreement=check_labels(df)), open(out, "w"))
    d = np.array([v["auc"] - b["auc"] for b, v in zip(base, var)])
    print(mode, "base auc", round(np.mean([b["auc"] for b in base]), 4), "var auc", round(np.mean([v["auc"] for v in var]), 4), "mean d", round(d.mean(), 4), "pos", int((d > 0).sum()), "of", len(d))
    if mode == "meta":
        t = np.array([v["top_meta"] - v["top_base"] for v in var])
        print("meta top30 mean signed fwd: base", round(np.mean([v["top_base"] for v in var]), 4), "meta", round(np.mean([v["top_meta"] for v in var]), 4), "mean d", round(t.mean(), 4), "pos", int((t > 0).sum()), "of", len(t))


if __name__ == "__main__" and (len(sys.argv) < 2 or sys.argv[1] != "probs"):
    main()


WINDOWS = [("q1", "2025-03-05", "2025-06-05"), ("q2", "2025-06-06", "2025-09-05"), ("q3", "2025-09-06", "2025-12-03"), ("q4", "2025-12-04", "2026-03-04"), ("w2", "2026-03-05", "2026-06-06"), ("w1", "2026-06-07", "2026-09-16"), ("h", "2026-09-17", "2026-10-17"), ("G", "2026-07-20", "2026-10-18")]


def parse_arm(spec):
    name, seed = spec.split("@")
    if name == "base":
        return name, None, int(seed)
    if name == "rank":
        return name, None, int(seed)
    k = {"sigma": 0.5, "sigma25": 0.25, "sigma75": 0.75}[name]
    return name, k, int(seed)


def window_probs(data_path, prices_path, out_path, specs):
    full, new = load(data_path, prices_path)
    full = full.dropna(subset=PRICE + new + ["imoex_mom5"]).reset_index(drop=True)
    rows = []
    for name, start, stop in WINDOWS:
        s, e = pd.Timestamp(start), pd.Timestamp(stop)
        tr = full[(full["label_date"] < s) & (full["date"] >= TRAIN_START) & full["keep"]]
        te = full[(full["date"] >= s) & (full["date"] <= e)]
        out = te[["ticker", "date"]].copy()
        for spec in specs:
            arm, k, seed = parse_arm(spec)
            col = "p_" + arm + "_s" + str(seed)
            if arm == "base":
                out[col] = models(tr[PRICE].to_numpy(float), tr["orig"].to_numpy(), te[PRICE].to_numpy(float), seed)
            elif arm == "rank":
                f = PRICE + new
                out[col] = models(tr[f].to_numpy(float), tr["orig"].to_numpy(), te[f].to_numpy(float), seed)
            else:
                thr = k * tr["sigma20"] * np.sqrt(HORIZON) * 100
                trs = tr[(tr["fwd"].abs() >= thr) & tr["sigma20"].notna()]
                out[col] = models(trs[PRICE].to_numpy(float), (trs["fwd"] > 0).astype(int).to_numpy(), te[PRICE].to_numpy(float), seed)
        out["window"] = name
        rows.append(out)
    pd.concat(rows).to_csv(out_path, index=False)


if __name__ == "__main__" and len(sys.argv) > 1 and sys.argv[1] == "probs":
    window_probs(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5].split(","))
