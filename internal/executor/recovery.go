package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	pb "github.com/tinkoff/invest-api-go-sdk/proto"

	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/storage"
)

const (
	statusRequiresReconciliation = "requires_reconciliation"
	statusNew                    = "new"
	statusCancelledOnRecovery    = "cancelled_on_recovery"
	statusClosedOnRecovery       = "closed_on_recovery"
)

type OrderRecoveryBroker interface {
	OpenOrders(ctx context.Context) ([]*pb.OrderState, error)
	CancelOrder(ctx context.Context, brokerOrderID string) error
}

type RecoveryReport struct {
	Pending   int
	Cancelled int
	Closed    int
}

func RecoverPendingOrders(ctx context.Context, store *storage.Store, broker OrderRecoveryBroker) (RecoveryReport, error) {
	var report RecoveryReport
	if store == nil || broker == nil {
		return report, errors.New("order recovery: store and broker are required")
	}
	events, err := store.ListAllAuditEvents(ctx)
	if err != nil {
		return report, fmt.Errorf("order recovery: list audit events: %w", err)
	}
	var pending []domain.AuditEvent
	for _, event := range events {
		if event.Stage != "executor" {
			continue
		}
		var order persistedOrder
		if err := json.Unmarshal([]byte(event.Payload), &order); err != nil {
			continue
		}
		if order.Status == statusRequiresReconciliation || order.Status == statusNew {
			pending = append(pending, event)
		}
	}
	report.Pending = len(pending)
	if len(pending) == 0 {
		return report, nil
	}

	open, err := broker.OpenOrders(ctx)
	if err != nil {
		return report, fmt.Errorf("order recovery: list open orders: %w", err)
	}
	byRequestID := make(map[string]string, len(open))
	for _, order := range open {
		if requestID := order.GetOrderRequestId(); requestID != "" {
			byRequestID[requestID] = order.GetOrderId()
		}
	}

	var errs []error
	for _, event := range pending {
		status := statusClosedOnRecovery
		brokerID, isOpen := byRequestID[event.ID]
		if isOpen {
			if err := broker.CancelOrder(ctx, brokerID); err != nil {
				errs = append(errs, fmt.Errorf("order recovery: cancel %q: %w", event.ID, err))
				continue
			}
			status = statusCancelledOnRecovery
		}
		if err := markRecovered(ctx, store, event, status); err != nil {
			errs = append(errs, err)
			continue
		}
		if isOpen {
			report.Cancelled++
		} else {
			report.Closed++
		}
	}
	return report, errors.Join(errs...)
}

func markRecovered(ctx context.Context, store *storage.Store, event domain.AuditEvent, status string) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
		return fmt.Errorf("order recovery: decode %q: %w", event.ID, err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		return err
	}
	payload["status"] = encoded
	updated, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("order recovery: encode %q: %w", event.ID, err)
	}
	event.Payload = string(updated)
	if err := store.UpsertAuditEvent(ctx, event); err != nil {
		return fmt.Errorf("order recovery: persist %q: %w", event.ID, err)
	}
	return nil
}
