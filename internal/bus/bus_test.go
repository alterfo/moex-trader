package bus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

func newTestStreamer(t *testing.T) Streamer {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

func TestNewPublisherRequiresStreamer(t *testing.T) {
	if _, err := NewPublisher(nil, ""); err == nil {
		t.Fatal("NewPublisher(nil) expected error")
	}
}

func TestNewConsumerRequiresStreamer(t *testing.T) {
	if _, err := NewConsumer(nil, ""); err == nil {
		t.Fatal("NewConsumer(nil) expected error")
	}
}

func TestPublisherConsumerRoundTrip(t *testing.T) {
	streamer := newTestStreamer(t)
	ctx := context.Background()
	stream := "test:stream"

	publisher, err := NewPublisher(streamer, stream)
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}
	consumer, err := NewConsumer(streamer, stream)
	if err != nil {
		t.Fatalf("NewConsumer() error = %v", err)
	}

	generatedAt := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	signal := domain.TradeSignal{
		Ticker:      "SBER",
		Action:      domain.ActionBuy,
		Confidence:  decimal.NewFromFloat(0.8),
		TargetLots:  1,
		Reasoning:   "positive momentum",
		GeneratedAt: generatedAt,
	}
	signalID, err := publisher.PublishTradeSignal(ctx, signal)
	if err != nil {
		t.Fatalf("PublishTradeSignal() error = %v", err)
	}
	if signalID == "" {
		t.Fatal("PublishTradeSignal() returned empty stream ID")
	}

	event := domain.AuditEvent{
		ID:        "audit-1",
		Ticker:    "SBER",
		Stage:     "llm",
		Payload:   `{"action":"BUY"}`,
		CreatedAt: generatedAt,
	}
	if _, err := publisher.PublishAuditEvent(ctx, event); err != nil {
		t.Fatalf("PublishAuditEvent() error = %v", err)
	}

	messages, err := consumer.ReadNow(ctx, "0")
	if err != nil {
		t.Fatalf("ReadNow() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("ReadNow() messages = %d, want 2", len(messages))
	}

	if messages[0].Type != TypeTradeSignal || messages[0].Ticker != "SBER" {
		t.Fatalf("first message = %+v, want trade signal for SBER", messages[0])
	}
	gotSignal, err := messages[0].DecodeTradeSignal()
	if err != nil {
		t.Fatalf("DecodeTradeSignal() error = %v", err)
	}
	if gotSignal.Ticker != signal.Ticker ||
		gotSignal.Action != signal.Action ||
		!gotSignal.Confidence.Equal(signal.Confidence) ||
		gotSignal.TargetLots != signal.TargetLots ||
		gotSignal.Reasoning != signal.Reasoning ||
		!gotSignal.GeneratedAt.Equal(signal.GeneratedAt) {
		t.Fatalf("decoded signal = %+v, want %+v", gotSignal, signal)
	}

	if messages[1].Type != TypeAuditEvent || messages[1].Ticker != "SBER" {
		t.Fatalf("second message = %+v, want audit event for SBER", messages[1])
	}
	gotEvent, err := messages[1].DecodeAuditEvent()
	if err != nil {
		t.Fatalf("DecodeAuditEvent() error = %v", err)
	}
	if gotEvent != event {
		t.Fatalf("decoded event = %+v, want %+v", gotEvent, event)
	}
}

func TestDefaultStreamUsedWhenEmpty(t *testing.T) {
	streamer := newTestStreamer(t)
	ctx := context.Background()

	publisher, err := NewPublisher(streamer, "")
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}
	consumer, err := NewConsumer(streamer, "")
	if err != nil {
		t.Fatalf("NewConsumer() error = %v", err)
	}

	if _, err := publisher.PublishTradeSignal(ctx, domain.TradeSignal{
		Ticker: "YDEX", Action: domain.ActionHold, Confidence: decimal.NewFromInt(1),
	}); err != nil {
		t.Fatalf("PublishTradeSignal() error = %v", err)
	}

	messages, err := consumer.ReadNow(ctx, "0")
	if err != nil {
		t.Fatalf("ReadNow() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("ReadNow() messages = %d, want 1", len(messages))
	}
	if messages[0].Ticker != "YDEX" {
		t.Fatalf("ticker = %q, want YDEX", messages[0].Ticker)
	}
}

func TestReadNowEmptyStream(t *testing.T) {
	streamer := newTestStreamer(t)
	consumer, err := NewConsumer(streamer, "missing:stream")
	if err != nil {
		t.Fatalf("NewConsumer() error = %v", err)
	}
	messages, err := consumer.ReadNow(context.Background(), "0")
	if err != nil {
		t.Fatalf("ReadNow() error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("ReadNow() messages = %d, want 0", len(messages))
	}
}

func TestDecodeTradeSignalWrongPayload(t *testing.T) {
	message := Message{ID: "1", Type: TypeTradeSignal, Payload: []byte(`{"ticker":`)}
	if _, err := message.DecodeTradeSignal(); err == nil {
		t.Fatal("DecodeTradeSignal() expected error for malformed payload")
	} else if !strings.Contains(err.Error(), "decode trade signal") {
		t.Fatalf("unexpected error: %v", err)
	}
}
