package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/logdash-io/go-sdk/logdash"
)

func main() {
	// Create a new Logdash instance
	ld := logdash.New(
		logdash.WithAPIKey("your-api-key"),
	)

	// Create a slog.Handler that wraps the Logdash logger
	handler := logdash.NewSlogTextHandler(ld.Logger, slog.HandlerOptions{
		Level: slog.LevelDebug,
		// replace sensitive information
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "token" {
				a.Value = slog.StringValue("********")
			}
			return a
		},
	})

	// Create a new slog.Logger with the Logdash handler
	logger := slog.New(handler)

	// Use the slog logger
	logger.Info("Hello from slog!", "user", "john", "action", "login", "token", "1234567890")
	logger.Error("Something went wrong", "error", "connection timeout")
	logger.Debug("Debug information", "request_id", "12345")

	// You can also use slog.SetDefault to make this the default logger
	slog.SetDefault(logger)

	// Now all slog calls will use Logdash
	slog.Info("This will be logged through Logdash")
	slog.Warn("Warning message with attributes", "severity", "high", "component", "auth")

	// Shutdown method wait for flushing all enqueued logs before closing application
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ld.Shutdown(ctx)
}
