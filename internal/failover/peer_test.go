package failover

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPeerFetchDBDownloadsAndSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	createTestDB(t, source)
	modTime := time.Now().Add(-time.Minute).Truncate(time.Second)
	if err := os.Chtimes(source, modTime, modTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		freshness := DBFreshness(source)
		if header := r.Header.Get("If-Modified-Since"); header != "" {
			if since, err := http.ParseTime(header); err == nil && !freshness.After(since) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		w.Header().Set("Last-Modified", freshness.UTC().Format(http.TimeFormat))
		http.ServeFile(w, r, source)
	}))
	defer server.Close()

	dst := filepath.Join(dir, "trader.db")
	client := NewPeerClient(server.URL, "secret", nil)

	updated, err := client.FetchDB(context.Background(), dst, false)
	if err != nil {
		t.Fatalf("FetchDB() error = %v", err)
	}
	if !updated {
		t.Fatalf("FetchDB() updated = false, want true")
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(data[:16]) != "SQLite format 3\x00" {
		t.Fatalf("destination is not a sqlite database")
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}
	if !info.ModTime().Equal(modTime) {
		t.Fatalf("destination mtime = %s, want %s", info.ModTime(), modTime)
	}

	updated, err = client.FetchDB(context.Background(), dst, false)
	if err != nil {
		t.Fatalf("second FetchDB() error = %v", err)
	}
	if updated {
		t.Fatalf("second FetchDB() updated = true, want false")
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}

	updated, err = client.FetchDB(context.Background(), dst, true)
	if err != nil {
		t.Fatalf("forced FetchDB() error = %v", err)
	}
	if !updated {
		t.Fatalf("forced FetchDB() updated = false, want true")
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestPeerFetchDBRejectsNonSQLitePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a database"))
	}))
	defer server.Close()

	dst := filepath.Join(t.TempDir(), "trader.db")
	client := NewPeerClient(server.URL, "", nil)
	if _, err := client.FetchDB(context.Background(), dst, false); err == nil {
		t.Fatalf("FetchDB() accepted a non-sqlite payload")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("destination was created for a non-sqlite payload")
	}
}

func TestPeerHeartbeatParsesPayload(t *testing.T) {
	payload := Heartbeat{
		Node:      "aibox",
		Role:      RoleStandby,
		State:     StateActive,
		Since:     time.Now().Add(-time.Minute),
		UpdatedAt: time.Now(),
		Witness:   WitnessView{Reachable: true, Holder: "aibox"},
		Trader:    TraderView{Running: true, Restarts: 2},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	client := NewPeerClient(server.URL, "", nil)
	heartbeat, err := client.Heartbeat(context.Background())
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if heartbeat.Node != "aibox" || heartbeat.State != StateActive || !heartbeat.Trader.Running {
		t.Fatalf("heartbeat = %+v, want aibox active running", heartbeat)
	}
}

func TestPeerRequestYieldRequiresToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewPeerClient(server.URL, "", nil)
	if err := client.RequestYield(context.Background()); err == nil {
		t.Fatalf("RequestYield() without token succeeded, want error")
	}
	client = NewPeerClient(server.URL, "secret", nil)
	if err := client.RequestYield(context.Background()); err != nil {
		t.Fatalf("RequestYield() error = %v", err)
	}
}

func TestWitnessClientRenewAndRelease(t *testing.T) {
	holder := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/renew":
			var request struct {
				Node string `json:"node"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			holder = request.Node
			_ = json.NewEncoder(w).Encode(map[string]any{
				"holder":      holder,
				"expires_at":  time.Now().Add(5 * time.Minute).Format(time.RFC3339Nano),
				"granted":     true,
				"ttl_seconds": 300,
			})
		case "/release":
			holder = ""
			_ = json.NewEncoder(w).Encode(map[string]any{"holder": ""})
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"holder": holder})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewWitnessClient(server.URL, "token", nil)
	status, granted, err := client.Renew(context.Background(), "mac")
	if err != nil || !granted {
		t.Fatalf("Renew() = (%+v, %v, %v), want granted", status, granted, err)
	}
	if !status.HeldBy("mac", time.Now()) {
		t.Fatalf("status = %+v, want held by mac", status)
	}
	if err := client.Release(context.Background(), "mac"); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	status, err = client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Holder != "" {
		t.Fatalf("holder = %q after release, want empty", status.Holder)
	}
}
