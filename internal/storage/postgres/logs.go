package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bingbr/League-API-bot/internal/storage/logs"
)

func (db *Database) CreateLogTable(ctx context.Context) error {
	return db.createTable(ctx, createLogTableSQL, "create bot_logs table")
}

func (db *Database) Insert(ctx context.Context, entry logs.LogEntry) error {
	if err := db.ensureReady(); err != nil {
		return err
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	if entry.Attrs == nil {
		entry.Attrs = map[string]any{}
	}
	payload, err := json.Marshal(entry.Attrs)
	if err != nil {
		return fmt.Errorf("marshal log attrs: %w", err)
	}
	if _, err := db.pool.Exec(ctx, insertLogSQL, entry.Time, entry.Level, entry.Message, payload); err != nil {
		return fmt.Errorf("insert log entry: %w", err)
	}
	return nil
}

const insertLogSQL = `
INSERT INTO bot_logs (logged_at, level, message, attrs)
VALUES ($1, $2, $3, $4::jsonb)
`

const createLogTableSQL = `
CREATE TABLE IF NOT EXISTS bot_logs (
	id bigserial PRIMARY KEY,
	logged_at timestamptz NOT NULL,
	level text NOT NULL,
	message text NOT NULL,
	attrs jsonb NOT NULL DEFAULT '{}'::jsonb
)
`
