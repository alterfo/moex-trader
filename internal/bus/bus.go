package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/olegsidorkin/moex-trader/internal/domain"
)

const (
	DefaultStream   = "trader:stream"
	TypeTradeSignal = "trade_signal"
	TypeAuditEvent  = "audit_event"

	defaultBlock = 5 * time.Second
)

type Streamer interface {
	XAdd(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd
	XRead(ctx context.Context, a *redis.XReadArgs) *redis.XStreamSliceCmd
	Close() error
}

type RedisOptions struct {
	Addr     string
	Password string
	DB       int
}

func NewRedisStreamer(opts RedisOptions) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
	})
}

type Publisher struct {
	streamer Streamer
	stream   string
}

func NewPublisher(streamer Streamer, stream string) (*Publisher, error) {
	if streamer == nil {
		return nil, fmt.Errorf("bus: streamer is required")
	}
	if stream == "" {
		stream = DefaultStream
	}
	return &Publisher{streamer: streamer, stream: stream}, nil
}

func (p *Publisher) PublishTradeSignal(ctx context.Context, signal domain.TradeSignal) (string, error) {
	return p.publish(ctx, TypeTradeSignal, signal.Ticker, signal)
}

func (p *Publisher) PublishAuditEvent(ctx context.Context, event domain.AuditEvent) (string, error) {
	return p.publish(ctx, TypeAuditEvent, event.Ticker, event)
}

func (p *Publisher) publish(ctx context.Context, messageType, ticker string, value any) (string, error) {
	if p == nil || p.streamer == nil {
		return "", fmt.Errorf("bus: publisher is not initialized")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("bus: marshal %s: %w", messageType, err)
	}
	id, err := p.streamer.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]any{
			"type":    messageType,
			"ticker":  ticker,
			"payload": string(payload),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("bus: xadd %s to %s: %w", messageType, p.stream, err)
	}
	return id, nil
}

type Message struct {
	ID      string
	Type    string
	Ticker  string
	Payload json.RawMessage
}

func (m Message) DecodeTradeSignal() (domain.TradeSignal, error) {
	var signal domain.TradeSignal
	if err := json.Unmarshal(m.Payload, &signal); err != nil {
		return signal, fmt.Errorf("bus: decode trade signal %s: %w", m.ID, err)
	}
	return signal, nil
}

func (m Message) DecodeAuditEvent() (domain.AuditEvent, error) {
	var event domain.AuditEvent
	if err := json.Unmarshal(m.Payload, &event); err != nil {
		return event, fmt.Errorf("bus: decode audit event %s: %w", m.ID, err)
	}
	return event, nil
}

type Consumer struct {
	streamer Streamer
	stream   string
	block    time.Duration
}

func NewConsumer(streamer Streamer, stream string) (*Consumer, error) {
	if streamer == nil {
		return nil, fmt.Errorf("bus: streamer is required")
	}
	if stream == "" {
		stream = DefaultStream
	}
	return &Consumer{streamer: streamer, stream: stream, block: defaultBlock}, nil
}

func (c *Consumer) Read(ctx context.Context, since string) ([]Message, error) {
	return c.read(ctx, since, c.block)
}

func (c *Consumer) ReadNow(ctx context.Context, since string) ([]Message, error) {
	return c.read(ctx, since, -1)
}

func (c *Consumer) read(ctx context.Context, since string, block time.Duration) ([]Message, error) {
	if c == nil || c.streamer == nil {
		return nil, fmt.Errorf("bus: consumer is not initialized")
	}
	if since == "" {
		since = "$"
	}
	streams, err := c.streamer.XRead(ctx, &redis.XReadArgs{
		Streams: []string{c.stream, since},
		Count:   100,
		Block:   block,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []Message{}, nil
		}
		return nil, fmt.Errorf("bus: xread %s: %w", c.stream, err)
	}
	messages := make([]Message, 0)
	for _, stream := range streams {
		for _, entry := range stream.Messages {
			messages = append(messages, newMessage(entry))
		}
	}
	return messages, nil
}

func newMessage(entry redis.XMessage) Message {
	message := Message{ID: entry.ID}
	if value, ok := entry.Values["type"]; ok {
		message.Type = fmt.Sprint(value)
	}
	if value, ok := entry.Values["ticker"]; ok {
		message.Ticker = fmt.Sprint(value)
	}
	if value, ok := entry.Values["payload"]; ok {
		if payload, ok := value.(string); ok {
			message.Payload = json.RawMessage(payload)
		}
	}
	return message
}
