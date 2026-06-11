package observability

import (
	"context"
	"log"
	"log/slog"
)

// ConfigureStructuredLogging routes slog through the existing standard log writer.
// Call it after startup has configured log output so structured logs keep the same file target.
func ConfigureStructuredLogging() {
	slog.SetDefault(slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: slog.LevelInfo})))
}

func Logger(component string) *slog.Logger {
	if component == "" {
		return slog.Default()
	}
	return slog.Default().With("component", component)
}

func Info(component, message string, attrs ...slog.Attr) {
	Logger(component).LogAttrs(context.Background(), slog.LevelInfo, message, attrs...)
}

func Warn(component, message string, attrs ...slog.Attr) {
	Logger(component).LogAttrs(context.Background(), slog.LevelWarn, message, attrs...)
}

func Error(component, message string, attrs ...slog.Attr) {
	Logger(component).LogAttrs(context.Background(), slog.LevelError, message, attrs...)
}
