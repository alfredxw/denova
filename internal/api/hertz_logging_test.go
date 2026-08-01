package api

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/hlog"

	"denova/internal/observability"
)

func TestHertzSlogLoggerPreservesRequestContext(t *testing.T) {
	previousLogger := slog.Default()
	defer slog.SetDefault(previousLogger)
	var output bytes.Buffer
	observability.ConfigureStructuredLogging(&output)

	logger := &hertzSlogLogger{}
	ctx := observability.WithRequestID(context.Background(), "0198-hertz-request")
	logger.CtxInfof(ctx, "request failed status=%d", 500)

	entry := output.String()
	for _, expected := range []string{
		"msg=\"request failed status=500\"",
		"component=hertz",
		"request_id=0198-hertz-request",
	} {
		if !strings.Contains(entry, expected) {
			t.Fatalf("Hertz log does not contain %q: %s", expected, entry)
		}
	}

	logger.SetLevel(hlog.LevelWarn)
	output.Reset()
	logger.CtxInfof(ctx, "hidden")
	logger.CtxWarnf(ctx, "visible")
	if got := output.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "visible") {
		t.Fatalf("Hertz level filtering output = %q", got)
	}
}
