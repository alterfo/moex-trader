package failover

import "time"

type Role string

const (
	RolePrimary Role = "primary"
	RoleStandby Role = "standby"
)

func (r Role) Valid() bool {
	return r == RolePrimary || r == RoleStandby
}

type NodeState string

const (
	StateStarting NodeState = "starting"
	StateActive   NodeState = "active"
	StateStandby  NodeState = "standby"
	StateError    NodeState = "error"
	StateFenced   NodeState = "fenced"
)

type WitnessView struct {
	Reachable bool       `json:"reachable"`
	Holder    string     `json:"holder,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type TraderView struct {
	Running       bool      `json:"running"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	LastStartAt   time.Time `json:"last_start_at,omitempty"`
	Restarts      int       `json:"restarts"`
	LastFailureAt time.Time `json:"last_failure_at,omitempty"`
}

type Heartbeat struct {
	Node      string      `json:"node"`
	Role      Role        `json:"role"`
	State     NodeState   `json:"state"`
	Reason    string      `json:"reason,omitempty"`
	Since     time.Time   `json:"since"`
	UpdatedAt time.Time   `json:"updated_at"`
	Witness   WitnessView `json:"witness"`
	Trader    TraderView  `json:"trader"`
}

func (h Heartbeat) StateSince() time.Time {
	if !h.Since.IsZero() {
		return h.Since
	}
	return h.UpdatedAt
}

func (h Heartbeat) TraderStartedAt() time.Time {
	if !h.Trader.StartedAt.IsZero() {
		return h.Trader.StartedAt
	}
	return h.StateSince()
}
