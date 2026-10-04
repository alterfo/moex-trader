#!/usr/bin/env python3
import argparse
import json
import math
import os

import numpy as np


def sigmoid(x):
    return 1.0 / (1.0 + np.exp(-np.asarray(x, dtype=np.float64)))


def reconstruct_xgboost(model):
    order = model["feature_order"]
    num_features = len(order)
    num_trees = len(model["xgb_trees"])
    base_score = sigmoid(model["xgb_base_logit"])
    model_json = {
        "version": [3, 4, 1],
        "learner": {
            "attributes": {},
            "feature_names": [],
            "feature_types": [],
            "gradient_booster": {
                "name": "gbtree",
                "model": {
                    "cats": {"enc": [], "feature_segments": [], "sorted_idx": []},
                    "gbtree_model_param": {"num_parallel_tree": "1", "num_trees": str(num_trees)},
                    "iteration_indptr": [0] + [i + 1 for i in range(num_trees)],
                    "tree_info": [0] * num_trees,
                    "trees": [],
                },
            },
            "learner_model_param": {
                "base_score": "[%s]" % repr(float(base_score)),
                "boost_from_average": "0",
                "num_class": "0",
                "num_feature": str(num_features),
                "num_target": "1",
            },
            "objective": {"name": "binary:logistic", "reg_loss_param": {"scale_pos_weight": "1"}},
        },
    }
    trees = model_json["learner"]["gradient_booster"]["model"]["trees"]
    for i, tree in enumerate(model["xgb_trees"]):
        split_indices = [int(x) for x in tree["si"]]
        split_conditions = [float(np.float32(x)) for x in tree["sc"]]
        left_children = [int(x) for x in tree["lc"]]
        right_children = [int(x) for x in tree["rc"]]
        default_left = [int(x) for x in tree["dl"]]
        num_nodes = len(left_children)
        base_weights = [split_conditions[j] if left_children[j] == -1 else 0.0 for j in range(num_nodes)]
        parents = [2147483647] * num_nodes
        for j in range(num_nodes):
            if left_children[j] != -1:
                parents[left_children[j]] = j
                parents[right_children[j]] = j
        trees.append({
            "base_weights": base_weights,
            "categories": [],
            "categories_nodes": [],
            "categories_segments": [],
            "categories_sizes": [],
            "default_left": default_left,
            "id": i,
            "left_children": left_children,
            "loss_changes": [0.0] * num_nodes,
            "parents": parents,
            "right_children": right_children,
            "split_conditions": split_conditions,
            "split_indices": split_indices,
            "split_type": [0] * num_nodes,
            "sum_hessian": [0.0] * num_nodes,
            "tree_param": {
                "num_deleted": "0",
                "num_feature": str(num_features),
                "num_nodes": str(num_nodes),
                "size_leaf_vector": "1",
            },
        })
    import xgboost as xgb
    booster = xgb.Booster()
    booster.load_model(bytearray(json.dumps(model_json), "utf-8"))
    return booster


def reconstruct_lightgbm(model):
    order = model["feature_order"]
    num_features = len(order)
    blocks = []
    for tree_index, tree in enumerate(model["lgb_trees"]):
        num_nodes = len(tree["lc"])
        split_indices = tree["si"]
        split_conditions = tree["sc"]
        left_children = tree["lc"]
        right_children = tree["rc"]
        internal = [j for j in range(num_nodes) if left_children[j] != -1]
        leaves = [j for j in range(num_nodes) if left_children[j] == -1]
        internal_idx = {node: i for i, node in enumerate(internal)}
        leaf_idx = {node: i for i, node in enumerate(leaves)}
        split_feature = []
        thresholds = []
        left_child = []
        right_child = []
        for node in internal:
            split_feature.append(str(split_indices[node]))
            thresholds.append(repr(float(split_conditions[node])))
            left = left_children[node]
            right = right_children[node]
            left_child.append(str(internal_idx[left]) if left in internal_idx else str(-(leaf_idx[left] + 1)))
            right_child.append(str(internal_idx[right]) if right in internal_idx else str(-(leaf_idx[right] + 1)))
        lines = [
            "Tree=%d" % tree_index,
            "num_leaves=%d" % len(leaves),
            "num_cat=0",
            "split_feature=" + " ".join(split_feature),
            "split_gain=" + " ".join(["0"] * len(internal)),
            "threshold=" + " ".join(thresholds),
            "decision_type=" + " ".join(["2"] * len(internal)),
            "left_child=" + " ".join(left_child),
            "right_child=" + " ".join(right_child),
            "leaf_value=" + " ".join(repr(float(split_conditions[node])) for node in leaves),
            "leaf_weight=" + " ".join(["0"] * len(leaves)),
            "leaf_count=" + " ".join(["0"] * len(leaves)),
            "internal_value=" + " ".join(["0"] * len(internal)),
            "internal_weight=" + " ".join(["0"] * len(internal)),
            "internal_count=" + " ".join(["0"] * len(internal)),
            "is_linear=0",
            "shrinkage=0.03",
            "",
            "",
        ]
        blocks.append("\n".join(lines))
    header = (
        "tree\n"
        "version=v4\n"
        "num_class=1\n"
        "num_tree_per_iteration=1\n"
        "label_index=0\n"
        "max_feature_idx=%d\n"
        "objective=binary sigmoid:1\n"
        "feature_names=%s\n"
        "feature_infos=%s\n"
        "tree_sizes=%s\n"
        "\n"
        % (
            num_features - 1,
            " ".join(order),
            " ".join(["[0:1]"] * num_features),
            " ".join(str(len(block)) for block in blocks),
        )
    )
    import lightgbm as lgb
    return lgb.Booster(model_str=header + "".join(blocks) + "end of trees\n")


def logistic_probability(model, vectors):
    weights = model["logistic"]
    mean = np.asarray(weights["mean"], dtype=np.float64)
    std = np.asarray(weights["std"], dtype=np.float64)
    coef = np.asarray(weights["coef"], dtype=np.float64)
    bias = float(weights["bias"])
    std = np.where(std == 0, 1.0, std)
    return sigmoid(bias + (vectors - mean) / std @ coef)


def pct_change(closes, n):
    if len(closes) < n + 1 or closes[-n - 1] <= 0:
        return 0.0
    return (closes[-1] - closes[-n - 1]) / closes[-n - 1] * 100.0


def sma(closes, window):
    if len(closes) < window:
        return 0.0
    return float(np.mean(closes[-window:]))


def rsi(closes, period):
    if len(closes) < period + 1:
        return 0.0
    gain = 0.0
    loss = 0.0
    for i in range(len(closes) - period, len(closes)):
        change = closes[i] - closes[i - 1]
        if change > 0:
            gain += change
        else:
            loss += -change
    gain /= period
    loss /= period
    if loss == 0:
        return 100.0
    rs = gain / loss
    return 100.0 - 100.0 / (1.0 + rs)


def realized_vol_pct(closes, window, bars_per_year):
    if len(closes) < window + 1:
        return 0.0
    rets = []
    for i in range(len(closes) - window, len(closes)):
        if closes[i - 1] <= 0 or closes[i] <= 0:
            continue
        rets.append((closes[i] - closes[i - 1]) / closes[i - 1])
    if len(rets) < 2:
        return 0.0
    variance = float(np.var(np.asarray(rets, dtype=np.float64), ddof=1))
    if variance <= 0:
        return 0.0
    return math.sqrt(variance) * math.sqrt(bars_per_year) * 100.0


def dist_from_ma_pct(closes, window):
    ma = sma(closes, window)
    if ma <= 0:
        return 0.0
    return (closes[-1] - ma) / ma * 100.0


def volume_zscore(volumes, window):
    if len(volumes) < window + 1:
        return 0.0
    hist = volumes[-window - 1:-1]
    if len(hist) < 2:
        return 0.0
    mean = float(np.mean(hist))
    sd = float(np.sqrt(np.mean((hist - mean) ** 2)))
    if sd <= 0:
        return 0.0
    return float((volumes[-1] - mean) / sd)


def ema(values, period):
    if len(values) < period:
        return None
    out = np.zeros(len(values), dtype=np.float64)
    out[period - 1] = float(np.mean(values[:period]))
    k = 2.0 / float(period + 1)
    for i in range(period, len(values)):
        out[i] = (values[i] - out[i - 1]) * k + out[i - 1]
    return out


def smma(values, period):
    if len(values) < period:
        return None
    out = np.zeros(len(values), dtype=np.float64)
    out[period - 1] = float(np.mean(values[:period]))
    for i in range(period, len(values)):
        out[i] = (out[i - 1] * (period - 1) + values[i]) / period
    return out


def macd_hist_pct(closes, fast, slow, signal):
    if len(closes) < slow + signal:
        return 0.0
    ema_fast = ema(closes, fast)
    ema_slow = ema(closes, slow)
    start = slow - 1
    macd_line = np.asarray([ema_fast[i] - ema_slow[i] for i in range(start, len(closes))], dtype=np.float64)
    if len(macd_line) < signal:
        return 0.0
    signal_line = ema(macd_line, signal)
    hist = macd_line[-1] - signal_line[-1]
    price = closes[-1]
    if price <= 0:
        return 0.0
    return float(hist / price * 100.0)


def stochastic_k(candles, period):
    if len(candles) < period:
        return 0.0
    window = candles[-period:]
    highest = max(c["high"] for c in window)
    lowest = min(c["low"] for c in window)
    value_range = highest - lowest
    if value_range <= 0:
        return 50.0
    close = candles[-1]["close"]
    return (close - lowest) / value_range * 100.0


def williams_r(candles, period):
    if len(candles) < period:
        return 0.0
    window = candles[-period:]
    highest = max(c["high"] for c in window)
    lowest = min(c["low"] for c in window)
    value_range = highest - lowest
    if value_range <= 0:
        return -50.0
    close = candles[-1]["close"]
    return (highest - close) / value_range * -100.0


def alligator_spread_pct(candles):
    jaw_bars = 13
    lips_bars = 5
    jaw_shift = 8
    lips_shift = 3
    if len(candles) < jaw_bars + jaw_shift:
        return 0.0
    median = np.asarray([(c["high"] + c["low"]) / 2.0 for c in candles], dtype=np.float64)
    jaw_series = smma(median, jaw_bars)
    lips_series = smma(median, lips_bars)
    jaw_idx = len(median) - 1 - jaw_shift
    lips_idx = len(median) - 1 - lips_shift
    if jaw_idx < jaw_bars - 1 or lips_idx < lips_bars - 1:
        return 0.0
    jaw_value = jaw_series[jaw_idx]
    if jaw_value <= 0:
        return 0.0
    return float((lips_series[lips_idx] - jaw_value) / jaw_value * 100.0)


def realized_volatility(candles):
    closes = np.asarray([c["close"] for c in candles], dtype=np.float64)
    rets = []
    for i in range(1, len(closes)):
        if closes[i - 1] <= 0 or closes[i] <= 0:
            continue
        rets.append((closes[i] - closes[i - 1]) / closes[i - 1] * 100.0)
    if len(rets) < 2:
        return 0.0
    variance = float(np.var(np.asarray(rets, dtype=np.float64), ddof=1))
    if variance <= 0:
        return 0.0
    return math.sqrt(variance)


def build_feature_vectors(order, fixture, tickers):
    event_features = {
        "event_dividend", "event_buyback", "event_sanctions", "event_ipo",
        "event_report", "event_delisting", "event_mna", "event_default",
    }
    rows = []
    for ticker in fixture["tickers"]:
        if ticker not in tickers:
            continue
        candles = [
            {
                "begin": c["begin"],
                "high": float(c["high"]),
                "low": float(c["low"]),
                "close": float(c["close"]),
                "volume": float(c["volume"]),
            }
            for c in fixture["candles"][ticker]
        ]
        for day_index in range(300, len(candles)):
            input_candles = candles[:day_index]
            pf_candles = input_candles[-300:]
            closes = np.asarray([c["close"] for c in pf_candles], dtype=np.float64)
            volumes = np.asarray([c["volume"] for c in pf_candles], dtype=np.float64)
            values = {
                "return_pct": pct_change(closes, 1),
                "realized_volatility": realized_volatility(pf_candles),
                "news_sentiment": 0.0,
                "news_count": 0.0,
                "order_book_imbalance": 0.0,
                "mom_5d": pct_change(closes, 5),
                "mom_21d": pct_change(closes, 21),
                "mom_63d": pct_change(closes, 63),
                "reversal_1d": pct_change(closes, 1),
                "rsi_14": rsi(closes, 14),
                "dist_ma20_pct": dist_from_ma_pct(closes, 20),
                "dist_ma50_pct": dist_from_ma_pct(closes, 50),
                "realized_vol_21d_annualized_pct": realized_vol_pct(closes, 21, 252),
                "volume_zscore_20d": volume_zscore(volumes, 20),
                "macd_hist_pct": macd_hist_pct(closes, 12, 26, 9),
                "stoch_k_14": stochastic_k(pf_candles, 14),
                "williams_r_14": williams_r(pf_candles, 14),
                "alligator_spread_pct": alligator_spread_pct(pf_candles),
            }
            vector = [values.get(name, 0.0) for name in order]
            rows.append({
                "ticker": ticker,
                "date": candles[day_index]["begin"][:10],
                "features": vector,
            })
    return rows


def main():
    parser = argparse.ArgumentParser(description="Dump a Go-vs-Python inference parity fixture")
    parser.add_argument("--model", default=os.environ.get("ENSEMBLE_MODEL", "ensemble_model.json"))
    parser.add_argument("--candles", default=os.environ.get("GOLDEN_CANDLES", "internal/backtest/testdata/golden/candles.json"))
    parser.add_argument("--out", default=os.environ.get("PARITY_OUT", "internal/model/testdata/parity.json"))
    parser.add_argument("--tickers", default=os.environ.get("PARITY_TICKERS", "SBER,LKOH,GAZP"))
    args = parser.parse_args()

    with open(args.model) as handle:
        model = json.load(handle)
    with open(args.candles) as handle:
        fixture = json.load(handle)

    order = model["feature_order"]
    tickers = [ticker.strip().upper() for ticker in args.tickers.split(",") if ticker.strip()]
    rows = build_feature_vectors(order, fixture, tickers)
    vectors = np.asarray([row["features"] for row in rows], dtype=np.float64)
    if vectors.shape[0] == 0:
        raise SystemExit("no feature vectors produced from the golden fixture")

    xgb_booster = reconstruct_xgboost(model)
    lgb_booster = reconstruct_lightgbm(model)
    p_xgb = xgb_booster.predict(_dmatrix(vectors))
    p_lgb = lgb_booster.predict(vectors)
    p_logistic = logistic_probability(model, vectors)
    p_ensemble = (p_xgb + p_lgb + p_logistic) / 3.0

    for row, px, pl, plr, pe in zip(rows, p_xgb, p_lgb, p_logistic, p_ensemble):
        row["xgb"] = float(px)
        row["lgb"] = float(pl)
        row["logistic"] = float(plr)
        row["ensemble"] = float(pe)

    output = {"feature_order": order, "rows": rows}
    os.makedirs(os.path.dirname(args.out) or ".", exist_ok=True)
    with open(args.out, "w") as handle:
        json.dump(output, handle, indent=2)
        handle.write("\n")
    print("wrote %s (%d rows, %d features)" % (args.out, len(rows), len(order)))


def _dmatrix(vectors):
    import xgboost as xgb
    return xgb.DMatrix(vectors)


if __name__ == "__main__":
    main()
