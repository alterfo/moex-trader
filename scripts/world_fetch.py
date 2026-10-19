import sys, re, json, io, time
import subprocess
import pandas as pd

OUT = sys.argv[1]
CA = ".deploy/ca-bundle.pem"
H = {"User-Agent": "Mozilla/5.0"}


def get(url):
    r = subprocess.run(["curl", "-s", "--noproxy", "*", "-m", "90", "--cacert", CA, "-A", H["User-Agent"], url], capture_output=True, check=True)
    return r.stdout.decode("utf-8", "replace")


def iss_pages(url, key, cols):
    rows, start = [], 0
    while True:
        d = json.loads(get(f"{url}{'&' if '?' in url else '?'}start={start}&iss.meta=off"))[key]
        if not d["data"]:
            break
        rows += d["data"]
        start += len(d["data"])
    return pd.DataFrame(rows, columns=d["columns"])


def index_hist(secid):
    df = iss_pages(f"https://iss.moex.com/iss/history/engines/stock/markets/index/securities/{secid}.json?from=2013-01-01&till=2026-10-19", "history", None)
    df = df[["TRADEDATE", "CLOSE"]].rename(columns={"TRADEDATE": "date", "CLOSE": secid})
    return df.dropna().drop_duplicates("date")


def usdrub():
    df = iss_pages("https://iss.moex.com/iss/engines/currency/markets/selt/boards/CETS/securities/USD000UTSTOM/candles.json?from=2013-01-01&till=2026-10-19&interval=24", "candles", None)
    df["date"] = df["begin"].str[:10]
    return df[["date", "close"]].rename(columns={"close": "USDRUB"})


def keyrate():
    html = get("https://www.cbr.ru/hd_base/KeyRate/?UniDbQuery.Posted=True&UniDbQuery.From=01.01.2013&UniDbQuery.To=19.10.2026")
    rows = re.findall(r"<td>(\d\d\.\d\d\.\d{4})</td>\s*<td>([\d,]+)</td>", html)
    df = pd.DataFrame(rows, columns=["date", "KEYRATE"])
    df["date"] = pd.to_datetime(df["date"], format="%d.%m.%Y").dt.strftime("%Y-%m-%d")
    df["KEYRATE"] = df["KEYRATE"].str.replace(",", ".").astype(float)
    return df


def fred(sid, name):
    df = pd.read_csv(f"{OUT}/f_{sid}.csv")
    df.columns = ["date", name]
    df[name] = pd.to_numeric(df[name], errors="coerce")
    return df.dropna()


parts = []
for s in ["IMOEX", "RTSI", "MOEXOG", "MOEXFN", "MOEXMM", "MOEXTL", "MOEXCN", "MOEXEU", "MOEXCH", "MOEXTN", "RVI", "MOEXIT", "MOEXRE", "IMOEXW"]:
    try:
        parts.append(index_hist(s))
        print(s, len(parts[-1]), flush=True)
    except Exception as e:
        print(s, "fail", e, flush=True)
parts.append(usdrub())
parts.append(keyrate())
for sid, nm in [("DCOILBRENTEU", "BRENT"), ("SP500", "SPX"), ("VIXCLS", "VIX"), ("DGS10", "UST10"), ("DTWEXBGS", "DXY"), ("DEXUSEU", "EURUSD"), ("DHHNGSP", "HENRY")]:
    parts.append(fred(sid, nm))
wide = None
for p in parts:
    p = p.drop_duplicates("date")
    wide = p if wide is None else wide.merge(p, on="date", how="outer")
wide = wide.sort_values("date")
wide.to_csv(f"{OUT}/world_macro.csv", index=False)
print(wide.shape, wide.isna().mean().round(2).to_dict())
