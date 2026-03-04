package logger_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/logger"
)

func TestFrom_ReturnsDefault_WhenNoLoggerInContext(t *testing.T) {
	l := logger.From(context.Background())
	if l != slog.Default() {
		t.Fatal("expected slog.Default()")
	}
}

func TestNewContext_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := logger.NewContext(context.Background(), l)
	got := logger.From(ctx)
	got.DebugContext(ctx, "hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected logger to write, got: %s", buf.String())
	}
}
