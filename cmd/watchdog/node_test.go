package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/olegsidorkin/moex-trader/internal/failover"
)

func openSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path)
}

const (
	testWitnessToken   = "witness-token"
	testHeartbeatToken = "heartbeat-token"
)

type fakeWitness struct {
	mu        sync.Mutex
	holder    string
	expiresAt time.Time
	renews    int
	releases  int
}

func (f *fakeWitness) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"holder": f.holder, "expires_at": f.expiresAt})
	})
	mux.HandleFunc("/renew", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Node string `json:"node"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		f.mu.Lock()
		defer f.mu.Unlock()
		granted := f.holder == "" || f.holder == request.Node || time.Now().After(f.expiresAt)
		if granted {
			f.holder = request.Node
			f.expiresAt = time.Now().Add(5 * time.Minute)
			f.renews++
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"holder":      f.holder,
			"expires_at":  f.expiresAt,
			"granted":     granted,
			"ttl_seconds": 300,
		})
	})
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Node string `json:"node"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.holder == request.Node {
			f.holder = ""
			f.expiresAt = time.Time{}
			f.releases++
		}
		writeJSON(w, http.StatusOK, map[string]any{"holder": f.holder})
	})
	return mux
}

func (f *fakeWitness) snapshot() (string, time.Time, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holder, f.expiresAt, f.renews, f.releases
}

type fakePeer struct {
	mu         sync.Mutex
	heartbeat  failover.Heartbeat
	dbPath     string
	yieldCount int
}

func (f *fakePeer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		heartbeat := f.heartbeat
		f.mu.Unlock()
		if heartbeat.Node == "" {
			http.NotFound(w, r)
			return
		}
		heartbeat.UpdatedAt = time.Now()
		writeJSON(w, http.StatusOK, heartbeat)
	})
	mux.HandleFunc("/db", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testHeartbeatToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		path := f.dbPath
		f.mu.Unlock()
		if path == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("/control/yield", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.yieldCount++
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})
	return mux
}

func (f *fakePeer) setHeartbeat(heartbeat failover.Heartbeat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heartbeat = heartbeat
}

func (f *fakePeer) yields() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.yieldCount
}

func testConfig(t *testing.T, role, peerURL, witnessURL string) *Config {
	t.Helper()
	dir := t.TempDir()
	cfg := &Config{
		Role:     role,
		NodeName: "mac",
		PeerName: "aibox",
		Listen:   ":0",
		PeerURL:  peerURL,
		Witness:  WitnessConfig{URL: witnessURL, TTL: Duration(5 * time.Minute)},
		Trader: TraderConfig{
			Command:       []string{"/bin/sh", "-c", "echo 'starting trader:'; sleep 60"},
			StopGrace:     Duration(2 * time.Second),
			StartupGrace:  Duration(time.Minute),
			Backoff:       []Duration{Duration(time.Hour)},
			MaxFailures:   3,
			RetryCooldown: Duration(time.Hour),
			StartupMarker: "starting trader:",
		},
		DB: DBConfig{Path: filepath.Join(dir, "trader.db"), PullInterval: Duration(time.Hour)},
		Timing: TimingConfig{
			TickInterval:         Duration(50 * time.Millisecond),
			PeerStaleAfter:       Duration(time.Minute),
			FailoverAfter:        Duration(time.Minute),
			PeerErrorGrace:       Duration(time.Minute),
			HandoverCooldown:     Duration(time.Hour),
			YieldRequestCooldown: Duration(time.Millisecond),
			YieldRequestTTL:      Duration(time.Minute),
			FailureMemory:        Duration(time.Minute),
		},
	}
	cfg.applyDefaults()
	return cfg
}

func newTestWatchdog(t *testing.T, cfg *Config, witnessURL, peerURL string) *watchdog {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	return newWatchdog(
		cfg,
		failover.NewWitnessClient(witnessURL, testWitnessToken, nil),
		failover.NewPeerClient(peerURL, testHeartbeatToken, nil),
		nil,
		logger,
	)
}

func (w *watchdog) snapshot() (failover.NodeState, bool, failover.LeaseStatus, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state, w.runningLocked(), w.lease, w.failures
}

func tickUntil(t *testing.T, wd *watchdog, ctx context.Context, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition was not met within %s", timeout)
		}
		wd.tick(ctx)
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPrimaryTakesOverWhenPeerUnseen(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peerServer := httptest.NewServer(http.NotFoundHandler())
	defer peerServer.Close()

	cfg := testConfig(t, "primary", peerServer.URL, witnessServer.URL)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	ctx := context.Background()
	wd.tick(ctx)

	state, running, lease, _ := wd.snapshot()
	if !lease.HeldBy("mac", time.Now()) {
		t.Fatalf("lease = %+v, want held by mac", lease)
	}
	if !running {
		t.Fatalf("trader is not running after takeover (state=%s)", state)
	}
	tickUntil(t, wd, ctx, 2*time.Second, func() bool {
		state, _, _, _ := wd.snapshot()
		return state == failover.StateActive
	})
}

func TestPrimaryStaysStandbyWhilePeerActive(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peer := &fakePeer{}
	peer.setHeartbeat(failover.Heartbeat{
		Node:    "aibox",
		Role:    failover.RoleStandby,
		State:   failover.StateActive,
		Since:   time.Now().Add(-time.Hour),
		Witness: failover.WitnessView{Reachable: true, Holder: "aibox"},
		Trader:  failover.TraderView{Running: true},
	})
	peerServer := httptest.NewServer(peer.handler())
	defer peerServer.Close()

	cfg := testConfig(t, "primary", peerServer.URL, witnessServer.URL)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	wd.tick(context.Background())

	state, running, lease, _ := wd.snapshot()
	if running {
		t.Fatalf("trader started while peer is active (state=%s)", state)
	}
	if lease.HeldBy("mac", time.Now()) {
		t.Fatalf("lease = %+v, want not held by mac", lease)
	}
	holder, _, _, _ := witness.snapshot()
	if holder != "" {
		t.Fatalf("witness holder = %q, want empty", holder)
	}
}

func TestStandbyTakesOverAfterPeerSilence(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peerServer := httptest.NewServer(http.NotFoundHandler())
	defer peerServer.Close()

	cfg := testConfig(t, "standby", peerServer.URL, witnessServer.URL)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	wd.tick(context.Background())

	_, running, lease, _ := wd.snapshot()
	if !running || !lease.HeldBy("mac", time.Now()) {
		t.Fatalf("standby did not take over: running=%v lease=%+v", running, lease)
	}
}

func TestStandbyHandsOverOnYieldRequest(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peer := &fakePeer{}
	peerServer := httptest.NewServer(peer.handler())
	defer peerServer.Close()

	cfg := testConfig(t, "standby", peerServer.URL, witnessServer.URL)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	ctx := context.Background()
	wd.tick(ctx)
	if _, running, _, _ := wd.snapshot(); !running {
		t.Fatalf("standby did not start trading")
	}

	peer.setHeartbeat(failover.Heartbeat{
		Node:    "mac",
		Role:    failover.RolePrimary,
		State:   failover.StateStandby,
		Since:   time.Now().Add(-time.Minute),
		Witness: failover.WitnessView{Reachable: true},
	})
	wd.requestYield(time.Now())
	wd.tick(ctx)

	_, running, lease, _ := wd.snapshot()
	if running {
		t.Fatalf("standby kept trading after handover request")
	}
	if lease.HeldBy("mac", time.Now()) {
		t.Fatalf("lease still held after handover: %+v", lease)
	}
	holder, _, _, releases := witness.snapshot()
	if holder != "" || releases == 0 {
		t.Fatalf("witness holder=%q releases=%d, want released", holder, releases)
	}
}

func TestPrimaryRequestsHandoverFromActivePeer(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peer := &fakePeer{}
	peer.setHeartbeat(failover.Heartbeat{
		Node:    "aibox",
		Role:    failover.RoleStandby,
		State:   failover.StateActive,
		Since:   time.Now().Add(-time.Hour),
		Witness: failover.WitnessView{Reachable: true, Holder: "aibox"},
		Trader:  failover.TraderView{Running: true},
	})
	peerServer := httptest.NewServer(peer.handler())
	defer peerServer.Close()

	cfg := testConfig(t, "primary", peerServer.URL, witnessServer.URL)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	wd.tick(context.Background())

	if peer.yields() == 0 {
		t.Fatalf("primary did not request a handover from the active peer")
	}
}

func TestTraderCrashMarksErrorAndBacksOff(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peerServer := httptest.NewServer(http.NotFoundHandler())
	defer peerServer.Close()

	cfg := testConfig(t, "primary", peerServer.URL, witnessServer.URL)
	cfg.Trader.Command = []string{"/bin/sh", "-c", "echo boom; exit 1"}
	cfg.Trader.Backoff = []Duration{Duration(time.Hour)}
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	ctx := context.Background()
	wd.tick(ctx)
	tickUntil(t, wd, ctx, 2*time.Second, func() bool {
		state, running, _, failures := wd.snapshot()
		return state == failover.StateError && !running && failures >= 1
	})

	wd.tick(ctx)
	_, running, _, _ := wd.snapshot()
	if running {
		t.Fatalf("trader restarted despite the backoff")
	}
}

func TestFencingWhenWitnessDisappears(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	peerServer := httptest.NewServer(http.NotFoundHandler())
	defer peerServer.Close()

	cfg := testConfig(t, "primary", peerServer.URL, witnessServer.URL)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	ctx := context.Background()
	wd.tick(ctx)
	if _, running, _, _ := wd.snapshot(); !running {
		t.Fatalf("trader did not start")
	}

	witnessServer.Close()
	realNow := wd.now
	wd.now = func() time.Time { return realNow().Add(10 * time.Minute) }
	wd.tick(ctx)

	state, running, _, _ := wd.snapshot()
	if running {
		t.Fatalf("trader kept running without a valid lease")
	}
	if state != failover.StateFenced {
		t.Fatalf("state = %s, want fenced", state)
	}
}

func TestPeerAuthoritativeFollowsLastTraderStart(t *testing.T) {
	cfg := testConfig(t, "primary", "http://127.0.0.1:1", "http://127.0.0.1:1")
	wd := newTestWatchdog(t, cfg, "http://127.0.0.1:1", "http://127.0.0.1:1")
	now := time.Now()

	wd.mu.Lock()
	wd.peerSeen = true
	wd.peerHB = failover.Heartbeat{Trader: failover.TraderView{Running: true}}
	wd.mu.Unlock()
	if !wd.peerAuthoritativeLocked(now) {
		t.Fatalf("running peer should be authoritative")
	}

	wd.mu.Lock()
	wd.peerHB = failover.Heartbeat{Trader: failover.TraderView{LastStartAt: now.Add(-time.Hour)}}
	wd.lastTraderStart = now.Add(-2 * time.Hour)
	wd.mu.Unlock()
	if !wd.peerAuthoritativeLocked(now) {
		t.Fatalf("peer with a newer trader start should be authoritative")
	}

	wd.mu.Lock()
	wd.lastTraderStart = now.Add(-time.Minute)
	wd.mu.Unlock()
	if wd.peerAuthoritativeLocked(now) {
		t.Fatalf("peer with an older trader start must not be authoritative")
	}

	wd.mu.Lock()
	wd.peerHB = failover.Heartbeat{}
	wd.lastTraderStart = time.Time{}
	wd.mu.Unlock()
	if wd.peerAuthoritativeLocked(now) {
		t.Fatalf("peer that never ran the trader must not be authoritative")
	}

	wd.mu.Lock()
	wd.peerHB = failover.Heartbeat{Trader: failover.TraderView{LastStartAt: now.Add(-time.Hour)}}
	wd.lastTraderStart = time.Time{}
	wd.mu.Unlock()
	if !wd.peerAuthoritativeLocked(now) {
		t.Fatalf("peer should be authoritative when the local node never ran the trader")
	}
}

func TestHealthzServesHeartbeat(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peerServer := httptest.NewServer(http.NotFoundHandler())
	defer peerServer.Close()

	cfg := testConfig(t, "standby", peerServer.URL, witnessServer.URL)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	server := httptest.NewServer(wd.routes(testHeartbeatToken))
	defer server.Close()

	response, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	var heartbeat failover.Heartbeat
	if err := json.NewDecoder(response.Body).Decode(&heartbeat); err != nil {
		t.Fatalf("decode heartbeat: %v", err)
	}
	if heartbeat.Node != "mac" || heartbeat.Role != failover.RoleStandby {
		t.Fatalf("heartbeat = %+v, want mac standby", heartbeat)
	}

	response, err = http.Get(server.URL + "/db")
	if err != nil {
		t.Fatalf("GET /db: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /db without token = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestDBEndpointServesSnapshot(t *testing.T) {
	witness := &fakeWitness{}
	witnessServer := httptest.NewServer(witness.handler())
	defer witnessServer.Close()
	peerServer := httptest.NewServer(http.NotFoundHandler())
	defer peerServer.Close()

	cfg := testConfig(t, "standby", peerServer.URL, witnessServer.URL)
	createTestDatabase(t, cfg.DB.Path)
	wd := newTestWatchdog(t, cfg, witnessServer.URL, peerServer.URL)
	defer func() {
		_ = wd.shutdown()
	}()

	server := httptest.NewServer(wd.routes(testHeartbeatToken))
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/db", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+testHeartbeatToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET /db: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /db = %d, want %d", response.StatusCode, http.StatusOK)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("snapshot is empty")
	}
	tmp := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	if err := checkSQLiteTicker(t, tmp); err != nil {
		t.Fatalf("snapshot is not a valid sqlite database: %v", err)
	}
}

func createTestDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := openSQLite(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	if _, err := db.Exec(`CREATE TABLE trades (id INTEGER PRIMARY KEY, ticker TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO trades (ticker) VALUES ('SBER')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
}

func checkSQLiteTicker(t *testing.T, path string) error {
	t.Helper()
	db, err := openSQLite(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()
	var ticker string
	return db.QueryRow(`SELECT ticker FROM trades`).Scan(&ticker)
}
