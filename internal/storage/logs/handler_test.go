package logs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type stubDB struct {
	entries []LogEntry
	err     error
}

func (s *stubDB) Insert(_ context.Context, entry LogEntry) error {
	s.entries = append(s.entries, entry)
	return s.err
}

func TestDBHandler_WritesEntry(t *testing.T) {
	db := &stubDB{}
	handler := NewDBHandler(db, slog.LevelInfo, time.Second)
	logger := slog.New(handler)

	logger.Info("hello", "foo", "bar")

	if len(db.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(db.entries))
	}
	entry := db.entries[0]
	if entry.Message != "hello" {
		t.Fatalf("expected message 'hello', got %q", entry.Message)
	}
	if entry.Level != slog.LevelInfo.String() {
		t.Fatalf("expected level info, got %q", entry.Level)
	}
	if entry.Attrs["foo"] != "bar" {
		t.Fatalf("expected attrs.foo=bar, got %#v", entry.Attrs["foo"])
	}
}

func TestDBHandler_GroupedAttrs(t *testing.T) {
	db := &stubDB{}
	handler := NewDBHandler(db, slog.LevelInfo, time.Second)
	logger := slog.New(handler).WithGroup("ctx").With("user", "alice")

	logger.Info("grouped")

	if len(db.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(db.entries))
	}
	group, ok := db.entries[0].Attrs["ctx"].(map[string]any)
	if !ok {
		t.Fatalf("expected ctx group map, got %#v", db.entries[0].Attrs["ctx"])
	}
	if group["user"] != "alice" {
		t.Fatalf("expected ctx.user=alice, got %#v", group["user"])
	}
}

func TestDBHandler_MergesGroupAttrsWithSameKey(t *testing.T) {
	db := &stubDB{}
	handler := NewDBHandler(db, slog.LevelInfo, time.Second)
	logger := slog.New(handler)

	logger.Info("grouped",
		slog.Group("http", slog.String("method", "GET")),
		slog.Group("http", slog.Int("status", 200)),
	)

	if len(db.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(db.entries))
	}
	httpGroup, ok := db.entries[0].Attrs["http"].(map[string]any)
	if !ok {
		t.Fatalf("expected http group map, got %#v", db.entries[0].Attrs["http"])
	}
	if httpGroup["method"] != "GET" {
		t.Fatalf("expected http.method=GET, got %#v", httpGroup["method"])
	}
	if httpGroup["status"] != int64(200) {
		t.Fatalf("expected http.status=200, got %#v", httpGroup["status"])
	}
}

func TestNewDBHandler_DefaultTimeoutForNonPositive(t *testing.T) {
	handlerZero := NewDBHandler(&stubDB{}, slog.LevelInfo, 0)
	if handlerZero.timeout != 2*time.Second {
		t.Fatalf("expected default timeout for zero, got %v", handlerZero.timeout)
	}

	handlerNegative := NewDBHandler(&stubDB{}, slog.LevelInfo, -time.Second)
	if handlerNegative.timeout != 2*time.Second {
		t.Fatalf("expected default timeout for negative value, got %v", handlerNegative.timeout)
	}
}

func TestDBHandler_HandleWithNilContext(t *testing.T) {
	db := &stubDB{}
	handler := NewDBHandler(db, slog.LevelInfo, time.Second)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	var nilCtx context.Context

	if err := handler.Handle(nilCtx, record); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(db.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(db.entries))
	}
}

func TestDBHandler_HandleNilDB(t *testing.T) {
	handler := NewDBHandler(nil, slog.LevelInfo, time.Second)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("expected nil error for nil database, got %v", err)
	}
}

func TestDBHandler_FormatsErrorAttr(t *testing.T) {
	db := &stubDB{}
	handler := NewDBHandler(db, slog.LevelInfo, time.Second)
	logger := slog.New(handler)

	logger.Error("boom", "err", errors.New("kaboom"))

	if len(db.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(db.entries))
	}
	if db.entries[0].Attrs["err"] != "kaboom" {
		t.Fatalf("expected attrs.err=kaboom, got %#v", db.entries[0].Attrs["err"])
	}
}

func TestMultiHandler_DBReceivesDebugWhenTerminalFilters(t *testing.T) {
	db := &stubDB{}
	textHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	dbHandler := NewDBHandler(db, slog.LevelDebug, time.Second)
	logger := slog.New(slog.NewMultiHandler(textHandler, dbHandler))

	logger.Debug("debug-log", "scope", "db")

	if len(db.entries) != 1 {
		t.Fatalf("expected 1 DB entry, got %d", len(db.entries))
	}
	entry := db.entries[0]
	if entry.Level != slog.LevelDebug.String() {
		t.Fatalf("expected debug level, got %q", entry.Level)
	}
	if entry.Message != "debug-log" {
		t.Fatalf("expected debug-log message, got %q", entry.Message)
	}
	if entry.Attrs["scope"] != "db" {
		t.Fatalf("expected attrs.scope=db, got %#v", entry.Attrs["scope"])
	}
}
