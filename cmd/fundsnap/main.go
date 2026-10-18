package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultBase = "https://invest-public-api.tbank.ru/rest/tinkoff.public.invest.api.contract.v1."

type record struct {
	Date         string          `json:"date"`
	FetchedAt    string          `json:"fetched_at"`
	Ticker       string          `json:"ticker"`
	AssetUID     string          `json:"asset_uid"`
	Fundamentals json.RawMessage `json:"fundamentals"`
	Reports      json.RawMessage `json:"reports,omitempty"`
}

type client struct {
	base  string
	token string
	http  *http.Client
}

func (c *client) call(ctx context.Context, service, method string, body interface{}) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+service+"/"+method, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			last = err
		} else {
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			switch {
			case readErr != nil:
				last = readErr
			case resp.StatusCode == http.StatusOK:
				return json.RawMessage(data), nil
			case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
				last = fmt.Errorf("%s/%s: http %d", service, method, resp.StatusCode)
			default:
				return nil, fmt.Errorf("%s/%s: http %d: %s", service, method, resp.StatusCode, truncate(string(data), 200))
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	return nil, last
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (c *client) identify(ctx context.Context, classCode, ticker string) (string, string, error) {
	raw, err := c.call(ctx, "InstrumentsService", "ShareBy", map[string]string{
		"idType":    "INSTRUMENT_ID_TYPE_TICKER",
		"classCode": classCode,
		"id":        ticker,
	})
	if err != nil {
		return "", "", err
	}
	var out struct {
		Instrument struct {
			UID      string `json:"uid"`
			AssetUID string `json:"assetUid"`
		} `json:"instrument"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", err
	}
	if out.Instrument.AssetUID == "" {
		return "", "", fmt.Errorf("no assetUid for %s", ticker)
	}
	return out.Instrument.UID, out.Instrument.AssetUID, nil
}

func (c *client) snapshot(ctx context.Context, classCode, ticker string, day time.Time, withReports bool) (record, error) {
	instrumentUID, uid, err := c.identify(ctx, classCode, ticker)
	if err != nil {
		return record{}, err
	}
	fund, err := c.call(ctx, "InstrumentsService", "GetAssetFundamentals", map[string][]string{"assets": {uid}})
	if err != nil {
		return record{}, err
	}
	rec := record{Date: day.Format("2006-01-02"), FetchedAt: time.Now().UTC().Format(time.RFC3339), Ticker: ticker, AssetUID: uid, Fundamentals: fund}
	if withReports {
		reports, err := c.call(ctx, "InstrumentsService", "GetAssetReports", map[string]string{
			"instrumentId": instrumentUID,
			"from":         day.AddDate(0, -3, 0).Format(time.RFC3339),
			"to":           day.AddDate(0, 6, 0).Format(time.RFC3339),
		})
		if err == nil {
			rec.Reports = reports
		} else {
			log.Printf("fundsnap: %s reports: %v", ticker, err)
		}
	}
	return rec, nil
}

func existing(path, day string) (map[string]bool, error) {
	done := map[string]bool{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return done, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r struct {
			Date   string `json:"date"`
			Ticker string `json:"ticker"`
		}
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Date == day {
			done[r.Ticker] = true
		}
	}
	return done, sc.Err()
}

func run(ctx context.Context, c *client, tickers []string, classCode, out string, day time.Time, withReports bool) (int, int, error) {
	done, err := existing(out, day.Format("2006-01-02"))
	if err != nil {
		return 0, 0, err
	}
	f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	ok, failed := 0, 0
	for _, t := range tickers {
		if done[t] {
			continue
		}
		rec, err := c.snapshot(ctx, classCode, t, day, withReports)
		if err != nil {
			log.Printf("fundsnap: %s: %v", t, err)
			failed++
			continue
		}
		line, err := json.Marshal(rec)
		if err != nil {
			return ok, failed, err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return ok, failed, err
		}
		ok++
		time.Sleep(300 * time.Millisecond)
	}
	return ok, failed, nil
}

func main() {
	tickers := flag.String("tickers", "YDEX,OZON,SBER,LKOH,GAZP,GMKN,ROSN,NVTK,TATN,MTSS,MGNT,PLZL,CHMF,T,VTBR,RUAL,POSI,DATA", "comma-separated tickers")
	classCode := flag.String("class-code", "TQBR", "MOEX board class code")
	out := flag.String("out", "data/fundamentals.jsonl", "append-only JSONL snapshot file")
	base := flag.String("base", defaultBase, "T-Invest REST base up to and including the contract version prefix")
	tokenEnv := flag.String("token-env", "T_TOKEN", "environment variable holding a read-only token")
	reports := flag.Bool("reports", true, "also store GetAssetReports (report calendar)")
	flag.Parse()

	token := strings.TrimSpace(os.Getenv(*tokenEnv))
	if token == "" {
		log.Fatalf("fundsnap: %s is empty", *tokenEnv)
	}
	c := &client{base: *base, token: token, http: &http.Client{Timeout: 30 * time.Second}}
	var list []string
	for _, t := range strings.Split(*tickers, ",") {
		if t = strings.TrimSpace(t); t != "" {
			list = append(list, t)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	ok, failed, err := run(ctx, c, list, *classCode, *out, time.Now(), *reports)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("fundsnap: stored %d snapshots, %d failed", ok, failed)
	if ok == 0 && failed > 0 {
		os.Exit(1)
	}
}
