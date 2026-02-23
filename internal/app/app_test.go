package app

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func waitForSubstring(t *testing.T, timeout time.Duration, output func() string, substr string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(output(), substr) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected log output to contain %q; got: %s", substr, output())
}

func TestGoSafeRecoversAndLogsPanic(t *testing.T) {
	var out lockedBuffer
	logger := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	done := make(chan struct{})
	goSafe(logger, "panic_task", func() {
		defer close(done)
		panic("boom")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("background task was not executed")
	}

	waitForSubstring(t, 2*time.Second, out.String, "Background task panicked")
	waitForSubstring(t, 2*time.Second, out.String, "task=panic_task")
	waitForSubstring(t, 2*time.Second, out.String, "panic=boom")
	waitForSubstring(t, 2*time.Second, out.String, "stack=")
}

func TestGoSafeWithNilFunction(t *testing.T) {
	var out lockedBuffer
	logger := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	goSafe(logger, "  ", nil)

	waitForSubstring(t, 500*time.Millisecond, out.String, "Background task not started: nil func")
	waitForSubstring(t, 500*time.Millisecond, out.String, "task=unnamed")
}

func TestGoSafeRunsFunction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	done := make(chan struct{})
	goSafe(logger, "runs", func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("background task did not run")
	}
}
