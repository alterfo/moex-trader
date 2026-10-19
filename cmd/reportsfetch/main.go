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

type client struct {
	base  string
	token string
	http  *http.Client
}

func (c *client) call(ctx context.Context, service, method string, body interface{}) (json.RawMessage, int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+service+"/"+method, bytes.NewReader(payload))
		if err != nil {
			return nil, 0, err
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
				return data, resp.StatusCode, nil
			case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
				last = fmt.Errorf("http %d", resp.StatusCode)
			default:
				return nil, resp.StatusCode, fmt.Errorf("%s/%s: http %d: %s", service, method, resp.StatusCode, strings.TrimSpace(string(data)))
			}
		}
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 3 * time.Second):
		}
	}
	return nil, 0, last
}

func (c *client) uid(ctx context.Context, classCode, ticker string) (string, error) {
	raw, _, err := c.call(ctx, "InstrumentsService", "ShareBy", map[string]string{"idType": "INSTRUMENT_ID_TYPE_TICKER", "classCode": classCode, "id": ticker})
	if err != nil {
		return "", err
	}
	var out struct {
		Instrument struct {
			UID string `json:"uid"`
		} `json:"instrument"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Instrument.UID == "" {
		return "", fmt.Errorf("no uid")
	}
	return out.Instrument.UID, nil
}

type record struct {
	Ticker    string          `json:"ticker"`
	UID       string          `json:"uid"`
	Reports   json.RawMessage `json:"reports"`
	Dividends json.RawMessage `json:"dividends"`
}

func main() {
	list := flag.String("tickers-file", "", "file with one ticker per line")
	classCode := flag.String("class-code", "TQBR", "board class code")
	out := flag.String("out", "data/reports_history.jsonl", "output JSONL")
	from := flag.String("from", "2014-01-01", "history start")
	to := flag.String("to", "2027-06-30", "history end")
	pause := flag.Duration("pause", 400*time.Millisecond, "pause between calls")
	tokenEnv := flag.String("token-env", "T_TOKEN", "token env var")
	flag.Parse()

	token := strings.TrimSpace(os.Getenv(*tokenEnv))
	if token == "" {
		log.Fatalf("reportsfetch: %s is empty", *tokenEnv)
	}
	file, err := os.Open(*list)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	var tickers []string
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			tickers = append(tickers, t)
		}
	}
	c := &client{base: defaultBase, token: token, http: &http.Client{Timeout: 40 * time.Second}}
	outFile, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()
	done := map[string]bool{}
	if data, err := os.ReadFile(*out); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			var r struct {
				Ticker string `json:"ticker"`
			}
			if json.Unmarshal([]byte(line), &r) == nil && r.Ticker != "" {
				done[r.Ticker] = true
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	window := map[string]string{"from": *from + "T00:00:00Z", "to": *to + "T00:00:00Z"}
	ok, failed := 0, 0
	for _, t := range tickers {
		if done[t] {
			continue
		}
		time.Sleep(*pause)
		uid, err := c.uid(ctx, *classCode, t)
		if err != nil {
			failed++
			continue
		}
		time.Sleep(*pause)
		reports, _, err := c.call(ctx, "InstrumentsService", "GetAssetReports", map[string]string{"instrumentId": uid, "from": window["from"], "to": window["to"]})
		if err != nil {
			reports = json.RawMessage(`{}`)
		}
		time.Sleep(*pause)
		divs, _, err := c.call(ctx, "InstrumentsService", "GetDividends", map[string]string{"instrumentId": uid, "from": window["from"], "to": window["to"]})
		if err != nil {
			divs = json.RawMessage(`{}`)
		}
		line, _ := json.Marshal(record{Ticker: t, UID: uid, Reports: compact(reports), Dividends: compact(divs)})
		outFile.Write(append(line, '\n'))
		ok++
	}
	log.Printf("reportsfetch: stored %d, unresolved %d", ok, failed)
}

func compact(raw json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return json.RawMessage(`{}`)
	}
	return buf.Bytes()
}
