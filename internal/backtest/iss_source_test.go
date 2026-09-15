package backtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func TestISSSourceHistoryFollowsPaginationBeyondServerPageCap(t *testing.T) {
	const serverPageCap = 3
	totalRows := serverPageCap*2 + 1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/securities/SBER.json":
			payload := map[string]any{
				"description": map[string]any{
					"columns": []string{"name", "value"},
					"data":    [][]any{{"SECID", "SBER"}},
				},
				"boards": map[string]any{
					"columns": []string{"secid", "boardid", "engine", "market", "is_traded", "is_primary"},
					"data":    [][]any{{"SBER", "TQBR", "stock", "shares", 1, 1}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case "/engines/stock/markets/shares/boards/TQBR/securities/SBER/candles.json":
			start, _ := strconv.Atoi(r.URL.Query().Get("start"))
			rows := [][]any{}
			for i := start; i < totalRows && i < start+serverPageCap; i++ {
				day := 10 + i
				rows = append(rows, []any{
					100 + i, 101 + i, 102 + i, 99 + i, 1000,
					"2024-01-" + pad(day) + " 00:00:00", "2024-01-" + pad(day) + " 18:00:00",
				})
			}
			payload := map[string]any{
				"candles": map[string]any{
					"columns": []string{"open", "close", "high", "low", "volume", "begin", "end"},
					"data":    rows,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := moex.NewClient(server.URL, nil)
	source := NewISSSource(server.URL, client)

	candles, err := source.History(context.Background(), "SBER", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if len(candles) != totalRows {
		t.Fatalf("got %d candles, want %d (server caps each page at %d rows regardless of the requested limit)", len(candles), totalRows, serverPageCap)
	}
}

func pad(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}
