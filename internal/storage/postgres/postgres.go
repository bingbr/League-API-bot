package postgres

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDbNotInitialized = errors.New("postgres database not initialized")
	ErrSyncKeyRequired  = errors.New("sync key is required")
)

type Database struct {
	pool *pgxpool.Pool
}

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	cfg.MaxConns = int32(max(4, runtime.GOMAXPROCS(0)*2))
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 5 * time.Minute
	cfg.MaxConnIdleTime = 1 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func NewDB(pool *pgxpool.Pool) *Database {
	return &Database{pool: pool}
}

func (db *Database) ensureReady() error {
	if db == nil || db.pool == nil {
		return ErrDbNotInitialized
	}
	return nil
}

func (db *Database) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if err := db.ensureReady(); err != nil {
		return err
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (db *Database) createTable(ctx context.Context, statement, op string) error {
	if err := db.ensureReady(); err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, statement); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
