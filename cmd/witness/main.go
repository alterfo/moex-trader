package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type leaseStore struct {
	mu        sync.Mutex
	ttl       time.Duration
	grace     time.Duration
	path      string
	holder    string
	expiresAt time.Time
	notBefore time.Time
	now       func() time.Time
}

type persistedState struct {
	Holder    string    `json:"holder"`
	ExpiresAt time.Time `json:"expires_at"`
}

func newLeaseStore(path string, ttl, grace time.Duration, now func() time.Time) (*leaseStore, error) {
	if now == nil {
		now = time.Now
	}
	store := &leaseStore{ttl: ttl, grace: grace, path: path, now: now}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *leaseStore) load() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.notBefore = s.now().Add(s.ttl)
			return nil
		}
		return fmt.Errorf("load witness state %q: %w", s.path, err)
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("witness: state file %q is unreadable (%v); blocking leases for %s", s.path, err, s.ttl)
		s.notBefore = s.now().Add(s.ttl)
		return nil
	}
	s.holder = state.Holder
	s.expiresAt = state.ExpiresAt
	log.Printf("witness: restored lease holder=%q expires_at=%s", s.holder, s.expiresAt.Format(time.RFC3339))
	return nil
}

func (s *leaseStore) persistLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create witness state directory: %w", err)
	}
	data, err := json.Marshal(persistedState{Holder: s.holder, ExpiresAt: s.expiresAt})
	if err != nil {
		return fmt.Errorf("marshal witness state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write witness state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace witness state: %w", err)
	}
	return nil
}

func (s *leaseStore) renew(node string) (bool, string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.Before(s.notBefore) {
		return false, s.holder, s.expiresAt
	}
	if s.holder != "" && s.holder != node && now.Before(s.expiresAt.Add(s.grace)) {
		return false, s.holder, s.expiresAt
	}
	s.holder = node
	s.expiresAt = now.Add(s.ttl)
	if err := s.persistLocked(); err != nil {
		log.Printf("witness: persist state: %v", err)
	}
	return true, s.holder, s.expiresAt
}

func (s *leaseStore) release(node string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.holder != node {
		return
	}
	s.holder = ""
	s.expiresAt = time.Time{}
	if err := s.persistLocked(); err != nil {
		log.Printf("witness: persist state: %v", err)
	}
}

func (s *leaseStore) status() (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.holder, s.expiresAt
}

type leaseResponse struct {
	Holder     string    `json:"holder"`
	ExpiresAt  time.Time `json:"expires_at"`
	Granted    bool      `json:"granted"`
	TTLSeconds float64   `json:"ttl_seconds"`
}

type leaseRequest struct {
	Node string `json:"node"`
}

type server struct {
	store *leaseStore
	token string
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/status", s.withAuth(s.handleStatus))
	mux.HandleFunc("/renew", s.withAuth(s.handleRenew))
	mux.HandleFunc("/release", s.withAuth(s.handleRelease))
	return mux
}

func (s *server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	holder, expiresAt := s.store.status()
	writeJSON(w, http.StatusOK, leaseResponse{
		Holder:     holder,
		ExpiresAt:  expiresAt,
		TTLSeconds: s.store.ttl.Seconds(),
	})
}

func (s *server) handleRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request, err := decodeLeaseRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	granted, holder, expiresAt := s.store.renew(request.Node)
	if granted {
		log.Printf("witness: granted lease to %q until %s", holder, expiresAt.Format(time.RFC3339))
	} else {
		log.Printf("witness: denied lease to %q, holder=%q expires_at=%s", request.Node, holder, expiresAt.Format(time.RFC3339))
	}
	writeJSON(w, http.StatusOK, leaseResponse{
		Holder:     holder,
		ExpiresAt:  expiresAt,
		Granted:    granted,
		TTLSeconds: s.store.ttl.Seconds(),
	})
}

func (s *server) handleRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request, err := decodeLeaseRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.release(request.Node)
	log.Printf("witness: released lease by %q", request.Node)
	holder, expiresAt := s.store.status()
	writeJSON(w, http.StatusOK, leaseResponse{
		Holder:     holder,
		ExpiresAt:  expiresAt,
		TTLSeconds: s.store.ttl.Seconds(),
	})
}

func decodeLeaseRequest(r *http.Request) (leaseRequest, error) {
	var request leaseRequest
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096)).Decode(&request); err != nil {
		return leaseRequest{}, fmt.Errorf("decode request: %w", err)
	}
	request.Node = strings.TrimSpace(request.Node)
	if request.Node == "" {
		return leaseRequest{}, errors.New("node is required")
	}
	return request, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func main() {
	var listen string
	var tokenFile string
	var statePath string
	var ttl time.Duration
	var grace time.Duration
	flag.StringVar(&listen, "listen", ":8090", "listen address")
	flag.StringVar(&tokenFile, "token-file", "", "path to a file with the bearer token")
	flag.StringVar(&statePath, "state", "/var/db/moex-witness/state.json", "path to the persisted lease state")
	flag.DurationVar(&ttl, "ttl", 5*time.Minute, "lease time to live")
	flag.DurationVar(&grace, "grace", 5*time.Second, "extra wait before a lease can be taken over")
	flag.Parse()

	if err := run(listen, tokenFile, statePath, ttl, grace); err != nil {
		log.Fatal(err)
	}
}

func run(listen, tokenFile, statePath string, ttl, grace time.Duration) error {
	token := strings.TrimSpace(os.Getenv("MOEX_WITNESS_TOKEN"))
	if tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return fmt.Errorf("read token file %q: %w", tokenFile, err)
		}
		token = strings.TrimSpace(string(data))
	}
	if token == "" {
		return fmt.Errorf("witness token is empty: set MOEX_WITNESS_TOKEN or pass -token-file")
	}
	store, err := newLeaseStore(statePath, ttl, grace, time.Now)
	if err != nil {
		return err
	}
	srv := &server{store: store, token: token}

	httpServer := &http.Server{
		Addr:              listen,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("witness: listening on %s ttl=%s grace=%s state=%s", listen, ttl, grace, statePath)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown witness: %w", err)
	}
	log.Printf("witness: stopped")
	return nil
}
