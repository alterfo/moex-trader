package main

import (
	"log"
	"os"
	"testing"
	"time"
)

func TestStartTraderProcessDetectsStartupMarkerOnStderr(t *testing.T) {
	cfg := TraderConfig{
		Command:       []string{"sh", "-c", "echo 'starting trader: tickers=1' >&2; sleep 1"},
		StartupMarker: "starting trader:",
	}
	proc, err := startTraderProcess(cfg, nil, log.New(os.Stderr, "", 0))
	if err != nil {
		t.Fatalf("startTraderProcess() error = %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !proc.active.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !proc.active.Load() {
		t.Fatal("trader never became active: startup marker on stderr was not detected")
	}
	<-proc.exited
}
