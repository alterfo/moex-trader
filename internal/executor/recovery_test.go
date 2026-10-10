package executor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

type fakeRecoveryBroker struct {
	open      []*pb.OrderState
	cancelled []string
}

func (f *fakeRecoveryBroker) OpenOrders(context.Context) ([]*pb.OrderState, error) {
	return f.open, nil
}

func (f *fakeRecoveryBroker) CancelOrder(_ context.Context, id string) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}

func seedExecutorEvent(t *testing.T, exec *LiveExecutor, id, status string) {
	t.Helper()
	payload, err := json.Marshal(liveOrderIntent{Status: status, Ticker: "SBER", Action: domain.ActionBuy, TargetLots: 1})
	if err != nil {
		t.Fatal(err)
	}
	err = exec.store.UpsertAuditEvent(context.Background(), domain.AuditEvent{
		ID: id, Ticker: "SBER", Stage: "executor", Payload: string(payload), CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecoverPendingOrdersCancelsOpenOrderAndClosesTheRest(t *testing.T) {
	exec := newLiveExecutorForTest(t, &fakeOrderPoster{}, time.Now())
	seedExecutorEvent(t, exec, "req-open", statusRequiresReconciliation)
	seedExecutorEvent(t, exec, "req-gone", statusRequiresReconciliation)
	seedExecutorEvent(t, exec, "req-done", "filled")
	broker := &fakeRecoveryBroker{open: []*pb.OrderState{
		{OrderId: "broker-1", OrderRequestId: "req-open"},
		{OrderId: "broker-foreign", OrderRequestId: "someone-else"},
	}}

	report, err := RecoverPendingOrders(context.Background(), exec.store, broker)
	if err != nil {
		t.Fatalf("RecoverPendingOrders() error = %v", err)
	}
	if report.Pending != 2 || report.Cancelled != 1 || report.Closed != 1 {
		t.Fatalf("report = %+v, want pending=2 cancelled=1 closed=1", report)
	}
	if len(broker.cancelled) != 1 || broker.cancelled[0] != "broker-1" {
		t.Fatalf("cancelled = %v, want only broker-1", broker.cancelled)
	}
	for id, want := range map[string]string{
		"req-open": statusCancelledOnRecovery,
		"req-gone": statusClosedOnRecovery,
		"req-done": "filled",
	} {
		_, status, found, err := exec.loadPersistedOrder(context.Background(), id)
		if err != nil || !found || status != want {
			t.Fatalf("order %s: status=%q found=%v err=%v, want %q", id, status, found, err, want)
		}
	}

	again, err := RecoverPendingOrders(context.Background(), exec.store, broker)
	if err != nil || again.Pending != 0 {
		t.Fatalf("second run = %+v, err=%v, want nothing pending", again, err)
	}
}
