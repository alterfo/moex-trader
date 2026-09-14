package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

var ErrAuditEventNotFound = errors.New("audit event not found")

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite %q: %w", path, err)
	}
	return &Store{db: db}, nil
}

func buildDSN(path string) string {
	base := strings.TrimSpace(path)
	if base == "" {
		base = ":memory:"
	}
	if base != ":memory:" && !strings.HasPrefix(base, "file:") {
		base = "file:" + base
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + "_journal_mode=WAL&_busy_timeout=5000"
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations := []string{
		`CREATE TABLE audit_events (
			id TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			stage TEXT NOT NULL,
			payload TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE INDEX idx_audit_events_created_at ON audit_events(created_at);`,
		`CREATE TABLE trade_signals (
			id TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			action TEXT NOT NULL CHECK(action IN ('BUY', 'SELL', 'HOLD')),
			confidence TEXT NOT NULL,
			target_lots INTEGER NOT NULL CHECK(target_lots >= 0),
			reasoning TEXT NOT NULL,
			generated_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE kill_switch (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			active INTEGER NOT NULL,
			triggered_at INTEGER NOT NULL
		);
		INSERT INTO kill_switch (id, active, triggered_at) VALUES (1, 0, 0);`,
	}

	for index, statement := range migrations {
		version := index + 1
		var applied int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if applied > 0 {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, time.Now().UnixNano()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

func (s *Store) InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_events (id, ticker, stage, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		event.ID, event.Ticker, event.Stage, event.Payload, event.CreatedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("insert audit event %q: %w", event.ID, err)
	}
	return nil
}

func (s *Store) UpsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_events (id, ticker, stage, payload, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			ticker = excluded.ticker,
			stage = excluded.stage,
			payload = excluded.payload,
			created_at = excluded.created_at`,
		event.ID, event.Ticker, event.Stage, event.Payload, event.CreatedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("upsert audit event %q: %w", event.ID, err)
	}
	return nil
}

func (s *Store) GetAuditEvent(ctx context.Context, id string) (domain.AuditEvent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, ticker, stage, payload, created_at
		 FROM audit_events
		 WHERE id = ?`,
		id,
	)
	var event domain.AuditEvent
	var createdAt int64
	if err := row.Scan(&event.ID, &event.Ticker, &event.Stage, &event.Payload, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AuditEvent{}, ErrAuditEventNotFound
		}
		return domain.AuditEvent{}, fmt.Errorf("get audit event %q: %w", id, err)
	}
	event.CreatedAt = time.Unix(0, createdAt)
	return event, nil
}

func (s *Store) ListAuditEvents(ctx context.Context, since time.Time) ([]domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ticker, stage, payload, created_at
		 FROM audit_events
		 WHERE created_at >= ?
		 ORDER BY created_at ASC, id ASC`,
		since.UnixNano(),
	)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	return scanAuditEvents(rows)
}

func (s *Store) ListAllAuditEvents(ctx context.Context) ([]domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ticker, stage, payload, created_at
		 FROM audit_events
		 ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all audit events: %w", err)
	}
	defer rows.Close()

	return scanAuditEvents(rows)
}

func (s *Store) CurrentLots(ctx context.Context, ticker string) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT payload
		 FROM audit_events
		 WHERE ticker = ? AND stage = 'executor'
		 ORDER BY created_at ASC, id ASC`,
		ticker,
	)
	if err != nil {
		return 0, fmt.Errorf("list executor audit events for %q: %w", ticker, err)
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return 0, fmt.Errorf("scan executor audit event for %q: %w", ticker, err)
		}
		var fill struct {
			Action domain.Action `json:"action"`
			Lots   int           `json:"lots"`
		}
		if err := json.Unmarshal([]byte(payload), &fill); err != nil {
			continue
		}
		switch fill.Action {
		case domain.ActionBuy:
			total += fill.Lots
		case domain.ActionSell:
			total -= fill.Lots
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate executor audit events for %q: %w", ticker, err)
	}
	return total, nil
}

func scanAuditEvents(rows *sql.Rows) ([]domain.AuditEvent, error) {
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var event domain.AuditEvent
		var createdAt int64
		if err := rows.Scan(&event.ID, &event.Ticker, &event.Stage, &event.Payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		event.CreatedAt = time.Unix(0, createdAt)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

func (s *Store) InsertTradeSignal(ctx context.Context, signal domain.TradeSignal) error {
	if err := signal.Validate(); err != nil {
		return fmt.Errorf("validate trade signal: %w", err)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trade_signals (id, ticker, action, confidence, target_lots, reasoning, generated_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(),
		signal.Ticker,
		string(signal.Action),
		signal.Confidence.String(),
		signal.TargetLots,
		signal.Reasoning,
		signal.GeneratedAt.UnixNano(),
		time.Now().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("insert trade signal: %w", err)
	}
	return nil
}

func (s *Store) IsKillSwitchActive(ctx context.Context) (bool, error) {
	var active int
	if err := s.db.QueryRowContext(ctx, `SELECT active FROM kill_switch WHERE id = 1`).Scan(&active); err != nil {
		return false, fmt.Errorf("read kill switch: %w", err)
	}
	return active != 0, nil
}

func (s *Store) SetKillSwitchActive(ctx context.Context, active bool) error {
	value := 0
	triggeredAt := int64(0)
	if active {
		value = 1
		triggeredAt = time.Now().UnixNano()
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO kill_switch (id, active, triggered_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET active = excluded.active, triggered_at = excluded.triggered_at`,
		value, triggeredAt,
	); err != nil {
		return fmt.Errorf("set kill switch: %w", err)
	}
	return nil
}
