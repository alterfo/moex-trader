package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/alert/telegram"
	"github.com/olegsidorkin/moex-trader/internal/failover"
)

type watchdog struct {
	cfg     *Config
	role    failover.Role
	params  failover.Params
	witness *failover.WitnessClient
	peer    *failover.PeerClient
	alerter *telegram.Client
	logger  *log.Logger
	now     func() time.Time

	mu              sync.Mutex
	state           failover.NodeState
	stateSince      time.Time
	reason          string
	lease           failover.LeaseStatus
	peerHB          failover.Heartbeat
	peerSeen        bool
	lastPeerSeen    time.Time
	yieldRequested  time.Time
	lastYieldAt     time.Time
	lastYieldReqAt  time.Time
	lastFailureAt   time.Time
	failures        int
	restarts        int
	traderStarted   time.Time
	lastTraderStart time.Time
	nextStartAt     time.Time
	lastPullAt      time.Time
	syncedFromPeer  bool

	trader *traderProcess

	alertMu sync.Mutex
	alerts  map[string]time.Time
}

func newWatchdog(cfg *Config, witness *failover.WitnessClient, peer *failover.PeerClient, alerter *telegram.Client, logger *log.Logger) *watchdog {
	if logger == nil {
		logger = log.Default()
	}
	return &watchdog{
		cfg:        cfg,
		role:       failover.Role(cfg.Role),
		params:     cfg.params(),
		witness:    witness,
		peer:       peer,
		alerter:    alerter,
		logger:     logger,
		now:        time.Now,
		state:      failover.StateStandby,
		stateSince: time.Now(),
		reason:     "startup",
		alerts:     make(map[string]time.Time),
	}
}

func (w *watchdog) run(ctx context.Context) error {
	w.logger.Printf("watchdog: node=%s role=%s peer=%s witness=%s db=%s", w.cfg.NodeName, w.cfg.Role, w.cfg.PeerURL, w.cfg.Witness.URL, w.cfg.DB.Path)
	go w.runTunnel(ctx)

	ticker := time.NewTicker(w.cfg.Timing.TickInterval.Std())
	defer ticker.Stop()
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return w.shutdown()
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *watchdog) tick(ctx context.Context) {
	now := w.now()

	if heartbeat, err := w.peer.Heartbeat(ctx); err == nil {
		w.mu.Lock()
		w.peerHB = heartbeat
		w.peerSeen = true
		w.lastPeerSeen = now
		w.mu.Unlock()
	} else {
		w.logger.Printf("watchdog: peer heartbeat: %v", err)
	}

	status, err := w.witness.Status(ctx)
	w.mu.Lock()
	if err != nil {
		w.logger.Printf("watchdog: witness status: %v", err)
		w.lease.Reachable = false
	} else {
		w.lease = status
	}
	w.mu.Unlock()

	w.reapTrader(now)
	w.refreshTraderState(now)
	w.releaseLeaseAfterFailures(ctx, now)

	decision := failover.Evaluate(w.input(now))
	if decision.AcquireLease {
		status, granted, err := w.witness.Renew(ctx, w.cfg.NodeName)
		w.mu.Lock()
		if err != nil {
			w.logger.Printf("watchdog: witness renew: %v", err)
			w.lease.Reachable = false
		} else {
			w.lease = status
		}
		w.mu.Unlock()
		if granted {
			decision = failover.Evaluate(w.input(now))
		}
	}

	w.apply(ctx, now, decision)
	w.syncPassive(ctx, now)
}

func (w *watchdog) apply(ctx context.Context, now time.Time, decision failover.Decision) {
	w.mu.Lock()
	running := w.runningLocked()
	nextStartAt := w.nextStartAt
	w.mu.Unlock()

	if running && !decision.RunTrader {
		w.stopTrader(now, "election: "+decision.Reason, failover.StateStandby)
		running = false
	}

	if decision.ReleaseLease {
		if running {
			w.stopTrader(now, "handover to primary", failover.StateStandby)
			running = false
		}
		if err := w.witness.Release(ctx, w.cfg.NodeName); err != nil {
			w.logger.Printf("watchdog: witness release: %v", err)
		} else {
			w.mu.Lock()
			w.lease = failover.LeaseStatus{Reachable: true}
			w.lastYieldAt = now
			w.yieldRequested = time.Time{}
			w.setStateLocked(failover.StateStandby, "handed over to primary", now)
			w.mu.Unlock()
			w.logger.Printf("watchdog: released lease for primary handover")
			w.alert("handover-done", 10*time.Minute, "🔵 MOEX trader: %s передал управление главному ПК", w.cfg.NodeName)
		}
	}

	if decision.RequestPeerYield {
		if err := w.peer.RequestYield(ctx); err != nil {
			w.logger.Printf("watchdog: request peer yield: %v", err)
		} else {
			w.mu.Lock()
			w.lastYieldReqAt = now
			w.mu.Unlock()
			w.logger.Printf("watchdog: requested handover from peer")
			w.alert("handover-request", 10*time.Minute, "🟡 MOEX trader: %s просит %s передать управление", w.cfg.NodeName, w.cfg.PeerName)
		}
	}

	if decision.RunTrader && !running {
		if !nextStartAt.IsZero() && now.Before(nextStartAt) {
			return
		}
		w.startTrader(ctx, now, decision.Reason)
		return
	}

	if !decision.RunTrader && !running && !decision.ReleaseLease {
		state := failover.StateStandby
		switch {
		case strings.Contains(decision.Reason, "fencing"):
			state = failover.StateFenced
		case w.inhibitedAfterFailures(now):
			state = failover.StateError
		}
		w.mu.Lock()
		w.setStateLocked(state, decision.Reason, now)
		w.mu.Unlock()
	}
}

func (w *watchdog) startTrader(ctx context.Context, now time.Time, reason string) {
	if err := w.pullDB(ctx, now); err != nil {
		w.logger.Printf("watchdog: pre-start db sync skipped: %v", err)
	}
	proc, err := startTraderProcess(w.cfg.Trader, w.logger)
	if err != nil {
		w.mu.Lock()
		w.failures++
		w.lastFailureAt = now
		w.nextStartAt = now.Add(w.backoffLocked(w.failures))
		w.setStateLocked(failover.StateError, fmt.Sprintf("trader start failed: %v", err), now)
		failures := w.failures
		w.mu.Unlock()
		w.logger.Printf("watchdog: start trader: %v", err)
		w.alertCrashLoop(failures, err)
		return
	}
	w.mu.Lock()
	w.trader = proc
	w.traderStarted = proc.started
	w.lastTraderStart = now
	w.syncedFromPeer = false
	restarts := w.restarts
	w.restarts++
	w.setStateLocked(failover.StateStarting, reason, now)
	w.mu.Unlock()
	w.logger.Printf("watchdog: trader started pid=%d (%s)", proc.cmd.Process.Pid, reason)
	w.alert("takeover", 10*time.Minute, "🟢 MOEX trader: %s взял управление (%s, рестарт #%d)", w.cfg.NodeName, reason, restarts)
}

func (w *watchdog) stopTrader(now time.Time, reason string, state failover.NodeState) {
	w.mu.Lock()
	proc := w.trader
	w.trader = nil
	w.traderStarted = time.Time{}
	w.setStateLocked(state, reason, now)
	w.mu.Unlock()
	if proc == nil {
		return
	}
	proc.stop(w.cfg.Trader.StopGrace.Std(), w.logger)
	w.logger.Printf("watchdog: trader stopped (%s)", reason)
}

func (w *watchdog) reapTrader(now time.Time) {
	w.mu.Lock()
	proc := w.trader
	if proc == nil || proc.alive() {
		w.mu.Unlock()
		return
	}
	w.trader = nil
	w.traderStarted = time.Time{}
	w.failures++
	w.lastFailureAt = now
	w.nextStartAt = now.Add(w.backoffLocked(w.failures))
	failures := w.failures
	err := proc.err
	w.setStateLocked(failover.StateError, fmt.Sprintf("trader exited: %v", err), now)
	w.mu.Unlock()
	w.logger.Printf("watchdog: trader exited unexpectedly (failures=%d): %v", failures, err)
	w.alertCrashLoop(failures, err)
}

func (w *watchdog) refreshTraderState(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.trader == nil || !w.trader.alive() {
		return
	}
	if !w.trader.active.Load() {
		return
	}
	if w.state != failover.StateActive {
		w.failures = 0
		w.nextStartAt = time.Time{}
		w.setStateLocked(failover.StateActive, w.reason, now)
		w.logger.Printf("watchdog: trader is active")
	}
}

func (w *watchdog) inhibitedAfterFailures(now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.failures >= w.cfg.Trader.MaxFailures && now.Before(w.nextStartAt)
}

func (w *watchdog) releaseLeaseAfterFailures(ctx context.Context, now time.Time) {
	w.mu.Lock()
	failures := w.failures
	held := w.lease.HeldBy(w.cfg.NodeName, now)
	running := w.runningLocked()
	w.mu.Unlock()
	if running || failures < w.cfg.Trader.MaxFailures || !held {
		return
	}
	if err := w.witness.Release(ctx, w.cfg.NodeName); err != nil {
		w.logger.Printf("watchdog: release lease after failures: %v", err)
		return
	}
	w.mu.Lock()
	w.lease = failover.LeaseStatus{Reachable: true}
	w.nextStartAt = now.Add(w.cfg.Trader.PostFailureCooldown.Std())
	w.setStateLocked(failover.StateError, fmt.Sprintf("released lease after %d trader failures", failures), now)
	w.mu.Unlock()
	w.logger.Printf("watchdog: released lease after %d trader failures, peer may take over", failures)
	w.alert("lease-released", 15*time.Minute, "⚠️ MOEX trader: %s отпустил lease после %d падений трейдера, ждём резерв", w.cfg.NodeName, failures)
}

func (w *watchdog) backoffLocked(failures int) time.Duration {
	backoff := w.cfg.Trader.Backoff
	if failures >= w.cfg.Trader.MaxFailures {
		return w.cfg.Trader.RetryCooldown.Std()
	}
	if len(backoff) == 0 {
		return 15 * time.Second
	}
	index := failures - 1
	if index < 0 {
		index = 0
	}
	if index >= len(backoff) {
		index = len(backoff) - 1
	}
	return backoff[index].Std()
}

func (w *watchdog) alertCrashLoop(failures int, err error) {
	if failures < w.cfg.Trader.MaxFailures {
		return
	}
	w.alert("crashloop", 15*time.Minute, "⚠️ MOEX trader: %s трейдер не стартует (%d ошибок подряд): %v", w.cfg.NodeName, failures, err)
}

func (w *watchdog) syncPassive(ctx context.Context, now time.Time) {
	w.mu.Lock()
	if w.runningLocked() {
		w.mu.Unlock()
		return
	}
	lastPull := w.lastPullAt
	w.mu.Unlock()
	if !lastPull.IsZero() && now.Sub(lastPull) < w.cfg.DB.PullInterval.Std() {
		return
	}
	w.mu.Lock()
	w.lastPullAt = now
	w.mu.Unlock()
	if err := w.pullDB(ctx, now); err != nil {
		w.logger.Printf("watchdog: db sync skipped: %v", err)
	}
}

func (w *watchdog) pullDB(ctx context.Context, now time.Time) error {
	w.mu.Lock()
	peerRecent := w.peerSeen && now.Sub(w.lastPeerSeen) <= w.params.PeerStaleAfter*10
	running := w.runningLocked()
	force := !w.syncedFromPeer
	authoritative := w.peerAuthoritativeLocked(now)
	w.mu.Unlock()
	if running {
		return errors.New("local trader is running")
	}
	if !peerRecent {
		return errors.New("peer not seen recently")
	}
	if !authoritative {
		return errors.New("peer db is not authoritative")
	}
	updated, err := w.peer.FetchDB(ctx, w.cfg.DB.Path, force)
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}
	w.mu.Lock()
	w.syncedFromPeer = true
	w.mu.Unlock()
	freshness := failover.DBFreshness(w.cfg.DB.Path)
	age := now.Sub(freshness)
	w.logger.Printf("watchdog: db synced from peer (age=%s)", age.Round(time.Second))
	return nil
}

func (w *watchdog) peerAuthoritativeLocked(now time.Time) bool {
	if !w.peerSeen {
		return false
	}
	if w.peerHB.Trader.Running {
		return true
	}
	peerStart := w.peerHB.Trader.LastStartAt
	if peerStart.IsZero() {
		return false
	}
	if w.lastTraderStart.IsZero() {
		return true
	}
	return peerStart.After(w.lastTraderStart)
}

func (w *watchdog) input(now time.Time) failover.Input {
	w.mu.Lock()
	defer w.mu.Unlock()
	var peer *failover.Heartbeat
	if w.peerSeen {
		heartbeat := w.peerHB
		peer = &heartbeat
	}
	return failover.Input{
		Role:                w.role,
		Node:                w.cfg.NodeName,
		Now:                 now,
		Lease:               w.lease,
		Peer:                peer,
		LastPeerSeen:        w.lastPeerSeen,
		TraderRunning:       w.runningLocked(),
		YieldRequestedAt:    w.yieldRequested,
		LastYieldAt:         w.lastYieldAt,
		LastYieldRequestAt:  w.lastYieldReqAt,
		LastTraderFailureAt: w.lastFailureAt,
		SelfInhibitedUntil:  w.nextStartAt,
		Params:              w.params,
	}
}

func (w *watchdog) heartbeat(now time.Time) failover.Heartbeat {
	w.mu.Lock()
	defer w.mu.Unlock()
	heartbeat := failover.Heartbeat{
		Node:      w.cfg.NodeName,
		Role:      w.role,
		State:     w.state,
		Reason:    w.reason,
		Since:     w.stateSince,
		UpdatedAt: now,
		Witness: failover.WitnessView{
			Reachable: w.lease.Reachable,
			Holder:    w.lease.Holder,
		},
		Trader: failover.TraderView{
			Running:       w.runningLocked(),
			StartedAt:     w.traderStarted,
			LastStartAt:   w.lastTraderStart,
			Restarts:      w.restarts,
			LastFailureAt: w.lastFailureAt,
		},
	}
	if !w.lease.ExpiresAt.IsZero() {
		expiresAt := w.lease.ExpiresAt
		heartbeat.Witness.ExpiresAt = &expiresAt
	}
	return heartbeat
}

func (w *watchdog) runningLocked() bool {
	return w.trader != nil && w.trader.alive()
}

func (w *watchdog) setStateLocked(state failover.NodeState, reason string, now time.Time) {
	if w.state == state {
		w.reason = reason
		return
	}
	w.state = state
	w.stateSince = now
	w.reason = reason
	w.logger.Printf("watchdog: state -> %s (%s)", state, reason)
}

func (w *watchdog) requestYield(now time.Time) {
	w.mu.Lock()
	w.yieldRequested = now
	w.mu.Unlock()
}

func (w *watchdog) alert(key string, cooldown time.Duration, format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if w.alerter == nil || !w.alerter.Enabled() {
		w.logger.Printf("watchdog: alert: %s", text)
		return
	}
	w.alertMu.Lock()
	if last, ok := w.alerts[key]; ok && time.Since(last) < cooldown {
		w.alertMu.Unlock()
		return
	}
	w.alerts[key] = time.Now()
	w.alertMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := w.alerter.Send(ctx, text); err != nil {
		w.logger.Printf("watchdog: telegram alert: %v", err)
	}
}

func (w *watchdog) shutdown() error {
	w.mu.Lock()
	proc := w.trader
	w.trader = nil
	held := w.lease.HeldBy(w.cfg.NodeName, w.now())
	w.mu.Unlock()
	if proc != nil {
		proc.stop(w.cfg.Trader.StopGrace.Std(), w.logger)
	}
	if held {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.witness.Release(ctx, w.cfg.NodeName); err != nil {
			w.logger.Printf("watchdog: release lease on shutdown: %v", err)
		}
	}
	w.logger.Printf("watchdog: stopped")
	return nil
}

func (w *watchdog) runTunnel(ctx context.Context) {
	command := w.cfg.Tunnel.Command
	if len(command) == 0 {
		return
	}
	for ctx.Err() == nil {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			w.logger.Printf("watchdog: tunnel start: %v", err)
			if !sleepCtx(ctx, 5*time.Second) {
				return
			}
			continue
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			<-done
			return
		case err := <-done:
			w.logger.Printf("watchdog: tunnel exited: %v", err)
			if !sleepCtx(ctx, 5*time.Second) {
				return
			}
		}
	}
}

func sleepCtx(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
