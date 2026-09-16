package failover

import "time"

type Params struct {
	PeerStaleAfter       time.Duration
	FailoverAfter        time.Duration
	PeerErrorGrace       time.Duration
	HandoverCooldown     time.Duration
	YieldRequestCooldown time.Duration
	YieldRequestTTL      time.Duration
	StartupGrace         time.Duration
	FailureMemory        time.Duration
}

func (p Params) withDefaults() Params {
	if p.PeerStaleAfter <= 0 {
		p.PeerStaleAfter = 30 * time.Second
	}
	if p.FailoverAfter <= 0 {
		p.FailoverAfter = 10 * time.Minute
	}
	if p.PeerErrorGrace <= 0 {
		p.PeerErrorGrace = 2 * time.Minute
	}
	if p.HandoverCooldown <= 0 {
		p.HandoverCooldown = 30 * time.Minute
	}
	if p.YieldRequestCooldown <= 0 {
		p.YieldRequestCooldown = 60 * time.Second
	}
	if p.YieldRequestTTL <= 0 {
		p.YieldRequestTTL = 2 * time.Minute
	}
	if p.StartupGrace <= 0 {
		p.StartupGrace = 15 * time.Minute
	}
	if p.FailureMemory <= 0 {
		p.FailureMemory = 30 * time.Minute
	}
	return p
}

type Input struct {
	Role                Role
	Node                string
	Now                 time.Time
	Lease               LeaseStatus
	Peer                *Heartbeat
	LastPeerSeen        time.Time
	TraderRunning       bool
	YieldRequestedAt    time.Time
	LastYieldAt         time.Time
	LastYieldRequestAt  time.Time
	LastTraderFailureAt time.Time
	SelfInhibitedUntil  time.Time
	Params              Params
}

type Decision struct {
	AcquireLease     bool
	ReleaseLease     bool
	RunTrader        bool
	RequestPeerYield bool
	Reason           string
}

func Evaluate(in Input) Decision {
	p := in.Params.withDefaults()
	peerAge := time.Duration(1) << 62
	if !in.LastPeerSeen.IsZero() {
		peerAge = in.Now.Sub(in.LastPeerSeen)
		if peerAge < 0 {
			peerAge = 0
		}
	}
	peerFresh := in.Peer != nil && peerAge <= p.PeerStaleAfter
	peerRunning := false
	peerBroken := false
	peerWilling := false
	if peerFresh {
		peerRunning = in.Peer.Trader.Running
		switch in.Peer.State {
		case StateError, StateFenced:
			peerBroken = !peerRunning
		case StateStarting:
			if in.Now.Sub(in.Peer.TraderStartedAt()) > 2*p.StartupGrace {
				peerBroken = true
			}
		}
		peerWilling = !peerRunning && in.Peer.Witness.Reachable && in.Peer.State == StateStandby
	}

	held := in.Lease.HeldBy(in.Node, in.Now)

	recentFailure := !in.LastTraderFailureAt.IsZero() && in.Now.Sub(in.LastTraderFailureAt) < p.FailureMemory
	yieldRequestFresh := !in.YieldRequestedAt.IsZero() && in.Now.Sub(in.YieldRequestedAt) <= p.YieldRequestTTL
	cooldownElapsed := in.LastYieldAt.IsZero() || in.Now.Sub(in.LastYieldAt) >= p.HandoverCooldown

	if held && in.Role == RoleStandby && yieldRequestFresh && peerWilling && cooldownElapsed {
		return Decision{ReleaseLease: true, Reason: "primary requested handover"}
	}
	if in.TraderRunning && !held {
		return Decision{RunTrader: false, Reason: "lease lost, fencing local trader"}
	}
	if held {
		return Decision{AcquireLease: true, RunTrader: true, Reason: "lease held"}
	}

	switch in.Role {
	case RoleStandby:
		if peerFresh {
			if peerBroken && in.Now.Sub(in.Peer.StateSince()) >= p.PeerErrorGrace {
				return acquire(in, "peer trader failed")
			}
			return Decision{Reason: "peer fresh, staying standby"}
		}
		if peerAge >= p.FailoverAfter {
			return acquire(in, "peer silent")
		}
		return Decision{Reason: "peer silent, waiting for failover window"}
	case RolePrimary:
		if peerFresh && peerRunning {
			if in.Lease.Reachable && !recentFailure && in.Now.Sub(in.LastYieldRequestAt) >= p.YieldRequestCooldown {
				return Decision{RequestPeerYield: true, Reason: "peer active, requesting handover"}
			}
			return Decision{Reason: "peer active"}
		}
		return acquire(in, "primary wants lease")
	}
	return Decision{Reason: "unknown role"}
}

func acquire(in Input, reason string) Decision {
	if !in.SelfInhibitedUntil.IsZero() && in.Now.Before(in.SelfInhibitedUntil) {
		return Decision{Reason: reason + ": local trader inhibited after failures"}
	}
	if in.Lease.HeldByOther(in.Node, in.Now) {
		return Decision{Reason: reason + ": peer holds lease"}
	}
	return Decision{AcquireLease: true, Reason: reason}
}
