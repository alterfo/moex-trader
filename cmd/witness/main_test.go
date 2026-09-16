package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T, path string, now *time.Time) *leaseStore {
	t.Helper()
	store, err := newLeaseStore(path, 5*time.Minute, 5*time.Second, func() time.Time { return *now })
	if err != nil {
		t.Fatalf("newLeaseStore() error = %v", err)
	}
	return store
}

func TestRenewGrantsToFirstNode(t *testing.T) {
	now := time.Now()
	store := testStore(t, "", &now)

	granted, holder, expiresAt := store.renew("mac")
	if !granted || holder != "mac" {
		t.Fatalf("renew = (%v, %q), want granted to mac", granted, holder)
	}
	if !expiresAt.After(now) {
		t.Fatalf("expiresAt = %s, want in the future", expiresAt)
	}
}

func TestRenewDeniesSecondNodeWhileLeaseValid(t *testing.T) {
	now := time.Now()
	store := testStore(t, "", &now)
	store.renew("mac")

	granted, holder, _ := store.renew("aibox")
	if granted || holder != "mac" {
		t.Fatalf("renew = (%v, %q), want denied with holder mac", granted, holder)
	}
}

func TestRenewGrantsAfterExpiryAndGrace(t *testing.T) {
	now := time.Now()
	store := testStore(t, "", &now)
	store.renew("mac")

	now = now.Add(5*time.Minute + 6*time.Second)
	granted, holder, _ := store.renew("aibox")
	if !granted || holder != "aibox" {
		t.Fatalf("renew = (%v, %q), want granted to aibox", granted, holder)
	}
}

func TestRenewExtendsOwnLease(t *testing.T) {
	now := time.Now()
	store := testStore(t, "", &now)
	store.renew("mac")
	_, first := store.status()

	now = now.Add(4 * time.Minute)
	granted, _, expiresAt := store.renew("mac")
	if !granted {
		t.Fatalf("renew for the holder was denied")
	}
	if !expiresAt.After(first) {
		t.Fatalf("expiresAt = %s, want extension past %s", expiresAt, first)
	}
}

func TestReleaseFreesLease(t *testing.T) {
	now := time.Now()
	store := testStore(t, "", &now)
	store.renew("mac")

	store.release("aibox")
	if holder, _ := store.status(); holder != "mac" {
		t.Fatalf("release by foreign node changed holder to %q", holder)
	}
	store.release("mac")
	if holder, _ := store.status(); holder != "" {
		t.Fatalf("holder = %q after release, want empty", holder)
	}
}

func TestStateFilePersistsLease(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write empty state: %v", err)
	}
	store := testStore(t, path, &now)
	store.renew("mac")

	reloaded := testStore(t, path, &now)
	if holder, _ := reloaded.status(); holder != "mac" {
		t.Fatalf("reloaded holder = %q, want mac", holder)
	}
}

func TestMissingStateFileBlocksLeases(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "missing.json")
	store := testStore(t, path, &now)

	granted, _, _ := store.renew("mac")
	if granted {
		t.Fatalf("renew granted on a fresh witness with no state file, want blocked")
	}

	now = now.Add(5*time.Minute + time.Second)
	granted, _, _ = store.renew("mac")
	if !granted {
		t.Fatalf("renew still blocked after ttl elapsed, want granted")
	}
}

func TestCorruptStateFileBlocksLeases(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}
	store := testStore(t, path, &now)
	granted, _, _ := store.renew("mac")
	if granted {
		t.Fatalf("renew granted with a corrupt state file, want blocked")
	}
}

func TestHTTPRenewRequiresToken(t *testing.T) {
	now := time.Now()
	store := testStore(t, "", &now)
	srv := &server{store: store, token: "secret"}

	request := httptest.NewRequest(http.MethodPost, "/renew", strings.NewReader(`{"node":"mac"}`))
	recorder := httptest.NewRecorder()
	srv.routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodPost, "/renew", strings.NewReader(`{"node":"mac"}`))
	request.Header.Set("Authorization", "Bearer secret")
	recorder = httptest.NewRecorder()
	srv.routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload leaseResponse
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Granted || payload.Holder != "mac" {
		t.Fatalf("payload = %+v, want granted to mac", payload)
	}
}

func TestHTTPStatusReportsHolder(t *testing.T) {
	now := time.Now()
	store := testStore(t, "", &now)
	store.renew("aibox")
	srv := &server{store: store, token: "secret"}

	request := httptest.NewRequest(http.MethodGet, "/status", nil)
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	srv.routes().ServeHTTP(recorder, request)

	var payload leaseResponse
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Holder != "aibox" {
		t.Fatalf("holder = %q, want aibox", payload.Holder)
	}
}
