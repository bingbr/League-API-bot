package logs

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type LogEntry struct {
	Time    time.Time
	Level   string
	Message string
	Attrs   map[string]any
}

type Database interface {
	Insert(ctx context.Context, entry LogEntry) error
}

type DBHandler struct {
	database Database
	level    slog.Level
	timeout  time.Duration
	attrs    []slog.Attr
	groups   []string
}

func NewDBHandler(database Database, level slog.Leveler, timeout time.Duration) *DBHandler {
	lvl := slog.LevelInfo
	if level != nil {
		lvl = level.Level()
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &DBHandler{database: database, level: lvl, timeout: timeout}
}

func (h *DBHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h == nil {
		return false
	}
	return level >= h.level
}

func (h *DBHandler) Handle(ctx context.Context, r slog.Record) error {
	if h == nil || h.database == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Build the Attribute Map
	data := make(map[string]any)
	for _, attr := range h.attrs {
		addToMap(data, h.groups, attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		addToMap(data, h.groups, attr)
		return true
	})

	// Insert into Database
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	if err := h.database.Insert(ctx, LogEntry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   data,
	}); err != nil {
		return fmt.Errorf("db log insert: %w", err)
	}
	return nil
}

func (h *DBHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newH := *h // Shallow copy
	newH.attrs = append(h.attrs[:len(h.attrs):len(h.attrs)], attrs...)
	return &newH
}

func (h *DBHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newH := *h
	newH.groups = append(h.groups[:len(h.groups):len(h.groups)], name)
	return &newH
}

func addToMap(m map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	// Drill down into groups
	for _, g := range groups {
		sub, ok := m[g].(map[string]any)
		if !ok {
			sub = make(map[string]any)
			m[g] = sub
		}
		m = sub
	}

	// Handle the value (recurse if it's a group, otherwise set value)
	if attr.Value.Kind() == slog.KindGroup {
		target := m
		if attr.Key != "" {
			sub, ok := m[attr.Key].(map[string]any)
			if !ok {
				sub = make(map[string]any)
				m[attr.Key] = sub
			}
			target = sub
		}
		for _, child := range attr.Value.Group() {
			addToMap(target, nil, child)
		}
	} else {
		m[attr.Key] = attrValue(attr.Value)
	}
}

// Convert slog.Value to a Go native type for storage.
// Handles basic types and falls back to Any with some special handling for error and fmt.Stringer.
func attrValue(value slog.Value) any {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindString:
		return value.String()
	case slog.KindTime:
		return value.Time().UTC()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindAny:
		anyValue := value.Any()
		switch typed := anyValue.(type) {
		case error:
			return typed.Error()
		case fmt.Stringer:
			return typed.String()
		default:
			return anyValue
		}
	default:
		return value.Any()
	}
}
