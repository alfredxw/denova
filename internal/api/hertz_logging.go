package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

var installHertzLogger sync.Once

// configureHertzLogging routes framework diagnostics through Denova's
// process-wide slog handler. It must run before Hertz starts registering
// routes because hlog's global logger is not safe to replace concurrently.
func configureHertzLogging() {
	installHertzLogger.Do(func() {
		hlog.SetLogger(&hertzSlogLogger{})
	})
}

type hertzSlogLogger struct {
	minimumLevel atomic.Int32
}

var _ hlog.FullLogger = (*hertzSlogLogger)(nil)

func (l *hertzSlogLogger) Trace(values ...any) {
	l.log(context.Background(), hlog.LevelTrace, fmt.Sprint(values...))
}
func (l *hertzSlogLogger) Debug(values ...any) {
	l.log(context.Background(), hlog.LevelDebug, fmt.Sprint(values...))
}
func (l *hertzSlogLogger) Info(values ...any) {
	l.log(context.Background(), hlog.LevelInfo, fmt.Sprint(values...))
}
func (l *hertzSlogLogger) Notice(values ...any) {
	l.log(context.Background(), hlog.LevelNotice, fmt.Sprint(values...))
}
func (l *hertzSlogLogger) Warn(values ...any) {
	l.log(context.Background(), hlog.LevelWarn, fmt.Sprint(values...))
}
func (l *hertzSlogLogger) Error(values ...any) {
	l.log(context.Background(), hlog.LevelError, fmt.Sprint(values...))
}
func (l *hertzSlogLogger) Fatal(values ...any) {
	l.log(context.Background(), hlog.LevelFatal, fmt.Sprint(values...))
}

func (l *hertzSlogLogger) Tracef(format string, values ...any) {
	l.logf(context.Background(), hlog.LevelTrace, format, values...)
}
func (l *hertzSlogLogger) Debugf(format string, values ...any) {
	l.logf(context.Background(), hlog.LevelDebug, format, values...)
}
func (l *hertzSlogLogger) Infof(format string, values ...any) {
	l.logf(context.Background(), hlog.LevelInfo, format, values...)
}
func (l *hertzSlogLogger) Noticef(format string, values ...any) {
	l.logf(context.Background(), hlog.LevelNotice, format, values...)
}
func (l *hertzSlogLogger) Warnf(format string, values ...any) {
	l.logf(context.Background(), hlog.LevelWarn, format, values...)
}
func (l *hertzSlogLogger) Errorf(format string, values ...any) {
	l.logf(context.Background(), hlog.LevelError, format, values...)
}
func (l *hertzSlogLogger) Fatalf(format string, values ...any) {
	l.logf(context.Background(), hlog.LevelFatal, format, values...)
}

func (l *hertzSlogLogger) CtxTracef(ctx context.Context, format string, values ...any) {
	l.logf(ctx, hlog.LevelTrace, format, values...)
}
func (l *hertzSlogLogger) CtxDebugf(ctx context.Context, format string, values ...any) {
	l.logf(ctx, hlog.LevelDebug, format, values...)
}
func (l *hertzSlogLogger) CtxInfof(ctx context.Context, format string, values ...any) {
	l.logf(ctx, hlog.LevelInfo, format, values...)
}
func (l *hertzSlogLogger) CtxNoticef(ctx context.Context, format string, values ...any) {
	l.logf(ctx, hlog.LevelNotice, format, values...)
}
func (l *hertzSlogLogger) CtxWarnf(ctx context.Context, format string, values ...any) {
	l.logf(ctx, hlog.LevelWarn, format, values...)
}
func (l *hertzSlogLogger) CtxErrorf(ctx context.Context, format string, values ...any) {
	l.logf(ctx, hlog.LevelError, format, values...)
}
func (l *hertzSlogLogger) CtxFatalf(ctx context.Context, format string, values ...any) {
	l.logf(ctx, hlog.LevelFatal, format, values...)
}

func (l *hertzSlogLogger) SetLevel(level hlog.Level) {
	l.minimumLevel.Store(int32(level))
}

// SetOutput is intentionally owned by ConfigureStructuredLogging. Hertz's
// adapter must not fork framework logs into a second, uncorrelated sink.
func (*hertzSlogLogger) SetOutput(io.Writer) {}

func (l *hertzSlogLogger) logf(ctx context.Context, level hlog.Level, format string, values ...any) {
	l.log(ctx, level, fmt.Sprintf(format, values...))
}

func (l *hertzSlogLogger) log(ctx context.Context, level hlog.Level, message string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if level >= hlog.Level(l.minimumLevel.Load()) {
		slog.Default().Log(ctx, slogLevelForHertz(level), strings.TrimSpace(message), "component", "hertz")
	}
	if level == hlog.LevelFatal {
		os.Exit(1)
	}
}

func slogLevelForHertz(level hlog.Level) slog.Level {
	switch level {
	case hlog.LevelTrace:
		return slog.LevelDebug - 4
	case hlog.LevelDebug:
		return slog.LevelDebug
	case hlog.LevelInfo:
		return slog.LevelInfo
	case hlog.LevelNotice:
		return slog.LevelInfo + 2
	case hlog.LevelWarn:
		return slog.LevelWarn
	case hlog.LevelError:
		return slog.LevelError
	case hlog.LevelFatal:
		return slog.LevelError + 4
	default:
		return slog.LevelInfo
	}
}
