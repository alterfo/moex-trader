package failover

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

func testParams() Params {
	return Params{
		PeerStaleAfter:       30 * time.Second,
		FailoverAfter:        10 * time.Minute,
		PeerErrorGrace:       2 * time.Minute,
		HandoverCooldown:     30 * time.Minute,
		YieldRequestCooldown: 60 * time.Second,
		YieldRequestTTL:      2 * time.Minute,
		StartupGrace:         15 * time.Minute,
		FailureMemory:        30 * time.Minute,
	}
}

func peerHeartbeat(node string, state NodeState, running bool, since time.Time) *Heartbeat {
	return &Heartbeat{
		Node:      node,
		Role:      RolePrimary,
		State:     state,
		Since:     since,
		UpdatedAt: testNow,
		Witness:   WitnessView{Reachable: true},
		Trader:    TraderView{Running: running, StartedAt: since},
	}
}

func baseInput(role Role) Input {
	return Input{
		Role:         role,
		Node:         "mac",
		Now:          testNow,
		Lease:        LeaseStatus{Reachable: true, Holder: "", ExpiresAt: testNow.Add(5 * time.Minute)},
		LastPeerSeen: testNow.Add(-5 * time.Second),
		Peer:         peerHeartbeat("aibox", StateStandby, false, testNow.Add(-time.Hour)),
		Params:       testParams(),
	}
}

func TestPrimaryAcquiresWithoutPeer(t *testing.T) {
	in := baseInput(RolePrimary)
	in.Peer = nil
	in.LastPeerSeen = time.Time{}

	decision := Evaluate(in)
	if !decision.AcquireLease || decision.RunTrader {
		t.Fatalf("decision = %+v, want acquire without running before the lease is granted", decision)
	}
}

func TestPrimaryAcquiresWhenPeerIdle(t *testing.T) {
	decision := Evaluate(baseInput(RolePrimary))
	if !decision.AcquireLease {
		t.Fatalf("decision = %+v, want acquire", decision)
	}
}

func TestPrimaryWaitsWhenPeerActive(t *testing.T) {
	in := baseInput(RolePrimary)
	in.Peer = peerHeartbeat("aibox", StateActive, true, testNow.Add(-time.Hour))
	in.LastYieldRequestAt = testNow.Add(-30 * time.Second)

	decision := Evaluate(in)
	if decision.AcquireLease || decision.RequestPeerYield || decision.RunTrader {
		t.Fatalf("decision = %+v, want wait", decision)
	}
}

func TestPrimaryRequestsHandoverWhenPeerActive(t *testing.T) {
	in := baseInput(RolePrimary)
	in.Peer = peerHeartbeat("aibox", StateActive, true, testNow.Add(-time.Hour))

	decision := Evaluate(in)
	if !decision.RequestPeerYield {
		t.Fatalf("decision = %+v, want handover request", decision)
	}
}

func TestPrimaryDoesNotRequestHandoverAfterRecentFailure(t *testing.T) {
	in := baseInput(RolePrimary)
	in.Peer = peerHeartbeat("aibox", StateActive, true, testNow.Add(-time.Hour))
	in.LastTraderFailureAt = testNow.Add(-5 * time.Minute)

	decision := Evaluate(in)
	if decision.RequestPeerYield || decision.AcquireLease {
		t.Fatalf("decision = %+v, want no handover after recent failure", decision)
	}
}

func TestPrimaryDoesNotAcquireWhilePeerHoldsLease(t *testing.T) {
	in := baseInput(RolePrimary)
	in.Peer = peerHeartbeat("aibox", StateError, false, testNow.Add(-10*time.Minute))
	in.Lease.Holder = "aibox"

	decision := Evaluate(in)
	if decision.AcquireLease {
		t.Fatalf("decision = %+v, want wait for peer lease to expire", decision)
	}
}

func TestStandbyWaitsWhilePeerActive(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Peer = peerHeartbeat("aibox", StateActive, true, testNow.Add(-time.Hour))

	decision := Evaluate(in)
	if decision.AcquireLease || decision.RunTrader {
		t.Fatalf("decision = %+v, want standby", decision)
	}
}

func TestStandbyWaitsInsideFailoverWindow(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Peer = nil
	in.LastPeerSeen = testNow.Add(-9 * time.Minute)

	decision := Evaluate(in)
	if decision.AcquireLease {
		t.Fatalf("decision = %+v, want wait inside failover window", decision)
	}
}

func TestStandbyAcquiresAfterFailoverWindow(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Peer = nil
	in.LastPeerSeen = testNow.Add(-10*time.Minute - time.Second)

	decision := Evaluate(in)
	if !decision.AcquireLease {
		t.Fatalf("decision = %+v, want acquire after failover window", decision)
	}
}

func TestStandbyWaitsInsidePeerErrorGrace(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Peer = peerHeartbeat("aibox", StateError, false, testNow.Add(-time.Minute))

	decision := Evaluate(in)
	if decision.AcquireLease {
		t.Fatalf("decision = %+v, want wait inside error grace", decision)
	}
}

func TestStandbyAcquiresAfterPeerErrorGrace(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Peer = peerHeartbeat("aibox", StateError, false, testNow.Add(-3*time.Minute))

	decision := Evaluate(in)
	if !decision.AcquireLease {
		t.Fatalf("decision = %+v, want acquire after error grace", decision)
	}
}

func TestStandbyAcquiresWhenPeerStuckStarting(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Peer = peerHeartbeat("aibox", StateStarting, true, testNow.Add(-40*time.Minute))

	decision := Evaluate(in)
	if !decision.AcquireLease {
		t.Fatalf("decision = %+v, want acquire when peer startup is stuck", decision)
	}
}

func TestStandbyYieldsOnHandoverRequest(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Node = "aibox"
	in.TraderRunning = true
	in.Peer = peerHeartbeat("mac", StateStandby, false, testNow.Add(-time.Minute))
	in.LastPeerSeen = testNow
	in.Lease.Holder = "aibox"
	in.YieldRequestedAt = testNow.Add(-10 * time.Second)

	decision := Evaluate(in)
	if !decision.ReleaseLease || decision.RunTrader {
		t.Fatalf("decision = %+v, want release for handover", decision)
	}
}

func TestStandbyIgnoresHandoverRequestDuringCooldown(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Node = "aibox"
	in.TraderRunning = true
	in.Peer = peerHeartbeat("mac", StateStandby, false, testNow.Add(-time.Minute))
	in.LastPeerSeen = testNow
	in.Lease.Holder = "aibox"
	in.YieldRequestedAt = testNow.Add(-10 * time.Second)
	in.LastYieldAt = testNow.Add(-5 * time.Minute)

	decision := Evaluate(in)
	if decision.ReleaseLease {
		t.Fatalf("decision = %+v, want handover blocked by cooldown", decision)
	}
}

func TestStandbyIgnoresStaleHandoverRequest(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Node = "aibox"
	in.TraderRunning = true
	in.Peer = peerHeartbeat("mac", StateStandby, false, testNow.Add(-time.Minute))
	in.LastPeerSeen = testNow
	in.Lease.Holder = "aibox"
	in.YieldRequestedAt = testNow.Add(-5 * time.Minute)

	decision := Evaluate(in)
	if decision.ReleaseLease {
		t.Fatalf("decision = %+v, want stale request ignored", decision)
	}
}

func TestFencingWhenLeaseLost(t *testing.T) {
	in := baseInput(RolePrimary)
	in.TraderRunning = true
	in.Lease.Holder = "aibox"

	decision := Evaluate(in)
	if decision.RunTrader {
		t.Fatalf("decision = %+v, want fence", decision)
	}
}

func TestHeldLeaseKeepsRunning(t *testing.T) {
	in := baseInput(RolePrimary)
	in.TraderRunning = true
	in.Lease.Holder = "mac"

	decision := Evaluate(in)
	if !decision.RunTrader || !decision.AcquireLease {
		t.Fatalf("decision = %+v, want renew and run", decision)
	}
}

func TestFencedWhenWitnessUnreachable(t *testing.T) {
	in := baseInput(RolePrimary)
	in.TraderRunning = true
	in.Lease = LeaseStatus{Reachable: false, Holder: "mac", ExpiresAt: testNow.Add(-time.Second)}

	decision := Evaluate(in)
	if decision.RunTrader {
		t.Fatalf("decision = %+v, want fence on expired lease", decision)
	}
}

func TestAcquireBlockedWhenPeerLeaseStillValid(t *testing.T) {
	in := baseInput(RoleStandby)
	in.Peer = nil
	in.LastPeerSeen = testNow.Add(-20 * time.Minute)
	in.Lease.Holder = "aibox"
	in.Lease.ExpiresAt = testNow.Add(2 * time.Minute)

	decision := Evaluate(in)
	if decision.AcquireLease {
		t.Fatalf("decision = %+v, want wait for peer lease", decision)
	}
}

func TestLeaseStatusHelpers(t *testing.T) {
	lease := LeaseStatus{Reachable: true, Holder: "mac", ExpiresAt: testNow.Add(time.Minute)}
	if !lease.HeldBy("mac", testNow) {
		t.Fatalf("HeldBy(mac) = false, want true")
	}
	if lease.HeldBy("aibox", testNow) {
		t.Fatalf("HeldBy(aibox) = true, want false")
	}
	if !lease.HeldByOther("aibox", testNow) {
		t.Fatalf("HeldByOther(aibox) = false, want true")
	}
	if lease.HeldByOther("mac", testNow) {
		t.Fatalf("HeldByOther(mac) = true, want false")
	}
	expired := LeaseStatus{Reachable: true, Holder: "mac", ExpiresAt: testNow.Add(-time.Second)}
	if expired.HeldBy("mac", testNow) {
		t.Fatalf("expired lease HeldBy = true, want false")
	}
	unreachable := LeaseStatus{Reachable: false, Holder: "mac", ExpiresAt: testNow.Add(time.Minute)}
	if unreachable.HeldBy("mac", testNow) {
		t.Fatalf("unreachable lease HeldBy = true, want false")
	}
}
