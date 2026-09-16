package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/failover"
)

func (w *watchdog) routes(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", w.handleHealthz)
	mux.HandleFunc("/db", w.auth(token, w.handleDB))
	mux.HandleFunc("/control/yield", w.auth(token, w.handleYield))
	return mux
}

func (w *watchdog) auth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(rw, r)
	}
}

func (w *watchdog) handleHealthz(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(rw, http.StatusOK, w.heartbeat(time.Now()))
}

func (w *watchdog) handleDB(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	freshness := failover.DBFreshness(w.cfg.DB.Path)
	if header := r.Header.Get("If-Modified-Since"); header != "" {
		if since, err := http.ParseTime(header); err == nil && !freshness.After(since) {
			rw.WriteHeader(http.StatusNotModified)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tmp, err := os.CreateTemp("", "moex-db-*.sqlite")
	if err != nil {
		http.Error(rw, "create snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		http.Error(rw, "close snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if err := failover.SnapshotDB(ctx, w.cfg.DB.Path, tmpPath); err != nil {
		w.logger.Printf("watchdog: snapshot db: %v", err)
		http.Error(rw, "snapshot db: "+err.Error(), http.StatusInternalServerError)
		return
	}
	file, err := os.Open(tmpPath)
	if err != nil {
		http.Error(rw, "open snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = file.Close()
	}()
	rw.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(rw, r, "trader-sandbox.db", freshness, file)
}

func (w *watchdog) handleYield(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.requestYield(time.Now())
	w.logger.Printf("watchdog: handover requested by peer")
	rw.WriteHeader(http.StatusAccepted)
}

func writeJSON(rw http.ResponseWriter, status int, payload any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(payload)
}
