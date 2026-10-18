package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newServer(t *testing.T, fail map[string]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/ShareBy"):
			id, _ := body["id"].(string)
			if fail[id] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"instrument":{"uid":"inst-` + id + `","assetUid":"uid-` + id + `"}}`))
		case strings.HasSuffix(r.URL.Path, "/GetAssetFundamentals"):
			_, _ = w.Write([]byte(`{"fundamentals":[{"assetUid":"x","peRatioTtm":4.2}]}`))
		case strings.HasSuffix(r.URL.Path, "/GetAssetReports"):
			if id, _ := body["instrumentId"].(string); !strings.HasPrefix(id, "inst-") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"events":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRunStoresOneRecordPerTickerAndIsIdempotent(t *testing.T) {
	srv := newServer(t, map[string]bool{"BAD": true})
	defer srv.Close()
	c := &client{base: srv.URL + "/", token: "secret", http: srv.Client()}
	out := filepath.Join(t.TempDir(), "f.jsonl")
	day := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	ok, failed, err := run(context.Background(), c, []string{"SBER", "BAD", "GAZP"}, "TQBR", out, day, true)
	if err != nil || ok != 2 || failed != 1 {
		t.Fatalf("run = %d ok, %d failed, err %v", ok, failed, err)
	}
	ok, failed, err = run(context.Background(), c, []string{"SBER", "GAZP"}, "TQBR", out, day, true)
	if err != nil || ok != 0 || failed != 0 {
		t.Fatalf("second run = %d ok, %d failed, err %v; want no duplicates", ok, failed, err)
	}
	data, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	var rec record
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Date != "2026-10-19" || rec.Ticker != "SBER" || rec.AssetUID != "uid-SBER" || !strings.Contains(string(rec.Fundamentals), "peRatioTtm") || rec.Reports == nil {
		t.Fatalf("unexpected record %+v", rec)
	}
}

func TestCallDoesNotRetryClientErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := &client{base: srv.URL + "/", token: "bad", http: srv.Client()}
	if _, err := c.call(context.Background(), "InstrumentsService", "ShareBy", map[string]string{}); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
